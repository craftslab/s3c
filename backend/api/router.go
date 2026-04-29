package api

import (
	"net/url"
	"strings"

	"github.com/craftslab/s3c/backend/app"
	"github.com/craftslab/s3c/backend/config"
	"github.com/craftslab/s3c/backend/storage"
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
		v1.GET("/buckets", h.ListBuckets)
		v1.POST("/buckets", h.CreateBucket)
		v1.DELETE("/buckets/:bucket", h.DeleteBucket)

		v1.GET("/objects/:bucket", h.ListObjects)
		v1.GET("/objects/:bucket/*key", h.DownloadObject)
		v1.POST("/objects/:bucket", h.UploadObject)
		v1.DELETE("/objects/:bucket/*key", h.DeleteObject)
		v1.GET("/search/:bucket", h.SearchObjects)

		v1.POST("/operations/:bucket/download", h.BatchDownload)
		v1.POST("/operations/:bucket/delete", h.BatchDelete)
		v1.POST("/operations/:bucket/move", h.BatchMove)
		v1.POST("/operations/:bucket/rename", h.BatchRename)

		v1.GET("/tasks", h.ListTasks)
		v1.GET("/history", h.ListHistory)

		v1.GET("/cleanup-policies", h.ListCleanupPolicies)
		v1.POST("/cleanup-policies", h.CreateCleanupPolicy)
		v1.PUT("/cleanup-policies/:id", h.UpdateCleanupPolicy)
		v1.DELETE("/cleanup-policies/:id", h.DeleteCleanupPolicy)
		v1.POST("/cleanup-policies/:id/run", h.RunCleanupPolicy)

		v1.GET("/webhooks", h.ListWebhooks)
		v1.POST("/webhooks", h.CreateWebhook)
		v1.PUT("/webhooks/:id", h.UpdateWebhook)
		v1.DELETE("/webhooks/:id", h.DeleteWebhook)
		v1.GET("/webhook-deliveries", h.ListWebhookDeliveries)

		v1.GET("/presign/download/:bucket/*key", h.GenerateDownloadLink)
		v1.GET("/presign/upload/:bucket/*key", h.GenerateUploadLink)
	}

	return r
}
