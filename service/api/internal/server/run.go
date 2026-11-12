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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
	"github.com/pradita/ac-project/service/api/internal/platform/health"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	"github.com/pradita/ac-project/service/api/internal/platform/logging"
	"github.com/pradita/ac-project/service/api/internal/platform/ratelimit"
)

var (
	// errWeakInternalToken refuses a service token that is short, or the local one outside the demo.
	errWeakInternalToken = errors.New(
		"INTERNAL_API_TOKEN: at least 32 characters and not the local value outside the demo",
	)
	// errNotPositive refuses a count setting that is not a whole number above zero.
	errNotPositive = errors.New("not a whole number above zero")
	// errPartialIDP refuses an identity provider set up in part.
	errPartialIDP = errors.New(
		"IDP_TOKEN_URL, IDP_ADMIN_URL, IDP_ADMIN_CLIENT_ID and IDP_ADMIN_CLIENT_SECRET go together",
	)
	// errNotHTTPURL refuses a setting that should be an http(s) URL.
	errNotHTTPURL = errors.New("not an http(s) URL")
)

// consumerPoll is how often the service's event subscribers look for new events, and gracefulTimeout how long the
// requests in flight may finish after SIGTERM (ECS stopTimeout is 30 s).
const (
	consumerPoll    = 250 * time.Millisecond
	gracefulTimeout = 25 * time.Second
)

// Main runs one domain service (IR180): it serves only the operations of domains, with /healthz and /readyz and the
// container healthcheck subcommand (container design §3). It returns the process's exit code, for main to pass to
// os.Exit.
func Main(service string, domains ...string) int {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		return health.Check(envOr("ADDR", ":8080"))
	}

	err := run(service, domains)
	if err != nil {
		slog.Error("startup", "err", err)

		return 1
	}

	return 0
}

// run serves until SIGINT or SIGTERM; Main exits on its error once its deferred clean-up has run.
func run(service string, domains []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := logging.New(
		os.Stdout,
		service,
	) // JSON logs that mask secrets and personal data (backend Go design: log/slog, IR369)
	slog.SetDefault(logger)

	cfg, err := ConfigFromEnv(logger, domains)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	verifier, err := auth.NewVerifierFromEnv(ctx)
	if err != nil {
		return apperr.Wrap(err, "auth.NewVerifierFromEnv")
	}

	srv, err := New(ctx, cfg, verifier)
	if err != nil {
		return err
	}
	defer srv.DB.Close()
	// graceful shutdown as in the Echo cookbook: StartConfig.Start returns once ctx is cancelled (SIGINT/SIGTERM)
	// and in-flight requests have finished or GracefulTimeout has passed (ECS stopTimeout is 30 s)
	for _, c := range srv.Consumers { // event subscribers of this service (IR183)
		c.Logf = func(f string, a ...any) { logger.Warn(fmt.Sprintf(f, a...)) }
		go c.Run(ctx, consumerPoll)
	}

	return serve(ctx, cfg.Addr, srv.Echo)
}

// serve runs e on addr until ctx is cancelled and the requests in flight have finished or gracefulTimeout has passed.
// A normal stop returns nil; a failure to listen is an error, so the process exits non-zero.
func serve(ctx context.Context, addr string, e *echo.Echo) error {
	startConfig := echo.StartConfig{
		Address:          addr,
		HideBanner:       true,
		HidePort:         true,
		GracefulTimeout:  gracefulTimeout,
		CertFilesystem:   nil,
		TLSConfig:        nil,
		Listener:         nil,
		ListenerNetwork:  "",
		ListenerAddrFunc: nil,
		OnShutdownError:  nil,
		BeforeServeFunc:  nil,
	}

	err := startConfig.Start(ctx, e)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return apperr.Wrap(err, "serve")
	}

	return nil
}

// writeLimitDefault is the writes per membership and minute when WRITE_LIMIT_PER_MINUTE does not set another
// (network architecture §6).
const writeLimitDefault = 60

// withWriteLimit sets the write limit per membership (IR375) and the record of denied writes (IR603) on the shared
// counter store at address.
func withWriteLimit(cfg *Config, address string) error {
	store, err := ratelimit.NewValkey(address)
	if err != nil {
		return fmt.Errorf("RATE_LIMIT_URL: %w", err)
	}

	limit := writeLimitDefault
	if v := os.Getenv("WRITE_LIMIT_PER_MINUTE"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 {
			return fmt.Errorf("WRITE_LIMIT_PER_MINUTE: %q: %w", v, errNotPositive)
		}
	}

	cfg.WriteLimit = &ratelimit.Window{
		Store:  store,
		Limit:  limit,
		Window: time.Minute,
		Prefix: "ac:rl:write:", Now: nil,
	}
	// denied writes recorded in the audit log: one per actor and operation a minute, across instances (IR603)
	cfg.DenyLimit = &ratelimit.Window{
		Store:  store,
		Limit:  1,
		Window: time.Minute,
		Prefix: "ac:rl:denied:", Now: nil,
	}

	return nil
}

