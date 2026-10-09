package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pradita/ac-project/service/api/internal/ops"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
)

// Main runs one domain service (IR180): it serves only the operations of domains, with /healthz and /readyz and the
// container healthcheck subcommand (container design §3).
func Main(service string, domains ...string) {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" { // container healthcheck subcommand (container design §3)
		r, err := http.Get("http://127.0.0.1:8080/healthz")
		if err != nil || r.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service) // JSON logs (backend Go design: log/slog)
	slog.SetDefault(logger)
	cfg, err := ConfigFromEnv(logger, domains)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	v, err := auth.NewVerifierFromEnv(ctx)
	if err != nil {
		slog.Error("auth", "err", err)
		os.Exit(1)
	}
	s, err := New(ctx, cfg, v)
	if err != nil {
		slog.Error("startup", "err", err)
		os.Exit(1)
	}
	// graceful shutdown as in the Echo cookbook: StartConfig.Start returns once ctx is cancelled (SIGINT/SIGTERM)
	// and in-flight requests have finished or GracefulTimeout has passed (ECS stopTimeout is 30 s)
	for _, c := range s.Consumers { // event subscribers of this service (IR183)
		c.Logf = func(f string, a ...any) { logger.Warn(fmt.Sprintf(f, a...)) }
		go c.Run(ctx, 250*time.Millisecond)
	}
	sc := echo.StartConfig{Address: cfg.Addr, HideBanner: true, HidePort: true, GracefulTimeout: 25 * time.Second}
	if err := sc.Start(ctx, s.Echo); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("serve", "err", err)
	}
	s.DB.Close()
}

// ConfigFromEnv reads the configuration of a domain service (or its worker, IR195) from the environment.
func ConfigFromEnv(logger *slog.Logger, domains []string) (Config, error) {
	cfg := Config{
		Logger:            logger,
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		DatabaseReaderURL: os.Getenv("DATABASE_READER_URL"),
		Addr:              envOr("ADDR", ":8080"),
		DemoOps:           os.Getenv("DEMO_OPS") == "1",
		Domains:           domains,
		IdentityURL:       os.Getenv("IDENTITY_INTERNAL_URL"),
		InternalToken:     os.Getenv("INTERNAL_API_TOKEN"),
		ServiceURLs:       map[string]string{},
	}
	for _, d := range ops.Domains { // <DOMAIN>_API_URL of the other services (IR190), optional
		if v := os.Getenv(strings.ToUpper(d) + "_API_URL"); v != "" {
			cfg.ServiceURLs[d] = v
		}
	}
	if cfg.DemoOps { // fixture.clock unless DEMO_CLOCK_START overrides it (IR36); shared with the workers (IR168)
		start, err := democlock.StartFromEnv(os.Getenv)
		if err != nil {
			return cfg, fmt.Errorf("DEMO_CLOCK_START: %w", err)
		}
		cfg.DemoStart = start
	}
	return cfg, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
