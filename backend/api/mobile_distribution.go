package api

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/craftslab/kipup/backend/app"
	"github.com/gin-gonic/gin"
)

type mobileAppReleaseRequest struct {
	Title               string `json:"title" binding:"required"`
	Version             string `json:"version" binding:"required"`
	Platform            string `json:"platform" binding:"required"`
	Bucket              string `json:"bucket" binding:"required"`
	ObjectKey           string `json:"objectKey" binding:"required"`
	CollaborationToken  string `json:"collaborationToken"`
	ExpiresAt           string `json:"expiresAt" binding:"required"`
	OfflineGraceSeconds int    `json:"offlineGraceSeconds"`
}

type mobileAppDownloadLinkRequest struct {
	ExpiresAt string `json:"expiresAt" binding:"required"`
}

type mobileAppActivationRequest struct {
	Platform   string `json:"platform" binding:"required"`
	DeviceID   string `json:"deviceId" binding:"required"`
	DeviceName string `json:"deviceName"`
	AppVersion string `json:"appVersion"`
}

type mobileAppValidationRequest struct {
	ActivationToken string `json:"activationToken" binding:"required"`
	DeviceID        string `json:"deviceId" binding:"required"`
}

func (h *Handler) ListMobileAppReleases(c *gin.Context) {
	releases := h.service.ListMobileAppReleases()
	response := make([]gin.H, 0, len(releases))
	for _, release := range releases {
		response = append(response, mobileAppReleaseResponse(release))
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) CreateMobileAppRelease(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	var req mobileAppReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	expiresAt, err := parseRequiredRFC3339(req.ExpiresAt, "expiresAt")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	objectMeta, err := h.statMobileObject(c, req.Bucket, req.ObjectKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	release, err := h.service.CreateMobileAppRelease(user, app.MobileAppReleaseInput{
		Title:               req.Title,
		Version:             req.Version,
		Platform:            req.Platform,
		Bucket:              req.Bucket,
		ObjectKey:           req.ObjectKey,
		ContentType:         objectMeta.ContentType,
		Size:                objectMeta.Size,
		CollaborationToken:  req.CollaborationToken,
		ExpiresAt:           *expiresAt,
		OfflineGraceSeconds: req.OfflineGraceSeconds,
	})
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusCreated, mobileAppReleaseResponse(release))
}

func (h *Handler) RevokeMobileAppRelease(c *gin.Context) {
	release, err := h.service.RevokeMobileAppRelease(c.Param("id"))
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, mobileAppReleaseResponse(release))
}

func (h *Handler) CreateMobileAppDownloadLink(c *gin.Context) {
	user, ok := currentUserFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": app.ErrUnauthorized.Error()})
		return
	}
	var req mobileAppDownloadLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	expiresAt, err := parseRequiredRFC3339(req.ExpiresAt, "expiresAt")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	link, release, err := h.service.CreateMobileAppDownloadLink(c.Param("id"), user, *expiresAt)
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusCreated, mobileAppDownloadLinkResponse(c, link, release))
}

func (h *Handler) ListMobileAppInstallations(c *gin.Context) {
	items, err := h.service.ListMobileAppInstallations(c.Param("id"))
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) RevokeMobileAppInstallation(c *gin.Context) {
	installation, err := h.service.RevokeMobileAppInstallation(c.Param("id"))
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, installation)
}

func (h *Handler) GetMobileAppDownloadLink(c *gin.Context) {
	link, release, err := h.service.GetMobileAppDownloadLink(c.Param("token"))
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, mobileAppDownloadLinkResponse(c, link, release))
}

func (h *Handler) DownloadMobileAppBinary(c *gin.Context) {
	link, release, err := h.service.GetMobileAppDownloadLink(c.Param("token"))
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	obj, err := h.client.GetObject(c.Request.Context(), release.Bucket, release.ObjectKey)
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
		contentType = release.ContentType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	h.service.RecordMobileAppDownload(link.ID)
	c.DataFromReader(http.StatusOK, info.Size, contentType, obj, map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, path.Base(release.FileName)),
	})
}

func (h *Handler) ActivateMobileApp(c *gin.Context) {
	var req mobileAppActivationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	installation, release, err := h.service.ActivateMobileAppInstallation(c.Param("token"), app.MobileAppActivationRequest{
		Platform:   req.Platform,
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		AppVersion: req.AppVersion,
	})
	if err != nil {
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"activationToken":     installation.ActivationToken,
		"installation":        installation,
		"release":             mobileAppReleaseResponse(release),
		"expiresAt":           installation.ExpiresAt,
		"offlineGraceSeconds": installation.OfflineGraceSeconds,
	})
}

