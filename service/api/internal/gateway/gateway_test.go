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
	for op, want := range map[string]string{"jobs.create": ops.DomainMaintenance, "units.save": ops.DomainEquipment, "preferences.update": ops.DomainIdentity, "invoices.create": ops.DomainBilling, "factors.save": ops.DomainEnergy} {
		if ops.DomainOf(op) == "" {
			t.Fatalf("%s is not in the catalog", op)
		}
		rt := ops.SpecByName()[op].Routes[0]
		req := httptest.NewRequest(rt.Method, rt.Path, strings.NewReader(`{"limit":1}`))
		req.Header.Set("Authorization", "Bearer tok")
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		var body struct{ Data struct{ Domain string } }
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		if w.Code != http.StatusCreated || body.Data.Domain != want {
			t.Fatalf("%s → %d %s, want %s", op, w.Code, w.Body, want)
		}
		got := seen[want] // path, body, the user's token and the gateway's correlation ID reach the service
		if !strings.HasPrefix(got, rt.Path+` {"limit":1} Bearer tok `) || strings.HasSuffix(got, " ") {
			t.Fatalf("%s forwarded as %q", op, got)
		}
	}
	// unknown routes (the retired POST /v1/ops/<operation> too) never leave the gateway
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/ops/jobs.list", strings.NewReader(`{}`)))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "error.unknownOperation") {
		t.Fatalf("unknown: %d %s", w.Code, w.Body)
	}
}

// TestRoutesEveryRESTRouteToItsDomain sends every catalog route (IR222) with sample path values and a query string:
// the owning service receives the same method, path, query and body.
func TestRoutesEveryRESTRouteToItsDomain(t *testing.T) {
	var got string
	targets := Targets{}
	for _, d := range ops.Domains {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			got = d + " " + r.Method + " " + r.URL.RequestURI() + " " + string(b)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)
		u, _ := url.Parse(srv.URL)
		targets[d] = u
	}
	e := New(targets, nil)
	routes := 0
	for _, s := range ops.Catalog {
		if len(s.Routes) == 0 {
			t.Fatalf("%s has no REST route", s.Name)
		}
		for _, rt := range s.Routes {
			routes++
			path := rt.Path
			for _, p := range ops.PathParams(rt.Path) {
				path = strings.Replace(path, "{"+p+"}", "v-"+strings.ReplaceAll(p, ".", "-"), 1)
			}
			body := ""
			if rt.Method != http.MethodGet && rt.Method != http.MethodDelete {
				body = `{"x":1}`
			}
			got = ""
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest(rt.Method, path+"?q=1", strings.NewReader(body)))
			if want := ops.DomainOf(s.Name) + " " + rt.Method + " " + path + "?q=1 " + body; w.Code != http.StatusOK || got != want {
				t.Fatalf("%s %s (%s): %d, upstream saw %q, want %q", rt.Method, rt.Path, s.Name, w.Code, got, want)
			}
		}
	}
	if routes != 220 {
		t.Fatalf("%d REST routes, want 220", routes)
	}
}

func TestUpstreamDownIsUnavailable(t *testing.T) {
	dead, _ := url.Parse("http://127.0.0.1:1")
	targets := Targets{}
	for _, d := range ops.Domains {
		targets[d] = dead
	}
	w := httptest.NewRecorder()
	New(targets, nil).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/jobs?limit=1", nil))
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
