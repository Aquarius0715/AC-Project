// Command gateway is the Core API entry for the web apps (IR180): it forwards POST /v1/ops/:operation to the
// business-domain service that owns the operation (IDENTITY_API_URL, EQUIPMENT_API_URL, MAINTENANCE_API_URL,
// BILLING_API_URL, ENERGY_API_URL).
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
// @description				Every operation of the operation catalog is POST /v1/ops/{operation} with a JSON body (the handler's input type; unknown fields are rejected with VALIDATION error.malformedInput, an empty body is {}, bodies above 1 MiB are rejected). The caller is the BFF of one of the four web apps, which forwards the user's Keycloak access token; the gateway forwards each operation to the domain service that owns it (identity, equipment, maintenance, billing, energy).
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
