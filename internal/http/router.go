package http

import (
	"context"
	"log/slog"
	"net/http"
	"simplek8/internal/cache"
	"simplek8/internal/http/handlers"
	"simplek8/internal/store"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	e     *echo.Echo
	store store.Store
	cache *cache.RedisCache
}

func NewServer(deployHandler *handlers.DeployHandler,
	proxyHandler *handlers.ProxyHandler,
	provisionHandler *handlers.ProvisionHandler,
	jobsHandler *handlers.JobsHandler,
	st store.Store,
	c *cache.RedisCache) *Server {

	e := echo.New()
	e.HideBanner = true
	e.Validator = NewAppValidator()

	e.Use(middleware.Recover())

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true,
		LogURI:    true,
		LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			slog.Info("request",
				"method", v.Method,
				"uri", v.URI,
				"status", v.Status,
			)
			return nil
		},
	}))

	e.GET("/healthz", func(c echo.Context) error {
		return c.String(200, "ok")
	})

	srv := &Server{e: e, store: st, cache: c}
	e.GET("/readyz", srv.handleReadyz)

	e.POST("/api/deploy", deployHandler.PostDeploy)
	e.POST("/v1/chat/completions", proxyHandler.Proxy)
	e.POST("/api/provision", provisionHandler.HandleProvision)
	e.POST("/api/destroy", provisionHandler.HandleDestroy)
	e.GET("/api/jobs/:id", jobsHandler.GetJobStatus)
	e.GET("/api/jobs", jobsHandler.ListJobs)
	e.GET("/api/status/:jobId", jobsHandler.GetStatusByJobID)
	e.GET("/api/clusters", provisionHandler.HandleListClusters)
	e.GET("/api/deployments", deployHandler.HandleListDeployments)
	e.DELETE("/api/deployments/:id", deployHandler.HandleDeleteDeployment)
	return srv

}
func (s *Server) Start(address string) error {
	return s.e.Start(address)
}

func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down HTTP server, draining in-flight requests")
	return s.e.Shutdown(ctx)
}

// GracefulShutdown blocks until the server is stopped.
// Call Start in a separate goroutine, then call this to wait for shutdown via SIGTERM/SIGINT.
func (s *Server) GracefulShutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		slog.Error("server shutdown timed out", "error", err)
	}
}

// handleReadyz checks that all backing services (Postgres, Redis) are reachable.
// Checks are performed concurrently with an internal timeout to prevent cascading hangs.
// Returns 200 if all checks pass, 503 if any fail.
// Response body is JSON: {"status":"ready"} or {"status":"not_ready","errors":{...}}
func (s *Server) handleReadyz(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	errs := make(map[string]string)
	errCh := make(chan struct {
		service string
		err     error
	}, 2)

	var expectedChecks int

	if s.store != nil {
		expectedChecks++
		go func() {
			err := s.store.Ping(ctx)
			errCh <- struct {
				service string
				err     error
			}{"postgres", err}
		}()
	}

	if s.cache != nil {
		expectedChecks++
		go func() {
			err := s.cache.Ping(ctx)
			errCh <- struct {
				service string
				err     error
			}{"redis", err}
		}()
	}

	for i := 0; i < expectedChecks; i++ {
		res := <-errCh
		if res.err != nil {
			errs[res.service] = res.err.Error()
		}
	}

	if len(errs) > 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"errors": errs,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "ready",
	})
}
