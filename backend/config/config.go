package config

import (
	"os"
	"strconv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	ListenAddr            string
	AdminUsername         string
	AdminPassword         string
	S3Endpoint            string
	S3PublicURL           string
	PublicBaseURL         string
	S3AccessKey           string
	S3SecretKey           string
	S3UseSSL              bool
	S3Region              string
	DataFile              string
	CleanupIntervalSecond int
}

// Load returns a Config populated from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		ListenAddr:    getEnv("LISTEN_ADDR", ":8080"),
		AdminUsername: getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword: getEnv("ADMIN_PASSWORD", "admin"),
		S3Endpoint:    getEnv("S3_ENDPOINT", "localhost:9000"),
		// S3PublicURL is the externally reachable base URL used in returned presigned links.
		// Example: https://s3.example.com
		S3PublicURL: getEnv("S3_PUBLIC_URL", ""),
		// PublicBaseURL is the externally reachable base URL of this app (nginx entry),
		// used to construct shareable proxy links like /api/download.
		// Example: http://47.88.100.1:3000 or https://kipup.example.com
		PublicBaseURL:         getEnv("PUBLIC_BASE_URL", ""),
		S3AccessKey:           getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:           getEnv("S3_SECRET_KEY", "minioadmin"),
		S3UseSSL:              getEnv("S3_USE_SSL", "false") == "true",
		S3Region:              getEnv("S3_REGION", "us-east-1"),
		DataFile:              getEnv("DATA_FILE", "./data/state.json"),
		CleanupIntervalSecond: getEnvInt("CLEANUP_INTERVAL_SECONDS", 3600),
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultValue
}
