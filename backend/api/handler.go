package api

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/craftslab/s3c/backend/storage"
	"github.com/gin-gonic/gin"
)

// Handler holds the dependencies for all HTTP handlers.
type Handler struct {
	client *storage.Client
}

// ----- bucket handlers -----

// ListBuckets returns all buckets as JSON.
func (h *Handler) ListBuckets(c *gin.Context) {
	buckets, err := h.client.ListBuckets(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, buckets)
}

// CreateBucket creates a new bucket.
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
	c.JSON(http.StatusCreated, gin.H{"message": "bucket created"})
}

// DeleteBucket removes a bucket.
func (h *Handler) DeleteBucket(c *gin.Context) {
	bucket := c.Param("bucket")
	if err := h.client.RemoveBucket(c.Request.Context(), bucket); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "bucket deleted"})
}

// ----- object handlers -----

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

// ListObjects lists objects in a bucket under an optional prefix.
func (h *Handler) ListObjects(c *gin.Context) {
	bucket := c.Param("bucket")
	prefix := c.Query("prefix")

	raw := h.client.ListObjects(c.Request.Context(), bucket, prefix)
	items := make([]ObjectItem, 0, len(raw))
	for _, obj := range raw {
		isDir := strings.HasSuffix(obj.Key, "/")
		name := path.Base(strings.TrimSuffix(obj.Key, "/"))
		items = append(items, ObjectItem{
			Key:          obj.Key,
			Name:         name,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			IsDir:        isDir,
			ContentType:  obj.ContentType,
			ETag:         obj.ETag,
		})
	}
	c.JSON(http.StatusOK, items)
}

// DownloadObject streams an S3 object directly to the HTTP response.
// The file is never fully buffered in memory; data flows from S3 → client.
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

	c.DataFromReader(http.StatusOK, info.Size, contentType, obj, map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, path.Base(key)),
	})
}

// UploadObject handles one or more file uploads, streaming each file part
// directly to S3 without buffering the entire payload to disk.
func (h *Handler) UploadObject(c *gin.Context) {
	bucket := c.Param("bucket")
	prefix := c.Query("prefix")

	mr, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart request: " + err.Error()})
		return
	}

	var uploaded []gin.H
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		filename := part.FileName()
		if filename == "" {
			// skip non-file fields
			continue
		}

		key := filename
		if prefix != "" {
			key = strings.TrimSuffix(prefix, "/") + "/" + filename
		}

		contentType := part.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		// size=-1 tells the MinIO SDK to use multipart upload transparently,
		// which is the correct strategy for large files.
		if _, err := h.client.PutObjectStream(c.Request.Context(), bucket, key, part, -1, contentType); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		uploaded = append(uploaded, gin.H{"key": key, "name": filename})
	}

	if len(uploaded) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files found in request"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"uploaded": uploaded})
}

// DeleteObject removes a single object or, when the key ends with "/",
// recursively removes all objects under that prefix.
func (h *Handler) DeleteObject(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

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

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// parseExpiry reads the "expiry" query parameter (in seconds) and returns a
// time.Duration.  When the parameter is absent or invalid the default of 24 h
// is used.  The maximum accepted value is 7 days (604 800 s).
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

// GenerateDownloadLink returns a presigned GET URL for a specific object.
// Query param: expiry (seconds, default 86400, max 604800).
func (h *Handler) GenerateDownloadLink(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	expiry := parseExpiry(c)
	u, err := h.client.PresignedGetObject(c.Request.Context(), bucket, key, expiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":        u,
		"expires_in": int64(expiry.Seconds()),
	})
}

// GenerateUploadLink returns a presigned PUT URL for uploading to a specific key.
// Query param: expiry (seconds, default 86400, max 604800).
func (h *Handler) GenerateUploadLink(c *gin.Context) {
	bucket := c.Param("bucket")
	key := strings.TrimPrefix(c.Param("key"), "/")

	expiry := parseExpiry(c)
	u, err := h.client.PresignedPutObject(c.Request.Context(), bucket, key, expiry)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":        u,
		"key":        key,
		"expires_in": int64(expiry.Seconds()),
	})
}

// ----- streaming proxy handlers (for shared links) -----

func parseRemoteURL(raw string) (*url.URL, error) {
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
	return u, nil
}

// ProxyUpload accepts a browser upload and streams the file to a presigned PUT URL.
// Data flows client → this server → remote URL without buffering the whole file.
func (h *Handler) ProxyUpload(c *gin.Context) {
	remoteRaw := c.Query("url")
	remote, err := parseRemoteURL(remoteRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mr, err := c.Request.MultipartReader()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart request: " + err.Error()})
		return
	}

	// Use the first file part.
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
	defer part.Close()

	pr, pw := io.Pipe()
	copyErrCh := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(pw, part)
		_ = pw.CloseWithError(copyErr)
		copyErrCh <- copyErr
	}()

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPut, remote.String(), pr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Keep headers minimal for presigned URLs.
	if ct := part.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	// Drain a small portion for error visibility without buffering large payloads.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = resp.Status
		}
		c.JSON(resp.StatusCode, gin.H{"error": msg})
		return
	}

	// Ensure the upstream upload stream completed.
	if copyErr := <-copyErrCh; copyErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upload stream error: " + copyErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "uploaded"})
}

// ProxyDownload streams bytes from a presigned GET URL back to the browser.
// Data flows remote URL → this server → client without full buffering.
func (h *Handler) ProxyDownload(c *gin.Context) {
	remoteRaw := c.Query("url")
	remote, err := parseRemoteURL(remoteRaw)
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
