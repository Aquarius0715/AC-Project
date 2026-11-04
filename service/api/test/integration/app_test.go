package integration_test

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

// clock is the test servers' business time.
func clock() time.Time { return time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) }

type actor struct{ token, membership string }

// The fixture users: the static test token of each and the membership it acts in.
func hq() *actor        { return &actor{"tok-hq", membershipHQOperator} }
func customerA() *actor { return &actor{"tok-a", membershipCustomerA} }
func customerB() *actor { return &actor{"tok-b", membershipCustomerB} }
func techB() *actor     { return &actor{"tok-tb", "tech-external-b"} }
func techA() *actor     { return &actor{"tok-ta", "tech-external-a"} }
func contrA() *actor    { return &actor{"tok-ca", "contractor-a"} }
func contrB() *actor    { return &actor{"tok-cb", "contractor-b"} }
func restrMgr() *actor  { return &actor{"tok-rm", "hq-restriction-manager"} }
func overrider() *actor { return &actor{"tok-oo", "hq-override-only"} }

func server(t *testing.T) *apiserver.Server {
	t.Helper()

	return serverWith(t, false)
}

func serverWith(t *testing.T, demo bool) *apiserver.Server {
	t.Helper()

	return serverCfg(t, func(c *apiserver.Config) { c.DemoOps = demo })
}

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
	defer func() { _ = conn.Close(ctx) }()

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
	tok, authTime, signed := strings.Cut(token, "@")

	verified, err := auth.StaticVerifier(v).Verify(ctx, tok)
	if err != nil || !signed {
		return verified, apperr.Wrap(err, "auth.StaticVerifier.Verify")
	}

	sec, err := strconv.ParseInt(authTime, 10, 64)
	if err != nil {
		return auth.Identity{}, apperr.Wrap(err, "strconv.ParseInt")
	}

	verified.SignedIn = time.Unix(sec, 0).UTC()

	return verified, nil
}

// testVerifier maps the static test tokens to the fixture users.
func testVerifier(t *testing.T) testTokens {
	t.Helper()
	f := seedFixture(t)

	tokens := testTokens{}
	for _, seeded := range f.Actors {
		tokens["tok-"+
			map[string]string{
				membershipHQOperator: "hq", membershipCustomerA: "a", membershipCustomerB: "b", "tech-external-b": "tb",
				membershipTechInternalA: "ti", "tech-external-a": "ta", "contractor-a": "ca", "contractor-b": "cb",
				"hq-restriction-manager": "rm", "hq-override-only": "oo",
			}[seeded.MembershipID]] = seed.ID(seeded.UserID).
			String()
	}

	tokens["tok-rv"] = seed.ID("user-test-revoke").
		String()
		// TestRevokeSessions' own user: revoking a seed user would end the other tests' tokens
	return tokens
}

