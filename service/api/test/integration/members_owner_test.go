package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestClientMembersAreNotOwners: the owner-only client operations refuse a member of the same customer (IR378). Every
// seed client is an owner, so no test reached this branch before:
//   - client users (list, invite, resend);
//   - the default rules' switches;
//   - the filter-care settings.
func TestClientMembersAreNotOwners(t *testing.T) {
	s := server(t)
	member := seed.ID("test-member-a")
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from)
		VALUES ($1,$2,$3,$4,'client','member',$5) ON CONFLICT DO NOTHING`, member, seed.ID("tenant-a"), seed.ID("user-customer-a"), seed.ID("org-customer-a"), clock.Add(-24*time.Hour))
	owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission)
		SELECT tenant_id, $1, permission FROM identity.membership_permissions WHERE membership_id = $2 ON CONFLICT DO NOTHING`, member, seed.ID("customer-a"))
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.membership_permissions WHERE membership_id = $1`, member)
		owner(t, `DELETE FROM identity.memberships WHERE id = $1`, member)
	})
	m := actor{"tok-a", "test-member-a"} // customer A's user, signed in with the member membership
	if code, res := post(s, &m, "session.get", `{}`); code != 200 {
		t.Fatalf("the member signs in: %d %v", code, res)
	}
	custA := seed.ID("cust-a").String()
	var clientUser string
	ownerScan(t, `SELECT id::text FROM identity.client_users WHERE customer_id = $1 ORDER BY id LIMIT 1`, []any{seed.ID("cust-a")}, &clientUser)
	for _, c := range []struct {
		op, body string
		version  int
		write    bool
	}{
		{"clientUsers.list", `{"limit":10}`, 0, false},
		{"clientUsers.resendInvite", `{"id":"` + clientUser + `"}`, 0, true},
		{"clientUsers.save", `{"customerId":"` + custA + `","email":"member.invite@example.com","clientRole":"member","displayName":"Invited Member"}`, 0, true},
		{"clientUsers.remove", `{"id":"` + clientUser + `","reason":"left"}`, 1, true},
		{"policies.setDefaultRule", `{"policyId":"` + defaultPolicyID(t) + `","ruleKey":"ventilation_co2","customerId":"` + custA + `","enabled":false,"reason":"quiet"}`, 97, true},
		{"filterCare.saveSettings", `{"thresholdHours":100,"fallbackDays":14,"recipients":"all_users","channels":["inApp","email"]}`, 0, true},
	} {
		var code int
		var res map[string]any
		if c.write {
			code, res = write(s, &m, c.op, c.body, c.version)
		} else {
			code, res = post(s, &m, c.op, c.body)
		}
		if code != 403 {
			t.Errorf("%s as a member: %d %v", c.op, code, res)
		}
	}
	// the owner of the same customer may
	if code, res := post(s, &customerA, "clientUsers.list", `{"limit":10}`); code != 200 {
		t.Errorf("the owner's list: %d %v", code, res)
	}
}

// TestMembersSaveNewUser (IR461): HQ creates a person with the membership (newUser instead of userId); the e-mail of an
// existing person is a field error, and newUser goes with neither userId nor id.
func TestMembersSaveNewUser(t *testing.T) {
	s := server(t)
	email := "hq-new-" + uuid.NewString()[:8] + "@example.com"
	body := func(user string) string {
		return `{` + user + `"organizationId":"` + seed.ID("org-operator-a").String() + `","role":"admin","permissions":["job.read"],"scopes":[{"kind":"tenant","id":"` +
			seed.ID("tenant-a").String() + `"}],"validFrom":"` + clock.Format(time.RFC3339) + `","validUntil":null,"reason":"new colleague"}`
	}
	newUser := `"newUser":{"email":"` + email + `","displayName":"New Colleague"},`
	if code, m := write(s, &hq, "members.save", body(`"userId":"`+seed.ID("user-customer-a").String()+`",`+newUser), 0); code != 422 || fields(m) != "newUser=error.notAllowed" {
		t.Errorf("newUser with userId: %d %s", code, fields(m))
	}
	code, m := write(s, &hq, "members.save", body(newUser), 0)
	if code != 200 || data(m)["role"] != "admin" {
		t.Fatalf("create with a new person: %d %v", code, m)
	}
	var name string
	ownerScan(t, `SELECT u.display_name FROM identity.users u JOIN identity.memberships m ON m.user_id = u.id WHERE m.id = $1`, []any{data(m)["id"]}, &name)
	if name != "New Colleague" {
		t.Errorf("person: %q", name)
	}
	if code, m := write(s, &hq, "members.save", body(newUser), 0); code != 422 || fields(m) != "newUser.email=errors.email_taken" {
		t.Errorf("the same e-mail again: %d %s", code, fields(m))
	}

	// a contractor's technician created by HQ: the company's user list shows them as not signed in yet, until the first
	// sign-in
	tech := `{"newUser":{"email":"hq-tech-` + uuid.NewString()[:8] + `@example.com","displayName":"HQ Tech"},"organizationId":"` + seed.ID("org-contractor-a").String() +
		`","role":"technician","employment":"external","permissions":["alert.read"],"scopes":[{"kind":"unit","id":"` + seed.ID("unit-non-rto").String() +
		`"}],"validFrom":"` + clock.Format(time.RFC3339) + `","validUntil":"` + clock.Add(30*24*time.Hour).Format(time.RFC3339) + `","reason":"new field technician"}`
	code, m = write(s, &hq, "members.save", tech, 0)
	if code != 200 {
		t.Fatalf("a new technician: %d %v", code, m)
	}
	membership := data(m)["id"].(string)
	var status, user string
	ownerScan(t, `SELECT x.status, m.user_id::text FROM identity.partner_users x JOIN identity.memberships m ON m.id = x.membership_id WHERE m.id = $1`, []any{membership}, &status, &user)
	if status != "invited" {
		t.Errorf("listed before the first sign-in: %s", status)
	}
	(&auth.Authenticator{DB: s.DB, Now: clockFunc}).RecordSignIn(context.Background(), &ops.Principal{TenantID: seed.ID("tenant-a"), UserID: uuid.MustParse(user), MembershipID: uuid.MustParse(membership)}, clock)
	ownerScan(t, `SELECT status FROM identity.partner_users WHERE membership_id = $1`, []any{membership}, &status)
	if status != "active" {
		t.Errorf("listed after the first sign-in: %s", status)
	}
}
