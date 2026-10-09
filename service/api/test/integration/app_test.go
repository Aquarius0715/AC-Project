package integration

import (
	"context"
	"encoding/json"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

var clock = time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

type actor struct{ token, membership string }

var (
	hq        = actor{"tok-hq", "hq-operator"}
	customerA = actor{"tok-a", "customer-a"}
	customerB = actor{"tok-b", "customer-b"}
	techB     = actor{"tok-tb", "tech-external-b"}
	techA     = actor{"tok-ta", "tech-external-a"}
	contrA    = actor{"tok-ca", "contractor-a"}
	contrB    = actor{"tok-cb", "contractor-b"}
	restrMgr  = actor{"tok-rm", "hq-restriction-manager"}
	overrider = actor{"tok-oo", "hq-override-only"}
)

func server(t *testing.T) *apiserver.Server { t.Helper(); return serverWith(t, false) }

func serverWith(t *testing.T, demo bool) *apiserver.Server {
	t.Helper()
	return serverCfg(t, func(c *apiserver.Config) { c.DemoOps = demo })
}

func clockFunc() time.Time { return clock }

// testDB is the URL of the test database for a login role: AC_TEST_DB names the database (ac_test by default;
// make test-cluster uses ac_test_cluster, so both suites can run at the same time).
func testDB(role string) string {
	name := os.Getenv("AC_TEST_DB")
	if name == "" {
		name = "ac_test"
	}
	return "postgres://" + role + ":local@localhost:5432/" + name + "?sslmode=disable"
}

// seedFixture applies the demo fixture (idempotent) and returns it.
func seedFixture(t *testing.T) *seed.Fixture {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip("database not available:", err)
	}
	defer conn.Close(ctx)
	f, err := seed.Load("../../../../docs/04-agentic-sdlc/fixture-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return seed.Apply(ctx, tx, f) }); err != nil {
		t.Fatal(err)
	}
	return f
}

// testVerifier maps the static test tokens to the fixture users.
func testVerifier(t *testing.T) auth.StaticVerifier {
	t.Helper()
	f := seedFixture(t)
	v := auth.StaticVerifier{}
	for _, a := range f.Actors {
		v["tok-"+map[string]string{"hq-operator": "hq", "customer-a": "a", "customer-b": "b", "tech-external-b": "tb", "tech-internal-a": "ti", "tech-external-a": "ta", "contractor-a": "ca", "contractor-b": "cb", "hq-restriction-manager": "rm", "hq-override-only": "oo"}[a.MembershipID]] = seed.ID(a.UserID).String()
	}
	return v
}

// serverCfg builds a server on the test database; opt adjusts the configuration (domains, identity source). In
// cluster mode an all-domain server is the split-service facade (cluster_test.go).
func serverCfg(t *testing.T, opt func(*apiserver.Config)) *apiserver.Server {
	t.Helper()
	v := testVerifier(t)
	cfg := apiserver.Config{DatabaseURL: testDB("ac_app_login"), Clock: clockFunc}
	opt(&cfg)
	if clusterMode && len(cfg.Domains) == 0 {
		f := clusterFacade(t, cfg, v)
		lastServer = f
		return f
	}
	s, err := apiserver.New(context.Background(), cfg, v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.DB.Close)
	if len(cfg.Domains) == 0 {
		lastServer = s
	}
	return s
}

// lastServer is the most recent all-domain test server; drainEvents applies its pending events (IR183/IR184).
var lastServer *apiserver.Server

