package server_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	"github.com/pradita/ac-project/service/api/internal/platform/ratelimit"
	"github.com/pradita/ac-project/service/api/internal/server"
)

// envRateLimit and envWriteLimit are the settings the tests change most, and unusedRedis a Redis URL that nothing
// listens on.
const (
	envRateLimit  = "RATE_LIMIT_URL"
	envWriteLimit = "WRITE_LIMIT_PER_MINUTE"
	unusedRedis   = "redis://127.0.0.1:1/0"
)

// idpEnv are the settings of the identity provider's admin API, which go together (IR386).
func idpEnv() []string {
	return []string{
		"IDP_TOKEN_URL",
		"IDP_ADMIN_URL",
		"IDP_ADMIN_CLIENT_ID",
		"IDP_ADMIN_CLIENT_SECRET",
	}
}

// clearEnv starts a test from an environment without the settings ConfigFromEnv reads.
func clearEnv(t *testing.T) {
	t.Helper()

	for _, k := range append(
		[]string{
			envRateLimit, envWriteLimit, "DEMO_OPS", "DEMO_CLOCK_START", "ADDR", "ENERGY_API_URL",
			"INTERNAL_API_TOKEN",
		}, idpEnv()...) {
		t.Setenv(k, "")
	}
}

// ConfigFromEnv reads a domain service's environment (container design §4). Without settings the service listens on
// :8080 with no write limit, no other services and no identity provider.
//
//nolint:paralleltest // clearEnv sets the environment (t.Setenv), so the test cannot run in parallel
func TestConfigFromEnvDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := server.ConfigFromEnv(nil, []string{"identity"})
	if err != nil || cfg.WriteLimit != nil || cfg.Addr != ":8080" || len(cfg.Domains) != 1 ||
		len(cfg.ServiceURLs) != 0 ||
		cfg.IdentityProvider != nil {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
}

// TestDemoClockFromEnv: in the demo the scenario clock is the shared one, from fixture.clock unless DEMO_CLOCK_START
// names another start or now, the real clock (the zero start, DEC-96); outside the demo there is none.
func TestDemoClockFromEnv(t *testing.T) {
	clearEnv(t)

	cfg, err := server.ConfigFromEnv(nil, nil)
	if err != nil || cfg.DemoShared || !cfg.DemoStart.IsZero() {
		t.Fatalf("outside the demo: shared %t from %v, %v", cfg.DemoShared, cfg.DemoStart, err)
	}

	t.Setenv("DEMO_OPS", "1")

	for value, want := range map[string]string{"": democlock.DefaultStart, democlock.RealTime: ""} {
		t.Setenv("DEMO_CLOCK_START", value)

		cfg, err := server.ConfigFromEnv(nil, nil)

		got := ""
		if !cfg.DemoStart.IsZero() {
			got = cfg.DemoStart.Format(time.RFC3339)
		}

		if err != nil || !cfg.DemoShared || got != want {
			t.Errorf(
				"DEMO_CLOCK_START=%q: shared %t from %q, %v; want %q",
				value,
				cfg.DemoShared,
				got,
				err,
				want,
			)
		}
	}
}

// The write limit (IR375) is on with RATE_LIMIT_URL, 60 a minute unless WRITE_LIMIT_PER_MINUTE sets another.
func TestConfigFromEnvWriteLimit(t *testing.T) {
	clearEnv(t)
	t.Setenv(envRateLimit, unusedRedis)
	t.Setenv("ENERGY_API_URL", "http://energy-api:8080")
	t.Setenv("ADDR", ":9090")

	cfg, err := server.ConfigFromEnv(nil, nil)

	window, matched := cfg.WriteLimit.(*ratelimit.Window)
	if err != nil || !matched || window.Limit != 60 || window.Window != time.Minute ||
		cfg.ServiceURLs["energy"] != "http://energy-api:8080" ||
		cfg.Addr != ":9090" {
		t.Fatalf("limit on: %+v %v", cfg, err)
	}

	t.Setenv(envWriteLimit, "5")

	cfg, err = server.ConfigFromEnv(nil, nil)

	window, matched = cfg.WriteLimit.(*ratelimit.Window)
	if err != nil || !matched || window.Limit != 5 {
		t.Fatalf("limit 5: %v", err)
	}
}