func (h *Handler) ValidateMobileApp(c *gin.Context) {
	var req mobileAppValidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.ValidateMobileAppInstallation(req.ActivationToken, req.DeviceID)
	if err != nil {
		if shouldReturnMobileValidation(err) {
			c.JSON(http.StatusOK, mobileAppValidationResponse(result))
			return
		}
		writeMobileAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, mobileAppValidationResponse(result))
}

func mobileAppReleaseResponse(release app.MobileAppRelease) gin.H {
	now := time.Now().UTC()
	return gin.H{
		"id":                  release.ID,
		"title":               release.Title,
		"version":             release.Version,
		"platform":            release.Platform,
		"bucket":              release.Bucket,
		"objectKey":           release.ObjectKey,
		"fileName":            release.FileName,
		"contentType":         release.ContentType,
		"size":                release.Size,
		"status":              release.Status,
		"createdBy":           release.CreatedBy,
		"collaborationToken":  release.CollaborationToken,
		"collaborationTitle":  release.CollaborationTitle,
		"offlineGraceSeconds": release.OfflineGraceSeconds,
		"createdAt":           release.CreatedAt,
		"updatedAt":           release.UpdatedAt,
		"expiresAt":           release.ExpiresAt,
		"revokedAt":           release.RevokedAt,
		"expired":             !release.ExpiresAt.After(now),
	}
}

func mobileAppDownloadLinkResponse(c *gin.Context, link app.MobileAppDownloadLink, release app.MobileAppRelease) gin.H {
	return gin.H{
		"id":               link.ID,
		"releaseId":        link.ReleaseID,
		"token":            link.Token,
		"createdBy":        link.CreatedBy,
		"createdAt":        link.CreatedAt,
		"expiresAt":        link.ExpiresAt,
		"platform":         link.Platform,
		"downloaded":       link.Downloaded,
		"downloadPagePath": fmt.Sprintf("/mobile-download/%s", link.Token),
		"downloadPageUrl":  fmt.Sprintf("%s://%s/mobile-download/%s", schemeFromRequest(c), c.Request.Host, link.Token),
		"binaryPath":       fmt.Sprintf("/api/v1/mobile/download-links/%s/file", link.Token),
		"release":          mobileAppReleaseResponse(release),
	}
}

func mobileAppValidationResponse(result app.MobileAppValidationResult) gin.H {
	return gin.H{
		"valid":               result.Valid,
		"reason":              result.Reason,
		"expiresAt":           result.ExpiresAt,
		"offlineGraceSeconds": result.OfflineGraceSeconds,
		"serverTime":          result.ServerTime,
		"release":             mobileAppReleaseResponse(result.Release),
		"installation":        result.Installation,
		"collaborationToken":  result.CollaborationToken,
		"collaborationTitle":  result.CollaborationTitle,
	}
}

func (h *Handler) statMobileObject(c *gin.Context, bucket, key string) (objectStat, error) {
	obj, err := h.client.GetObject(c.Request.Context(), strings.TrimSpace(bucket), strings.TrimSpace(strings.TrimPrefix(key, "/")))
	if err != nil {
		return objectStat{}, err
	}
	defer obj.Close()
	info, err := obj.Stat()
	if err != nil {
		return objectStat{}, err
	}
	return objectStat{Size: info.Size, ContentType: info.ContentType}, nil
}

type objectStat struct {
	Size        int64
	ContentType string
}

func parseRequiredRFC3339(raw, field string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid RFC3339 timestamp such as 2006-01-02T15:04:05Z07:00", field)
	}
	value = value.UTC()
	return &value, nil
}

func shouldReturnMobileValidation(err error) bool {
	return errors.Is(err, app.ErrMobileAppReleaseExpired) ||
		errors.Is(err, app.ErrMobileAppReleaseRevoked) ||
		errors.Is(err, app.ErrMobileAppInstallationRevoked) ||
		errors.Is(err, app.ErrCollaborationSessionClosed) ||
		errors.Is(err, app.ErrCollaborationSessionExpired)
}

func writeMobileAppError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, app.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, app.ErrMobileAppReleaseNotFound), errors.Is(err, app.ErrMobileDownloadLinkNotFound), errors.Is(err, app.ErrMobileAppInstallationNotFound):
		status = http.StatusNotFound
	case errors.Is(err, app.ErrMobileAppActivationDenied):
		status = http.StatusForbidden
	case errors.Is(err, app.ErrMobileAppReleaseExpired), errors.Is(err, app.ErrMobileDownloadLinkExpired), errors.Is(err, app.ErrMobileAppReleaseRevoked), errors.Is(err, app.ErrMobileAppInstallationRevoked), errors.Is(err, app.ErrCollaborationSessionClosed), errors.Is(err, app.ErrCollaborationSessionExpired):
		status = http.StatusGone
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func schemeFromRequest(c *gin.Context) string {
	if proto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if c.Request.TLS != nil {
		return "https"
	}
	return "http"
}
