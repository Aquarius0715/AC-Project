package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/bootstrap"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestBootstrapAdmin (IR509, DEC-84): an empty tenant starts from one HQ administrator. bootstrap.Admin creates the
// tenant and its HQ organization, the person and an admin membership with every HQ permission over the tenant; the
// identity consumer opens the sign-in account (role admin, so the two-step set-up of IR468) and the administrator can
// act at once. A second run for the tenant is refused, and so is an address someone already holds.
func TestBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	run := func(in bootstrap.Input) (bootstrap.Result, error) {
		var r bootstrap.Result
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			var err error
			r, err = bootstrap.Admin(ctx, tx, in)
			return err
		})
		return r, err
	}
	tag := uuid.NewString()[:8]
	in := bootstrap.Input{Tenant: "Bootstrap " + tag, Organization: "HQ " + tag, Email: "first-admin-" + tag + "@example.com", Name: "First Admin", At: clock}
	for name, bad := range map[string]bootstrap.Input{
		"no tenant": {Organization: "HQ", Email: in.Email, Name: in.Name}, "no address": {Tenant: in.Tenant, Organization: "HQ", Email: "nobody", Name: in.Name},
		"no name": {Tenant: in.Tenant, Organization: "HQ", Email: in.Email},
	} {
		if _, err := run(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	r, err := run(in)
	if err != nil || !r.NewTenant || !r.NewOrganization {
		t.Fatalf("bootstrap: %+v %v", r, err)
	}
	var perms, hqPerms, scopes, events, audits int
	ownerScan(t, `SELECT count(*) FROM identity.membership_permissions WHERE membership_id = $1`, []any{r.MembershipID}, &perms)
	ownerScan(t, `SELECT count(*) FROM identity.permissions WHERE code NOT LIKE 'partner.%'`, nil, &hqPerms)
	ownerScan(t, `SELECT count(*) FROM identity.membership_scopes WHERE membership_id = $1 AND kind = 'tenant' AND ref_id = $2`, []any{r.MembershipID, r.TenantID}, &scopes)
	ownerScan(t, `SELECT count(*) FROM platform.outbox WHERE aggregate_id = $1 AND event_type = 'AccountRequested' AND payload->>'role' = 'admin'`, []any{r.MembershipID}, &events)
	ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE tenant_id = $1 AND action = 'bootstrap.admin' AND target_id = $2`, []any{r.TenantID, r.MembershipID.String()}, &audits)
	if perms != hqPerms || perms < 30 || scopes != 1 || events != 1 || audits != 1 {
		t.Errorf("permissions %d of %d, tenant scope %d, account events %d, audit %d", perms, hqPerms, scopes, events, audits)
	}
	if _, err := run(in); !errors.Is(err, bootstrap.ErrAdminExists) {
		t.Errorf("second run for the tenant: %v", err)
	}
	other := in
	other.Tenant = "Bootstrap other " + tag
	if _, err := run(other); !errors.Is(err, bootstrap.ErrEmailTaken) {
		t.Errorf("an address someone holds: %v", err)
	}
	owner(t, `DELETE FROM platform.tenants WHERE name = $1 AND NOT EXISTS (SELECT 1 FROM identity.organizations o WHERE o.tenant_id = platform.tenants.id)`, other.Tenant)

	// the identity consumer opens the account; the administrator then works in Access & roles
	fake := &fakeIdP{}
	v := testVerifier(t)
	v["tok-boot-"+tag] = r.UserID.String()
	s, err := apiserver.New(ctx, apiserver.Config{DatabaseURL: testDB("ac_app_login"), Clock: clockFunc, IdentityProvider: fake, Domains: ops.Domains}, v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.DB.Close)
	if err := s.DrainEvents(ctx); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	opened := slices.ContainsFunc(fake.created, func(a idp.Account) bool {
		return a.Email == in.Email && a.Role == "admin" && a.TenantID == r.TenantID.String() && a.Membership == r.MembershipID.String()
	})
	fake.mu.Unlock()
	if !opened {
		t.Fatalf("the sign-in account: %+v", fake.created)
	}
	req := httptest.NewRequest(ops.ClientRequest("members.list", `{"limit":100}`)) // the new tenant's headers, not tenant-a's
	req.Header.Set("Authorization", "Bearer tok-boot-"+tag)
	req.Header.Set("X-Tenant-Id", r.TenantID.String())
	req.Header.Set("X-Membership-Id", r.MembershipID.String())
	w := httptest.NewRecorder()
	s.Echo.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != 200 || len(items(m)) != 1 || items(m)[0]["id"] != r.MembershipID.String() || !slices.Contains(toStrings(items(m)[0]["permissions"]), "identity.write") {
		t.Fatalf("the administrator lists the tenant's memberships: %d %v", w.Code, m)
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
