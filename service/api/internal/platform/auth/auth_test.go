package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// The static test tokens, the app clients and the claims and roles the tests of this package share.
const (
	tokenHQ       = "tok-hq"
	tokenCustomer = "tok-cust"
	appClient     = "ac-web"
	adminClient   = "ac-admin-web"
	claimSubject  = "sub"
	claimAZP      = "azp"
	roleClient    = "client"
	clientOwner   = "owner"
)

// scenarioClock is the business time of the tests: fixture.clock.
func scenarioClock() time.Time { return time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) }

func setup(t *testing.T) *auth.Authenticator {
	t.Helper()

	ctx := context.Background()
	//nolint:gosec // local test database password, not a secret
	owner := "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable"

	conn, err := pgx.Connect(ctx, owner)
	if err != nil {
		t.Skip("database not available:", err)
	}

	fixture, err := seed.Load("../../../../../docs/04-agentic-sdlc/fixture-contract.json")
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

	_ = conn.Close(ctx)

	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		//nolint:gosec // local test database password, not a secret
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}

	txm, err := db.Open(ctx, url, url)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(txm.Close)

	staticVerifier := auth.StaticVerifier{
		tokenHQ:       seed.ID("user-hq-operator").String(),
		tokenCustomer: seed.ID("user-customer-a").String(),
		"tok-nobody":  "no-such-subject",
	}

	return &auth.Authenticator{
		Verifier: staticVerifier,
		DB:       txm,
		Now:      scenarioClock,
		Source:   nil,
	}
}

// membershipHeaders are the headers of a request with the bearer token in the selected Membership.
func membershipHeaders(token, tenant, membership string) map[string]string {
	return map[string]string{
		echo.HeaderAuthorization: "Bearer " + token,
		"X-Tenant-Id":            tenant,
		"X-Membership-Id":        membership,
	}
}

func call(
	t *testing.T,
	authenticator *auth.Authenticator,
	hdr map[string]string,
) (*httptest.ResponseRecorder, *ops.Principal) {
	t.Helper()

	app := echo.New()
	app.HTTPErrorHandler = ops.HTTPErrorHandler

	var got *ops.Principal

	app.GET(
		"/",
		func(c *echo.Context) error {
			got = ops.PrincipalFrom(c.Request().Context())

			return c.NoContent(204)
		},
		authenticator.Middleware(),
	)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	return w, got
}

//nolint:paralleltest // shares the test database with the other DB tests
func TestMiddleware(t *testing.T) {
	authenticator := setup(t)
	tenant := seed.ID("tenant-a").String()

	rec, got := call(
		t,
		authenticator,
		membershipHeaders(tokenHQ, tenant, seed.ID("hq-operator").String()),
	)
	if rec.Code != 204 || got == nil || got.Role != "admin" || !got.Permissions["asset.write"] ||
		got.UserID != seed.ID("user-hq-operator") {
		t.Fatalf("hq: %d %+v", rec.Code, got)
	}

	rec, got = call(
		t,
		authenticator,
		membershipHeaders(tokenCustomer, tenant, seed.ID("customer-a").String()),
	)
	if rec.Code != 204 || got.Role != roleClient || got.ClientRole != clientOwner ||
		len(got.Scopes["organization"]) != 1 {
		t.Fatalf("customer: %d %+v", rec.Code, got)
	}
}

