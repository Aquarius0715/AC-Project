package integration

import (
	"testing"
	"time"

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
		{"clientUsers.resendInvite", `{"id":"` + clientUser + `"}`, 0, false},
		{"clientUsers.save", `{"customerId":"` + custA + `","email":"member.invite@example.com","clientRole":"member"}`, 0, true},
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
