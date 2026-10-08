package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/db"
	"github.com/pradita/ac-project/service/core/seed"
)

var clock = time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

func setup(t *testing.T) *Authenticator {
	t.Helper()
	ctx := context.Background()
	owner := "postgres://postgres:local@localhost:5432/ac?sslmode=disable"
	conn, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Skip("database not available:", err)
	}
	f, err := seed.Load("../../../../docs/04-agentic-sdlc/fixture-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return seed.Apply(ctx, tx, f) }); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://ac_app_login:local@localhost:5432/ac?sslmode=disable"
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
	var got *ops.Principal
	e.GET("/", func(c echo.Context) error { got = ops.PrincipalFrom(c.Request().Context()); return c.NoContent(204) }, a.Middleware())
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
}
