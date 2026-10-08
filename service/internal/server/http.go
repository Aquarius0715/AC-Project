package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
	"github.com/pradita/ac-project/service/internal/platform/auth"
	"github.com/pradita/ac-project/service/internal/platform/db"
)

// requestTimeout bounds one operation (Echo ContextTimeout middleware); the database work uses the request context.
const requestTimeout = 30 * time.Second

// newEcho assembles the HTTP layer following the Echo guide: one Echo instance, the Recover / RequestID /
// RequestLogger / ContextTimeout middleware chain, a versioned route group, and a central HTTPErrorHandler that
// turns every returned error into the DomainError body of service-contracts.ts.
func newEcho(reg *ops.Registry, m *db.TxManager, a *auth.Authenticator, logger *slog.Logger) *echo.Echo {
	e := echo.New()
	if logger != nil {
		e.Logger = logger
	}
	e.HTTPErrorHandler = ops.HTTPErrorHandler
	e.Use(middleware.Recover())
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator: func() string { return uuid.Must(uuid.NewV7()).String() }, // correlation IDs are UUIDv7 (D07)
	}))
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus: true, LogMethod: true, LogURI: true, LogRequestID: true, LogLatency: true, HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			level := slog.LevelInfo
			if v.Status >= http.StatusInternalServerError {
				level = slog.LevelError
			}
			e.Logger.LogAttrs(c.Request().Context(), level, "request", slog.String("method", v.Method), slog.String("uri", v.URI),
				slog.Int("status", v.Status), slog.String("request_id", v.RequestID), slog.Duration("latency", v.Latency))
			return nil
		},
	}))
	e.Use(middleware.ContextTimeout(requestTimeout))

	e.GET("/healthz", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/readyz", func(c *echo.Context) error {
		if err := m.Writer.Ping(c.Request().Context()); err != nil {
			return apperr.E(apperr.Unavailable, "error.unavailable")
		}
		return c.NoContent(http.StatusOK)
	})
	v1 := e.Group("/v1", a.Middleware())
	v1.POST("/ops/:operation", reg.Dispatch)
	return e
}
