package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"simplek8/internal/config"
	"simplek8/internal/crypto"
	"simplek8/internal/store"
	"simplek8/internal/worker"
	"strconv"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()
	concurrency := parseConcurrency(os.Getenv("WORKER_CONCURRENCY"), 10)

	db, err := store.NewPostgresStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to initialize worker database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	var cipher *crypto.Cipher
	if len(cfg.KubeconfigKey) > 0 {
		var cipherErr error
		cipher, cipherErr = crypto.NewCipher(cfg.KubeconfigKey)
		if cipherErr != nil {
			slog.Error("failed to initialise kubeconfig cipher", "error", cipherErr)
			os.Exit(1)
		}
		slog.Info("kubeconfig encryption enabled")
	}

	workerServer := worker.NewServer(cfg.RedisAddr, cfg.RedisPassword, concurrency, db, cipher)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	slog.Info("starting worker", "redisAddr", cfg.RedisAddr, "concurrency", concurrency)
	if err := workerServer.Run(ctx); err != nil {
		slog.Error("worker stopped with error", "error", err)
		os.Exit(1)
	}

	slog.Info("worker stopped")
}

func parseConcurrency(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
