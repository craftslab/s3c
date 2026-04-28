package main

import (
	"log"

	"github.com/craftslab/s3c/backend/api"
	"github.com/craftslab/s3c/backend/config"
	"github.com/craftslab/s3c/backend/storage"
)

func main() {
	cfg := config.Load()

	client, err := storage.NewClient(cfg)
	if err != nil {
		log.Fatalf("failed to create S3 client: %v", err)
	}

	router := api.NewRouter(client, cfg)
	log.Printf("Starting server on %s", cfg.ListenAddr)
	if err := router.Run(cfg.ListenAddr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