//nolint:paralleltest // shares the test database with the other DB tests
func TestMiddlewareRefuses(t *testing.T) {
	authenticator := setup(t)
	tenant := seed.ID("tenant-a").String()
	hqMembership := seed.ID("hq-operator").String()

	if w, got := call(t, authenticator, nil); w.Code != 204 || got != nil {
		t.Fatal("no token stays anonymous")
	}

	bad := []map[string]string{
		{echo.HeaderAuthorization: "Basic x"},
		{echo.HeaderAuthorization: "Bearer "},
		{echo.HeaderAuthorization: "Bearer unknown"},
		{echo.HeaderAuthorization: "Bearer " + tokenHQ},        // no membership headers
		membershipHeaders(tokenHQ, tenant, "nope"),             // malformed id
		membershipHeaders(tokenCustomer, tenant, hqMembership), // someone else's membership
		membershipHeaders("tok-nobody", tenant, hqMembership),
		membershipHeaders(
			tokenHQ,
			seed.ID("tenant-b").String(),
			hqMembership,
		), // wrong tenant (RLS)
	}
	for i, h := range bad {
		if w, _ := call(t, authenticator, h); w.Code != http.StatusUnauthorized {
			t.Errorf("case %d: %d", i, w.Code)
		}
	}
	// outside the validity window
	authenticator.Now = func() time.Time { return time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC) }
	if recorder, _ := call(
		t,
		authenticator,
		membershipHeaders(tokenHQ, tenant, hqMembership),
	); recorder.Code != 401 {
		t.Fatalf("expired membership: %d", recorder.Code)
	}

	authenticator.Now = nil
}

// clientVerifier says which app each token was issued to, as the OIDC verifier does (IR510).
type clientVerifier struct {
	auth.Verifier

	client string
	admin  bool
}

func (v clientVerifier) Verify(ctx context.Context, token string) (auth.Identity, error) {
	id, err := v.Verifier.Verify(ctx, token)
	id.Client, id.AdminApp = v.client, v.admin

	return id, apperr.Wrap(err, "auth.Verifier.Verify")
}

// TestHQThroughItsApp (DEC-85, IR510): an HQ membership works only with a token of HQ's app, whose sign-in asks for
// the one-time code every time; the other apps' tokens carry the other roles, and a verifier that does not name the
// app (the static test tokens) is not judged.
//
//nolint:paralleltest // shares the test database with the other DB tests
func TestHQThroughItsApp(t *testing.T) {
	authenticator := setup(t)
	static := authenticator.Verifier
	tenant := seed.ID("tenant-a").String()
	hqHeaders := membershipHeaders(tokenHQ, tenant, seed.ID("hq-operator").String())
	customer := membershipHeaders(tokenCustomer, tenant, seed.ID("customer-a").String())

	authenticator.Verifier = clientVerifier{static, appClient, false}
	if w, _ := call(t, authenticator, hqHeaders); w.Code != http.StatusForbidden {
		t.Errorf("HQ through another app: %d", w.Code)
	}

	if w, _ := call(t, authenticator, customer); w.Code != 204 {
		t.Errorf("a customer through the customer app: %d", w.Code)
	}

	authenticator.Verifier = clientVerifier{static, adminClient, true}
	if w, got := call(t, authenticator, hqHeaders); w.Code != 204 || got.Role != "admin" {
		t.Errorf("HQ through its app: %d", w.Code)
	}
}

func TestStaticVerifier(t *testing.T) {
	t.Parallel()

	_, err := (auth.StaticVerifier{}).Verify(t.Context(), "x")
	if err == nil {
		t.Fatal("unknown token")
	}

	identity, err := (auth.StaticVerifier{"x": claimSubject}).Verify(
		t.Context(),
		"x",
	)
	if err != nil || identity.Subject != claimSubject ||
		!identity.SignedIn.IsZero() {
		t.Fatalf("static tokens carry no sign-in time: %+v %v", identity, err)
	}
}

// signedVerifier gives the static tokens a sign-in time, as an OIDC token's auth_time.
type signedVerifier struct {
	auth.StaticVerifier

	at time.Time
}

func (v signedVerifier) Verify(ctx context.Context, token string) (auth.Identity, error) {
	id, err := v.StaticVerifier.Verify(ctx, token)
	id.SignedIn = v.at

	return id, apperr.Wrap(err, "auth.StaticVerifier.Verify")
}