// idpEnv are the IDP_* settings of the identity provider's admin API: token URL, admin URL, client ID and secret.
func idpEnv() []string {
	return []string{
		os.Getenv("IDP_TOKEN_URL"),
		os.Getenv("IDP_ADMIN_URL"),
		os.Getenv("IDP_ADMIN_CLIENT_ID"),
		os.Getenv("IDP_ADMIN_CLIENT_SECRET"),
	}
}

// identityProvider is the identity provider's admin API from the IDP_* settings: the token endpoint and admin API of
// the realm with a service-account client. All four go together, and the URLs are http(s).
func identityProvider(values []string) (*idp.Keycloak, error) {
	if slices.Contains(values, "") {
		return nil, errPartialIDP
	}

	for i, key := range []string{"IDP_TOKEN_URL", "IDP_ADMIN_URL"} {
		u, err := url.Parse(values[i])
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("%s: %w", key, errNotHTTPURL)
		}
	}

	// IDP_PUBLIC_URL is optional: the address people open the identity provider by, when it answers on several (IR651)
	public := os.Getenv("IDP_PUBLIC_URL")
	if public != "" {
		u, err := url.Parse(public)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("IDP_PUBLIC_URL: %w", errNotHTTPURL)
		}
	}

	return &idp.Keycloak{
		TokenURL:     values[0],
		AdminURL:     strings.TrimRight(values[1], "/"),
		ClientID:     values[2],
		ClientSecret: values[3], PublicURL: public, Client: nil, Now: nil,
	}, nil
}

// minInternalToken is the shortest service token accepted outside the demo.
const minInternalToken = 32

// checkInternalToken refuses, outside the demo, a service token that is not a real secret: a short one, or the public
// local value of compose.yaml (IR430), which anyone could send to the internal endpoints.
func checkInternalToken(token string, demo bool) error {
	if !demo && token != "" &&
		(len(token) < minInternalToken || strings.Contains(token, "change-me")) {
		return errWeakInternalToken
	}

	return nil
}

// serviceURLsFromEnv are the other services' URLs, <DOMAIN>_API_URL, where set (IR190).
func serviceURLsFromEnv() map[string]string {
	urls := map[string]string{}

	for _, d := range ops.Domains {
		if v := os.Getenv(strings.ToUpper(d) + "_API_URL"); v != "" {
			urls[d] = v
		}
	}

	return urls
}

// accountsPerDayFromEnv is ACCOUNTS_PER_ORG_PER_DAY, the accounts a contractor or customer owner adds per organization
// in 24 hours (IR464); 0 when it is unset, which keeps the default of 30.
func accountsPerDayFromEnv() (int, error) {
	raw := os.Getenv("ACCOUNTS_PER_ORG_PER_DAY")
	if raw == "" {
		return 0, nil
	}

	accounts, err := strconv.Atoi(raw)
	if err != nil || accounts < 1 {
		return 0, fmt.Errorf("ACCOUNTS_PER_ORG_PER_DAY: %q: %w", raw, errNotPositive)
	}

	return accounts, nil
}

// accountAppsFromEnv are the apps a new account's set-up e-mail returns to (IR461).
func accountAppsFromEnv() identity.AccountApps {
	return identity.AccountApps{
		Customer:   envOr("APP_URL_CUSTOMER", "http://localhost:3000"),
		Partner:    envOr("APP_URL_PARTNER", "http://localhost:3001"),
		Technician: envOr("APP_URL_TECHNICIAN", "http://localhost:3002"),
		Admin:      envOr("APP_URL_ADMIN", "http://localhost:3003"),
	}
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
		ServiceURLs:       serviceURLsFromEnv(),
		Clock:             nil,
		DemoShared:        false,
		DemoStart:         time.Time{},
		PrincipalTTL:      0,
		DemoClock:         nil,
		WriteLimit:        nil,
		DenyLimit:         nil,
		IdentityProvider:  nil,
		AccountApps:       accountAppsFromEnv(),
		AccountsPerDay:    0,
		WrapTx:            nil,
	}

	err := checkInternalToken(cfg.InternalToken, cfg.DemoOps)
	if err != nil {
		return cfg, err
	}
	// the write limit per membership (IR375): RATE_LIMIT_URL is the shared counter store, WRITE_LIMIT_PER_MINUTE the
	// limit (60 by default, network architecture §6); without a store there is no limit
	if address := os.Getenv("RATE_LIMIT_URL"); address != "" {
		err := withWriteLimit(&cfg, address)
		if err != nil {
			return cfg, err
		}
	}

	cfg.AccountsPerDay, err = accountsPerDayFromEnv()
	if err != nil {
		return cfg, err
	}
	// the identity provider's admin API ends a user's sessions when HQ signs them out everywhere (IR386)
	if values := idpEnv(); strings.Join(values, "") != "" {
		provider, err := identityProvider(values)
		if err != nil {
			return cfg, err
		}

		cfg.IdentityProvider = provider
	}

	// fixture.clock unless DEMO_CLOCK_START overrides it (IR36), or the real clock (now, DEC-96); shared with the
	// workers (IR168)
	if cfg.DemoOps {
		start, err := democlock.StartFromEnv(os.Getenv)
		if err != nil {
			return cfg, fmt.Errorf("DEMO_CLOCK_START: %w", err)
		}

		cfg.DemoShared, cfg.DemoStart = true, start
	}

	return cfg, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}

	return def
}
