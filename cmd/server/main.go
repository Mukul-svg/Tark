package main

import (
	"context"
	"log/slog"
	"os"
	"simplek8/internal/app"
	"simplek8/internal/cache"
	"simplek8/internal/config"
	"simplek8/internal/store"
	"simplek8/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	ctx := context.Background()
	db, err := store.NewPostgresStore(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("connected to database", "url", cfg.DatabaseURL)

	rdb, err := cache.NewRedisCache(ctx, cfg.RedisAddr, cfg.RedisPassword, 0)
	if err != nil {
		slog.Error("failed to initialize redis cache", "error", err)
		os.Exit(1)
	}
	defer rdb.Close()
	slog.Info("connected to redis", "addr", cfg.RedisAddr)

	queueClient := worker.NewClient(cfg.RedisAddr, cfg.RedisPassword)
	defer queueClient.Close()

	application, err := app.New(cfg, db, rdb, queueClient)
	if err != nil {
		slog.Error("failed to create application", "error", err)
		os.Exit(1)
	}

	slog.Info("starting server", "port", cfg.Port)
	if err := application.RunWithGracefulShutdown(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
