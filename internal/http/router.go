package http

import (
	"context"
	"log/slog"
	"simplek8/internal/http/handlers"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	e *echo.Echo
}

func NewServer(deployHandler *handlers.DeployHandler,
	proxyHandler *handlers.ProxyHandler,
	provisionHandler *handlers.ProvisionHandler,
	jobsHandler *handlers.JobsHandler) *Server {

	e := echo.New()
	e.HideBanner = true

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
	e.POST("/api/deploy", deployHandler.PostDeploy)
	e.POST("/v1/chat/completions", proxyHandler.Proxy)
	e.POST("/api/provision", provisionHandler.HandleProvision)
	e.POST("/api/destroy", provisionHandler.HandleDestroy)
	e.GET("/api/jobs/:id", jobsHandler.GetJobStatus)
	e.GET("/api/clusters", provisionHandler.HandleListClusters)
	e.GET("/api/deployments", deployHandler.HandleListDeployments)
	e.DELETE("/api/deployments/:id", deployHandler.HandleDeleteDeployment)
	return &Server{e: e}

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
		slog.Error("HTTP server shutdown timed out or failed", "error", err)
	}
}
