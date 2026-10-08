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
