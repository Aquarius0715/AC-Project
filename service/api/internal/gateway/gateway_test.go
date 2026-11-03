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
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

func TestRoutesEachOperationToItsDomain(t *testing.T) {
	seen := map[string]string{}
	targets := Targets{}

	for _, d := range ops.Domains {
		srv := httptest.NewServer(
			http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				seen[d] = r.URL.Path + " " + string(
					b,
				) + " " + r.Header.Get(
					"Authorization",
				) + " " + r.Header.Get(
					"X-Request-ID",
				)
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusCreated)
				_, _ = writer.Write([]byte(`{"data":{"domain":"` + d + `"}}`))
			}),
		)
		t.Cleanup(srv.Close)
		u, _ := url.Parse(srv.URL)
		targets[d] = u
	}

	app := New(targets, nil)

	for operation, want := range map[string]string{
		"jobs.create": ops.DomainMaintenance, "units.save": ops.DomainEquipment,
		"preferences.update": ops.DomainIdentity, "invoices.create": ops.DomainBilling, "factors.save": ops.DomainEnergy,
	} {
		if ops.DomainOf(operation) == "" {
			t.Fatalf("%s is not in the catalog", operation)
		}

		route := ops.SpecByName()[operation].Routes[0]
		req := httptest.NewRequest(route.Method, route.Path, strings.NewReader(`{"limit":1}`))
		req.Header.Set("Authorization", "Bearer tok")

		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		var body struct{ Data struct{ Domain string } }

		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusCreated || body.Data.Domain != want {
			t.Fatalf("%s → %d %s, want %s", operation, rec.Code, rec.Body, want)
		}

		got := seen[want] // path, body, the user's token and the gateway's correlation ID reach the service
		if !strings.HasPrefix(got, route.Path+` {"limit":1} Bearer tok `) ||
			strings.HasSuffix(got, " ") {
			t.Fatalf("%s forwarded as %q", operation, got)
		}
	}
	// unknown routes (the retired POST /v1/ops/<operation> too) never leave the gateway
	recorder := httptest.NewRecorder()
	app.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/v1/ops/jobs.list", strings.NewReader(`{}`)),
	)

	if recorder.Code != http.StatusNotFound ||
		!strings.Contains(recorder.Body.String(), apperr.KeyUnknownOperation) {
		t.Fatalf("unknown: %d %s", recorder.Code, recorder.Body)
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

	app := New(targets, nil)
	routes := 0

	for _, spec := range ops.Catalog {
		if len(spec.Routes) == 0 {
			t.Fatalf("%s has no REST route", spec.Name)
		}

		for _, route := range spec.Routes {
			routes++

			path := route.Path
			for _, p := range ops.PathParams(route.Path) {
				path = strings.Replace(path, "{"+p+"}", "v-"+strings.ReplaceAll(p, ".", "-"), 1)
			}

			body := ""
			if route.Method != http.MethodGet && route.Method != http.MethodDelete {
				body = `{"x":1}`
			}

			got = ""
			rec := httptest.NewRecorder()
			app.ServeHTTP(
				rec,
				httptest.NewRequest(route.Method, path+"?q=1", strings.NewReader(body)),
			)

			if want := ops.DomainOf(
				spec.Name,
			) + " " + route.Method + " " + path + "?q=1 " + body; rec.Code != http.StatusOK ||
				got != want {
				t.Fatalf(
					"%s %s (%s): %d, upstream saw %q, want %q",
					route.Method,
					route.Path,
					spec.Name,
					rec.Code,
					got,
					want,
				)
			}
		}
	}

	if routes != 233 {
		t.Fatalf("%d REST routes, want 233", routes)
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

	if w.Code != http.StatusServiceUnavailable ||
		!strings.Contains(w.Body.String(), `"UNAVAILABLE"`) {
		t.Fatalf("down: %d %s", w.Code, w.Body)
	}
}

func TestTargetsFromEnv(t *testing.T) {
	env := map[string]string{}

	_, err := TargetsFromEnv(func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("missing URLs must fail")
	}

	for _, d := range ops.Domains {
		env[envKey(d)] = "http://" + d + "-api:8080"
	}

	tg, err := TargetsFromEnv(func(k string) string { return env[k] })
	if err != nil || tg[ops.DomainBilling].Host != "billing-api:8080" ||
		envKey("billing") != "BILLING_API_URL" {
		t.Fatalf("targets %v %v", tg, err)
	}

	env["ENERGY_API_URL"] = "not a url"

	_, targetsFromEnvErr := TargetsFromEnv(func(k string) string { return env[k] })
	if targetsFromEnvErr == nil {
		t.Fatal("relative URL must fail")
	}
}

// Every response of the gateway, proxied or its own, is not sniffed and not cached, and carries one request ID: the
// upstream service's copy of the header is replaced, not added to it (security loop, IR407).
func TestResponsesAreNotSniffedOrCachedAndCarryOneRequestID(t *testing.T) {
	targets := Targets{}

	for _, d := range ops.Domains {
		srv := httptest.NewServer(
			http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Header().Set("X-Request-ID", "upstream-id")
				writer.Header().Set("Cache-Control", "public, max-age=3600")
				writer.Header().Set("X-Content-Type-Options", "sniff-me")
				writer.WriteHeader(http.StatusOK)
				_, _ = writer.Write([]byte(`{"data":{}}`))
			}),
		)
		t.Cleanup(srv.Close)
		u, _ := url.Parse(srv.URL)
		targets[d] = u
	}

	app := New(targets, nil)
	rt := ops.SpecByName()["jobs.create"].Routes[0]
	req := httptest.NewRequest(rt.Method, rt.Path, strings.NewReader(`{"limit":1}`))
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-Request-ID", "client-chosen") // dropped: the gateway makes its own (IR435)

	for _, request := range []*http.Request{req, httptest.NewRequest(http.MethodGet, "/healthz", nil)} {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, request)

		header := w.Result().Header
		if got := header.Values(
			"X-Request-ID",
		); len(got) != 1 || got[0] == "upstream-id" ||
			got[0] == "client-chosen" {
			t.Errorf(
				"%s %s: X-Request-Id = %q, want one gateway ID",
				request.Method,
				request.URL.Path,
				got,
			)
		}

		if got := header.Values("X-Content-Type-Options"); len(got) != 1 || got[0] != "nosniff" {
			t.Errorf(
				"%s %s: X-Content-Type-Options = %q, want one nosniff",
				request.Method,
				request.URL.Path,
				got,
			)
		}

		if got := header.Values("Cache-Control"); len(got) != 1 || got[0] != "no-store" {
			t.Errorf(
				"%s %s: Cache-Control = %q, want one no-store",
				request.Method,
				request.URL.Path,
				got,
			)
		}
	}
}
