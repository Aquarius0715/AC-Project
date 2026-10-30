package integration

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestHQRevokeTurnsSignInOff (IR508, DEC-83): HQ's revoke in Access & roles ends the access and turns the sign-in off,
// and only HQ gives it back. The contractor cannot extend the access end, enable the account again or add the person
// as a new account; disabling and removing stay its own. HQ's later end clears the revoke: the sign-in comes back
// unless the company disabled the account, which it may then enable again.
func TestHQRevokeTurnsSignInOff(t *testing.T) {
	fake := &fakeIdP{}
	s := serverCfg(t, func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains })
	drain := func() {
		t.Helper()
		if err := s.DrainEvents(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	switched := func(want string) bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return slices.Contains(fake.switched, want)
	}
	email := "revoked-" + uuid.NewString()[:8] + "@example.com"
	access := clock.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	code, m := write(s, &contrA, "partnerUsers.save", `{"email":"`+email+`","partnerRole":"technician","displayName":"Revoked Tech","accessUntil":"`+access+`"}`, 0)
	if code != 200 {
		t.Fatalf("add: %d %v", code, m)
	}
	id, membership := data(m)["id"].(string), data(m)["membershipId"].(string)
	drain()
	// signed in once, an hour into the membership (the test clock stands still)
	owner(t, `UPDATE identity.partner_users SET status = 'active' WHERE id = $1`, id)
	owner(t, `UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '1 hour' WHERE id = $1`, membership, clock)
	var user string
	ownerScan(t, `SELECT user_id::text FROM identity.memberships WHERE id = $1`, []any{membership}, &user)
	from := clock.Add(-time.Hour).Format(time.RFC3339)
	hqSave := func(until string) (int, map[string]any) {
		var v int
		ownerScan(t, `SELECT version FROM identity.memberships WHERE id = $1`, []any{membership}, &v)
		return write(s, &hq, "members.save", `{"id":"`+membership+`","userId":"`+user+`","organizationId":"`+seed.ID("org-contractor-a").String()+`","role":"technician","employment":"external",`+
			`"permissions":["alert.read","alert.resolve","control.diagnose","device.maintain"],"scopes":[],"validFrom":"`+from+`","validUntil":`+until+`,"reason":"access review"}`, v)
	}
	partner := func(body string) (int, map[string]any) {
		var v int
		ownerScan(t, `SELECT version FROM identity.partner_users WHERE id = $1`, []any{id}, &v)
		return write(s, &contrA, "partnerUsers.save", `{"id":"`+id+`","email":"`+email+`","partnerRole":"technician",`+body+`,"reason":"back on the team"}`, v)
	}
	listed := func() map[string]any {
		t.Helper()
		_, m := post(s, &contrA, "partnerUsers.list", `{"limit":100}`)
		for _, it := range items(m) {
			if it["id"] == id {
				return it
			}
		}
		t.Fatalf("the user is not listed: %v", m)
		return nil
	}

	// HQ revokes: the access ends now, the sign-in goes off, and the company's list says so
	code, m = hqSave(`"` + clock.Format(time.RFC3339) + `"`)
	if code != 200 || data(m)["revokedAt"] == nil {
		t.Fatalf("revoke: %d %v", code, m)
	}
	drain()
	if !switched("sub-" + email + "=false") {
		t.Errorf("the sign-in was not turned off: %v", fake.switched)
	}
	if u := listed(); u["revokedAt"] == nil {
		t.Errorf("the company's list: %v", u)
	}
	// the contractor gives nothing back: no access end, no enabling; disabling is its own
	if code, m := partner(`"accessUntil":"` + access + `"`); code != 409 || m["messageKey"] != "errors.revoked_by_hq" {
		t.Errorf("extend: %d %v", code, m)
	}
	if code, m := partner(`"status":"disabled"`); code != 200 {
		t.Fatalf("disable: %d %v", code, m)
	}
	if code, m := partner(`"status":"active"`); code != 409 || m["messageKey"] != "errors.revoked_by_hq" {
		t.Errorf("enable: %d %v", code, m)
	}
	// saving again keeps the first revoke (the test clock stands still: the first is moved a minute back)
	first := clock.Add(-time.Minute)
	owner(t, `UPDATE identity.memberships SET revoked_at = $2 WHERE id = $1`, membership, first)
	if code, m := hqSave(`"` + clock.Format(time.RFC3339) + `"`); code != 200 {
		t.Fatalf("save again: %d %v", code, m)
	}
	var kept time.Time
	ownerScan(t, `SELECT revoked_at FROM identity.memberships WHERE id = $1`, []any{membership}, &kept)
	if !kept.Equal(first) {
		t.Errorf("the revoke moved: %v, want %v", kept, first)
	}

	// HQ gives the access back: the revoke is cleared, the account stays off while the company keeps it disabled ...
	later := clock.Add(10 * 24 * time.Hour).Format(time.RFC3339)
	if code, m := hqSave(`"` + later + `"`); code != 200 || data(m)["revokedAt"] != nil {
		t.Fatalf("give back: %d %v", code, m)
	}
	drain()
	if switched("sub-" + email + "=true") {
		t.Errorf("the sign-in came back while the company keeps the account disabled: %v", fake.switched)
	}
	// ... and comes back when the company enables it, with HQ's end
	if code, m := partner(`"status":"active"`); code != 200 || data(m)["accessUntil"] != later {
		t.Fatalf("enable after HQ: %d %v", code, m)
	}
	drain()
	if !switched("sub-" + email + "=true") {
		t.Errorf("the sign-in did not come back: %v", fake.switched)
	}

	// revoked again and removed: the person cannot come back as a new account of any company
	if code, m := hqSave(`"` + clock.Format(time.RFC3339) + `"`); code != 200 {
		t.Fatalf("revoke again: %d %v", code, m)
	}
	var v int
	ownerScan(t, `SELECT version FROM identity.partner_users WHERE id = $1`, []any{id}, &v)
	if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+id+`","reason":"left"}`, v); code != 200 {
		t.Fatalf("remove: %d %v", code, m)
	}
	for name, a := range map[string]*actor{"the same company": &contrA, "another company": &contrB} {
		if code, m := write(s, a, "partnerUsers.save", `{"email":"`+email+`","partnerRole":"contractor","displayName":"Again"}`, 0); code != 422 || fields(m) != "email=errors.revoked_by_hq" {
			t.Errorf("%s adds the person again: %d %s", name, code, fields(m))
		}
	}
}

