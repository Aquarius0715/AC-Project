package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestPartnerUsers (FR-P11): a contractor lists its company's accounts, invites contractors and technicians, changes
// a pending invite, resends it and removes it; another company's accounts are NOT_FOUND, and other roles are refused.
func TestPartnerUsers(t *testing.T) {
	s := server(t)
	email := "tech-" + uuid.NewString()[:8] + "@example.com"
	save := func(a *actor, body string, v int) (int, map[string]any) {
		return write(s, a, "partnerUsers.save", body, v)
	}
	invite := `{"email":"` + email + `","partnerRole":"technician"}`
	want := map[string]string{ // the field each case is refused for
		"bad email":    "email=error.invalid",
		"bad role":     "partnerRole=error.invalid",
		"bad status":   "status=error.invalid",
		"blank reason": "reason=error.length",
	}
	for name, b := range map[string]string{
		"bad email":    `{"email":"not-an-email","partnerRole":"technician"}`,
		"bad role":     `{"email":"` + email + `","partnerRole":"admin"}`,
		"bad status":   `{"email":"` + email + `","partnerRole":"technician","status":"invited"}`,
		"blank reason": `{"email":"` + email + `","partnerRole":"technician","reason":" "}`,
	} {
		code, m := save(&contrA, b, 0)
		refusal(t, name, code, m, want[name])
	}
	if code, m := save(&contrA, `{"email":"`+email+`","partnerRole":"technician","status":"active"}`, 0); code != 422 || fields(m) != "status=error.notAllowed" {
		t.Errorf("status on invite: %d %s", code, fields(m))
	}
	for _, a := range []*actor{&techA, &customerA, &hq} {
		if code, _ := post(s, a, "partnerUsers.list", `{"limit":10}`); code != 403 {
			t.Errorf("%s lists contractor users: %d", a.membership, code)
		}
		if code, _ := save(a, invite, 0); code != 403 {
			t.Errorf("%s invites: %d", a.membership, code)
		}
	}
	code, m := save(&contrA, invite, 0)
	if code != 200 || data(m)["status"] != "invited" || data(m)["partnerRole"] != "technician" || data(m)["membershipId"] != nil ||
		data(m)["organizationId"] != seed.ID("org-contractor-a").String() || data(m)["invitedByMembershipId"] != seed.ID("contractor-a").String() {
		t.Fatalf("invite: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, m := save(&contrA, strings.Replace(invite, email, strings.ToUpper(email[:1])+email[1:], 1), 0); code != 422 || fields(m) != "email=errors.email_taken" {
		t.Errorf("duplicate email (case-insensitive): %d %s", code, fields(m))
	}
	if code, _ := save(&contrB, invite, 0); code != 200 { // the same email at another company is another account
		t.Errorf("same email at company B: %d", code)
	}
	_, m = post(s, &contrA, "partnerUsers.list", `{"limit":100}`)
	byMembership := map[string]map[string]any{}
	for _, it := range items(m) {
		if it["organizationId"] != seed.ID("org-contractor-a").String() {
			t.Fatalf("another company's account listed: %v", it)
		}
		if mid, ok := it["membershipId"].(string); ok {
			byMembership[mid] = it
		}
	}
	self, tech := byMembership[seed.ID("contractor-a").String()], byMembership[seed.ID("tech-external-a").String()]
	if self == nil || self["partnerRole"] != "contractor" || self["status"] != "active" || tech == nil || tech["partnerRole"] != "technician" || tech["accessUntil"] == nil {
		t.Fatalf("seed accounts listed: %v", m)
	}
	if _, m := post(s, &contrA, "partnerUsers.list", `{"filters":{"status":"invited","partnerRole":"technician","search":"`+email[:12]+`"},"limit":100}`); len(items(m)) != 1 {
		t.Errorf("filtered list: %v", m)
	}
	if code, m := post(s, &contrA, "partnerUsers.list", `{"filters":{"partnerRole":"owner"}}`); code != 422 || fields(m) != "filters=error.invalid" {
		t.Errorf("bad role filter: %d %s", code, fields(m))
	}
	// resend: preview only, pending invites of the own company
	if code, m := post(s, &contrA, "partnerUsers.resendInvite", `{"id":"`+id+`"}`); code != 200 || data(m)["deliveryState"] != "preview" || data(m)["target"].(map[string]any)["kind"] != "partner_user" {
		t.Fatalf("resend: %d %v", code, m)
	}
	if code, _ := post(s, &contrA, "partnerUsers.resendInvite", `{"id":"`+tech["id"].(string)+`"}`); code != 409 {
		t.Error("resend to an active account")
	}
	if code, _ := post(s, &contrB, "partnerUsers.resendInvite", `{"id":"`+id+`"}`); code != 404 {
		t.Error("resend another company's invite")
	}
	// a pending invite: email and role can change, its status cannot
	upd := func(id, email, role, status string, v int) (int, map[string]any) {
		st := ""
		if status != "" {
			st = `,"status":"` + status + `"`
		}
		return save(&contrA, `{"id":"`+id+`","email":"`+email+`","partnerRole":"`+role+`"`+st+`,"reason":"correction"}`, v)
	}
	if code, m := save(&contrA, `{"id":"`+id+`","email":"`+email+`","partnerRole":"contractor"}`, 1); code != 422 || fields(m) != "reason=error.required" {
		t.Errorf("update without reason: %d %s", code, fields(m))
	}
	if code, m := upd(id, email, "technician", "active", 1); code != 409 || m["messageKey"] != "errors.invite_pending" {
		t.Errorf("activate a pending invite: %d %v", code, m)
	}
	if code, _ := upd(id, email, "contractor", "", 5); code != 409 {
		t.Error("stale version")
	}
	email2 := "fixed-" + email
	if code, m := upd(id, email2, "contractor", "", 1); code != 200 || data(m)["partnerRole"] != "contractor" || data(m)["email"] != email2 || data(m)["status"] != "invited" || ver(m) != 2 {
		t.Fatalf("change the invite: %d %v", code, m)
	}
	if code, _ := save(&contrB, `{"id":"`+id+`","email":"`+email2+`","partnerRole":"technician","reason":"x"}`, 2); code != 404 {
		t.Error("company B changes company A's invite")
	}
	// the caller's own account is not changed here
	sv := int(self["version"].(float64))
	if code, m := upd(self["id"].(string), self["email"].(string), "contractor", "disabled", sv); code != 409 || m["messageKey"] != "errors.own_account" {
		t.Errorf("disable the own account: %d %v", code, m)
	}
	if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+self["id"].(string)+`","reason":"leaving"}`, sv); code != 409 || m["messageKey"] != "errors.own_account" {
		t.Errorf("remove the own account: %d %v", code, m)
	}
	if code, _ := write(s, &contrB, "partnerUsers.remove", `{"id":"`+id+`","reason":"x"}`, 2); code != 404 {
		t.Error("company B removes company A's invite")
	}
	if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+id+`","reason":"sent to the wrong person"}`, 2); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, m)
	}
	if code, _ := write(s, &contrA, "partnerUsers.remove", `{"id":"`+id+`","reason":"again"}`, 2); code != 404 {
		t.Error("remove twice")
	}
}

