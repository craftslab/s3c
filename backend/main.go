package main

import (
	"context"
	"log"
	"time"

	"github.com/craftslab/kipup/backend/api"
	"github.com/craftslab/kipup/backend/app"
	"github.com/craftslab/kipup/backend/config"
	"github.com/craftslab/kipup/backend/storage"
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
	if err := service.EnsureAdmin(cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("failed to initialize admin user: %v", err)
	}
	service.StartCleanupScheduler(context.Background(), time.Duration(cfg.CleanupIntervalSecond)*time.Second)

	router := api.NewRouter(client, service, cfg)
	log.Printf("Starting server on %s", cfg.ListenAddr)
	if err := router.Run(cfg.ListenAddr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
