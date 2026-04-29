package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/craftslab/kipup/backend/app"
	"github.com/craftslab/kipup/backend/storage"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
)

// Handler holds the dependencies for all HTTP handlers.
type Handler struct {
	client         *storage.Client
	service        *app.Service
	publicBaseURL  string
	allowedS3Hosts map[string]struct{}
}

// ObjectItem is the JSON representation of a single S3 object or prefix.
type ObjectItem struct {
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
	IsDir        bool      `json:"isDir"`
	ContentType  string    `json:"contentType"`
	ETag         string    `json:"etag"`
}

func objectItemFromKey(key string) ObjectItem {
	isDir := strings.HasSuffix(key, "/")
	name := path.Base(strings.TrimSuffix(key, "/"))
	return ObjectItem{Key: key, Name: name, IsDir: isDir}
}

func objectItemFromInfo(key string, size int64, modified time.Time, contentType, etag string) ObjectItem {
	item := objectItemFromKey(key)
	item.Size = size
	item.LastModified = modified
	item.ContentType = contentType
	item.ETag = etag
	return item
}

type resumableUploadInitRequest struct {
	Key         string `json:"key" binding:"required"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
	TaskID      string `json:"taskId"`
	TotalItems  int    `json:"totalItems"`
}

type resumableUploadPartInfo struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size,omitempty"`
}

type resumableUploadCompleteRequest struct {
	Key            string                    `json:"key" binding:"required"`
	UploadID       string                    `json:"uploadId" binding:"required"`
	ContentType    string                    `json:"contentType"`
	TaskID         string                    `json:"taskId"`
	TotalItems     int                       `json:"totalItems"`
	CompletedItems int                       `json:"completedItems"`
	Parts          []resumableUploadPartInfo `json:"parts" binding:"required"`
}

func sanitizeUploadRelativePath(name string) (string, error) {
	raw := strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", errors.New("upload path contains invalid '..' segments")
		}
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+raw), "/")
	if cleaned == "" || cleaned == "." {
		return "", errors.New("upload path cannot be empty or only '.'")
	}
	return cleaned, nil
}

func buildUploadObjectKey(prefix, name string) (string, error) {
	cleanName, err := sanitizeUploadRelativePath(name)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(prefix) == "" {
		return cleanName, nil
	}
	cleanPrefix, err := sanitizeUploadRelativePath(strings.TrimSuffix(strings.TrimSpace(prefix), "/"))
	if err != nil {
		return "", err
	}
	return cleanPrefix + "/" + cleanName, nil
}

func uploadedKeysFromTask(task app.Task) []string {
	seen := make(map[string]struct{}, len(task.Items))
	keys := make([]string, 0, len(task.Items))
	for _, item := range task.Items {
		if item.Status != "uploaded" || item.SourceKey == "" {
			continue
		}
		if _, ok := seen[item.SourceKey]; ok {
			continue
		}
		seen[item.SourceKey] = struct{}{}
		keys = append(keys, item.SourceKey)
	}
	sort.Strings(keys)
	return keys
}

func actorFromRequest(c *gin.Context) string {
	for _, key := range []string{"X-User", "X-Forwarded-User", "X-Remote-User"} {
		if value := strings.TrimSpace(c.GetHeader(key)); value != "" {
			return value
		}
	}
	return "anonymous"
}

// ----- bucket handlers -----

func (h *Handler) ListBuckets(c *gin.Context) {
	buckets, err := h.client.ListBuckets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, buckets)
}

