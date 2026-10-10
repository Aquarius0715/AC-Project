package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

var clock = time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

func setup(t *testing.T) *Authenticator {
	t.Helper()
	ctx := context.Background()
	owner := "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable"
	conn, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Skip("database not available:", err)
	}
	f, err := seed.Load("../../../../../docs/04-agentic-sdlc/fixture-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return seed.Apply(ctx, tx, f) }); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}
	m, err := db.Open(ctx, url, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	v := StaticVerifier{
		"tok-hq":     seed.ID("user-hq-operator").String(),
		"tok-cust":   seed.ID("user-customer-a").String(),
		"tok-nobody": "no-such-subject",
	}
	return &Authenticator{Verifier: v, DB: m, Now: func() time.Time { return clock }}
}

func call(a *Authenticator, hdr map[string]string) (*httptest.ResponseRecorder, *ops.Principal) {
	e := echo.New()
	e.HTTPErrorHandler = ops.HTTPErrorHandler
	var got *ops.Principal
	e.GET("/", func(c *echo.Context) error { got = ops.PrincipalFrom(c.Request().Context()); return c.NoContent(204) }, a.Middleware())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w, got
}

func TestMiddleware(t *testing.T) {
	a := setup(t)
	tenant := seed.ID("tenant-a").String()
	hq := seed.ID("hq-operator").String()
	w, p := call(a, map[string]string{"Authorization": "Bearer tok-hq", "X-Tenant-Id": tenant, "X-Membership-Id": hq})
	if w.Code != 204 || p == nil || p.Role != "admin" || !p.Permissions["asset.write"] || p.UserID != seed.ID("user-hq-operator") {
		t.Fatalf("hq: %d %+v", w.Code, p)
	}
	w, p = call(a, map[string]string{"Authorization": "Bearer tok-cust", "X-Tenant-Id": tenant, "X-Membership-Id": seed.ID("customer-a").String()})
	if w.Code != 204 || p.Role != "client" || p.ClientRole != "owner" || len(p.Scopes["organization"]) != 1 {
		t.Fatalf("customer: %d %+v", w.Code, p)
	}
	if w, p := call(a, nil); w.Code != 204 || p != nil {
		t.Fatal("no token stays anonymous")
	}
	bad := []map[string]string{
		{"Authorization": "Basic x"},
		{"Authorization": "Bearer "},
		{"Authorization": "Bearer unknown"},
		{"Authorization": "Bearer tok-hq"}, // no membership headers
		{"Authorization": "Bearer tok-hq", "X-Tenant-Id": tenant, "X-Membership-Id": "nope"}, // malformed id
		{"Authorization": "Bearer tok-cust", "X-Tenant-Id": tenant, "X-Membership-Id": hq},   // someone else's membership
		{"Authorization": "Bearer tok-nobody", "X-Tenant-Id": tenant, "X-Membership-Id": hq},
		{"Authorization": "Bearer tok-hq", "X-Tenant-Id": seed.ID("tenant-b").String(), "X-Membership-Id": hq}, // wrong tenant (RLS)
	}
	for i, h := range bad {
		if w, _ := call(a, h); w.Code != http.StatusUnauthorized {
			t.Errorf("case %d: %d", i, w.Code)
		}
	}
	// outside the validity window
	a.Now = func() time.Time { return time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC) }
	if w, _ := call(a, map[string]string{"Authorization": "Bearer tok-hq", "X-Tenant-Id": tenant, "X-Membership-Id": hq}); w.Code != 401 {
		t.Fatalf("expired membership: %d", w.Code)
	}
	a.Now = nil
}

func TestStaticVerifier(t *testing.T) {
	if _, err := (StaticVerifier{}).Verify(context.Background(), "x"); err == nil {
		t.Fatal("unknown token")
	}
	if id, err := (StaticVerifier{"x": "sub"}).Verify(context.Background(), "x"); err != nil || id.Subject != "sub" || !id.SignedIn.IsZero() {
		t.Fatalf("static tokens carry no sign-in time: %+v %v", id, err)
	}
}

// signedVerifier gives the static tokens a sign-in time, as an OIDC token's auth_time.
type signedVerifier struct {
	StaticVerifier
	at time.Time
}

func (v signedVerifier) Verify(ctx context.Context, token string) (Identity, error) {
	id, err := v.StaticVerifier.Verify(ctx, token)
	id.SignedIn = v.at
	return id, err
}

// IR268: the last sign-in moves only for a later auth_time — also in a process that has not seen the user (another
// replica) — takes the business clock, and is recorded by the middleware for a verified token.
func TestRecordSignIn(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable")
	if err != nil {
		t.Skip("database not available:", err)
	}
	t.Cleanup(func() { owner.Close(ctx) }) // registered first, so it closes after the reset below
	user := seed.ID("user-customer-a")
	set := func(last *time.Time) {
		if _, err := owner.Exec(ctx, `UPDATE identity.users SET last_sign_in_at = $2, sign_in_auth_time = CASE WHEN $2::timestamptz IS NULL THEN NULL ELSE sign_in_auth_time END WHERE id = $1`, user, last); err != nil {
			t.Fatal(err)
		}
	}
	read := func() (last, auth *time.Time) {
		if err := owner.QueryRow(ctx, `SELECT last_sign_in_at, sign_in_auth_time FROM identity.users WHERE id = $1`, user).Scan(&last, &auth); err != nil {
			t.Fatal(err)
		}
		return last, auth
	}
	same := func(x *time.Time, want time.Time) bool { return x != nil && x.Equal(want) }
	set(nil)
	t.Cleanup(func() { set(nil) })
	p, err := a.Load(ctx, user.String(), seed.ID("tenant-a"), seed.ID("customer-a"))
	if err != nil {
		t.Fatal(err)
	}
	a.RecordSignIn(ctx, p, time.Time{})
	if last, auth := read(); last != nil || auth != nil {
		t.Fatal("no sign-in time, nothing recorded")
	}
	t1 := time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC)
	a.RecordSignIn(ctx, p, t1)
	if last, auth := read(); !same(last, clock) || !same(auth, t1) {
		t.Fatalf("first sign-in: %v %v", last, auth)
	}
	earlier := clock.Add(-30 * 24 * time.Hour)
	set(&earlier)
	cold := &Authenticator{DB: a.DB, Now: a.Now} // another replica, nothing cached
	cold.RecordSignIn(ctx, p, t1)
	cold.RecordSignIn(ctx, p, t1.Add(-time.Hour)) // the token of an older session
	if last, _ := read(); !same(last, earlier) {
		t.Fatalf("the same or an older sign-in must not move it: %v", last)
	}
	cold.RecordSignIn(ctx, p, t1.Add(time.Hour))
	if last, auth := read(); !same(last, clock) || !same(auth, t1.Add(time.Hour)) {
		t.Fatalf("a later sign-in: %v %v", last, auth)
	}
	set(&earlier)
	a.Verifier = signedVerifier{StaticVerifier: a.Verifier.(StaticVerifier), at: t1.Add(2 * time.Hour)}
	if w, _ := call(a, map[string]string{"Authorization": "Bearer tok-cust", "X-Tenant-Id": seed.ID("tenant-a").String(), "X-Membership-Id": seed.ID("customer-a").String()}); w.Code != 204 {
		t.Fatalf("middleware: %d", w.Code)
	}
	if last, auth := read(); !same(last, clock) || !same(auth, t1.Add(2*time.Hour)) {
		t.Fatalf("the middleware records the token's sign-in: %v %v", last, auth)
	}
}
