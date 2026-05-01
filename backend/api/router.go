package api

import (
	"net/url"
	"strings"

	"github.com/craftslab/kipup/backend/app"
	"github.com/craftslab/kipup/backend/config"
	"github.com/craftslab/kipup/backend/storage"
	cors "github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// NewRouter constructs the gin engine with all routes registered.
func NewRouter(client *storage.Client, service *app.Service, cfg *config.Config) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-User", "X-Task-ID"},
		ExposeHeaders:    []string{"Content-Disposition", "Content-Length"},
		AllowCredentials: false,
	}))

	h := &Handler{
		client:         client,
		service:        service,
		publicBaseURL:  cfg.PublicBaseURL,
		allowedS3Hosts: map[string]struct{}{cfg.S3Endpoint: {}},
	}
	if raw := strings.TrimSpace(cfg.S3PublicURL); raw != "" {
		if !strings.Contains(raw, "://") {
			if cfg.S3UseSSL {
				raw = "https://" + raw
			} else {
				raw = "http://" + raw
			}
		}
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			h.allowedS3Hosts[u.Host] = struct{}{}
		}
	}

	r.POST("/upload", h.ProxyUpload)
	r.GET("/download", h.ProxyDownload)
	r.POST("/api/upload", h.ProxyUpload)
	r.GET("/api/download", h.ProxyDownload)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/sign-up", h.SignUp)
		v1.POST("/auth/sign-in", h.SignIn)
		v1.GET("/collaboration/sessions/:token/stream", h.StreamCollaboration)
		v1.POST("/mobile/collaboration/session", h.GetMobileCollaborationSession)
		v1.POST("/mobile/collaboration/messages", h.CreateMobileCollaborationMessage)
		v1.POST("/mobile/collaboration/read", h.MarkMobileCollaborationRead)
		v1.POST("/mobile/collaboration/messages/:messageId/reactions", h.ToggleMobileCollaborationReaction)
		v1.POST("/mobile/collaboration/messages/:messageId/recall", h.RecallMobileCollaborationMessage)
		v1.DELETE("/mobile/collaboration/messages/:messageId", h.DeleteMobileCollaborationMessage)
		v1.GET("/mobile/collaboration/export", h.ExportMobileCollaborationTranscript)
		v1.GET("/mobile/download-links/:token", h.GetMobileAppDownloadLink)
		v1.GET("/mobile/download-links/:token/file", h.DownloadMobileAppBinary)
		v1.POST("/mobile/download-links/:token/activate", h.ActivateMobileApp)
		v1.POST("/mobile/installations/validate", h.ValidateMobileApp)

		authenticated := v1.Group("/")
		authenticated.Use(h.requireAuth())
		authenticated.GET("/auth/me", h.Me)
		authenticated.POST("/auth/sign-out", h.SignOut)

		authenticated.GET("/users", h.requireAdmin(), h.ListUsers)
		authenticated.POST("/users/temp", h.requireAdmin(), h.CreateTemporaryUser)
		authenticated.PUT("/users/:username", h.requireAdmin(), h.UpdateUser)
		authenticated.DELETE("/users/:username", h.requireAdmin(), h.DeleteUser)

		authenticated.GET("/buckets", h.ListBuckets)
		authenticated.POST("/buckets", h.requirePermission(app.PermissionCreate), h.CreateBucket)
		authenticated.DELETE("/buckets/:bucket", h.requirePermission(app.PermissionDelete), h.DeleteBucket)

		authenticated.GET("/objects/:bucket", h.ListObjects)
		authenticated.GET("/objects/:bucket/*key", h.requirePermission(app.PermissionDownload), h.DownloadObject)
		authenticated.POST("/objects/:bucket", h.requirePermission(app.PermissionUpload), h.UploadObject)
		authenticated.DELETE("/objects/:bucket/*key", h.requirePermission(app.PermissionDelete), h.DeleteObject)
		authenticated.GET("/search/:bucket", h.requirePermission(app.PermissionSearch), h.SearchObjects)
		authenticated.POST("/uploads/:bucket/resumable/init", h.requirePermission(app.PermissionUpload), h.InitResumableUpload)
		authenticated.GET("/uploads/:bucket/resumable/status", h.requirePermission(app.PermissionUpload), h.GetResumableUploadStatus)
		authenticated.PUT("/uploads/:bucket/resumable/part", h.requirePermission(app.PermissionUpload), h.UploadResumablePart)
		authenticated.POST("/uploads/:bucket/resumable/complete", h.requirePermission(app.PermissionUpload), h.CompleteResumableUpload)
		authenticated.DELETE("/uploads/:bucket/resumable", h.requirePermission(app.PermissionUpload), h.AbortResumableUpload)

		authenticated.POST("/operations/:bucket/download", h.requirePermission(app.PermissionDownload), h.BatchDownload)
		authenticated.POST("/operations/:bucket/delete", h.requirePermission(app.PermissionDelete), h.BatchDelete)
		authenticated.POST("/operations/:bucket/move", h.requirePermission(app.PermissionMove), h.BatchMove)
		authenticated.POST("/operations/:bucket/rename", h.requirePermission(app.PermissionRename), h.BatchRename)

		authenticated.GET("/tasks", h.ListTasks)
		authenticated.GET("/history", h.ListHistory)

		authenticated.GET("/cleanup-policies", h.requirePermission(app.PermissionCleanup), h.ListCleanupPolicies)
		authenticated.POST("/cleanup-policies", h.requirePermission(app.PermissionCleanup), h.CreateCleanupPolicy)
		authenticated.PUT("/cleanup-policies/:id", h.requirePermission(app.PermissionCleanup), h.UpdateCleanupPolicy)
		authenticated.DELETE("/cleanup-policies/:id", h.requirePermission(app.PermissionCleanup), h.DeleteCleanupPolicy)
		authenticated.POST("/cleanup-policies/:id/run", h.requirePermission(app.PermissionCleanup), h.RunCleanupPolicy)

		authenticated.GET("/webhooks", h.requirePermission(app.PermissionWebhook), h.ListWebhooks)
		authenticated.POST("/webhooks", h.requirePermission(app.PermissionWebhook), h.CreateWebhook)
		authenticated.PUT("/webhooks/:id", h.requirePermission(app.PermissionWebhook), h.UpdateWebhook)
		authenticated.DELETE("/webhooks/:id", h.requirePermission(app.PermissionWebhook), h.DeleteWebhook)
		authenticated.GET("/webhook-deliveries", h.requirePermission(app.PermissionWebhook), h.ListWebhookDeliveries)

		authenticated.GET("/presign/download/:bucket/*key", h.requirePermission(app.PermissionPresign), h.GenerateDownloadLink)
		authenticated.GET("/presign/upload/:bucket/*key", h.requirePermission(app.PermissionPresign), h.GenerateUploadLink)

		authenticated.GET("/collaboration/sessions", h.ListCollaborationSessions)
		authenticated.POST("/collaboration/sessions", h.CreateCollaborationSession)
		authenticated.GET("/collaboration/sessions/:token", h.GetCollaborationSession)
		authenticated.PUT("/collaboration/sessions/:token", h.UpdateCollaborationSession)
		authenticated.POST("/collaboration/sessions/:token/close", h.CloseCollaborationSession)
		authenticated.DELETE("/collaboration/sessions/:token", h.DeleteCollaborationSession)
		authenticated.POST("/collaboration/sessions/:token/messages", h.CreateCollaborationMessage)
		authenticated.POST("/collaboration/sessions/:token/read", h.MarkCollaborationRead)
		authenticated.POST("/collaboration/sessions/:token/messages/:messageId/reactions", h.ToggleCollaborationReaction)
		authenticated.POST("/collaboration/sessions/:token/messages/:messageId/recall", h.RecallCollaborationMessage)
		authenticated.DELETE("/collaboration/sessions/:token/messages/:messageId", h.DeleteCollaborationMessage)
		authenticated.GET("/collaboration/sessions/:token/export", h.ExportCollaborationTranscript)
		authenticated.POST("/collaboration/sessions/:token/attachments", h.CreateCollaborationAttachment)
		authenticated.GET("/collaboration/sessions/:token/attachments/:attachmentId/download", h.DownloadCollaborationAttachment)
		authenticated.DELETE("/collaboration/sessions/:token/attachments/:attachmentId", h.DeleteCollaborationAttachment)
		authenticated.POST("/collaboration/sessions/:token/files", h.CreateCollaborationSharedFile)
		authenticated.GET("/collaboration/sessions/:token/files/:fileId/download", h.DownloadCollaborationSharedFile)
		authenticated.DELETE("/collaboration/sessions/:token/files/:fileId", h.DeleteCollaborationSharedFile)
		authenticated.POST("/collaboration/sessions/:token/stream-token", h.CreateCollaborationStreamToken)
		authenticated.POST("/collaboration/sessions/:token/signal", h.PublishCollaborationSignal)

		authenticated.GET("/mobile/releases", h.requireAdmin(), h.ListMobileAppReleases)
		authenticated.POST("/mobile/releases", h.requireAdmin(), h.CreateMobileAppRelease)
		authenticated.POST("/mobile/releases/:id/revoke", h.requireAdmin(), h.RevokeMobileAppRelease)
		authenticated.POST("/mobile/releases/:id/download-links", h.requireAdmin(), h.CreateMobileAppDownloadLink)
		authenticated.GET("/mobile/releases/:id/installations", h.requireAdmin(), h.ListMobileAppInstallations)
		authenticated.POST("/mobile/installations/:id/revoke", h.requireAdmin(), h.RevokeMobileAppInstallation)
	}

	return r
}
