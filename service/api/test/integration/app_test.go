package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
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

func server(t *testing.T) *apiserver.Server {
	t.Helper()

	return serverWith(t, false)
}

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

	fixture, err := seed.Load("../../../../docs/04-agentic-sdlc/fixture-contract.json")
	if err != nil {
		t.Fatal(err)
	}

	beginFuncErr := pgx.BeginFunc(
		ctx,
		conn,
		func(tx pgx.Tx) error { return seed.Apply(ctx, tx, fixture) },
	)
	if beginFuncErr != nil {
		t.Fatal(beginFuncErr)
	}

	return fixture
}

// testTokens maps the static test tokens to the fixture users. "tok-a@1789000000" is tok-a signed in at that Unix
// time, as an OIDC token's auth_time (IR268).
type testTokens auth.StaticVerifier

func (v testTokens) Verify(ctx context.Context, token string) (auth.Identity, error) {
	tok, at, signed := strings.Cut(token, "@")

	id, err := auth.StaticVerifier(v).Verify(ctx, tok)
	if err != nil || !signed {
		return id, apperr.Wrap(err, "auth.StaticVerifier.Verify")
	}

	sec, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return auth.Identity{}, apperr.Wrap(err, "strconv.ParseInt")
	}

	id.SignedIn = time.Unix(sec, 0).UTC()

	return id, nil
}

// testVerifier maps the static test tokens to the fixture users.
func testVerifier(t *testing.T) testTokens {
	t.Helper()
	f := seedFixture(t)

	v := testTokens{}
	for _, a := range f.Actors {
		v["tok-"+
			map[string]string{
				"hq-operator": "hq", "customer-a": "a", "customer-b": "b", "tech-external-b": "tb",
				"tech-internal-a": "ti", "tech-external-a": "ta", "contractor-a": "ca", "contractor-b": "cb",
				"hq-restriction-manager": "rm", "hq-override-only": "oo",
			}[a.MembershipID]] = seed.ID(a.UserID).
			String()
	}

	v["tok-rv"] = seed.ID("user-test-revoke").
		String()
		// TestRevokeSessions' own user: revoking a seed user would end the other tests' tokens
	return v
}

// serverCfg builds a server on the test database; opt adjusts the configuration (domains, identity source). In
// cluster mode an all-domain server is the split-service facade (cluster_test.go), unless it injects failures.
func serverCfg(t *testing.T, opt func(*apiserver.Config)) *apiserver.Server {
	t.Helper()
	v := testVerifier(t)
	cfg := apiserver.Config{
		DatabaseURL:       testDB("ac_app_login"),
		Clock:             clockFunc,
		DatabaseReaderURL: "",
		Addr:              "",
		DemoOps:           false,
		DemoStart:         time.Time{},
		Logger:            nil,
		Domains:           nil,
		IdentityURL:       "",
		InternalToken:     "",
		ServiceURLs:       nil,
		PrincipalTTL:      0,
		DemoClock:         nil,
		WriteLimit:        nil,
		DenyLimit:         nil,
		IdentityProvider:  nil,
		AccountApps: identity.AccountApps{
			Customer:   "",
			Partner:    "",
			Technician: "",
			Admin:      "",
		},
		AccountsPerDay: 0,
		WrapTx:         nil,
	}
	opt(&cfg)

	if clusterMode && len(cfg.Domains) == 0 &&
		cfg.WrapTx == nil { // a failing transaction is injected in-process only
		f := clusterFacade(t, cfg, v)
		lastServer = f

		return f
	}

	srv, err := apiserver.New(context.Background(), cfg, v)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(srv.DB.Close)

	if len(cfg.Domains) == 0 {
		lastServer = srv
	}

	return srv
}

// lastServer is the most recent all-domain test server; drainEvents applies its pending events (IR183/IR184).
var lastServer *apiserver.Server

