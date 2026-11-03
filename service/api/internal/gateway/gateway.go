// Package gateway is the single entry of the Core API for the web apps (IR180): the REST routes of the operation
// catalog (IR222) are forwarded unchanged (headers, query, body, status) to the business-domain service that owns
// the route's operation. It holds no business logic and does not authenticate; every
// service verifies the token itself. In production the ALB can route the same paths from the catalog's route table
// without this process.
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
	targets := Targets{}

	for _, domain := range ops.Domains {
		key := envKey(domain)

		raw := getenv(key)
		if raw == "" {
			return nil, fmt.Errorf("gateway: %s is required", key)
		}

		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("gateway: %s is not an absolute URL", key)
		}

		targets[domain] = u
	}

	return targets, nil
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

// setNotSniffedOrCached sets the two headers every gateway response carries: the browser keeps no copy of the body and
// does not guess its type (personal data, IR407).
func setNotSniffedOrCached(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
}

// notSniffedOrCached is the middleware that sets setNotSniffedOrCached on the gateway's own responses (/healthz, /docs,
// errors of unknown routes).
func notSniffedOrCached(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		setNotSniffedOrCached(c.Response().Header())

		return next(c)
	}
}

// New builds the gateway Echo instance (same middleware conventions as the services, IR173).
func New(targets Targets, logger *slog.Logger) *echo.Echo {
	app := echo.New()
	if logger != nil {
		app.Logger = logger
	}

	app.HTTPErrorHandler = ops.HTTPErrorHandler
	app.Use(middleware.Recover())
	// the request ID is the gateway's own: one a client sends is dropped, so no caller chooses the correlation ID the
	// services log and store with its writes (IR435)
	app.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Request().Header.Del(echo.HeaderXRequestID)

			return next(c)
		}
	})
	app.Use(
		middleware.RequestIDWithConfig(
			middleware.RequestIDConfig{
				Generator:        func() string { return uuid.Must(uuid.NewV7()).String() },
				Skipper:          nil,
				RequestIDHandler: nil,
				TargetHeader:     "",
			},
		),
	)
	app.Use(notSniffedOrCached)

	proxies := map[string]*httputil.ReverseProxy{}

	for domain, u := range targets {
		reverseProxy := httputil.NewSingleHostReverseProxy(u)
		// set field by field: a literal would have to name every field, Dial and DialTLS too, which are deprecated
		transport := new(http.Transport)
		transport.ResponseHeaderTimeout = 35 * time.Second
		transport.MaxIdleConnsPerHost = 64
		reverseProxy.Transport = transport
		// the gateway's middleware sets the request ID and the two headers on every response; the upstream's copies of
		// them are dropped here, or the proxy would add them a second time (IR407)
		reverseProxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Del(echo.HeaderXRequestID)
			resp.Header.Del("X-Content-Type-Options")
			resp.Header.Del("Cache-Control")

			return nil
		}
		reverseProxy.ErrorHandler = func(writer http.ResponseWriter, r *http.Request, err error) {
			app.Logger.Error("upstream failed", "domain", domain, "error", err)

			domainErr := apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
			domainErr.CorrelationID = writer.Header().Get(echo.HeaderXRequestID)
			writer.Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			writer.WriteHeader(domainErr.HTTPStatus())
			_, _ = fmt.Fprintf(
				writer,
				`{"code":%q,"messageKey":%q,"fieldErrors":{},"correlationId":%q,"retryAfterSeconds":null}`,
				domainErr.Code,
				domainErr.MessageKey,
				domainErr.CorrelationID,
			)
		}
		proxies[domain] = reverseProxy
	}

	app.GET("/healthz", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	mountDocs(app) // /docs and /swagger.json (IR220)

	for _, s := range ops.Catalog { // REST routes go to the owning service unchanged (IR222)
		p := proxies[ops.DomainOf(s.Name)]
		for _, rt := range s.Routes {
			app.Add(
				rt.Method,
				ops.EchoPath(rt.Path),
				func(c *echo.Context) error { return forward(c, p) },
			)
		}
	}

	return app
}

func forward(ectx *echo.Context, reverseProxy *httputil.ReverseProxy) error {
	if reverseProxy == nil {
		return apperr.E(apperr.NotFound, apperr.KeyUnknownOperation)
	}
	// the services keep the correlation ID: forward the one this request got
	ectx.Request().Header.Set(
		echo.HeaderXRequestID,
		ectx.Response().Header().Get(echo.HeaderXRequestID),
	)
	reverseProxy.ServeHTTP(ectx.Response(), ectx.Request())

	return nil
}
