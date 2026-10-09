// Package gateway is the single entry of the Core API for the web apps (IR180): POST /v1/ops/:operation is
// forwarded unchanged (headers, body, status) to the business-domain service that owns the operation in the
// operation catalog. It holds no business logic and does not authenticate; every service verifies the token itself.
// In production the ALB can route the same paths (`/v1/ops/<prefix>.*`) without this process.
package gateway

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Targets maps each domain to the base URL of its service.
type Targets map[string]*url.URL

// TargetsFromEnv reads <DOMAIN>_API_URL for every domain (IDENTITY_API_URL, EQUIPMENT_API_URL, …).
func TargetsFromEnv(getenv func(string) string) (Targets, error) {
	t := Targets{}
	for _, d := range ops.Domains {
		key := envKey(d)
		raw := getenv(key)
		if raw == "" {
			return nil, fmt.Errorf("gateway: %s is required", key)
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("gateway: %s is not an absolute URL", key)
		}
		t[d] = u
	}
	return t, nil
}

func envKey(domain string) string {
	b := []byte(domain)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b) + "_API_URL"
}

// New builds the gateway Echo instance (same middleware conventions as the services, IR173).
func New(targets Targets, logger *slog.Logger) *echo.Echo {
	e := echo.New()
	if logger != nil {
		e.Logger = logger
	}
	e.HTTPErrorHandler = ops.HTTPErrorHandler
	e.Use(middleware.Recover())
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{Generator: func() string { return uuid.Must(uuid.NewV7()).String() }}))
	proxies := map[string]*httputil.ReverseProxy{}
	for d, u := range targets {
		p := httputil.NewSingleHostReverseProxy(u)
		p.Transport = &http.Transport{ResponseHeaderTimeout: 35 * time.Second, MaxIdleConnsPerHost: 64}
		p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			e.Logger.Error("upstream failed", "domain", d, "error", err)
			de := apperr.E(apperr.Unavailable, "error.unavailable")
			de.CorrelationID = w.Header().Get(echo.HeaderXRequestID)
			w.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			w.WriteHeader(de.HTTPStatus())
			_, _ = fmt.Fprintf(w, `{"code":%q,"messageKey":%q,"fieldErrors":{},"correlationId":%q,"retryAfterSeconds":null}`, de.Code, de.MessageKey, de.CorrelationID)
		}
		proxies[d] = p
	}
	e.GET("/healthz", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	mountDocs(e) // /docs and /swagger.json (IR220)
	e.POST("/v1/ops/:operation", func(c *echo.Context) error {
		p := proxies[ops.DomainOf(c.Param("operation"))]
		if p == nil {
			return apperr.E(apperr.NotFound, "error.unknownOperation")
		}
		// the services keep the correlation ID: forward the one this request got
		c.Request().Header.Set(echo.HeaderXRequestID, c.Response().Header().Get(echo.HeaderXRequestID))
		p.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	return e
}
