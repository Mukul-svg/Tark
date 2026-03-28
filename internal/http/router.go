package http

import (
	"log/slog"
	"simplek8/internal/http/handlers"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Server struct {
	e *echo.Echo
}

func NewServer(deployHandler *handlers.DeployHandler,
	proxyHandler *handlers.ProxyHandler,
	provsionHandler *handlers.ProvisionHandler,
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
	e.POST("/api/provision", provsionHandler.HandleProvision)
	e.POST("/api/destroy", provsionHandler.HandleDestroy)
	e.GET("/api/jobs/:id", jobsHandler.GetJobStatus)
	return &Server{e: e}

}
func (s *Server) Start(address string) error {
	return s.e.Start(address)
}