// TestPartnerUserAccess: disabling a technician's account ends its membership now and keeps its access end; enabling
// it again brings that end back; an accepted account's email and role are fixed; the company keeps an active
// contractor.
func TestPartnerUserAccess(t *testing.T) {
	s := server(t)
	membership := seed.ID("tech-external-a")
	var until time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &until)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, membership, until)
		owner(t, `UPDATE identity.partner_users SET status = 'active', access_until = NULL WHERE membership_id = $1`, membership)
	})
	var id, email string
	var v int
	ownerScan(t, `SELECT id::text, email::text, version FROM identity.partner_users WHERE membership_id = $1`, []any{membership}, &id, &email, &v)
	save := func(email, role, status string, v int) (int, map[string]any) {
		return write(s, &contrA, "partnerUsers.save", `{"id":"`+id+`","email":"`+email+`","partnerRole":"`+role+`","status":"`+status+`","reason":"on leave"}`, v)
	}
	if code, m := save("other@example.com", "technician", "active", v); code != 422 || fields(m) != "email=error.immutable" {
		t.Errorf("change an accepted email: %d %s", code, fields(m))
	}
	if code, m := save(email, "contractor", "active", v); code != 422 || fields(m) != "partnerRole=error.immutable" {
		t.Errorf("change an accepted role: %d %s", code, fields(m))
	}
	code, m := save(email, "technician", "disabled", v)
	if code != 200 || data(m)["status"] != "disabled" || data(m)["accessUntil"] != until.UTC().Format(time.RFC3339) {
		t.Fatalf("disable: %d %v", code, m)
	}
	var now time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &now)
	if !now.Equal(clock) {
		t.Errorf("membership after disabling: %v, want %v", now, clock)
	}
	code, m = save(email, "technician", "active", v+1)
	if code != 200 || data(m)["status"] != "active" {
		t.Fatalf("enable: %d %v", code, m)
	}
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &now)
	if !now.Equal(until) {
		t.Errorf("membership after enabling: %v, want %v", now, until)
	}
	// an access end that has passed while disabled is not brought back
	if code, _ := save(email, "technician", "disabled", v+2); code != 200 {
		t.Fatal("disable again")
	}
	owner(t, `UPDATE identity.partner_users SET access_until = $2 WHERE id = $1`, id, clock.Add(-time.Minute))
	if code, m := save(email, "technician", "active", v+3); code != 409 || m["messageKey"] != "errors.access_expired" {
		t.Errorf("enable after the access end: %d %v", code, m)
	}
	// the last active contractor: another contractor account, with the company's other active contractor accounts
	// (the caller's own, and any another test added) switched off for the test
	other := uuid.NewString()
	var active []string
	ownerScan(t, `SELECT COALESCE(array_agg(id::text), '{}') FROM identity.partner_users WHERE organization_id = $1 AND partner_role = 'contractor' AND status = 'active'`,
		[]any{seed.ID("org-contractor-a")}, &active)
	owner(t, `INSERT INTO identity.partner_users (id, tenant_id, organization_id, email, partner_role, status, invited_at, invited_by_membership_id)
		VALUES ($1, $2, $3, $4, 'contractor', 'active', $5, $6)`, other, seed.ID("tenant-a"), seed.ID("org-contractor-a"), "boss-"+other[:8]+"@example.com", clock, seed.ID("contractor-a"))
	owner(t, `UPDATE identity.partner_users SET status = 'disabled' WHERE id = ANY($1::uuid[])`, active)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.partner_users SET status = 'active' WHERE id = ANY($1::uuid[])`, active)
		owner(t, `DELETE FROM identity.partner_users WHERE id = $1`, other)
	})
	if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+other+`","reason":"leaving"}`, 1); code != 409 || m["messageKey"] != "errors.last_contractor" {
		t.Errorf("remove the last active contractor: %d %v", code, m)
	}
}

