package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/gateway"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/scheduler"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// clusterMode runs the whole suite against the split services (AC_TEST_CLUSTER=1, `make test-cluster`, IR194): one
// server per business domain connected with its own least-privilege database role, principals from identity-api,
// internal queries over HTTP, all behind the gateway. Pending events of every service are applied before and after
// each request, so the suite keeps its single-process expectations while every cross-domain table access fails.
var clusterMode = os.Getenv("AC_TEST_CLUSTER") == "1"

// facades caches one cluster per DemoOps setting for the whole test binary.
var facades = map[bool]*apiserver.Server{}

// splitCluster is one server per business domain wired to each other like the compose services.
type splitCluster struct {
	srv  map[string]*apiserver.Server
	urls map[string]string
}

// serviceURL is the test database URL of a domain service's login role (cmd/migrate creates ac_<domain>_login).
func serviceURL(domain string) string {
	return "postgres://ac_" + domain + "_login:local@localhost:5432/ac_test?sslmode=disable"
}

// buildCluster starts the five services from base (clock, demo flags). keep=false closes them when t ends.
func buildCluster(t *testing.T, base apiserver.Config, v auth.Verifier, keep bool) splitCluster {
	t.Helper()
	ctx := context.Background()
	cl := splitCluster{srv: map[string]*apiserver.Server{}, urls: map[string]string{}}
	start := func(d string) {
		cfg := base
		cfg.Domains, cfg.InternalToken, cfg.DatabaseURL = []string{d}, "test-internal", serviceURL(d)
		cfg.PrincipalTTL = -1 // the suite edits memberships directly and expects the next request to see them
		if d != ops.DomainIdentity {
			cfg.IdentityURL = cl.urls[ops.DomainIdentity]
		}
		s, err := apiserver.New(ctx, cfg, v)
		if err != nil {
			t.Fatalf("%s service: %v", d, err)
		}
		hs := httptest.NewServer(s.Echo)
		if !keep {
			t.Cleanup(func() { hs.Close(); s.DB.Close() })
		}
		cl.srv[d], cl.urls[d] = s, hs.URL
	}
	start(ops.DomainIdentity) // principals for the others
	for _, d := range ops.Domains {
		if d != ops.DomainIdentity {
			start(d)
		}
	}
	for d, s := range cl.srv {
		s.Registry.Remote = &ops.HTTPQueries{Targets: cl.targets(d), Token: "test-internal"}
	}
	return cl
}

// statusRecorder keeps the status of a proxied response.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

// Flush supports the reverse proxy's flushes (http.Flusher).
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// newCluster is a fresh cluster for one test (the explicit split-service tests).
func newCluster(t *testing.T) splitCluster {
	t.Helper()
	all := server(t) // seeds the fixture
	_ = all
	return buildCluster(t, apiserver.Config{Clock: clockFunc}, testVerifier(t), false)
}

// targets are the other services' URLs as seen from domain d.
func (cl splitCluster) targets(d string) map[string]*url.URL {
	out := map[string]*url.URL{}
	for o, raw := range cl.urls {
		if o != d {
			out[o], _ = url.Parse(raw)
		}
	}
	return out
}

// others are the other services' URLs as configuration (Config.ServiceURLs).
func (cl splitCluster) others(d string) map[string]string {
	out := map[string]string{}
	for o, raw := range cl.urls {
		if o != d {
			out[o] = raw
		}
	}
	return out
}

func (cl splitCluster) consumers() []*events.Consumer {
	var out []*events.Consumer
	for _, d := range ops.Domains {
		out = append(out, cl.srv[d].Consumers...)
	}
	return out
}

// drain applies pending events in every service until nothing is left (chains cross services).
func (cl splitCluster) drain(t *testing.T) {
	t.Helper()
	f := &apiserver.Server{Consumers: cl.consumers()}
	if err := f.DrainEvents(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// clusterFacade is the suite's server in cluster mode: the gateway in front of the cluster for /v1/ops, the
// all-domain server for health checks, its database for worker paths (scheduler ticks, device callbacks) and the
// cluster's consumers for drainEvents.
func clusterFacade(t *testing.T, base apiserver.Config, v auth.Verifier) *apiserver.Server {
	t.Helper()
	if f, ok := facades[base.DemoOps]; ok {
		return f
	}
	if base.DemoOps { // one scenario clock for every service, like platform.demo_clock in compose (IR168)
		base.DemoClock = apiserver.NewScenarioClock(base.Clock)
	}
	all, err := apiserver.New(context.Background(), base, v)
	if err != nil {
		t.Fatal(err)
	}
	cl := buildCluster(t, base, v, true)
	targets := gateway.Targets{}
	for d, raw := range cl.urls {
		targets[d], _ = url.Parse(raw)
	}
	gw := gateway.New(targets, nil)
	f := &apiserver.Server{Registry: all.Registry, DB: all.DB, Consumers: cl.consumers()}
	e := echo.New()
	e.Any("/*", echo.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			all.Echo.ServeHTTP(w, r)
			return
		}
		if err := f.DrainEvents(r.Context()); err != nil { // direct SQL of the test and worker paths (BeforeDispatch)
			http.Error(w, "drain before: "+err.Error(), http.StatusInternalServerError)
			return
		}
		rec := &statusRecorder{ResponseWriter: w}
		gw.ServeHTTP(rec, r)
		if (r.URL.Path == "/v1/demo/advance-clock" || r.URL.Path == "/v1/ops/demo.advanceClock") && rec.status == http.StatusOK && base.DemoClock != nil { // the workers' tick on the new time
			if _, err := cl.tick(context.WithoutCancel(r.Context()), base.DemoClock.Now()); err != nil {
				panic("tick after the jump: " + err.Error())
			}
		}
		if err := f.DrainEvents(context.WithoutCancel(r.Context())); err != nil { // the write's events (AfterCommit)
			panic("drain after: " + err.Error())
		}
	})))
	f.Echo = e
	facades[base.DemoOps] = f
	clusters[f] = cl
	return f
}

// clusters maps a facade to its services (schedTick).
var clusters = map[*apiserver.Server]splitCluster{}

// tick runs each domain's scheduler with that domain's role and registry (IR195), then applies the events.
func (cl splitCluster) tick(ctx context.Context, now time.Time) (scheduler.Result, error) {
	var total scheduler.Result
	for _, d := range scheduler.Domains {
		r, err := scheduler.TickDomains(ctx, cl.srv[d].DB, now, []string{d}, cl.srv[d].Registry)
		if err != nil {
			return total, err
		}
		total.ExpiredOffers += r.ExpiredOffers
		total.ExpiredCommands += r.ExpiredCommands
		total.Runs += r.Runs
		total.Confirmed += r.Confirmed
		total.ExpiredProposals += r.ExpiredProposals
		total.FrozenHistories += r.FrozenHistories
		total.Jobs += r.Jobs
	}
	f := &apiserver.Server{Consumers: cl.consumers()}
	return total, f.DrainEvents(ctx)
}

// schedTick is the worker tick of the suite: the domain schedulers of the cluster in cluster mode, otherwise the
// all-domain tick on the server's database.
func schedTick(ctx context.Context, s *apiserver.Server, now time.Time) (scheduler.Result, error) {
	if cl, ok := clusters[s]; ok {
		return cl.tick(ctx, now)
	}
	r, err := scheduler.Tick(ctx, s.DB, now, s.Registry)
	if err == nil {
		err = s.DrainEvents(ctx) // the jobs' events (inline mode)
	}
	return r, err
}