// serverCfg builds a server on the test database; opt adjusts the configuration (domains, identity source). In
// cluster mode an all-domain server is the split-service facade (cluster_test.go), unless it injects failures.
func serverCfg(t *testing.T, opt func(*apiserver.Config)) *apiserver.Server {
	t.Helper()
	tokens := testVerifier(t)
	cfg := apiserver.Config{
		DatabaseURL:       testDB("ac_app_login"),
		Clock:             clock,
		DatabaseReaderURL: "",
		Addr:              "",
		DemoOps:           false,
		DemoShared:        false,
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

	if clusterMode() && len(cfg.Domains) == 0 &&
		cfg.WrapTx == nil { // a failing transaction is injected in-process only
		f := clusterFacade(t, cfg, tokens)
		lastServer = f

		return f
	}

	srv, err := apiserver.New(context.Background(), cfg, tokens)
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
//
//nolint:gochecknoglobals // shared state: serverCfg sets it and drainEvents reads it
var lastServer *apiserver.Server

func drainEvents(t *testing.T) {
	t.Helper()

	drainEventsContext(context.Background(), t)
}

// drainEventsContext is drainEvents on the caller's context.
func drainEventsContext(ctx context.Context, t *testing.T) {
	t.Helper()

	if lastServer == nil {
		return
	}

	err := lastServer.DrainEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func post(srv *apiserver.Server, who *actor, op, body string) (int, map[string]any) {
	method, target, reqBody := ops.ClientRequest(op, body)
	req := httptest.NewRequestWithContext(context.Background(), method, target, reqBody)
	req.Header.Set("Content-Type", "application/json")

	if who != nil {
		req.Header.Set("Authorization", "Bearer "+who.token)
		req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
		req.Header.Set("X-Membership-Id", seed.ID(who.membership).String())
	}

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	var m map[string]any

	_ = json.Unmarshal(rec.Body.Bytes(), &m)

	return rec.Code, m
}

func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()

	d, _ := m["data"].(map[string]any)
	raw, _ := d["items"].([]any)

	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		out = append(out, asObj(t, r))
	}

	return out
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestHealth(t *testing.T) {
	s := server(t)

	for _, p := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		s.Echo.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, nil))

		if w.Code != 200 || w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestSessionGet(t *testing.T) {
	srv := server(t)
	code, res := post(srv, hq(), "session.get", `{}`)

	doc := asObj(t, res["data"])
	if code != 200 || doc["role"] != roleAdmin ||
		doc["membershipId"] != seed.ID("hq-operator").String() ||
		len(asArr(t, doc["permissions"])) == 0 ||
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

	if _, session := post(
		srv,
		contrA(),
		"session.get",
		`{}`,
	); asObj(t, session["data"])["organizationId"] != seed.ID("org-contractor-a").
		String() { // IR276: the own company
		t.Errorf("contractor session organization: %v", session["data"])
	}

	if _, session := post(
		srv,
		customerA(),
		"session.get",
		`{}`,
	); asObj(t, session["data"])["clientRole"] != clientRoleOwner ||
		asObj(t, session["data"])["organizationName"] == "" ||
		asObj(t, session["data"])["customerId"] != seed.ID("cust-a").
			String() { // IR243: the client's customer
		t.Errorf("client owner session: %v", session["data"])
	}

	if doc["customerId"] != nil {
		t.Errorf("HQ session customer: %v", doc["customerId"])
	}

	meta := asObj(t, res["meta"]) // Meta of service-contracts.ts
	if meta["snapshotAt"] != clock().Format(time.RFC3339) || meta["correlationId"] == "" ||
		meta["eventCursor"] == nil {
		t.Errorf("meta: %v", meta)
	}

	if code, _ := post(srv, nil, "session.get", `{}`); code != 401 {
		t.Fatalf("anonymous: %d", code)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestUnitsListScopes(t *testing.T) {
	srv := server(t)
	_, res := post(srv, hq(), "units.list", `{"limit":100}`)
	all := items(t, res)
	_, res = post(srv, customerA(), "units.list", `{"limit":100}`)
	mine := items(t, res)
	_, res = post(srv, customerB(), "units.list", `{"limit":100}`)
	other := items(t, res)
	_, res = post(srv, techInt(), "units.list", `{"limit":100}`)
	tech := items(t, res)
	_, res = post(srv, techB(), "units.list", `{"limit":100}`)
	external := items(t, res)
	total := func(a *actor) int {
		_, m := post(srv, a, "units.list", `{"limit":1}`)

		return int(asNum(t, asObj(t, m["data"])["total"]))
	}

	hqN, aN, bN := total(hq()), total(customerA()), total(customerB())
	if len(all) == 0 || len(mine) == 0 || aN+bN > hqN || aN >= hqN {
		t.Fatalf("hq %d, a %d, b %d", hqN, aN, bN)
	}

	_ = other

	for _, unit := range mine {
		if unit["customerOrgId"] != seed.ID("org-customer-a").String() {
			t.Fatalf("customer-a sees another customer's unit: %v", unit["displayName"])
		}
		// SR27 on the fixture clock: online + fresh observation + fresh valid measured power → on/off; offline → unknown
		want := map[string]string{
			seed.ID("unit-online-rto").String():  "on",
			seed.ID("unit-non-rto").String():     powerOff,
			seed.ID("unit-offline-rto").String(): powerUnknown,
			seed.ID("unit-limited").String():     "on",
		}
		if w, ok := want[unit["id"].(string)]; ok && unit["effectivePowerState"] != w {
			t.Fatalf("%v: power %v want %s", unit["displayName"], unit["effectivePowerState"], w)
		}
	}
	// internal technicians: Membership.scopes; external technicians: only units of their own Assignments (IR49(b))
	seen := map[string]bool{}
	for _, u := range tech {
		seen[asStr(t, u["id"])] = true
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
	code, res := post(srv, hq(), "units.list", `{"limit":2}`)

	doc := asObj(t, res["data"])
	if code != 200 || len(items(t, res)) != 2 || doc["nextCursor"] == nil ||
		int(asNum(t, doc["total"])) != hqN {
		t.Fatalf("page 1: %d %v", code, doc)
	}

	code, res = post(
		srv,
		hq(),
		"units.list",
		`{"limit":2,"cursor":"`+asStr(t, doc["nextCursor"])+`"}`,
	)
	if code != 200 || len(items(t, res)) == 0 {
		t.Fatalf("page 2: %d", code)
	}
	// filters
	_, res = post(
		srv,
		hq(),
		"units.list",
		`{"filters":{"unitIds":["`+seed.ID("unit-online-rto").String()+`"]}}`,
	)
	if got := items(t, res); len(got) != 1 || got[0]["displayName"] != "Bedroom AC" {
		t.Fatalf("unitIds filter: %v", got)
	}

	_, res = post(srv, hq(), "units.list", `{"filters":{"unitIds":[]}}`)
	if len(items(t, res)) != 0 {
		t.Fatal("empty unitIds returns zero")
	}

	_, res = post(
		srv,
		hq(),
		"units.list",
		`{"filters":{"propertyId":"`+seed.ID("property-home-a").
			String()+
			`","spaceId":"`+seed.ID("floor-1").
			String()+
			`","includeDescendants":true}}`,
	)
	if len(items(t, res)) == 0 {
		t.Fatal("includeDescendants under floor-1")
	}

	_, res = post(
		srv,
		hq(),
		"units.list",
		`{"filters":{"customerId":"`+seed.ID("cust-a").String()+`"},"limit":100}`,
	)
	if len(items(t, res)) != len(mine) {
		t.Fatal("customerId filter equals customer-a's own view")
	}

	_, res = post(srv, hq(), "units.list", `{"filters":{"powerState":"unknown"},"limit":1}`)
	if len(items(t, res)) != 1 {
		t.Fatal("derived powerState filter")
	}

	for body, want := range map[string]string{
		`{"filters":{"unassignedOnly":true,` +
			`"spaceId":"` + seed.ID("floor-1").String() + `"}}`: "filters.unassignedOnly=error.conflictsWithSpaceId",
		`{"filters":{"status":"bogus"}}`:    refusedFilterStatusInvalid,
		`{"filters":{"connections":["x"]}}`: "filters.connections=error.invalid",
		`{"filters":{"powerState":"x"}}`:    "filters.powerState=error.invalid",
		`{"filters":{"nope":1}}`:            "nope=error.notAllowed",
		bodySortByName:                      refusedSortFieldInvalid,
		`{"limit":500}`:                     "limit=error.range",
	} {
		code, m := post(srv, hq(), "units.list", body)
		refusal(t, body, code, m, want)
	}

	for _, body := range []string{
		`{"filters":{"status":"online"}}`, `{"filters":{"connections":["online",` +
			`"offline"]}}`, `{"filters":{"unassignedOnly":true}}`,
		`{"filters":{"organizationId":"` + seed.ID("org-customer-b").String() + `"}}`, `{"sort":{"field":"createdAt",` +
			`"direction":"desc"}}`,
	} {
		if code, _ := post(srv, hq(), "units.list", body); code != 200 {
			t.Errorf("%s: %d", body, code)
		}
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestUnitsGet(t *testing.T) {
	srv := server(t)

	unitID := seed.ID("unit-online-rto").String()
	if code, res := post(
		srv,
		customerA(),
		"units.get",
		`{"id":"`+unitID+`"}`,
	); code != 200 ||
		asObj(t, res["data"])["displayName"] != "Bedroom AC" {
		t.Fatalf("own unit: %d", code)
	}

	if code, res := post(
		srv,
		customerB(),
		"units.get",
		`{"id":"`+unitID+`"}`,
	); code != 404 ||
		res["code"] != "NOT_FOUND" {
		t.Fatalf("other customer's unit must be NOT_FOUND (D01): %d", code)
	}

	if code, res := post(
		srv,
		hq(),
		"units.get",
		`{}`,
	); code != 404 ||
		res["messageKey"] != apperr.KeyUnknownOperation { // GET /v1/units/ has no route
		t.Fatalf("missing id: %d %v", code, res)
	}
}
