package server

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
)

// requestTimeout bounds one operation (Echo ContextTimeout middleware); the database work uses the request context.
const requestTimeout = 30 * time.Second

// newEcho assembles the HTTP layer following the Echo guide: one Echo instance, the Recover / RequestID /
// RequestLogger / ContextTimeout middleware chain, a versioned route group, and a central HTTPErrorHandler that
// turns every returned error into the DomainError body of service-contracts.ts.
func newEcho(reg *ops.Registry, m *db.TxManager, a *auth.Authenticator, logger *slog.Logger, internalToken string, servesIdentity bool) *echo.Echo {
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
	if internalToken != "" && servesIdentity {
		e.GET(auth.PrincipalPath, principalHandler(a, internalToken)) // IR181 step 1; not routed by the gateway
	}
	v1 := e.Group("/v1", a.Middleware())
	v1.POST("/ops/:operation", reg.Dispatch)
	return e
}

// principalHandler is identity-api's internal principal endpoint: it resolves a token subject's membership from the
// identity tables for the other services (RemoteSource). Callers authenticate with the shared INTERNAL_API_TOKEN.
func principalHandler(a *auth.Authenticator, token string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		got, _ := strings.CutPrefix(c.Request().Header.Get(echo.HeaderAuthorization), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			return apperr.E(apperr.Unauthenticated, "error.unauthenticated")
		}
		q := c.QueryParams()
		tenant, err1 := uuid.Parse(q.Get("tenant"))
		membership, err2 := uuid.Parse(q.Get("membership"))
		if err1 != nil || err2 != nil || q.Get("subject") == "" {
			return apperr.Fields(map[string]string{"_": "error.malformedInput"})
		}
		p, err := a.Load(c.Request().Context(), q.Get("subject"), tenant, membership)
		if err != nil {
			if de := apperr.From(err); de.Code == apperr.NotFound {
				return apperr.E(apperr.NotFound, "error.membershipInvalid")
			}
			return err
		}
		return c.JSON(http.StatusOK, auth.ToDTO(p))
	}
}
