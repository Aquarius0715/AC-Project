package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/bootstrap"
	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestBootstrapAdmin (IR509, DEC-84): an empty tenant starts from one HQ administrator. bootstrap.Admin creates the
// tenant and its HQ organization, the person and an admin membership with every HQ permission over the tenant; the
// identity consumer opens the sign-in account (role admin, so the two-step set-up of IR468) and the administrator can
// act at once. A second run for the tenant is refused, and so is an address someone already holds.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestBootstrapAdmin(t *testing.T) {
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	run := func(req bootstrap.Input) (bootstrap.Result, error) {
		var result bootstrap.Result

		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			var err error

			result, err = bootstrap.Admin(ctx, tx, req)

			return apperr.Wrap(err, "bootstrap.Admin")
		})

		return result, apperr.Wrap(err, "transaction")
	}
	tag := uuid.NewString()[:8]

	input := bootstrap.Input{
		Tenant:       "Bootstrap " + tag,
		Organization: "HQ " + tag,
		Email:        "first-admin-" + tag + "@example.com",
		Name:         "First Admin",
		At:           clock(),
	}
	for name, bad := range map[string]bootstrap.Input{
		"no tenant":  {Organization: "HQ", Email: input.Email, Name: input.Name, Tenant: "", At: time.Time{}},
		"no address": {Tenant: input.Tenant, Organization: "HQ", Email: "nobody", Name: input.Name, At: time.Time{}},
		caseNoName:   {Tenant: input.Tenant, Organization: "HQ", Email: input.Email, Name: "", At: time.Time{}},
	} {
		_, err := run(bad)
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	result, err := run(input)
	if err != nil || !result.NewTenant || !result.NewOrganization {
		t.Fatalf("bootstrap: %+v %v", result, err)
	}

	var perms, hqPerms, scopes, events, audits int
	ownerScan(
		t,
		`SELECT count(*) FROM identity.membership_permissions WHERE membership_id = $1`,
		[]any{result.MembershipID},
		&perms,
	)
	ownerScan(
		t,
		`SELECT count(*) FROM identity.permissions WHERE code NOT LIKE 'partner.%'`,
		nil,
		&hqPerms,
	)
	ownerScan(
		t,
		`SELECT count(*) FROM identity.membership_scopes WHERE membership_id = $1 AND kind = 'tenant' AND ref_id = $2`,
		[]any{result.MembershipID, result.TenantID},
		&scopes,
	)
	ownerScan(
		t,
		`SELECT count(*) FROM platform.outbox WHERE aggregate_id = $1 AND event_type = 'AccountRequested' AND payload->>'role'
			= 'admin'`,
		[]any{result.MembershipID},
		&events,
	)
	ownerScan(
		t,
		`SELECT count(*) FROM audit.audit_log WHERE tenant_id = $1 AND action = 'bootstrap.admin' AND target_id = $2`,
		[]any{result.TenantID, result.MembershipID.String()},
		&audits,
	)

	if perms != hqPerms || perms < 30 || scopes != 1 || events != 1 || audits != 1 {
		t.Errorf(
			"permissions %d of %d, tenant scope %d, account events %d, audit %d",
			perms,
			hqPerms,
			scopes,
			events,
			audits,
		)
	}

	_, runErr := run(input)
	if !errors.Is(runErr, bootstrap.ErrAdminExists) {
		t.Errorf("second run for the tenant: %v", runErr)
	}

	other := input

	other.Tenant = "Bootstrap other " + tag

	_, runErr = run(other)
	if !errors.Is(runErr, bootstrap.ErrEmailTaken) {
		t.Errorf("an address someone holds: %v", runErr)
	}

	owner(
		t,
		`DELETE FROM platform.tenants WHERE name = $1 AND NOT EXISTS (SELECT 1 FROM identity.organizations o WHERE o.tenant_id
			= platform.tenants.id)`,
		other.Tenant,
	)

	// the identity consumer opens the account; the administrator then works in Access & roles
	fake := &fakeIDP{
		mu:         sync.Mutex{},
		subjects:   nil,
		err:        nil,
		created:    nil,
		updated:    nil,
		setups:     nil,
		switched:   nil,
		failing:    false,
		lost:       nil,
		generation: 0,
	}
	tokens := testVerifier(t)
	tokens["tok-boot-"+tag] = result.UserID.String()

	srv, err := apiserver.New(
		ctx,
		apiserver.Config{
			DatabaseURL:       testDB("ac_app_login"),
			Clock:             clock,
			IdentityProvider:  fake,
			Domains:           ops.Domains,
			DatabaseReaderURL: "",
			Addr:              "",
			DemoOps:           false,
			DemoShared:        false,
			DemoStart:         time.Time{},
			Logger:            nil,
			IdentityURL:       "",
			InternalToken:     "",
			ServiceURLs:       nil,
			PrincipalTTL:      0,
			DemoClock:         nil,
			WriteLimit:        nil,
			DenyLimit:         nil,
			AccountApps: identity.AccountApps{
				Customer:   "",
				Partner:    "",
				Technician: "",
				Admin:      "",
			},
			AccountsPerDay: 0,
			WrapTx:         nil,
		},
		tokens,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(srv.DB.Close)

	drainEventsErr := srv.DrainEvents(ctx)
	if drainEventsErr != nil {
		t.Fatal(drainEventsErr)
	}

	fake.mu.Lock()
	opened := slices.ContainsFunc(fake.created, func(a idp.Account) bool {
		return a.Email == input.Email && a.Role == roleAdmin &&
			a.TenantID == result.TenantID.String() &&
			a.Membership == result.MembershipID.String()
	})
	fake.mu.Unlock()

	if !opened {
		t.Fatalf("the sign-in account: %+v", fake.created)
	}

	method, target, reqBody := ops.ClientRequest("members.list", `{"limit":100}`)
	req := httptest.NewRequestWithContext(
		t.Context(),
		method,
		target,
		reqBody,
	) // the new tenant's headers, not tenant-a's
	req.Header.Set("Authorization", "Bearer tok-boot-"+tag)
	req.Header.Set("X-Tenant-Id", result.TenantID.String())
	req.Header.Set("X-Membership-Id", result.MembershipID.String())

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	var res map[string]any

	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if rec.Code != 200 || len(items(t, res)) != 1 ||
		items(t, res)[0]["id"] != result.MembershipID.String() ||
		!slices.Contains(toStrings(items(t, res)[0]["permissions"]), "identity.write") {
		t.Fatalf("the administrator lists the tenant's memberships: %d %v", rec.Code, res)
	}
}

// toStrings reads a JSON array of strings.
func toStrings(v any) []string {
	raw, _ := v.([]any)

	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}

	return out
}
