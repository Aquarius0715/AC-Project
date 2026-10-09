// Command gateway is the Core API entry for the web apps (IR180): it forwards the REST routes of the operation
// catalog (IR222) to the business-domain service that owns each route's operation (IDENTITY_API_URL,
// EQUIPMENT_API_URL, MAINTENANCE_API_URL, BILLING_API_URL, ENERGY_API_URL).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/gateway"
)

// @title						AC Project Core API
// @version					1.0
// @description				Every operation of the operation catalog is served at its REST routes (IR222): reads with GET and their input in the path and the query string (cursor, limit, sort=field:direction, the catalogued filters by name; lists repeat the parameter or separate values with commas, an empty value is the empty list; nested fields as object.field); creates with POST on the collection, updates with PUT on the item, deletes with DELETE (input in the path and the query string); business commands with POST /v1/<resource>/{id}/<verb>. POST, PUT and PATCH take the handler's input type as a JSON body without query parameters; a path parameter or a fixed route field that the body repeats with another value is VALIDATION error.pathMismatch; unknown fields and parameters are VALIDATION; bodies above 1 MiB are rejected (8 MiB for photo uploads, 16 MiB for a sign-off). The caller is the BFF of one of the four web apps, which forwards the user's Keycloak access token; the gateway forwards each route to the domain service that owns its operation (identity, equipment, maintenance, billing, energy). The operation name stays the operation ID (catalog key, authorization, audit, meta.operation).
// @description				Writes carry an Idempotency-Key (8–128 characters): the same key replays the stored response (D04). Writes with a version target carry X-Expected-Version (the write-version catalog says which) and answer CONFLICT error.versionConflict when it is stale.
// @description				A success is {data, meta}; a failure is a ServiceError {code, messageKey, fieldErrors, correlationId, retryAfterSeconds} with the HTTP status of its code: VALIDATION 422, UNAUTHENTICATED 401, FORBIDDEN 403, NOT_FOUND 404, CONFLICT and OFFLINE 409, RATE_LIMITED 429, UNAVAILABLE 503, TIMEOUT 504.
// @description				Built by `make swagger` from the swag annotations of the handlers (service/api/internal/modules, server); do not edit swagger.json by hand.
// @BasePath					/
// @schemes					http https
//
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Keycloak access token of the signed-in membership, forwarded by the BFF: "Bearer <token>"
func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" { // container healthcheck subcommand
		r, err := http.Get("http://127.0.0.1" + addr + "/healthz")
		if err != nil || r.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "gateway")
	targets, err := gateway.TargetsFromEnv(os.Getenv)
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	sc := echo.StartConfig{Address: addr, HideBanner: true, HidePort: true, GracefulTimeout: 25 * time.Second}
	if err := sc.Start(ctx, gateway.New(targets, logger)); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("serve", "err", err)
	}
}