func drainEvents(t *testing.T) {
	t.Helper()

	if lastServer == nil {
		return
	}

	err := lastServer.DrainEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func post(srv *apiserver.Server, a *actor, op, body string) (int, map[string]any) {
	req := httptest.NewRequest(ops.ClientRequest(op, body))
	req.Header.Set("Content-Type", "application/json")

	if a != nil {
		req.Header.Set("Authorization", "Bearer "+a.token)
		req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
		req.Header.Set("X-Membership-Id", seed.ID(a.membership).String())
	}

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	var m map[string]any

	_ = json.Unmarshal(rec.Body.Bytes(), &m)

	return rec.Code, m
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

		if w.Code != 200 || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}

func TestSessionGet(t *testing.T) {
	srv := server(t)
	code, res := post(srv, &hq, "session.get", `{}`)

	doc := res["data"].(map[string]any)
	if code != 200 || doc["role"] != "admin" ||
		doc["membershipId"] != seed.ID("hq-operator").String() ||
		len(doc["permissions"].([]any)) == 0 ||
		doc["clientRole"] != nil {
		t.Fatalf("%d %v", code, res)
	}

	var user, org string // IR241: the shell names the user and the membership's organization
	ownerScan(
		t,
		`SELECT u.display_name, o.name FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id JOIN
			identity.organizations o ON o.id = m.organization_id WHERE m.id = $1`,
		[]any{seed.ID("hq-operator")},
		&user,
		&org,
	)

	if doc["displayName"] != user || doc["organizationName"] != org || user == "" || org == "" {
		t.Errorf("names: %v %v (%s, %s)", doc["displayName"], doc["organizationName"], user, org)
	}

	if _, c := post(
		srv,
		&contrA,
		"session.get",
		`{}`,
	); c["data"].(map[string]any)["organizationId"] != seed.ID("org-contractor-a").
		String() { // IR276: the own company
		t.Errorf("contractor session organization: %v", c["data"])
	}

	if _, c := post(
		srv,
		&customerA,
		"session.get",
		`{}`,
	); c["data"].(map[string]any)["clientRole"] != "owner" ||
		c["data"].(map[string]any)["organizationName"] == "" ||
		c["data"].(map[string]any)["customerId"] != seed.ID("cust-a").
			String() { // IR243: the client's customer
		t.Errorf("client owner session: %v", c["data"])
	}

	if doc["customerId"] != nil {
		t.Errorf("HQ session customer: %v", doc["customerId"])
	}

	meta := res["meta"].(map[string]any) // Meta of service-contracts.ts
	if meta["snapshotAt"] != clock.Format(time.RFC3339) || meta["correlationId"] == "" ||
		meta["eventCursor"] == nil {
		t.Errorf("meta: %v", meta)
	}

	if code, _ := post(srv, nil, "session.get", `{}`); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
}

func TestUnitsListScopes(t *testing.T) {
	srv := server(t)
	_, res := post(srv, &hq, "units.list", `{"limit":100}`)
	all := items(res)
	_, res = post(srv, &customerA, "units.list", `{"limit":100}`)
	mine := items(res)
	_, res = post(srv, &customerB, "units.list", `{"limit":100}`)
	other := items(res)
	_, res = post(srv, &techInt, "units.list", `{"limit":100}`)
	tech := items(res)
	_, res = post(srv, &techB, "units.list", `{"limit":100}`)
	external := items(res)
	total := func(a *actor) int {
		_, m := post(srv, a, "units.list", `{"limit":1}`)

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
		want := map[string]string{
			seed.ID("unit-online-rto").String(): "on", seed.ID("unit-non-rto").String(): "off",
			seed.ID("unit-offline-rto").String(): "unknown", seed.ID("unit-limited").String(): "on",
		}
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
	code, res := post(srv, &hq, "units.list", `{"limit":2}`)

	doc := res["data"].(map[string]any)
	if code != 200 || len(items(res)) != 2 || doc["nextCursor"] == nil ||
		int(doc["total"].(float64)) != hqN {
		t.Fatalf("page 1: %d %v", code, doc)
	}

	code, res = post(
		srv,
		&hq,
		"units.list",
		`{"limit":2,"cursor":"`+doc["nextCursor"].(string)+`"}`,
	)
	if code != 200 || len(items(res)) == 0 {
		t.Fatalf("page 2: %d", code)
	}
	// filters
	_, res = post(
		srv,
		&hq,
		"units.list",
		`{"filters":{"unitIds":["`+seed.ID("unit-online-rto").String()+`"]}}`,
	)
	if got := items(res); len(got) != 1 || got[0]["displayName"] != "Bedroom AC" {
		t.Fatalf("unitIds filter: %v", got)
	}

	_, res = post(srv, &hq, "units.list", `{"filters":{"unitIds":[]}}`)
	if len(items(res)) != 0 {
		t.Fatal("empty unitIds returns zero")
	}

	_, res = post(
		srv,
		&hq,
		"units.list",
		`{"filters":{"propertyId":"`+seed.ID("property-home-a").
			String()+
			`","spaceId":"`+seed.ID("floor-1").
			String()+
			`","includeDescendants":true}}`,
	)
	if len(items(res)) == 0 {
		t.Fatal("includeDescendants under floor-1")
	}

	_, res = post(
		srv,
		&hq,
		"units.list",
		`{"filters":{"customerId":"`+seed.ID("cust-a").String()+`"},"limit":100}`,
	)
	if len(items(res)) != len(mine) {
		t.Fatal("customerId filter equals customer-a's own view")
	}

	_, res = post(srv, &hq, "units.list", `{"filters":{"powerState":"unknown"},"limit":1}`)
	if len(items(res)) != 1 {
		t.Fatal("derived powerState filter")
	}

	for body, want := range map[string]string{
		`{"filters":{"unassignedOnly":true,` +
			`"spaceId":"` + seed.ID("floor-1").String() + `"}}`: "filters.unassignedOnly=error.conflictsWithSpaceId",
		`{"filters":{"status":"bogus"}}`:              "filters.status=error.invalid",
		`{"filters":{"connections":["x"]}}`:           "filters.connections=error.invalid",
		`{"filters":{"powerState":"x"}}`:              "filters.powerState=error.invalid",
		`{"filters":{"nope":1}}`:                      "nope=error.notAllowed",
		`{"sort":{"field":"name","direction":"asc"}}`: "sort.field=error.invalid",
		`{"limit":500}`:                               "limit=error.range",
	} {
		code, m := post(srv, &hq, "units.list", body)
		refusal(t, body, code, m, want)
	}

	for _, body := range []string{
		`{"filters":{"status":"online"}}`, `{"filters":{"connections":["online",` +
			`"offline"]}}`, `{"filters":{"unassignedOnly":true}}`,
		`{"filters":{"organizationId":"` + seed.ID("org-customer-b").String() + `"}}`, `{"sort":{"field":"createdAt",` +
			`"direction":"desc"}}`,
	} {
		if code, _ := post(srv, &hq, "units.list", body); code != 200 {
			t.Errorf("%s: %d", body, code)
		}
	}
}

func TestUnitsGet(t *testing.T) {
	srv := server(t)

	id := seed.ID("unit-online-rto").String()
	if code, res := post(
		srv,
		&customerA,
		"units.get",
		`{"id":"`+id+`"}`,
	); code != 200 ||
		res["data"].(map[string]any)["displayName"] != "Bedroom AC" {
		t.Fatalf("own unit: %d", code)
	}

	if code, res := post(
		srv,
		&customerB,
		"units.get",
		`{"id":"`+id+`"}`,
	); code != 404 ||
		res["code"] != "NOT_FOUND" {
		t.Fatalf("other customer's unit must be NOT_FOUND (D01): %d", code)
	}

	if code, res := post(
		srv,
		&hq,
		"units.get",
		`{}`,
	); code != 404 ||
		res["messageKey"] != apperr.KeyUnknownOperation { // GET /v1/units/ has no route
		t.Fatalf("missing id: %d %v", code, res)
	}
}
