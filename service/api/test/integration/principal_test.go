package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// IR181 step 1: identity-api resolves principals for the other services; a billing-only service authenticates
// without reading identity tables, and answers UNAVAILABLE when identity-api is down.
func TestPrincipalFromIdentityService(t *testing.T) {
	identity := serverCfg(t, func(c *apiserver.Config) {
		c.Domains = []string{ops.DomainIdentity}
		c.InternalToken = "internal-test"
	})
	idSrv := httptest.NewServer(identity.Echo)
	defer idSrv.Close()

	get := func(token, subject, membership string) (int, map[string]any) {
		req, _ := http.NewRequest(http.MethodGet, idSrv.URL+auth.PrincipalPath+"?subject="+subject+"&tenant="+seed.ID("tenant-a").String()+"&membership="+membership, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		return res.StatusCode, m
	}
	user := seed.ID("user-customer-a").String()
	if code, m := get("internal-test", user, seed.ID("customer-a").String()); code != 200 || m["role"] != "client" || m["clientRole"] != "owner" {
		t.Fatalf("principal: %d %v", code, m)
	}
	if code, _ := get("wrong", user, seed.ID("customer-a").String()); code != 401 {
		t.Fatalf("wrong internal token: %d", code)
	}
	if code, _ := get("internal-test", user, seed.ID("customer-b").String()); code != 404 {
		t.Fatalf("another user's membership: %d", code)
	}

	billing := serverCfg(t, func(c *apiserver.Config) {
		c.Domains = []string{ops.DomainBilling}
		c.IdentityURL = idSrv.URL
		c.InternalToken = "internal-test"
	})
	if code, m := post(billing, &customerA, "invoices.list", `{"limit":1}`); code != 200 {
		t.Fatalf("billing-api with remote principal: %d %v", code, m)
	}
	if code, _ := post(billing, &customerB, "invoices.list", `{"limit":1}`); code != 200 {
		t.Fatalf("second member: %d", code)
	}
	idSrv.Close()
	down := serverCfg(t, func(c *apiserver.Config) {
		c.Domains = []string{ops.DomainBilling}
		c.IdentityURL = idSrv.URL
		c.InternalToken = "internal-test"
	})
	if code, m := post(down, &customerA, "invoices.list", `{"limit":1}`); code != 503 || m["code"] != "UNAVAILABLE" {
		t.Fatalf("identity down: %d %v", code, m)
	}
}
