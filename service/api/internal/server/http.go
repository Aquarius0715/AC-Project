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
func newEcho(
	reg *ops.Registry,
	txm *db.TxManager,
	authenticator *auth.Authenticator,
	logger *slog.Logger,
	internalToken string,
	servesIdentity bool,
) (*echo.Echo, error) {
	app := echo.New()
	if logger != nil {
		app.Logger = logger
	}

	app.HTTPErrorHandler = ops.HTTPErrorHandler
	app.Use(middleware.Recover())
	app.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		Generator:        func() string { return uuid.Must(uuid.NewV7()).String() }, // correlation IDs are UUIDv7 (D07)
		Skipper:          nil,
		RequestIDHandler: nil,
		TargetHeader:     "",
	}))
	app.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogMethod:    true,
		LogURI:       true,
		LogRequestID: true,
		LogLatency:   true,
		HandleError:  true,
		LogValuesFunc: func(ectx *echo.Context, v middleware.RequestLoggerValues) error {
			level := slog.LevelInfo
			if v.Status >= http.StatusInternalServerError {
				level = slog.LevelError
			}

			app.Logger.LogAttrs(
				ectx.Request().Context(),
				level,
				"request",
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int(
					"status",
					v.Status,
				),
				slog.String("request_id", v.RequestID),
				slog.Duration("latency", v.Latency),
			)

			return nil
		},
		Skipper:          nil,
		BeforeNextFunc:   nil,
		LogProtocol:      false,
		LogRemoteIP:      false,
		LogHost:          false,
		LogURIPath:       false,
		LogRoutePath:     false,
		LogReferer:       false,
		LogUserAgent:     false,
		LogContentLength: false,
		LogResponseSize:  false,
		LogHeaders:       nil,
		LogQueryParams:   nil,
		LogFormValues:    nil,
	}))
	app.Use(middleware.ContextTimeout(requestTimeout))

	app.GET("/healthz", func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	app.GET("/readyz", func(ectx *echo.Context) error {
		err := txm.Writer.Ping(ectx.Request().Context())
		if err != nil {
			return apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
		}

		return ectx.NoContent(http.StatusOK)
	})

	if internalToken != "" && servesIdentity {
		app.GET(
			auth.PrincipalPath,
			principalHandler(authenticator, internalToken),
		) // IR181 step 1; not routed by the gateway
	}

	if internalToken != "" { // internal queries between the services (IR190, IR195); not routed by the gateway
		app.Group("/internal/v1", authenticator.Middleware()).
			POST("/queries/:query", reg.ServeQuery(internalToken))
		app.POST(
			ops.SystemQueryPath,
			reg.ServeSystemQuery(internalToken),
		) // worker calls without a user
	}

	v1 := app.Group("/v1", authenticator.Middleware())

	err := reg.MountREST(
		v1,
	)
	if err != nil { // the catalog's REST routes of the served operations (IR222)
		return nil, apperr.Wrap(err, "ops.Registry.MountREST")
	}

	return app, nil
}

// principalHandler is identity-api's internal principal endpoint: it resolves a token subject's membership from the
// identity tables for the other services (RemoteSource) and records the token's sign-in time when given (IR268).
// Callers authenticate with the shared INTERNAL_API_TOKEN.
func principalHandler(authenticator *auth.Authenticator, token string) echo.HandlerFunc {
	return func(ectx *echo.Context) error {
		got, _ := strings.CutPrefix(ectx.Request().Header.Get(echo.HeaderAuthorization), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			return apperr.E(apperr.Unauthenticated, apperr.KeyUnauthenticated)
		}

		params := ectx.QueryParams()
		tenant, err1 := uuid.Parse(params.Get("tenant"))
		membership, err2 := uuid.Parse(params.Get("membership"))

		var (
			signedIn time.Time
			err3     error
		)
		if raw := params.Get("signedIn"); raw != "" {
			signedIn, err3 = time.Parse(time.RFC3339, raw)
		}

		if err1 != nil || err2 != nil || err3 != nil || params.Get("subject") == "" {
			return apperr.Fields(map[string]string{"_": apperr.KeyMalformedInput})
		}

		principal, err := authenticator.Principal(
			ectx.Request().Context(),
			params.Get("subject"),
			tenant,
			membership,
			signedIn,
		)
		if err != nil {
			de := apperr.From(err)
			if de.Code == apperr.NotFound {
				return apperr.E(apperr.NotFound, apperr.KeyMembershipInvalid)
			}

			return apperr.Wrap(err, "auth.Authenticator.Principal")
		}

		return ectx.JSON(http.StatusOK, auth.ToDTO(principal))
	}
}
