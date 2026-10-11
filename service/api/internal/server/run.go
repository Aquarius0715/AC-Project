package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pradita/ac-project/service/api/internal/ops"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	"github.com/pradita/ac-project/service/api/internal/platform/logging"
	"github.com/pradita/ac-project/service/api/internal/platform/ratelimit"
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
	logger := logging.New(os.Stdout, service) // JSON logs that mask secrets and personal data (backend Go design: log/slog, IR369)
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
	// the write limit per membership (IR375): RATE_LIMIT_URL is the shared counter store, WRITE_LIMIT_PER_MINUTE the
	// limit (60 by default, network architecture §6); without a store there is no limit
	if url := os.Getenv("RATE_LIMIT_URL"); url != "" {
		store, err := ratelimit.NewValkey(url)
		if err != nil {
			return cfg, fmt.Errorf("RATE_LIMIT_URL: %w", err)
		}
		limit := 60
		if v := os.Getenv("WRITE_LIMIT_PER_MINUTE"); v != "" {
			if limit, err = strconv.Atoi(v); err != nil || limit < 1 {
				return cfg, fmt.Errorf("WRITE_LIMIT_PER_MINUTE: %q", v)
			}
		}
		cfg.WriteLimit = &ratelimit.Window{Store: store, Limit: limit, Window: time.Minute, Prefix: "ac:rl:write:"}
	}
	// the identity provider's admin API ends a user's sessions when HQ signs them out everywhere (IR386): the token
	// endpoint and admin API of the realm with a service-account client; all four or none
	if v := []string{os.Getenv("IDP_TOKEN_URL"), os.Getenv("IDP_ADMIN_URL"), os.Getenv("IDP_ADMIN_CLIENT_ID"), os.Getenv("IDP_ADMIN_CLIENT_SECRET")}; v[0]+v[1]+v[2]+v[3] != "" {
		if v[0] == "" || v[1] == "" || v[2] == "" || v[3] == "" {
			return cfg, fmt.Errorf("IDP_TOKEN_URL, IDP_ADMIN_URL, IDP_ADMIN_CLIENT_ID and IDP_ADMIN_CLIENT_SECRET go together")
		}
		for i, k := range []string{"IDP_TOKEN_URL", "IDP_ADMIN_URL"} {
			if u, err := url.Parse(v[i]); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return cfg, fmt.Errorf("%s: not an http(s) URL", k)
			}
		}
		cfg.IdentityProvider = &idp.Keycloak{TokenURL: v[0], AdminURL: strings.TrimRight(v[1], "/"), ClientID: v[2], ClientSecret: v[3]}
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
