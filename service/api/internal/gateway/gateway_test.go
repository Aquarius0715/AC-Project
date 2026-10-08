package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

func TestRoutesEachOperationToItsDomain(t *testing.T) {
	seen := map[string]string{}
	targets := Targets{}
	for _, d := range ops.Domains {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			seen[d] = r.URL.Path + " " + string(b) + " " + r.Header.Get("Authorization") + " " + r.Header.Get("X-Request-Id")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"domain":"` + d + `"}}`))
		}))
		t.Cleanup(srv.Close)
		u, _ := url.Parse(srv.URL)
		targets[d] = u
	}
	e := New(targets, nil)
	for op, want := range map[string]string{"jobs.list": ops.DomainMaintenance, "units.list": ops.DomainEquipment, "session.get": ops.DomainIdentity, "invoices.list": ops.DomainBilling, "energy.summary": ops.DomainEnergy} {
		if ops.DomainOf(op) == "" {
			t.Fatalf("%s is not in the catalog", op)
		}
		req := httptest.NewRequest(http.MethodPost, "/v1/ops/"+op, strings.NewReader(`{"limit":1}`))
		req.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		var body struct{ Data struct{ Domain string } }
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != http.StatusCreated || body.Data.Domain != want {
			t.Fatalf("%s → %d %s, want %s", op, w.Code, w.Body, want)
		}
		got := seen[want]
		if !strings.HasPrefix(got, "/v1/ops/"+op+` {"limit":1} Bearer tok `) || strings.HasSuffix(got, " ") {
			t.Fatalf("%s forwarded as %q", op, got)
		}
	}
	// unknown operations never leave the gateway
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/ops/nope.nope", strings.NewReader(`{}`)))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "error.unknownOperation") {
		t.Fatalf("unknown: %d %s", w.Code, w.Body)
	}
}

func TestUpstreamDownIsUnavailable(t *testing.T) {
	dead, _ := url.Parse("http://127.0.0.1:1")
	targets := Targets{}
	for _, d := range ops.Domains {
		targets[d] = dead
	}
	w := httptest.NewRecorder()
	New(targets, nil).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/ops/jobs.list", strings.NewReader(`{}`)))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"UNAVAILABLE"`) {
		t.Fatalf("down: %d %s", w.Code, w.Body)
	}
}

func TestTargetsFromEnv(t *testing.T) {
	env := map[string]string{}
	if _, err := TargetsFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("missing URLs must fail")
	}
	for _, d := range ops.Domains {
		env[envKey(d)] = "http://" + d + "-api:8080"
	}
	tg, err := TargetsFromEnv(func(k string) string { return env[k] })
	if err != nil || tg[ops.DomainBilling].Host != "billing-api:8080" || envKey("billing") != "BILLING_API_URL" {
		t.Fatalf("targets %v %v", tg, err)
	}
	env["ENERGY_API_URL"] = "not a url"
	if _, err := TargetsFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("relative URL must fail")
	}
}
