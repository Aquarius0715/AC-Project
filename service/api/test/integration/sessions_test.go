package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestRevokeSessions: HQ signs a user out everywhere (members.revokeSessions, IR384, Figma Admin 03-8). Every membership
// of the user in the tenant records the revocation. A token signed in before it, or one that does not say when, is
// UNAUTHENTICATED errors.session_revoked, in identity and in the other services; a sign-in after it works. The access
// stays. A client membership needs asset.write, any other identity.write; a reason is required; another tenant's
// membership is NOT_FOUND and keeps its sessions.
func TestRevokeSessions(t *testing.T) {
	s := server(t)
	tenantA, tenantB := seed.ID("tenant-a"), seed.ID("tenant-b")
	user := seed.ID("user-test-revoke")
	tech, client, foreign := seed.ID("test-revoke-tech"), seed.ID("test-revoke-client"), seed.ID("test-revoke-tenant-b")
	assets := seed.ID("test-revoke-assets") // an HQ membership with asset.write but not identity.write
	from := clock.Add(-24 * time.Hour)
	owner(t, `INSERT INTO identity.users (id, email, display_name) VALUES ($1, 'revoke.test@example.com', 'Revoke Test') ON CONFLICT DO NOTHING`, user)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from) VALUES ($1,$2,$3,$4,'technician','internal',$5) ON CONFLICT DO NOTHING`,
		tech, tenantA, user, seed.ID("org-operator-a"), from)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1,$2,$3,$4,'client','member',$5) ON CONFLICT DO NOTHING`,
		client, tenantA, user, seed.ID("org-customer-a"), from)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1,$2,$3,$4,'client','member',$5) ON CONFLICT DO NOTHING`,
		foreign, tenantB, user, seed.ID("org-customer-tb"), from)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, valid_from) VALUES ($1,$2,$3,$4,'admin',$5) ON CONFLICT DO NOTHING`,
		assets, tenantA, seed.ID("user-hq-override-only"), seed.ID("org-operator-a"), from)
	for m, copyOf := range map[string]string{"test-revoke-tech": "tech-internal-a", "test-revoke-client": "customer-a"} {
		owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission)
			SELECT tenant_id, $1, permission FROM identity.membership_permissions WHERE membership_id = $2 ON CONFLICT DO NOTHING`, seed.ID(m), seed.ID(copyOf))
	}
	owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1,$2,'asset.read'), ($1,$2,'asset.write') ON CONFLICT DO NOTHING`, tenantA, assets)
	owner(t, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'unit',$3) ON CONFLICT DO NOTHING`, tenantA, tech, seed.ID("unit-online-rto"))
	t.Cleanup(func() {
		for _, m := range []any{tech, client, foreign, assets} {
			owner(t, `DELETE FROM identity.membership_scopes WHERE membership_id = $1`, m)
			owner(t, `DELETE FROM identity.membership_permissions WHERE membership_id = $1`, m)
			owner(t, `DELETE FROM identity.memberships WHERE id = $1`, m)
		}
	})
	signedIn := func(membership string, at time.Time) actor { return actor{fmt.Sprintf("tok-rv@%d", at.Unix()), membership} }
	earlier := time.Now().Add(-time.Hour)
	old, oldClient := signedIn("test-revoke-tech", earlier), signedIn("test-revoke-client", earlier)
	if code, m := post(s, &old, "session.get", `{}`); code != 200 {
		t.Fatalf("before the revocation: %d %v", code, m)
	}
	body := func(id any, reason string) string { return fmt.Sprintf(`{"membershipId":"%s","reason":%q}`, id, reason) }

	// the input and who may
	code, m := write(s, &hq, "members.revokeSessions", body(tech, "   "), 0)
	refusal(t, "an empty reason", code, m, "reason=error.length")
	code, m = write(s, &hq, "members.revokeSessions", body("00000000-0000-0000-0000-000000000000", "Phone lost"), 0) // the path parameter, as the REST binding sends it
	refusal(t, "no membership", code, m, "membershipId=error.required")
	if code, _ := write(s, &overrider, "members.revokeSessions", body(tech, "Phone lost"), 0); code != 403 {
		t.Errorf("without identity.write or asset.write: %d", code)
	}
	assetsOnly := actor{"tok-oo", "test-revoke-assets"}
	if code, _ := write(s, &assetsOnly, "members.revokeSessions", body(tech, "Phone lost"), 0); code != 404 {
		t.Errorf("asset.write on a technician membership: %d", code)
	}
	if code, _ := write(s, &hq, "members.revokeSessions", body(foreign, "Phone lost"), 0); code != 404 {
		t.Errorf("another tenant's membership: %d", code)
	}
	if code, m := post(s, &old, "session.get", `{}`); code != 200 {
		t.Fatalf("the refused calls ended nothing: %d %v", code, m)
	}

	// asset.write signs out a client membership's user: all of the user's memberships in the tenant
	reason := "Phone reported lost " + uuid.NewString()[:8] // one per run: the test database keeps earlier runs' audit rows
	code, m = write(s, &assetsOnly, "members.revokeSessions", body(client, reason), 0)
	if code != 200 {
		t.Fatalf("revoke: %d %v", code, m)
	}
	var ids []string
	for _, v := range data(m)["membershipIds"].([]any) {
		ids = append(ids, v.(string))
	}
	if got := strings.Join(ids, ","); !strings.Contains(got, tech.String()) || !strings.Contains(got, client.String()) || strings.Contains(got, foreign.String()) {
		t.Errorf("revoked memberships %s", got)
	}
	for _, c := range []struct {
		who actor
		op  string
	}{{old, "session.get"}, {oldClient, "session.get"}, {actor{"tok-rv", "test-revoke-tech"}, "session.get"}, {oldClient, "units.list"}} {
		if code, m := post(s, &c.who, c.op, `{}`); code != 401 || m["messageKey"] != "errors.session_revoked" {
			t.Errorf("%s as %s signed in before: %d %v", c.op, c.who.token, code, m)
		}
	}
	// a sign-in in the revocation's own second: auth_time has whole seconds, so it cannot be told from one just before
	revokedAt, err := time.Parse(time.RFC3339Nano, data(m)["revokedAt"].(string))
	if err != nil {
		t.Fatal(err)
	}
	sameSecond := signedIn("test-revoke-tech", revokedAt.Truncate(time.Second))
	if code, m := post(s, &sameSecond, "session.get", `{}`); code != 401 || m["messageKey"] != "errors.session_revoked" {
		t.Errorf("a sign-in in the revocation's second: %d %v", code, m)
	}
	later := signedIn("test-revoke-tech", time.Now().Add(time.Minute))
	if code, m := post(s, &later, "session.get", `{}`); code != 200 {
		t.Errorf("a sign-in after the revocation: %d %v", code, m)
	}
	var revoked *time.Time
	ownerScan(t, `SELECT sessions_revoked_at FROM identity.memberships WHERE id = $1`, []any{foreign}, &revoked)
	if revoked != nil {
		t.Error("another tenant's membership was revoked too")
	}
	var audits int
	ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE action = 'members.revokeSessions' AND target_id = $1 AND reason = $2`, []any{client.String(), reason}, &audits)
	if audits != 1 {
		t.Errorf("audit entries: %d", audits)
	}
}