// A bad write limit, a RATE_LIMIT_URL that is not redis:// or a bad DEMO_CLOCK_START stops the start instead of
// running unlimited or on the wrong clock.
//
//nolint:paralleltest // the cases set the environment (t.Setenv), so the test cannot run in parallel
func TestConfigFromEnvRefusesBadSettings(t *testing.T) {
	for _, bad := range []map[string]string{
		{envRateLimit: unusedRedis, envWriteLimit: "0"},
		{envRateLimit: unusedRedis, envWriteLimit: "-1"},
		{envRateLimit: unusedRedis, envWriteLimit: "sixty"},
		{envRateLimit: "http://valkey:6379"},
		{"DEMO_OPS": "1", "DEMO_CLOCK_START": "yesterday"},
	} {
		clearEnv(t)

		for k, v := range bad {
			t.Setenv(k, v)
		}

		_, err := server.ConfigFromEnv(nil, nil)
		if err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

// The identity provider's admin API (IR386) takes all four IDP_* values or none, and its URLs are http(s).
func TestConfigFromEnvIdentityProvider(t *testing.T) {
	clearEnv(t)
	t.Setenv("IDP_TOKEN_URL", "http://idp.test/realms/ac/protocol/openid-connect/token")
	t.Setenv("IDP_ADMIN_URL", "http://idp.test/admin/realms/ac/")
	t.Setenv("IDP_ADMIN_CLIENT_ID", "svc")
	t.Setenv("IDP_ADMIN_CLIENT_SECRET", "test-secret")

	cfg, err := server.ConfigFromEnv(nil, []string{"identity"})

	keycloak, matched := cfg.IdentityProvider.(*idp.Keycloak)
	if err != nil || !matched || keycloak.AdminURL != "http://idp.test/admin/realms/ac" ||
		keycloak.ClientID != "svc" ||
		keycloak.ClientSecret != "test-secret" {
		t.Fatalf("identity provider: %+v %v", cfg.IdentityProvider, err)
	}

	for _, missing := range idpEnv() { // one value without the others is a mistake, not "off"
		old := os.Getenv(missing)
		t.Setenv(missing, "")

		_, err := server.ConfigFromEnv(nil, nil)
		if err == nil {
			t.Errorf("IDP_* without %s accepted", missing)
		}

		t.Setenv(missing, old)
	}

	for _, bad := range []string{"keycloak:8080/admin/realms/ac", "ftp://idp.test/admin", "http:///admin"} {
		t.Setenv("IDP_ADMIN_URL", bad)

		_, err := server.ConfigFromEnv(nil, nil)
		if err == nil {
			t.Errorf("IDP_ADMIN_URL=%q accepted", bad)
		}
	}
}

// TestInternalTokenOutsideTheDemo (IR430): outside the demo a short service token or the public local value of
// compose.yaml stops the start; in the demo the local value is fine.
func TestInternalTokenOutsideTheDemo(t *testing.T) {
	t.Setenv(envRateLimit, "")
	t.Setenv("DEMO_OPS", "")

	for _, bad := range []string{"short", "local-internal-token-change-me"} {
		t.Setenv("INTERNAL_API_TOKEN", bad)

		_, err := server.ConfigFromEnv(
			nil,
			nil,
		)
		if err == nil ||
			!strings.Contains(err.Error(), "INTERNAL_API_TOKEN") {
			t.Errorf("%q outside the demo: %v", bad, err)
		}
	}

	t.Setenv("INTERNAL_API_TOKEN", strings.Repeat("k", 32))

	_, err := server.ConfigFromEnv(nil, nil)
	if err != nil {
		t.Errorf("a real token: %v", err)
	}

	t.Setenv("DEMO_OPS", "1")
	t.Setenv("INTERNAL_API_TOKEN", "local-internal-token-change-me")

	_, err = server.ConfigFromEnv(nil, nil)
	if err != nil {
		t.Errorf("the local value in the demo: %v", err)
	}
}
