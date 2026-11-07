package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// catalogRoutes is the number of REST routes in the operation catalog (IR222).
const catalogRoutes = 234

// upstreams starts one service per domain, each answering with handler(domain), and returns their targets.
func upstreams(t *testing.T, handler func(domain string) http.HandlerFunc) Targets {
	t.Helper()

	targets := Targets{}

	for _, domain := range ops.Domains {
		srv := httptest.NewServer(handler(domain))
		t.Cleanup(srv.Close)
		u, _ := url.Parse(srv.URL)
		targets[domain] = u
	}

	return targets
}

// recordingUpstream answers 201 with its domain and keeps the path, body, token and request ID it got in seen.
func recordingUpstream(seen map[string]string) func(domain string) http.HandlerFunc {
	return func(domain string) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			b, _ := io.ReadAll(request.Body)
			seen[domain] = request.URL.Path + " " + string(
				b,
			) + " " + request.Header.Get(
				"Authorization",
			) + " " + request.Header.Get(
				"X-Request-ID",
			)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"data":{"domain":"` + domain + `"}}`))
		}
	}
}

func TestRoutesEachOperationToItsDomain(t *testing.T) {
	t.Parallel()

	seen := map[string]string{}
	app := New(upstreams(t, recordingUpstream(seen)), nil)

	for operation, want := range map[string]string{
		"jobs.create": ops.DomainMaintenance, "units.save": ops.DomainEquipment,
		"preferences.update": ops.DomainIdentity, "invoices.create": ops.DomainBilling, "factors.save": ops.DomainEnergy,
	} {
		if ops.DomainOf(operation) == "" {
			t.Fatalf("%s is not in the catalog", operation)
		}

		route := ops.SpecByName()[operation].Routes[0]
		req := httptest.NewRequestWithContext(
			t.Context(),
			route.Method,
			route.Path,
			strings.NewReader(`{"limit":1}`),
		)
		req.Header.Set("Authorization", "Bearer tok")

		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		var body struct {
			Data struct {
				Domain string `json:"domain"`
			} `json:"data"`
		}

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

	checkUnknownRouteStays(t, app)
}

// checkUnknownRouteStays: unknown routes (the retired POST /v1/ops/<operation> too) never leave the gateway.
func checkUnknownRouteStays(t *testing.T, app *echo.Echo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	app.ServeHTTP(
		recorder,
		httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/v1/ops/jobs.list",
			strings.NewReader(`{}`),
		),
	)

	if recorder.Code != http.StatusNotFound ||
		!strings.Contains(recorder.Body.String(), apperr.KeyUnknownOperation) {
		t.Fatalf("unknown: %d %s", recorder.Code, recorder.Body)
	}
}

// samplePath fills the path parameters of a catalog route with sample values (v-<field>).
func samplePath(route ops.Route) string {
	path := route.Path
	for _, param := range ops.PathParams(route.Path) {
		path = strings.Replace(path, "{"+param+"}", "v-"+strings.ReplaceAll(param, ".", "-"), 1)
	}

	return path
}

// sampleBody is the body a sample request of the route sends: none for GET and DELETE.
func sampleBody(route ops.Route) string {
	if route.Method == http.MethodGet || route.Method == http.MethodDelete {
		return ""
	}

	return `{"x":1}`
}

// TestRoutesEveryRESTRouteToItsDomain sends every catalog route (IR222) with sample path values and a query string:
// the owning service receives the same method, path, query and body.
func TestRoutesEveryRESTRouteToItsDomain(t *testing.T) {
	t.Parallel()

	var got string

	app := New(upstreams(t, func(domain string) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			b, _ := io.ReadAll(request.Body)
			got = domain + " " + request.Method + " " + request.URL.RequestURI() + " " + string(b)

			writer.WriteHeader(http.StatusOK)
		}
	}), nil)
	routes := 0

	for _, spec := range ops.Catalog {
		if len(spec.Routes) == 0 {
			t.Fatalf("%s has no REST route", spec.Name)
		}

		for _, route := range spec.Routes {
			routes++

			path, body := samplePath(route), sampleBody(route)

			got = ""
			rec := httptest.NewRecorder()
			app.ServeHTTP(
				rec,
				httptest.NewRequestWithContext(
					t.Context(),
					route.Method,
					path+"?q=1",
					strings.NewReader(body),
				),
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

	if routes != catalogRoutes {
		t.Fatalf("%d REST routes, want %d", routes, catalogRoutes)
	}
}

func TestUpstreamDownIsUnavailable(t *testing.T) {
	t.Parallel()

	dead, _ := url.Parse("http://127.0.0.1:1")

	targets := Targets{}
	for _, domain := range ops.Domains {
		targets[domain] = dead
	}

	recorder := httptest.NewRecorder()
	New(
		targets,
		nil,
	).ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/jobs?limit=1", nil))

	if recorder.Code != http.StatusServiceUnavailable ||
		!strings.Contains(recorder.Body.String(), `"UNAVAILABLE"`) {
		t.Fatalf("down: %d %s", recorder.Code, recorder.Body)
	}
}

func TestTargetsFromEnv(t *testing.T) {
	t.Parallel()

	env := map[string]string{}

	_, err := TargetsFromEnv(func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("missing URLs must fail")
	}

	for _, domain := range ops.Domains {
		env[envKey(domain)] = "http://" + domain + "-api:8080"
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
	t.Parallel()

	app := New(upstreams(t, func(string) http.HandlerFunc {
		return func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("X-Request-ID", "upstream-id")
			writer.Header().Set("Cache-Control", "public, max-age=3600")
			writer.Header().Set("X-Content-Type-Options", "sniff-me")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"data":{}}`))
		}
	}), nil)
	route := ops.SpecByName()["jobs.create"].Routes[0]
	req := httptest.NewRequestWithContext(
		t.Context(),
		route.Method,
		route.Path,
		strings.NewReader(`{"limit":1}`),
	)
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-Request-ID", "client-chosen") // dropped: the gateway makes its own (IR435)

	for _, request := range []*http.Request{
		req,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil),
	} {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, request)

		header := recorder.Result().Header
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
