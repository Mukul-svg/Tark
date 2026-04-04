package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"simplek8/internal/cache"
	"simplek8/internal/config"
	"simplek8/internal/http"
	"simplek8/internal/http/handlers"
	"simplek8/internal/store"
	"simplek8/internal/worker"
	"syscall"
	"time"
)

type App struct {
	cfg   *config.Config
	http  *http.Server
	proxy *handlers.ProxyHandler
	store store.Store
	cache *cache.RedisCache
	queue *worker.Client
}

func New(cfg *config.Config, st store.Store, c *cache.RedisCache, queueClient *worker.Client) (*App, error) {
	deployHandler := handlers.NewDeployHandler(st, queueClient)

	vllmURL := os.Getenv("VLLM_URL")
	if vllmURL == "" {
		vllmURL = fmt.Sprintf("http://vllm.%s.svc.cluster.local:8000", cfg.Namespace)
	}
	proxyHandler := handlers.NewProxyHandler(vllmURL, st, c)
	provisionHandler := handlers.NewProvisionHandler(st, queueClient)
	jobsHandler := handlers.NewJobsHandler(queueClient)

	server := http.NewServer(deployHandler, proxyHandler, provisionHandler, jobsHandler)

	return &App{
		cfg:   cfg,
		http:  server,
		proxy: proxyHandler,
		store: st,
		cache: c,
		queue: queueClient,
	}, nil
}

func (a *App) Run() error {
	addr := fmt.Sprintf(":%s", a.cfg.Port)
	return a.http.Start(addr)
}

// RunWithGracefulShutdown starts the HTTP server, listens for SIGTERM/SIGINT,
// and gracefully shuts down with in-flight request draining.
func (a *App) RunWithGracefulShutdown() error {
	addr := fmt.Sprintf(":%s", a.cfg.Port)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting HTTP server", "port", a.cfg.Port)
		if err := a.http.Start(addr); err != nil {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("HTTP server failed: %w", err)
		}
		return nil
	case sig := <-sigCh:
		slog.Info("received signal, initiating graceful shutdown", "signal", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("HTTP server shutdown: %w", err)
	}
	return nil
}