// setLastSignIn sets the user's last sign-in; nil also forgets the auth_time it was recorded for.
func setLastSignIn(t *testing.T, owner *pgx.Conn, user uuid.UUID, last *time.Time) {
	t.Helper()

	_, err := owner.Exec(
		context.Background(),
		`UPDATE identity.users SET last_sign_in_at = $2, sign_in_auth_time = CASE WHEN $2::timestamptz IS NULL THEN NULL ELSE
				sign_in_auth_time END WHERE id = $1`,
		user,
		last,
	)
	if err != nil {
		t.Fatal(err)
	}
}

// lastSignIn reads the user's last sign-in and the auth_time it was recorded for.
func lastSignIn(t *testing.T, owner *pgx.Conn, user uuid.UUID) (*time.Time, *time.Time) {
	t.Helper()

	var last, authTime *time.Time

	err := owner.QueryRow(
		context.Background(),
		`SELECT last_sign_in_at, sign_in_auth_time FROM identity.users WHERE id = $1`,
		user,
	).
		Scan(&last, &authTime)
	if err != nil {
		t.Fatal(err)
	}

	return last, authTime
}

// sameTime reports whether x is set to want.
func sameTime(x *time.Time, want time.Time) bool { return x != nil && x.Equal(want) }

// IR268: the last sign-in moves only for a later auth_time — also in a process that has not seen the user (another
// replica) — takes the business clock, and is recorded by the middleware for a verified token.
//
//nolint:paralleltest // shares the test database with the other DB tests
func TestRecordSignIn(t *testing.T) {
	authenticator := setup(t)
	ctx := context.Background()

	owner, err := pgx.Connect(
		ctx,
		"postgres://postgres:local@localhost:5432/ac_test?sslmode=disable",
	)
	if err != nil {
		t.Skip("database not available:", err)
	}

	// registered first, so it closes after the reset below
	t.Cleanup(func() { _ = owner.Close(ctx) })

	user := seed.ID("user-customer-a")

	setLastSignIn(t, owner, user, nil)
	t.Cleanup(func() { setLastSignIn(t, owner, user, nil) })

	principal, err := authenticator.Load(
		ctx,
		user.String(),
		seed.ID("tenant-a"),
		seed.ID("customer-a"),
	)
	if err != nil {
		t.Fatal(err)
	}

	authenticator.RecordSignIn(ctx, principal, time.Time{})

	if last, authTime := lastSignIn(t, owner, user); last != nil || authTime != nil {
		t.Fatal("no sign-in time, nothing recorded")
	}

	firstSignIn := time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC)
	authenticator.RecordSignIn(ctx, principal, firstSignIn)

	if last, authTime := lastSignIn(t, owner, user); !sameTime(last, scenarioClock()) ||
		!sameTime(authTime, firstSignIn) {
		t.Fatalf("first sign-in: %v %v", last, authTime)
	}

	earlier := scenarioClock().Add(-30 * 24 * time.Hour)
	setLastSignIn(t, owner, user, &earlier)

	cold := &auth.Authenticator{
		DB:       authenticator.DB,
		Now:      authenticator.Now,
		Verifier: nil,
		Source:   nil,
	} // another replica, nothing cached
	checkLaterSignInOnly(t, cold, owner, principal, firstSignIn, earlier)

	setLastSignIn(t, owner, user, &earlier)
	checkMiddlewareRecordsSignIn(t, authenticator, owner, firstSignIn.Add(2*time.Hour))
}

