package api

import (
	"github.com/craftslab/s3c/backend/config"
	"github.com/craftslab/s3c/backend/storage"
	cors "github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// NewRouter constructs the gin engine with all routes registered.
func NewRouter(client *storage.Client, cfg *config.Config) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Disposition", "Content-Length"},
		AllowCredentials: false,
	}))

	h := &Handler{client: client, publicBaseURL: cfg.PublicBaseURL}

	// Standalone streaming proxy endpoints used by the shared /upload page.
	// These routes are intentionally NOT under /api so the nginx frontend can
	// expose a simple same-origin URL: http://<host>:3000/upload?url=... .
	r.POST("/upload", h.ProxyUpload)
	r.GET("/download", h.ProxyDownload)
	// Also expose under /api/* so we can rely on the existing nginx /api proxy
	// settings (no body limits, buffering off) and avoid fragile routing rules.
	r.POST("/api/upload", h.ProxyUpload)
	r.GET("/api/download", h.ProxyDownload)

	v1 := r.Group("/api/v1")
	{
		// Bucket operations
		v1.GET("/buckets", h.ListBuckets)
		v1.POST("/buckets", h.CreateBucket)
		v1.DELETE("/buckets/:bucket", h.DeleteBucket)

		// Object operations
		v1.GET("/objects/:bucket", h.ListObjects)
		v1.GET("/objects/:bucket/*key", h.DownloadObject)
		v1.POST("/objects/:bucket", h.UploadObject)
		v1.DELETE("/objects/:bucket/*key", h.DeleteObject)

		// Presigned URL generation
		v1.GET("/presign/download/:bucket/*key", h.GenerateDownloadLink)
		v1.GET("/presign/upload/:bucket/*key", h.GenerateUploadLink)
	}

	return r
}