func drainEvents(t *testing.T) {
	t.Helper()
	if lastServer == nil {
		return
	}
	if err := lastServer.DrainEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func post(s *apiserver.Server, a *actor, op, body string) (int, map[string]any) {
	req := httptest.NewRequest(ops.ClientRequest(op, body))
	req.Header.Set("Content-Type", "application/json")
	if a != nil {
		req.Header.Set("Authorization", "Bearer "+a.token)
		req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
		req.Header.Set("X-Membership-Id", seed.ID(a.membership).String())
	}
	w := httptest.NewRecorder()
	s.Echo.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w.Code, m
}

func items(m map[string]any) []map[string]any {
	d, _ := m["data"].(map[string]any)
	raw, _ := d["items"].([]any)
	var out []map[string]any
	for _, r := range raw {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestHealth(t *testing.T) {
	s := server(t)
	for _, p := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		s.Echo.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != 200 || w.Header().Get("X-Request-Id") == "" {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}

func TestSessionGet(t *testing.T) {
	s := server(t)
	code, m := post(s, &hq, "session.get", `{}`)
	d := m["data"].(map[string]any)
	if code != 200 || d["role"] != "admin" || d["membershipId"] != seed.ID("hq-operator").String() || len(d["permissions"].([]any)) == 0 || d["clientRole"] != nil {
		t.Fatalf("%d %v", code, m)
	}
	var user, org string // IR241: the shell names the user and the membership's organization
	ownerScan(t, `SELECT u.display_name, o.name FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id JOIN identity.organizations o ON o.id = m.organization_id WHERE m.id = $1`,
		[]any{seed.ID("hq-operator")}, &user, &org)
	if d["displayName"] != user || d["organizationName"] != org || user == "" || org == "" {
		t.Errorf("names: %v %v (%s, %s)", d["displayName"], d["organizationName"], user, org)
	}
	if _, c := post(s, &customerA, "session.get", `{}`); c["data"].(map[string]any)["clientRole"] != "owner" || c["data"].(map[string]any)["organizationName"] == "" ||
		c["data"].(map[string]any)["customerId"] != seed.ID("cust-a").String() { // IR243: the client's customer
		t.Errorf("client owner session: %v", c["data"])
	}
	if d["customerId"] != nil {
		t.Errorf("HQ session customer: %v", d["customerId"])
	}
	meta := m["meta"].(map[string]any) // Meta of service-contracts.ts
	if meta["snapshotAt"] != clock.Format(time.RFC3339) || meta["correlationId"] == "" || meta["eventCursor"] == nil {
		t.Errorf("meta: %v", meta)
	}
	if code, _ := post(s, nil, "session.get", `{}`); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
}

func TestUnitsListScopes(t *testing.T) {
	s := server(t)
	_, m := post(s, &hq, "units.list", `{"limit":100}`)
	all := items(m)
	_, m = post(s, &customerA, "units.list", `{"limit":100}`)
	mine := items(m)
	_, m = post(s, &customerB, "units.list", `{"limit":100}`)
	other := items(m)
	_, m = post(s, &techInt, "units.list", `{"limit":100}`)
	tech := items(m)
	_, m = post(s, &techB, "units.list", `{"limit":100}`)
	external := items(m)
	total := func(a *actor) int {
		_, m := post(s, a, "units.list", `{"limit":1}`)
		return int(m["data"].(map[string]any)["total"].(float64))
	}
	hqN, aN, bN := total(&hq), total(&customerA), total(&customerB)
	if len(all) == 0 || len(mine) == 0 || aN+bN > hqN || aN >= hqN {
		t.Fatalf("hq %d, a %d, b %d", hqN, aN, bN)
	}
	_ = other
	for _, u := range mine {
		if u["customerOrgId"] != seed.ID("org-customer-a").String() {
			t.Fatalf("customer-a sees another customer's unit: %v", u["displayName"])
		}
		// SR27 on the fixture clock: online + fresh observation + fresh valid measured power → on/off; offline → unknown
		want := map[string]string{seed.ID("unit-online-rto").String(): "on", seed.ID("unit-non-rto").String(): "off",
			seed.ID("unit-offline-rto").String(): "unknown", seed.ID("unit-limited").String(): "on"}
		if w, ok := want[u["id"].(string)]; ok && u["effectivePowerState"] != w {
			t.Fatalf("%v: power %v want %s", u["displayName"], u["effectivePowerState"], w)
		}
	}
	// internal technicians: Membership.scopes; external technicians: only units of their own Assignments (IR49(b))
	seen := map[string]bool{}
	for _, u := range tech {
		seen[u["id"].(string)] = true
	}
	if seen[seed.ID("unit-other-customer").String()] {
		t.Fatal("internal technician sees a unit outside its scopes")
	}
	for _, k := range []string{"unit-online-rto", "unit-offline-rto", "unit-non-rto", "unit-limited"} {
		if !seen[seed.ID(k).String()] {
			t.Fatalf("internal technician misses scoped unit %s", k)
		}
	}
	if len(external) != 0 {
		t.Fatalf("external technician without an Assignment sees %d units", len(external))
	}
	// paging
	code, m := post(s, &hq, "units.list", `{"limit":2}`)
	d := m["data"].(map[string]any)
	if code != 200 || len(items(m)) != 2 || d["nextCursor"] == nil || int(d["total"].(float64)) != hqN {
		t.Fatalf("page 1: %d %v", code, d)
	}
	code, m = post(s, &hq, "units.list", `{"limit":2,"cursor":"`+d["nextCursor"].(string)+`"}`)
	if code != 200 || len(items(m)) == 0 {
		t.Fatalf("page 2: %d", code)
	}
	// filters
	_, m = post(s, &hq, "units.list", `{"filters":{"unitIds":["`+seed.ID("unit-online-rto").String()+`"]}}`)
	if got := items(m); len(got) != 1 || got[0]["displayName"] != "Bedroom AC" {
		t.Fatalf("unitIds filter: %v", got)
	}
	_, m = post(s, &hq, "units.list", `{"filters":{"unitIds":[]}}`)
	if len(items(m)) != 0 {
		t.Fatal("empty unitIds returns zero")
	}
	_, m = post(s, &hq, "units.list", `{"filters":{"propertyId":"`+seed.ID("property-home-a").String()+`","spaceId":"`+seed.ID("floor-1").String()+`","includeDescendants":true}}`)
	if len(items(m)) == 0 {
		t.Fatal("includeDescendants under floor-1")
	}
	_, m = post(s, &hq, "units.list", `{"filters":{"customerId":"`+seed.ID("cust-a").String()+`"},"limit":100}`)
	if len(items(m)) != len(mine) {
		t.Fatal("customerId filter equals customer-a's own view")
	}
	_, m = post(s, &hq, "units.list", `{"filters":{"powerState":"unknown"},"limit":1}`)
	if len(items(m)) != 1 {
		t.Fatal("derived powerState filter")
	}
	for _, body := range []string{
		`{"filters":{"unassignedOnly":true,"spaceId":"` + seed.ID("floor-1").String() + `"}}`,
		`{"filters":{"status":"bogus"}}`, `{"filters":{"connections":["x"]}}`, `{"filters":{"powerState":"x"}}`,
		`{"filters":{"nope":1}}`, `{"sort":{"field":"name","direction":"asc"}}`, `{"limit":500}`,
	} {
		if code, _ := post(s, &hq, "units.list", body); code != 422 {
			t.Errorf("%s: %d", body, code)
		}
	}
	for _, body := range []string{`{"filters":{"status":"online"}}`, `{"filters":{"connections":["online","offline"]}}`, `{"filters":{"unassignedOnly":true}}`,
		`{"filters":{"organizationId":"` + seed.ID("org-customer-b").String() + `"}}`, `{"sort":{"field":"createdAt","direction":"desc"}}`} {
		if code, _ := post(s, &hq, "units.list", body); code != 200 {
			t.Errorf("%s: %d", body, code)
		}
	}
}

func TestUnitsGet(t *testing.T) {
	s := server(t)
	id := seed.ID("unit-online-rto").String()
	if code, m := post(s, &customerA, "units.get", `{"id":"`+id+`"}`); code != 200 || m["data"].(map[string]any)["displayName"] != "Bedroom AC" {
		t.Fatalf("own unit: %d", code)
	}
	if code, m := post(s, &customerB, "units.get", `{"id":"`+id+`"}`); code != 404 || m["code"] != "NOT_FOUND" {
		t.Fatalf("other customer's unit must be NOT_FOUND (D01): %d", code)
	}
	if code, m := post(s, &hq, "units.get", `{}`); code != 404 || m["messageKey"] != "error.unknownOperation" { // GET /v1/units/ has no route
		t.Fatalf("missing id: %d %v", code, m)
	}
}
