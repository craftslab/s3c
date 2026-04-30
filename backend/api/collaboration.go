package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/craftslab/kipup/backend/app"
	"github.com/gin-gonic/gin"
)

type collaborationSessionRequest struct {
	Title        string   `json:"title" binding:"required"`
	Bucket       string   `json:"bucket" binding:"required"`
	Prefix       string   `json:"prefix"`
	AllowedUsers []string `json:"allowedUsers"`
	ExpiresAt    string   `json:"expiresAt"`
}

type collaborationMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

type collaborationSharedFileRequest struct {
	Bucket string `json:"bucket" binding:"required"`
	Key    string `json:"key" binding:"required"`
	Name   string `json:"name"`
}

type collaborationSignalRequest map[string]interface{}

func (h *Handler) ListCollaborationSessions(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	sessions := h.service.ListCollaborationSessions(user)
	response := make([]gin.H, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, collaborationSessionResponse(session, user, nil))
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) CreateCollaborationSession(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	var req collaborationSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	expiresAt, err := parseOptionalRFC3339(req.ExpiresAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := h.service.CreateCollaborationSession(user, req.Title, req.Bucket, req.Prefix, req.AllowedUsers, expiresAt)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, collaborationSessionResponse(session, user, nil))
}

func (h *Handler) GetCollaborationSession(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	session, onlineUsers, err := h.service.GetCollaborationSession(c.Param("token"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusOK, collaborationSessionResponse(session, user, onlineUsers))
}

func (h *Handler) UpdateCollaborationSession(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	var req collaborationSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	expiresAt, err := parseOptionalRFC3339(req.ExpiresAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	session, err := h.service.UpdateCollaborationSession(c.Param("token"), user, app.CollaborationSessionUpdate{
		Title:        req.Title,
		AllowedUsers: req.AllowedUsers,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusOK, collaborationSessionResponse(session, user, nil))
}

func (h *Handler) CloseCollaborationSession(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	session, err := h.service.CloseCollaborationSession(c.Param("token"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusOK, collaborationSessionResponse(session, user, nil))
}

func (h *Handler) DeleteCollaborationSession(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	session, err := h.service.DeleteCollaborationSession(c.Param("token"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusOK, collaborationSessionResponse(session, user, nil))
}

func (h *Handler) CreateCollaborationMessage(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	var req collaborationMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	message, err := h.service.AddCollaborationMessage(c.Param("token"), user, req.Content)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, message)
}

func (h *Handler) CreateCollaborationAttachment(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	session, _, err := h.service.GetCollaborationSession(c.Param("token"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()
	key, err := h.service.AttachmentObjectKey(session, fileHeader.Filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if _, err := h.client.PutObjectStream(c.Request.Context(), session.AttachmentBucket, key, file, fileHeader.Size, contentType); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	attachment, err := h.service.RegisterCollaborationAttachment(c.Param("token"), user, app.CollaborationAttachment{
		Name:        fileHeader.Filename,
		Bucket:      session.AttachmentBucket,
		Key:         key,
		Size:        fileHeader.Size,
		ContentType: contentType,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, attachment)
}

func (h *Handler) DownloadCollaborationAttachment(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	attachment, err := h.service.GetCollaborationAttachment(c.Param("token"), c.Param("attachmentId"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	h.streamCollaborationObject(c, attachment.Bucket, attachment.Key, attachment.Name)
}

func (h *Handler) DeleteCollaborationAttachment(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	attachment, err := h.service.GetCollaborationAttachment(c.Param("token"), c.Param("attachmentId"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	if err := h.client.RemoveObject(c.Request.Context(), attachment.Bucket, attachment.Key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	deleted, err := h.service.DeleteCollaborationAttachment(c.Param("token"), c.Param("attachmentId"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusOK, deleted)
}

func (h *Handler) CreateCollaborationSharedFile(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	var req collaborationSharedFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name, err := h.statObject(c, req.Bucket, req.Key)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		name = strings.TrimSpace(req.Name)
	}
	sharedFile, err := h.service.AddCollaborationFileRef(c.Param("token"), user, app.CollaborationFileRef{
		Bucket: strings.TrimSpace(req.Bucket),
		Key:    strings.TrimSpace(strings.TrimPrefix(req.Key, "/")),
		Name:   name,
	})
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, sharedFile)
}

func (h *Handler) DownloadCollaborationSharedFile(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	fileRef, err := h.service.GetCollaborationFileRef(c.Param("token"), c.Param("fileId"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	h.streamCollaborationObject(c, fileRef.Bucket, fileRef.Key, fileRef.Name)
}

func (h *Handler) DeleteCollaborationSharedFile(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	fileRef, err := h.service.DeleteCollaborationFileRef(c.Param("token"), c.Param("fileId"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusOK, fileRef)
}

func (h *Handler) CreateCollaborationStreamToken(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	streamToken, err := h.service.IssueCollaborationStreamToken(c.Param("token"), user)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"streamToken": streamToken})
}

func (h *Handler) StreamCollaboration(c *gin.Context) {
	streamToken := strings.TrimSpace(c.Query("streamToken"))
	ch, unsubscribe, onlineUsers, err := h.service.SubscribeCollaboration(c.Param("token"), streamToken)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}
	writeEvent := func(event app.CollaborationRealtimeEvent) bool {
		payload, err := json.Marshal(event)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(c.Writer, "event: update\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !writeEvent(app.CollaborationRealtimeEvent{
		Type:      "presence",
		Payload:   gin.H{"onlineUsers": onlineUsers},
		CreatedAt: time.Now().UTC(),
	}) {
		return
	}
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-keepalive.C:
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-ch:
			if !ok || !writeEvent(event) {
				return
			}
		}
	}
}

func (h *Handler) PublishCollaborationSignal(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	var req collaborationSignalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.PublishCollaborationSignal(c.Param("token"), user, req); err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "signal sent"})
}

func collaborationSessionResponse(session app.CollaborationSession, user app.User, onlineUsers []string) gin.H {
	return gin.H{
		"id":               session.ID,
		"token":            session.Token,
		"title":            session.Title,
		"creator":          session.Creator,
		"bucket":           session.Bucket,
		"prefix":           session.Prefix,
		"attachmentBucket": session.AttachmentBucket,
		"attachmentPrefix": session.AttachmentPrefix,
		"allowedUsers":     session.AllowedUsers,
		"status":           session.Status,
		"messages":         session.Messages,
		"attachments":      session.Attachments,
		"sharedFiles":      session.SharedFiles,
		"createdAt":        session.CreatedAt,
		"updatedAt":        session.UpdatedAt,
		"expiresAt":        session.ExpiresAt,
		"closedAt":         session.ClosedAt,
		"canManage":        user.IsAdmin() || strings.EqualFold(session.Creator, user.Username),
		"onlineUsers":      onlineUsers,
	}
}

func parseOptionalRFC3339(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, errors.New("expiresAt must be a valid RFC3339 timestamp such as 2006-01-02T15:04:05Z07:00")
	}
	value = value.UTC()
	return &value, nil
}

func writeCollaborationError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, app.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, app.ErrCollaborationSessionNotFound), errors.Is(err, app.ErrCollaborationAttachmentNotFound), errors.Is(err, app.ErrCollaborationFileNotFound):
		status = http.StatusNotFound
	case errors.Is(err, app.ErrInvalidCollaborationExpiry):
		status = http.StatusBadRequest
	case errors.Is(err, app.ErrCollaborationAccessDenied), errors.Is(err, app.ErrCollaborationManageDenied):
		status = http.StatusForbidden
	case errors.Is(err, app.ErrCollaborationSessionExpired), errors.Is(err, app.ErrCollaborationSessionClosed):
		status = http.StatusGone
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func (h *Handler) streamCollaborationObject(c *gin.Context, bucket, key, filename string) {
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
	c.DataFromReader(http.StatusOK, info.Size, contentType, obj, map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, path.Base(filename)),
	})
}

func (h *Handler) statObject(c *gin.Context, bucket, key string) (string, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(strings.TrimPrefix(key, "/"))
	if bucket == "" || key == "" {
		return "", errors.New("bucket and key are required")
	}
	obj, err := h.client.GetObject(c.Request.Context(), bucket, key)
	if err != nil {
		return "", err
	}
	defer obj.Close()
	info, err := obj.Stat()
	if err != nil {
		return "", err
	}
	if strings.HasSuffix(info.Key, "/") {
		return "", errors.New("only files can be shared; directory markers are not supported")
	}
	return path.Base(info.Key), nil
}