// TestPartnerInviteAccepted: when HQ creates the account for an invited email (members.save), the invite becomes that
// active account.
func TestPartnerInviteAccepted(t *testing.T) {
	s := server(t)
	user := newUser(t)
	email := user + "@ac.local"
	code, m := write(s, &contrA, "partnerUsers.save", `{"email":"`+email+`","partnerRole":"technician"}`, 0)
	if code != 200 {
		t.Fatalf("invite: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	from, until := clock.Add(-time.Hour).Format(time.RFC3339), clock.Add(30*24*time.Hour).Format(time.RFC3339)
	code, m = write(s, &hq, "members.save", `{"userId":"`+user+`","organizationId":"`+seed.ID("org-contractor-a").String()+`","role":"technician","employment":"external",
		"permissions":[],"scopes":[],"validFrom":"`+from+`","validUntil":"`+until+`","reason":"onboarding"}`, 0)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, m)
	}
	membership := data(m)["id"].(string)
	_, m = post(s, &contrA, "partnerUsers.list", `{"filters":{"search":"`+user[:13]+`"},"limit":10}`)
	if len(items(m)) != 1 || items(m)[0]["id"] != id || items(m)[0]["status"] != "active" || items(m)[0]["membershipId"] != membership || items(m)[0]["accessUntil"] != until {
		t.Fatalf("accepted invite: %v", m)
	}
}