// checkLaterSignInOnly: an authenticator that has recorded nothing yet leaves the last sign-in (earlier) for the
// sign-in recorded last (recorded) and an older one, and moves it for a later one.
func checkLaterSignInOnly(
	t *testing.T,
	cold *auth.Authenticator,
	owner *pgx.Conn,
	principal *ops.Principal,
	recorded, earlier time.Time,
) {
	t.Helper()

	ctx := context.Background()

	cold.RecordSignIn(ctx, principal, recorded)
	cold.RecordSignIn(ctx, principal, recorded.Add(-time.Hour)) // the token of an older session

	if last, _ := lastSignIn(t, owner, principal.UserID); !sameTime(last, earlier) {
		t.Fatalf("the same or an older sign-in must not move it: %v", last)
	}

	cold.RecordSignIn(ctx, principal, recorded.Add(time.Hour))

	if last, authTime := lastSignIn(t, owner, principal.UserID); !sameTime(last, scenarioClock()) ||
		!sameTime(authTime, recorded.Add(time.Hour)) {
		t.Fatalf("a later sign-in: %v %v", last, authTime)
	}
}

// checkMiddlewareRecordsSignIn calls the middleware with the customer's token signed in at signedIn: the call
// records the sign-in.
func checkMiddlewareRecordsSignIn(
	t *testing.T,
	authenticator *auth.Authenticator,
	owner *pgx.Conn,
	signedIn time.Time,
) {
	t.Helper()

	static, isStatic := authenticator.Verifier.(auth.StaticVerifier)
	if !isStatic {
		t.Fatalf("setup verifies with a StaticVerifier, not %T", authenticator.Verifier)
	}

	authenticator.Verifier = signedVerifier{
		StaticVerifier: static,
		at:             signedIn,
	}
	if rec, _ := call(
		t,
		authenticator,
		membershipHeaders(
			tokenCustomer,
			seed.ID("tenant-a").String(),
			seed.ID("customer-a").String(),
		),
	); rec.Code != 204 {
		t.Fatalf("middleware: %d", rec.Code)
	}

	if last, authTime := lastSignIn(
		t,
		owner,
		seed.ID("user-customer-a"),
	); !sameTime(
		last,
		scenarioClock(),
	) ||
		!sameTime(authTime, signedIn) {
		t.Fatalf("the middleware records the token's sign-in: %v %v", last, authTime)
	}
}

// issuerVerifier takes the token for the issuer it names, as a verified token of that address would be.
type issuerVerifier struct{}

func (issuerVerifier) Verify(_ context.Context, token string) (auth.Identity, error) {
	return auth.Identity{
		Subject: "subject-1", SignedIn: time.Time{}, Client: "", AdminApp: false, Issuer: token,
	}, nil
}

// sharedSource answers the same principal to every request, as RemoteSource's cache does.
type sharedSource struct{ principal *ops.Principal }

func (s sharedSource) Principal(
	context.Context, string, uuid.UUID, uuid.UUID, time.Time,
) (*ops.Principal, error) {
	return s.principal, nil
}

// TestPrincipalNamesTheAddressOfTheSignIn: the request's principal names the host of its token's issuer, port and
// path left out. A principal source may share one principal among requests of different addresses, so it stays as it
// was (a request must not see another's address).
func TestPrincipalNamesTheAddressOfTheSignIn(t *testing.T) {
	t.Parallel()

	shared := ops.TenantPrincipal(uuid.New())
	authenticator := &auth.Authenticator{
		Verifier: issuerVerifier{}, DB: nil, Now: nil, Source: sharedSource{principal: shared},
	}
	tenant, membership := uuid.NewString(), uuid.NewString()

	for issuer, want := range map[string]string{
		tailnetIssuer:                        "100.64.0.10",
		"http://192.0.2.10:8281/realms/ac":   "192.0.2.10",
		"https://auth.example.com/realms/ac": "auth.example.com",
		"":                                   "",
	} {
		rec, got := call(t, authenticator, membershipHeaders(issuer, tenant, membership))
		if issuer == "" { // no bearer token at all
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("no token: %d", rec.Code)
			}

			continue
		}

		if rec.Code != http.StatusNoContent || got == nil || got.IssuerHost != want {
			t.Errorf("%q: %d %+v, want the host %q", issuer, rec.Code, got, want)
		}
	}

	if shared.IssuerHost != "" {
		t.Errorf("the shared principal took the address %q", shared.IssuerHost)
	}
}
