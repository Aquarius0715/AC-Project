// Command api runs the Core API (POST /v1/ops/:operation, /healthz, /readyz).
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

	"github.com/pradita/ac-project/service/api/internal/app"
	"github.com/pradita/ac-project/service/core/platform/auth"
	"github.com/pradita/ac-project/service/core/platform/democlock"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" { // container healthcheck subcommand (container design §3)
		r, err := http.Get("http://127.0.0.1:8080/healthz")
		if err != nil || r.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg := app.Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		DatabaseReaderURL: os.Getenv("DATABASE_READER_URL"),
		Addr:              envOr("ADDR", ":8080"),
		DemoOps:           os.Getenv("DEMO_OPS") == "1",
	}
	if cfg.DemoOps { // fixture.clock unless DEMO_CLOCK_START overrides it (IR36); shared with the workers (IR168)
		start, err := democlock.StartFromEnv(os.Getenv)
		if err != nil {
			slog.Error("DEMO_CLOCK_START", "err", err)
			os.Exit(1)
		}
		cfg.DemoStart = start
	}
	v, err := auth.NewVerifierFromEnv(ctx)
	if err != nil {
		slog.Error("auth", "err", err)
		os.Exit(1)
	}
	s, err := app.New(ctx, cfg, v)
	if err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	go func() {
		if err := s.Echo.Start(cfg.Addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serve", "err", err)
			stop()
		}
	}()
	<-ctx.Done()
	shut, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	_ = s.Echo.Shutdown(shut)
	s.DB.Close()
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
