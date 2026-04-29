package main

import (
	"context"
	"log"
	"time"

	"github.com/craftslab/s3c/backend/api"
	"github.com/craftslab/s3c/backend/app"
	"github.com/craftslab/s3c/backend/config"
	"github.com/craftslab/s3c/backend/storage"
)

func main() {
	cfg := config.Load()

	client, err := storage.NewClient(cfg)
	if err != nil {
		log.Fatalf("failed to create S3 client: %v", err)
	}
	store, err := app.NewStore(cfg.DataFile)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	service := app.NewService(client, store)
	service.StartCleanupScheduler(context.Background(), time.Duration(cfg.CleanupIntervalSecond)*time.Second)

	router := api.NewRouter(client, service, cfg)
	log.Printf("Starting server on %s", cfg.ListenAddr)
	if err := router.Run(cfg.ListenAddr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
