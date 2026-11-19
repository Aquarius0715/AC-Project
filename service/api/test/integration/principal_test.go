package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// IR181 step 1: identity-api resolves principals for the other services; a billing-only service authenticates
// without reading identity tables, and answers UNAVAILABLE when identity-api is down.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPrincipalFromIdentityService(t *testing.T) {
	identity := serverCfg(t, func(c *apiserver.Config) {
		c.Domains = []string{ops.DomainIdentity}
		c.InternalToken = internalToken
	})

	idSrv := httptest.NewServer(identity.Echo)
	defer idSrv.Close()

	get := func(token, subject, membership string) (int, map[string]any) {
		req, _ := http.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			idSrv.URL+auth.PrincipalPath+"?subject="+subject+"&tenant="+seed.ID("tenant-a").
				String()+
				"&membership="+membership,
			http.NoBody,
		)
		req.Header.Set("Authorization", "Bearer "+token)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		var m map[string]any

		_ = json.NewDecoder(res.Body).Decode(&m)

		return res.StatusCode, m
	}

	user := seed.ID("user-customer-a").String()
	if code, res := get(
		"test-internal",
		user,
		seed.ID("customer-a").String(),
	); code != 200 || res["role"] != roleClient ||
		res["clientRole"] != clientRoleOwner {
		t.Fatalf("principal: %d %v", code, res)
	}

	if code, _ := get("wrong", user, seed.ID("customer-a").String()); code != 401 {
		t.Fatalf("wrong internal token: %d", code)
	}

	if code, _ := get("test-internal", user, seed.ID("customer-b").String()); code != 404 {
		t.Fatalf("another user's membership: %d", code)
	}

	cluster := newCluster(t) // billing asks equipment for customers (IR193)

	billing := serverCfg(t, func(c *apiserver.Config) {
		c.Domains = []string{ops.DomainBilling}
		c.IdentityURL = idSrv.URL
		c.InternalToken = internalToken
		c.ServiceURLs = cluster.others(ops.DomainBilling)
	})
	if code, m := post(billing, customerA(), "invoices.list", `{"limit":1}`); code != 200 {
		t.Fatalf("billing-api with remote principal: %d %v", code, m)
	}

	if code, _ := post(billing, customerB(), "invoices.list", `{"limit":1}`); code != 200 {
		t.Fatalf("second member: %d", code)
	}

	idSrv.Close()

	down := serverCfg(t, func(c *apiserver.Config) {
		c.Domains = []string{ops.DomainBilling}
		c.IdentityURL = idSrv.URL
		c.InternalToken = internalToken
		c.ServiceURLs = cluster.others(ops.DomainBilling)
	})
	if code, res := post(
		down,
		customerA(),
		"invoices.list",
		`{"limit":1}`,
	); code != 503 ||
		res["code"] != codeUnavailable {
		t.Fatalf("identity down: %d %v", code, res)
	}
}

// The principal endpoint refuses a query it cannot read with VALIDATION before it looks a membership up; a valid
// signedIn (RFC 3339) is read, so a membership that does not match is NOT_FOUND, not a refusal.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPrincipalRefusesMalformedQuery(t *testing.T) {
	identity := serverCfg(t, func(c *apiserver.Config) {
		c.Domains = []string{ops.DomainIdentity}
		c.InternalToken = internalToken
	})

	idSrv := httptest.NewServer(identity.Echo)
	defer idSrv.Close()

	user := seed.ID("user-customer-a").String()
	tenant := seed.ID("tenant-a").String()
	member := seed.ID("customer-a").String()

	// get asks for the principal with the query parameters that are not empty
	get := func(subject, tenant, membership, signedIn string) (int, map[string]any) {
		query := url.Values{}

		for key, value := range map[string]string{
			"subject": subject, "tenant": tenant, "membership": membership, "signedIn": signedIn,
		} {
			if value != "" {
				query.Set(key, value)
			}
		}

		req, _ := http.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			idSrv.URL+auth.PrincipalPath+"?"+query.Encode(),
			http.NoBody,
		)
		req.Header.Set("Authorization", "Bearer "+internalToken)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()

		var m map[string]any

		_ = json.NewDecoder(res.Body).Decode(&m)

		return res.StatusCode, m
	}

	malformed := []struct{ name, subject, tenant, membership, signedIn string }{
		{name: "no subject", subject: "", tenant: tenant, membership: member, signedIn: ""},
		{
			name:       "tenant not a UUID",
			subject:    user,
			tenant:     "tenant-a",
			membership: member,
			signedIn:   "",
		},
		{name: "no membership", subject: user, tenant: tenant, membership: "", signedIn: ""},
		{
			name:       "signedIn not a time",
			subject:    user,
			tenant:     tenant,
			membership: member,
			signedIn:   "yesterday",
		},
	}
	for _, c := range malformed {
		code, res := get(c.subject, c.tenant, c.membership, c.signedIn)
		if code != 422 || res["code"] != string(apperr.Validation) ||
			fields(res) != refusedMalformedInput {
			t.Errorf("%s: %d %v", c.name, code, res)
		}
	}

	other := seed.ID("customer-b").String()
	if code, res := get(user, tenant, other, "2026-09-14T01:00:00Z"); code != 404 ||
		res["messageKey"] != apperr.KeyMembershipInvalid {
		t.Fatalf("valid signedIn, another user's membership: %d %v", code, res)
	}
}
