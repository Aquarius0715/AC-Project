package server

import (
	"os"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	"github.com/pradita/ac-project/service/api/internal/platform/ratelimit"
)

// ConfigFromEnv reads a domain service's environment (container design §4). The write limit (IR375) is off without
// RATE_LIMIT_URL, 60 by default with it, and a bad limit or URL stops the start instead of running unlimited. The
// identity provider's admin API (IR386) takes all four IDP_* values or none, and its URLs are http(s).
func TestConfigFromEnv(t *testing.T) {
	idpEnv := []string{"IDP_TOKEN_URL", "IDP_ADMIN_URL", "IDP_ADMIN_CLIENT_ID", "IDP_ADMIN_CLIENT_SECRET"}
	for _, k := range append([]string{"RATE_LIMIT_URL", "WRITE_LIMIT_PER_MINUTE", "DEMO_OPS", "DEMO_CLOCK_START", "ADDR", "ENERGY_API_URL"}, idpEnv...) {
		t.Setenv(k, "")
	}
	cfg, err := ConfigFromEnv(nil, []string{"identity"})
	if err != nil || cfg.WriteLimit != nil || cfg.Addr != ":8080" || len(cfg.Domains) != 1 || len(cfg.ServiceURLs) != 0 || cfg.IdentityProvider != nil {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	t.Setenv("RATE_LIMIT_URL", "redis://127.0.0.1:1/0")
	t.Setenv("ENERGY_API_URL", "http://energy-api:8080")
	t.Setenv("ADDR", ":9090")
	cfg, err = ConfigFromEnv(nil, nil)
	w, ok := cfg.WriteLimit.(*ratelimit.Window)
	if err != nil || !ok || w.Limit != 60 || w.Window != time.Minute || cfg.ServiceURLs["energy"] != "http://energy-api:8080" || cfg.Addr != ":9090" {
		t.Fatalf("limit on: %+v %v", cfg, err)
	}
	t.Setenv("WRITE_LIMIT_PER_MINUTE", "5")
	if cfg, err = ConfigFromEnv(nil, nil); err != nil || cfg.WriteLimit.(*ratelimit.Window).Limit != 5 {
		t.Fatalf("limit 5: %v", err)
	}
	for _, bad := range []string{"0", "-1", "sixty"} {
		t.Setenv("WRITE_LIMIT_PER_MINUTE", bad)
		if _, err := ConfigFromEnv(nil, nil); err == nil {
			t.Errorf("WRITE_LIMIT_PER_MINUTE=%q accepted", bad)
		}
	}
	t.Setenv("WRITE_LIMIT_PER_MINUTE", "")
	t.Setenv("RATE_LIMIT_URL", "http://valkey:6379")
	if _, err := ConfigFromEnv(nil, nil); err == nil {
		t.Error("a RATE_LIMIT_URL that is not redis:// accepted")
	}
	t.Setenv("RATE_LIMIT_URL", "")
	t.Setenv("DEMO_OPS", "1")
	t.Setenv("DEMO_CLOCK_START", "yesterday")
	if _, err := ConfigFromEnv(nil, nil); err == nil {
		t.Error("a bad DEMO_CLOCK_START accepted")
	}
	t.Setenv("DEMO_OPS", "")
	t.Setenv("DEMO_CLOCK_START", "")

	t.Setenv("IDP_TOKEN_URL", "http://idp.test/realms/ac/protocol/openid-connect/token")
	t.Setenv("IDP_ADMIN_URL", "http://idp.test/admin/realms/ac/")
	t.Setenv("IDP_ADMIN_CLIENT_ID", "svc")
	t.Setenv("IDP_ADMIN_CLIENT_SECRET", "test-secret")
	cfg, err = ConfigFromEnv(nil, []string{"identity"})
	k, ok := cfg.IdentityProvider.(*idp.Keycloak)
	if err != nil || !ok || k.AdminURL != "http://idp.test/admin/realms/ac" || k.ClientID != "svc" || k.ClientSecret != "test-secret" {
		t.Fatalf("identity provider: %+v %v", cfg.IdentityProvider, err)
	}
	for _, missing := range idpEnv { // one value without the others is a mistake, not "off"
		old := os.Getenv(missing)
		t.Setenv(missing, "")
		if _, err := ConfigFromEnv(nil, nil); err == nil {
			t.Errorf("IDP_* without %s accepted", missing)
		}
		t.Setenv(missing, old)
	}
	for _, bad := range []string{"keycloak:8080/admin/realms/ac", "ftp://idp.test/admin", "http:///admin"} {
		t.Setenv("IDP_ADMIN_URL", bad)
		if _, err := ConfigFromEnv(nil, nil); err == nil {
			t.Errorf("IDP_ADMIN_URL=%q accepted", bad)
		}
	}
}