// TestHQRevokeOfAClientUser (IR508): a customer owner cannot change a user whose access HQ ended — a save would open
// the membership again — but may disable or remove them.
func TestHQRevokeOfAClientUser(t *testing.T) {
	s := server(t)
	user := newUser(t)
	cust := seed.ID("cust-b").String()
	from := clock.Add(-time.Hour).Format(time.RFC3339)
	body := func(until string) string {
		return `{"userId":"` + user + `","organizationId":"` + seed.ID("org-customer-b").String() + `","role":"client","permissions":[],"scopes":[],"validFrom":"` + from + `","validUntil":` + until + `,"reason":"access review"}`
	}
	code, m := write(s, &hq, "members.save", body("null"), 0)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, m)
	}
	membership := data(m)["id"].(string)
	var mv int
	ownerScan(t, `SELECT version FROM identity.memberships WHERE id = $1`, []any{membership}, &mv)
	revoke := `{"id":"` + membership + `",` + body(`"` + clock.Format(time.RFC3339) + `"`)[1:]
	if code, m := write(s, &hq, "members.save", revoke, mv); code != 200 || data(m)["revokedAt"] == nil {
		t.Fatalf("revoke: %d %v", code, m)
	}
	var id string
	var v int
	ownerScan(t, `SELECT id::text, version FROM identity.client_users WHERE membership_id = $1`, []any{membership}, &id, &v)
	save := func(role, status string) (int, map[string]any) {
		ownerScan(t, `SELECT version FROM identity.client_users WHERE id = $1`, []any{id}, &v)
		st := "" // a role change sends no status
		if status != "" {
			st = `,"status":"` + status + `"`
		}
		return write(s, &customerB, "clientUsers.save", `{"id":"`+id+`","customerId":"`+cust+`","email":"`+user+`@ac.local","clientRole":"`+role+`"`+st+`,"reason":"review"}`, v)
	}
	if code, m := save("owner", ""); code != 409 || m["messageKey"] != "errors.revoked_by_hq" {
		t.Errorf("role change: %d %v", code, m)
	}
	if code, m := save("member", "disabled"); code != 200 {
		t.Fatalf("disable: %d %v", code, m)
	}
	if code, m := save("member", "active"); code != 409 || m["messageKey"] != "errors.revoked_by_hq" {
		t.Errorf("enable: %d %v", code, m)
	}
	var end time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &end)
	if end.After(clock) {
		t.Errorf("the membership opened again: %v", end)
	}
}

