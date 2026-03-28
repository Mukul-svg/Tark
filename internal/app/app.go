package app

import (
	"fmt"
	"os"
	"simplek8/internal/cache"
	"simplek8/internal/config"
	"simplek8/internal/http"
	"simplek8/internal/http/handlers"
	"simplek8/internal/kube"
	"simplek8/internal/store"
	"simplek8/internal/worker"
)

type App struct {
	cfg   *config.Config
	kc    *kube.Client
	http  *http.Server
	proxy *handlers.ProxyHandler
	store store.Store
	cache *cache.RedisCache
	queue *worker.Client
}

func New(cfg *config.Config, st store.Store, c *cache.RedisCache, queueClient *worker.Client) (*App, error) {
	kc, err := kube.New()
	if err != nil {
		return nil, err
	}

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
		kc:    kc,
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
