package storage

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/craftslab/s3c/backend/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps the MinIO client and exposes S3 operations.
type Client struct {
	mc          *minio.Client
	publicBase  *url.URL
	publicReady bool
}

// NewClient creates a new S3/MinIO client from configuration.
func NewClient(cfg *config.Config) (*Client, error) {
	mc, err := minio.New(cfg.S3Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: cfg.S3UseSSL,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, err
	}

	var base *url.URL
	ready := false
	if raw := strings.TrimSpace(cfg.S3PublicURL); raw != "" {
		if !strings.Contains(raw, "://") {
			if cfg.S3UseSSL {
				raw = "https://" + raw
			} else {
				raw = "http://" + raw
			}
		}
		if u, perr := url.Parse(raw); perr == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
			base = u
			ready = true
		}
	}

	return &Client{mc: mc, publicBase: base, publicReady: ready}, nil
}

func (c *Client) rewritePresigned(raw string) string {
	if !c.publicReady || raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = c.publicBase.Scheme
	u.Host = c.publicBase.Host
	return u.String()
}

// ListBuckets returns metadata for all buckets.
func (c *Client) ListBuckets(ctx context.Context) ([]minio.BucketInfo, error) {
	return c.mc.ListBuckets(ctx)
}

// MakeBucket creates a new bucket in the given region.
func (c *Client) MakeBucket(ctx context.Context, bucket, region string) error {
	return c.mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region})
}

// RemoveBucket deletes an empty bucket.
func (c *Client) RemoveBucket(ctx context.Context, bucket string) error {
	return c.mc.RemoveBucket(ctx, bucket)
}

// ListObjects lists objects (and common-prefix directories) under a prefix.
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) []minio.ObjectInfo {
	var items []minio.ObjectInfo
	for obj := range c.mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	}) {
		if obj.Err == nil {
			items = append(items, obj)
		}
	}
	return items
}

// ListObjectsRecursive lists all objects recursively under a prefix.
func (c *Client) ListObjectsRecursive(ctx context.Context, bucket, prefix string) []minio.ObjectInfo {
	var items []minio.ObjectInfo
	for obj := range c.mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err == nil {
			items = append(items, obj)
		}
	}
	return items
}

// GetObject returns a streaming reader for the named object.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (*minio.Object, error) {
	return c.mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

// PutObjectStream uploads an object by streaming from reader.
func (c *Client) PutObjectStream(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) (minio.UploadInfo, error) {
	return c.mc.PutObject(ctx, bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType})
}

// CopyObject copies an object within the same bucket.
func (c *Client) CopyObject(ctx context.Context, bucket, sourceKey, targetKey string) error {
	_, err := c.mc.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: bucket, Object: targetKey},
		minio.CopySrcOptions{Bucket: bucket, Object: sourceKey},
	)
	return err
}

// RemoveObject deletes a single object.
func (c *Client) RemoveObject(ctx context.Context, bucket, key string) error {
	return c.mc.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

// PresignedGetObject returns a presigned URL for downloading an object.
func (c *Client) PresignedGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, bucket, key, expiry, url.Values{})
	if err != nil {
		return "", err
	}
	return c.rewritePresigned(u.String()), nil
}

// PresignedPutObject returns a presigned URL for uploading an object.
func (c *Client) PresignedPutObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedPutObject(ctx, bucket, key, expiry)
	if err != nil {
		return "", err
	}
	return c.rewritePresigned(u.String()), nil
}

// RemoveObjectsWithPrefix deletes all objects whose key starts with prefix.
func (c *Client) RemoveObjectsWithPrefix(ctx context.Context, bucket, prefix string) error {
	objectsCh := make(chan minio.ObjectInfo)
	go func() {
		defer close(objectsCh)
		for obj := range c.mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if obj.Err != nil {
				continue
			}
			objectsCh <- obj
		}
	}()
	for err := range c.mc.RemoveObjects(ctx, bucket, objectsCh, minio.RemoveObjectsOptions{}) {
		if err.Err != nil {
			return err.Err
		}
	}
	return nil
}