// TestHQSaveOfAnEndedAccessIsNoRevoke (IR514): HQ's form sends whole minutes. Saving other changes to a technician
// whose access ended by itself (the contractor's end is the day's last second) is no revoke: the sign-in is not
// touched and the contractor can still give a new access end.
func TestHQSaveOfAnEndedAccessIsNoRevoke(t *testing.T) {
	fake := &fakeIdP{}
	s := serverCfg(t, func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains })
	email := "ended-" + uuid.NewString()[:8] + "@example.com"
	code, m := write(s, &contrA, "partnerUsers.save", `{"email":"`+email+`","partnerRole":"technician","displayName":"Ended Tech","accessUntil":"`+clock.Add(30*24*time.Hour).Format(time.RFC3339)+`"}`, 0)
	if code != 200 {
		t.Fatalf("add: %d %v", code, m)
	}
	id, membership := data(m)["id"].(string), data(m)["membershipId"].(string)
	if err := s.DrainEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	ended := clock.Add(-time.Hour).Truncate(time.Minute).Add(59 * time.Second) // the last second of a minute, an hour ago
	owner(t, `UPDATE identity.partner_users SET status = 'active' WHERE id = $1`, id)
	owner(t, `UPDATE identity.memberships SET valid_from = $2, valid_until = $3 WHERE id = $1`, membership, clock.Add(-48*time.Hour), ended)
	var user string
	var v int
	ownerScan(t, `SELECT user_id::text, version FROM identity.memberships WHERE id = $1`, []any{membership}, &user, &v)
	code, m = write(s, &hq, "members.save", `{"id":"`+membership+`","userId":"`+user+`","organizationId":"`+seed.ID("org-contractor-a").String()+`","role":"technician","employment":"external",`+
		`"permissions":["alert.read"],"scopes":[],"validFrom":"`+clock.Add(-48*time.Hour).Format(time.RFC3339)+`","validUntil":"`+ended.Truncate(time.Minute).Format(time.RFC3339)+`","reason":"fewer permissions"}`, v)
	if code != 200 || data(m)["revokedAt"] != nil {
		t.Fatalf("HQ's save of an ended access: %d %v", code, m)
	}
	if err := s.DrainEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	switched := slices.ContainsFunc(fake.switched, func(x string) bool { return strings.HasPrefix(x, "sub-"+email+"=") })
	fake.mu.Unlock()
	if switched {
		t.Errorf("the sign-in was switched: %v", fake.switched)
	}
	ownerScan(t, `SELECT version FROM identity.partner_users WHERE id = $1`, []any{id}, &v)
	if code, m := write(s, &contrA, "partnerUsers.save", `{"id":"`+id+`","email":"`+email+`","partnerRole":"technician","accessUntil":"`+clock.Add(20*24*time.Hour).Format(time.RFC3339)+`","reason":"back"}`, v); code != 200 {
		t.Errorf("the contractor's new access end: %d %v", code, m)
	}
}