func (h *Handler) CreateBucket(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Region string `json:"region"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Region == "" {
		req.Region = "us-east-1"
	}
	if err := h.client.MakeBucket(c.Request.Context(), req.Name, req.Region); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.service.RecordHistory("bucket.create", req.Name, actorFromRequest(c), "success", "bucket created", nil, nil)
	c.JSON(http.StatusCreated, gin.H{"message": "bucket created"})
}

func (h *Handler) DeleteBucket(c *gin.Context) {
	bucket := c.Param("bucket")
	if err := h.client.RemoveBucket(c.Request.Context(), bucket); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.service.RecordHistory("bucket.delete", bucket, actorFromRequest(c), "success", "bucket deleted", nil, nil)
	c.JSON(http.StatusOK, gin.H{"message": "bucket deleted"})
}

// ----- object handlers -----

func (h *Handler) ListObjects(c *gin.Context) {
	bucket := c.Param("bucket")
	prefix := c.Query("prefix")

	raw := h.client.ListObjects(c.Request.Context(), bucket, prefix)
	items := make([]ObjectItem, 0, len(raw))
	for _, obj := range raw {
		items = append(items, objectItemFromInfo(obj.Key, obj.Size, obj.LastModified, obj.ContentType, obj.ETag))
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) SearchObjects(c *gin.Context) {
	bucket := c.Param("bucket")
	var minSizePtr, maxSizePtr *int64
	var modifiedAfterPtr, modifiedBeforePtr *time.Time
	if value := strings.TrimSpace(c.Query("minSize")); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			minSizePtr = &parsed
		}
	}
	if value := strings.TrimSpace(c.Query("maxSize")); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			maxSizePtr = &parsed
		}
	}
	if value := strings.TrimSpace(c.Query("modifiedAfter")); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			modifiedAfterPtr = &parsed
		}
	}
	if value := strings.TrimSpace(c.Query("modifiedBefore")); value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			modifiedBeforePtr = &parsed
		}
	}
	items, err := h.service.SearchObjects(c.Request.Context(), app.SearchRequest{
		Bucket:         bucket,
		Prefix:         c.Query("prefix"),
		Name:           c.Query("name"),
		MinSize:        minSizePtr,
		MaxSize:        maxSizePtr,
		ModifiedAfter:  modifiedAfterPtr,
		ModifiedBefore: modifiedBeforePtr,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response := make([]ObjectItem, 0, len(items))
	for _, obj := range items {
		response = append(response, objectItemFromInfo(obj.Key, obj.Size, obj.LastModified, obj.ContentType, obj.ETag))
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) DownloadObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	obj, err := h.client.GetObject(c.Request.Context(), bucket, key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	contentType := info.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	h.service.RecordHistory("object.download", bucket, actorFromRequest(c), "success", "object download started", []string{key}, nil)
	h.service.EmitEvent(app.Event{Type: "object.downloaded", Bucket: bucket, Actor: actorFromRequest(c), Keys: []string{key}})
	c.DataFromReader(http.StatusOK, info.Size, contentType, obj, map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, path.Base(key)),
	})
}

func (h *Handler) BatchDownload(c *gin.Context) {
	bucket := c.Param("bucket")
	var req app.BatchDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filename := fmt.Sprintf("%s-batch-%d.zip", bucket, time.Now().UTC().Unix())
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := h.service.StreamZip(c.Request.Context(), bucket, req.Keys, c.Writer); err != nil {
		h.service.RecordHistory("object.batch-download", bucket, actorFromRequest(c), "failed", err.Error(), req.Keys, nil)
		c.Status(http.StatusInternalServerError)
		return
	}
	h.service.RecordHistory("object.batch-download", bucket, actorFromRequest(c), "success", "batch download completed", req.Keys, nil)
	h.service.EmitEvent(app.Event{Type: "object.batch_downloaded", Bucket: bucket, Actor: actorFromRequest(c), Keys: req.Keys})
}

func (h *Handler) UploadObject(c *gin.Context) {
	bucket := c.Param("bucket")
	prefix := c.Query("prefix")
	actor := actorFromRequest(c)
	taskID := strings.TrimSpace(c.GetHeader("X-Task-ID"))
	totalItems := 0
	if rawTotal := strings.TrimSpace(c.GetHeader("X-Total-Items")); rawTotal != "" {
		parsed, err := strconv.Atoi(rawTotal)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "X-Total-Items must be a positive integer"})
			return
		}
		totalItems = parsed
	}

	mr, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart request: " + err.Error()})
		return
	}

	var uploaded []gin.H
	var uploadedKeys []string
	completed := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			h.service.FinishTask(taskID, app.TaskFailed, err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		filename := part.FileName()
		if filename == "" {
			continue
		}
		key, err := buildUploadObjectKey(prefix, filename)
		if err != nil {
			h.service.FinishTask(taskID, app.TaskFailed, err.Error())
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		contentType := part.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if taskID == "" {
			taskID = h.service.UpsertTask(taskID, "upload", bucket, prefix, actor, max(totalItems, 1), map[string]string{"mode": "stream"})
		}
		if _, err := h.client.PutObjectStream(c.Request.Context(), bucket, key, part, -1, contentType); err != nil {
			h.service.UpdateTaskProgress(taskID, key, completed, app.TaskItem{SourceKey: key, Status: "failed", Error: err.Error()})
			h.service.FinishTask(taskID, app.TaskFailed, err.Error())
			h.service.RecordHistory("object.upload", bucket, actor, "failed", err.Error(), uploadedKeys, map[string]string{"taskId": taskID})
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		completed++
		uploadedKeys = append(uploadedKeys, key)
		h.service.UpsertTask(taskID, "upload", bucket, prefix, actor, max(totalItems, completed), map[string]string{"mode": "stream"})
		h.service.UpdateTaskProgress(taskID, key, completed, app.TaskItem{SourceKey: key, Status: "uploaded"})
		uploaded = append(uploaded, gin.H{"key": key, "name": filename})
	}

	if len(uploaded) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files found in request"})
		return
	}

	h.service.FinishTask(taskID, app.TaskCompleted, fmt.Sprintf("uploaded %d file(s)", len(uploaded)))
	h.service.RecordHistory("object.upload", bucket, actor, "success", "upload completed", uploadedKeys, map[string]string{"taskId": taskID})
	h.service.EmitEvent(app.Event{Type: "object.uploaded", Bucket: bucket, Actor: actor, Keys: uploadedKeys, Metadata: map[string]string{"taskId": taskID}})
	c.JSON(http.StatusCreated, gin.H{"uploaded": uploaded, "taskId": taskID})
}

func (h *Handler) InitResumableUpload(c *gin.Context) {
	bucket := c.Param("bucket")
	prefix := c.Query("prefix")
	actor := actorFromRequest(c)
	var req resumableUploadInitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key, err := buildUploadObjectKey(prefix, req.Key)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	taskID := h.service.UpsertTask(req.TaskID, "upload", bucket, prefix, actor, max(req.TotalItems, 1), map[string]string{"mode": "resumable"})
	uploadID, err := h.client.NewMultipartUpload(c.Request.Context(), bucket, key, contentType)
	if err != nil {
		h.service.FinishTask(taskID, app.TaskFailed, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "taskId": taskID})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"taskId":      taskID,
		"uploadId":    uploadID,
		"key":         key,
		"partSize":    8 * 1024 * 1024,
		"contentType": contentType,
	})
}

func (h *Handler) GetResumableUploadStatus(c *gin.Context) {
	bucket := c.Param("bucket")
	uploadID := strings.TrimSpace(c.Query("uploadId"))
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uploadId is required"})
		return
	}
	key, err := buildUploadObjectKey(c.Query("prefix"), c.Query("key"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	parts, err := h.client.ListObjectParts(c.Request.Context(), bucket, key, uploadID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	response := make([]gin.H, 0, len(parts))
	for _, part := range parts {
		response = append(response, gin.H{"partNumber": part.PartNumber, "etag": part.ETag, "size": part.Size})
	}
	c.JSON(http.StatusOK, gin.H{"uploadId": uploadID, "key": key, "parts": response})
}

func (h *Handler) UploadResumablePart(c *gin.Context) {
	bucket := c.Param("bucket")
	uploadID := strings.TrimSpace(c.Query("uploadId"))
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uploadId is required"})
		return
	}
	key, err := buildUploadObjectKey(c.Query("prefix"), c.Query("key"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	partNumber, err := strconv.Atoi(strings.TrimSpace(c.Query("partNumber")))
	if err != nil || partNumber <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid partNumber is required"})
		return
	}
	if c.Request.ContentLength <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content length is required"})
		return
	}
	part, err := h.client.PutObjectPart(c.Request.Context(), bucket, key, uploadID, partNumber, c.Request.Body, c.Request.ContentLength)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"partNumber": part.PartNumber, "etag": part.ETag, "size": part.Size})
}

func (h *Handler) CompleteResumableUpload(c *gin.Context) {
	bucket := c.Param("bucket")
	prefix := c.Query("prefix")
	actor := actorFromRequest(c)
	var req resumableUploadCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	key, err := buildUploadObjectKey(prefix, req.Key)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	parts := make([]minio.CompletePart, 0, len(req.Parts))
	for _, part := range req.Parts {
		if part.PartNumber <= 0 || strings.TrimSpace(part.ETag) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each part requires partNumber and etag"})
			return
		}
		parts = append(parts, minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	taskID := h.service.UpsertTask(req.TaskID, "upload", bucket, prefix, actor, max(req.TotalItems, 1), map[string]string{"mode": "resumable"})
	if _, err := h.client.CompleteMultipartUpload(c.Request.Context(), bucket, key, req.UploadID, parts, contentType); err != nil {
		h.service.UpdateTaskProgress(taskID, key, req.CompletedItems, app.TaskItem{SourceKey: key, Status: "failed", Error: err.Error()})
		h.service.FinishTask(taskID, app.TaskFailed, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "taskId": taskID})
		return
	}
	completedItems := max(req.CompletedItems, 1)
	h.service.UpdateTaskProgress(taskID, key, completedItems, app.TaskItem{SourceKey: key, Status: "uploaded"})
	if completedItems >= max(req.TotalItems, 1) {
		keys := []string{key}
		if task, ok := h.service.GetTask(taskID); ok {
			if uploaded := uploadedKeysFromTask(task); len(uploaded) > 0 {
				keys = uploaded
			}
		}
		h.service.FinishTask(taskID, app.TaskCompleted, fmt.Sprintf("uploaded %d file(s)", completedItems))
		h.service.RecordHistory("object.upload", bucket, actor, "success", "upload completed", keys, map[string]string{"taskId": taskID, "mode": "resumable"})
		h.service.EmitEvent(app.Event{Type: "object.uploaded", Bucket: bucket, Actor: actor, Keys: keys, Metadata: map[string]string{"taskId": taskID}})
	}
	c.JSON(http.StatusOK, gin.H{"taskId": taskID, "key": key})
}

func (h *Handler) AbortResumableUpload(c *gin.Context) {
	bucket := c.Param("bucket")
	uploadID := strings.TrimSpace(c.Query("uploadId"))
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uploadId is required"})
		return
	}
	key, err := buildUploadObjectKey(c.Query("prefix"), c.Query("key"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.client.AbortMultipartUpload(c.Request.Context(), bucket, key, uploadID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "aborted"})
}

func (h *Handler) DeleteObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")
	actor := actorFromRequest(c)

	if strings.HasSuffix(key, "/") {
		if err := h.client.RemoveObjectsWithPrefix(c.Request.Context(), bucket, key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		if err := h.client.RemoveObject(c.Request.Context(), bucket, key); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	h.service.RecordHistory("object.delete", bucket, actor, "success", "delete completed", []string{key}, nil)
	h.service.EmitEvent(app.Event{Type: "object.deleted", Bucket: bucket, Actor: actor, Keys: []string{key}})
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *Handler) BatchDelete(c *gin.Context) {
	bucket := c.Param("bucket")
	var req app.BatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	taskID, err := h.service.BatchDelete(c.Request.Context(), bucket, actorFromRequest(c), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "taskId": taskID})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted", "taskId": taskID})
}

func (h *Handler) BatchMove(c *gin.Context) {
	bucket := c.Param("bucket")
	var req app.BatchMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	taskID, err := h.service.BatchMove(c.Request.Context(), bucket, actorFromRequest(c), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "taskId": taskID})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "moved", "taskId": taskID})
}

func (h *Handler) BatchRename(c *gin.Context) {
	bucket := c.Param("bucket")
	var req app.BatchRenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	taskID, err := h.service.BatchRename(c.Request.Context(), bucket, actorFromRequest(c), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "taskId": taskID})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "renamed", "taskId": taskID})
}

func (h *Handler) ListTasks(c *gin.Context) {
	tasks := h.service.ListTasks()
	statusFilter := strings.TrimSpace(c.Query("status"))
	bucketFilter := strings.TrimSpace(c.Query("bucket"))
	typeFilter := strings.TrimSpace(c.Query("type"))
	filtered := make([]app.Task, 0, len(tasks))
	for _, task := range tasks {
		if statusFilter != "" && string(task.Status) != statusFilter {
			continue
		}
		if bucketFilter != "" && task.Bucket != bucketFilter {
			continue
		}
		if typeFilter != "" && task.Type != typeFilter {
			continue
		}
		filtered = append(filtered, task)
	}
	c.JSON(http.StatusOK, filtered)
}

func (h *Handler) ListHistory(c *gin.Context) {
	entries := h.service.ListHistory()
	typeFilter := strings.TrimSpace(c.Query("type"))
	bucketFilter := strings.TrimSpace(c.Query("bucket"))
	actorFilter := strings.TrimSpace(c.Query("actor"))
	filtered := make([]app.HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if typeFilter != "" && entry.Type != typeFilter {
			continue
		}
		if bucketFilter != "" && entry.Bucket != bucketFilter {
			continue
		}
		if actorFilter != "" && entry.Actor != actorFilter {
			continue
		}
		filtered = append(filtered, entry)
	}
	c.JSON(http.StatusOK, filtered)
}

func (h *Handler) ListCleanupPolicies(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ListPolicies())
}

func (h *Handler) CreateCleanupPolicy(c *gin.Context) {
	var policy app.CleanupPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(policy.Name) == "" || strings.TrimSpace(policy.Bucket) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and bucket are required"})
		return
	}
	created := h.service.CreatePolicy(policy)
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) UpdateCleanupPolicy(c *gin.Context) {
	var policy app.CleanupPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.service.UpdatePolicy(c.Param("id"), policy)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) DeleteCleanupPolicy(c *gin.Context) {
	if err := h.service.DeletePolicy(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "policy deleted"})
}

func (h *Handler) RunCleanupPolicy(c *gin.Context) {
	deleted, err := h.service.RunPolicy(c.Request.Context(), c.Param("id"), actorFromRequest(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (h *Handler) ListWebhooks(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ListWebhooks())
}

func (h *Handler) CreateWebhook(c *gin.Context) {
	var hook app.Webhook
	if err := c.ShouldBindJSON(&hook); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(hook.Name) == "" || strings.TrimSpace(hook.URL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and url are required"})
		return
	}
	created := h.service.CreateWebhook(hook)
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) UpdateWebhook(c *gin.Context) {
	var hook app.Webhook
	if err := c.ShouldBindJSON(&hook); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.service.UpdateWebhook(c.Param("id"), hook)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) DeleteWebhook(c *gin.Context) {
	if err := h.service.DeleteWebhook(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "webhook deleted"})
}

func (h *Handler) ListWebhookDeliveries(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.ListDeliveries())
}

// ----- presign helpers -----

func parseExpiry(c *gin.Context) time.Duration {
	const defaultExpiry = 24 * time.Hour
	const maxExpiry = 7 * 24 * time.Hour

	s := c.Query("expiry")
	if s == "" {
		return defaultExpiry
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return defaultExpiry
	}
	d := time.Duration(v) * time.Second
	if d > maxExpiry {
		d = maxExpiry
	}
	return d
}

func (h *Handler) resolvePublicBaseURL(c *gin.Context) string {
	if strings.TrimSpace(h.publicBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(h.publicBaseURL), "/")
	}
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}
	if !strings.Contains(host, ":") {
		if fp := c.GetHeader("X-Forwarded-Port"); fp != "" {
			isDefault := (scheme == "http" && fp == "80") || (scheme == "https" && fp == "443")
			if !isDefault {
				host = host + ":" + fp
			}
		}
	}
	return scheme + "://" + host
}

func (h *Handler) GenerateDownloadLink(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")
	expiry := parseExpiry(c)
	u, err := h.client.PresignedGetObject(c.Request.Context(), bucket, key, expiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	base := h.resolvePublicBaseURL(c)
	if base != "" {
		filename := path.Base(key)
		u = base + "/api/download?url=" + url.QueryEscape(u) + "&filename=" + url.QueryEscape(filename)
	}
	c.JSON(http.StatusOK, gin.H{"url": u, "expires_in": int64(expiry.Seconds())})
}

func (h *Handler) GenerateUploadLink(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")
	expiry := parseExpiry(c)
	u, err := h.client.PresignedPutObject(c.Request.Context(), bucket, key, expiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": u, "key": key, "expires_in": int64(expiry.Seconds())})
}

// ----- streaming proxy handlers -----

func (h *Handler) parseRemoteURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, fmt.Errorf("missing url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported url scheme")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid url host")
	}
	if len(h.allowedS3Hosts) > 0 {
		if _, ok := h.allowedS3Hosts[u.Host]; !ok {
			return nil, fmt.Errorf("invalid url host: %s", u.Host)
		}
	}
	return u, nil
}

func (h *Handler) ProxyUpload(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid upload body: %v", r)})
		}
	}()

	remoteRaw := c.Query("url")
	remote, err := h.parseRemoteURL(remoteRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var src io.ReadCloser = c.Request.Body
	contentType := c.Request.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		mr, mErr := c.Request.MultipartReader()
		if mErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart request: " + mErr.Error()})
			return
		}
		var part *multipart.Part
		for {
			p, perr := mr.NextPart()
			if perr == io.EOF {
				break
			}
			if perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": perr.Error()})
				return
			}
			if p.FileName() == "" {
				continue
			}
			part = p
			break
		}
		if part == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no file found in request"})
			return
		}
		src = part
		if ct := part.Header.Get("Content-Type"); ct != "" {
			contentType = ct
		}
		defer part.Close()
	}

	pr, pw := io.Pipe()
	copyErrCh := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(pw, src)
		_ = pw.CloseWithError(copyErr)
		copyErrCh <- copyErr
	}()

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPut, remote.String(), pr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Request.ContentLength > 0 {
		req.ContentLength = c.Request.ContentLength
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = resp.Status
		}
		c.JSON(resp.StatusCode, gin.H{"error": msg})
		return
	}
	if copyErr := <-copyErrCh; copyErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload stream error: " + copyErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "uploaded"})
}

func (h *Handler) ProxyDownload(c *gin.Context) {
	remoteRaw := c.Query("url")
	remote, err := h.parseRemoteURL(remoteRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, remote.String(), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = resp.Status
		}
		c.JSON(resp.StatusCode, gin.H{"error": msg})
		return
	}
	filename := c.Query("filename")
	headers := map[string]string{}
	if filename != "" {
		headers["Content-Disposition"] = fmt.Sprintf(`attachment; filename="%s"`, path.Base(filename))
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.DataFromReader(resp.StatusCode, resp.ContentLength, ct, resp.Body, headers)
}
