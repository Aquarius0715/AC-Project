package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestClientUsers(t *testing.T) {
	s := server(t)
	cust := seed.ID("cust-b").String()
	email := "member-" + uuid.NewString()[:8] + "@example.com"
	save := func(a *actor, body string, v int) (int, map[string]any) {
		return write(s, a, "clientUsers.save", body, v)
	}
	invite := `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member"}`
	want := map[string]string{ // the field each case is refused for
		"bad email":    "email=error.invalid",
		"bad role":     "clientRole=error.invalid",
		"bad status":   "status=error.invalid",
		"blank reason": "reason=error.length",
	}
	for name, b := range map[string]string{
		"bad email":    `{"customerId":"` + cust + `","email":"not-an-email","clientRole":"member"}`,
		"bad role":     `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"admin"}`,
		"bad status":   `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member","status":"invited"}`,
		"blank reason": `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member","reason":" "}`,
	} {
		code, m := save(&hq, b, 0)
		refusal(t, name, code, m, want[name])
	}
	if code, m := save(&hq, `{"customerId":"`+cust+`","email":"`+email+`","clientRole":"member","status":"active"}`, 0); code != 422 || fields(m) != "status=error.notAllowed" {
		t.Errorf("status on invite: %d %s", code, fields(m))
	}
	if code, _ := save(&customerA, invite, 0); code != 404 {
		t.Error("client invites into another customer")
	}
	code, m := save(&customerB, invite, 0)
	if code != 200 || data(m)["status"] != "invited" || data(m)["membershipId"] != nil || data(m)["invitedByMembershipId"] != seed.ID("customer-b").String() {
		t.Fatalf("invite: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, m := save(&hq, strings.Replace(invite, email, strings.ToUpper(email[:1])+email[1:], 1), 0); code != 422 || fields(m) != "email=errors.email_taken" {
		t.Errorf("duplicate email (case-insensitive): %d %s", code, fields(m))
	}
	_, m = post(s, &customerB, "clientUsers.list", `{"limit":100}`)
	var owner map[string]any
	for _, it := range items(m) {
		if it["membershipId"] == seed.ID("customer-b").String() {
			owner = it
		}
	}
	if owner == nil || owner["clientRole"] != "owner" || owner["status"] != "active" {
		t.Fatalf("seed owner listed: %v", m)
	}
	if _, m := post(s, &hq, "clientUsers.list", `{"filters":{"customerId":"`+cust+`","status":"invited","clientRole":"member"},"limit":100}`); len(items(m)) == 0 {
		t.Error("HQ filtered list")
	}
	if code, m := post(s, &hq, "clientUsers.list", `{"filters":{"status":"gone"}}`); code != 422 || fields(m) != "filters=error.invalid" {
		t.Errorf("bad status filter: %d %s", code, fields(m))
	}
	if _, m := post(s, &customerA, "clientUsers.list", `{"limit":100}`); len(items(m)) == 0 || items(m)[0]["customerId"] != seed.ID("cust-a").String() {
		t.Error("client list scoped to own customer")
	}
	// resend: the invitation e-mail is queued again (IR418)
	if code, m := write(s, &customerB, "clientUsers.resendInvite", `{"id":"`+id+`"}`, 0); code != 200 || data(m)["deliveryState"] != "queued" || data(m)["templateKey"] != "invite" {
		t.Fatalf("resend: %d %v", code, m)
	}
	if code, _ := write(s, &customerB, "clientUsers.resendInvite", `{"id":"`+owner["id"].(string)+`"}`, 0); code != 409 {
		t.Error("resend to an active user")
	}
	if code, _ := write(s, &customerA, "clientUsers.resendInvite", `{"id":"`+id+`"}`, 0); code != 404 {
		t.Error("resend another customer's invite")
	}
	// updates are HQ only; the last active owner is protected
	upd := func(target map[string]any, role, status string, v int) (int, map[string]any) {
		return save(&hq, `{"id":"`+target["id"].(string)+`","customerId":"`+cust+`","email":"`+target["email"].(string)+`","clientRole":"`+role+`","status":"`+status+`","reason":"admin change"}`, v)
	}
	ov := int(owner["version"].(float64))
	if code, _ := upd(owner, "member", "active", ov); code != 409 {
		t.Error("demote the last owner")
	}
	if code, _ := upd(owner, "owner", "disabled", ov); code != 409 {
		t.Error("disable the last owner")
	}
	if code, _ := write(s, &hq, "clientUsers.remove", `{"id":"`+owner["id"].(string)+`","reason":"leaving"}`, ov); code != 409 {
		t.Error("remove the last owner")
	}
	if code, m := save(&hq, `{"id":"`+id+`","customerId":"`+cust+`","email":"`+email+`","clientRole":"member"}`, 1); code != 422 || fields(m) != "reason=error.required" {
		t.Errorf("update without reason: %d %s", code, fields(m))
	}
	if code, _ := upd(map[string]any{"id": id, "email": email}, "member", "active", 1); code != 409 {
		t.Error("activate a pending invite")
	}
	if code, _ := upd(map[string]any{"id": id, "email": email}, "member", "disabled", 5); code != 409 {
		t.Error("stale client user version")
	}
	code, m = upd(map[string]any{"id": id, "email": email}, "owner", "disabled", 1)
	if code != 200 || data(m)["status"] != "disabled" || data(m)["clientRole"] != "owner" || ver(m) != 2 {
		t.Fatalf("update: %d %v", code, m)
	}
	if code, m := write(s, &hq, "clientUsers.remove", `{"id":"`+id+`","reason":"no longer with the company"}`, 2); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "clientUsers.remove", `{"id":"`+id+`","reason":"again"}`, 2); code != 404 {
		t.Error("remove twice")
	}
}

func TestTwoFactor(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM identity.two_factor WHERE user_id = (SELECT user_id FROM identity.memberships WHERE id = $1)`, seed.ID("tech-external-b"))
	code, m := post(s, &techB, "twoFactor.get", `{}`)
	if code != 200 || data(m)["enabled"] != false || data(m)["setupKey"] == nil || data(m)["recoveryCodesLeft"] != nil {
		t.Fatalf("get: %d %v", code, m)
	}
	want := map[string]string{ // the field each case is refused for
		"{\"code\":\"12345\"}":  "code=error.invalid",
		"{\"code\":\"abcdef\"}": "code=error.invalid",
		"{}":                    "code=error.invalid",
	}
	for _, b := range []string{`{"code":"12345"}`, `{"code":"abcdef"}`, `{}`} {
		code, m := write(s, &techB, "twoFactor.enable", b, 0)
		refusal(t, b, code, m, want[b])
	}
	if code, _ := write(s, &techB, "twoFactor.disable", `{"code":"123456"}`, 0); code != 409 {
		t.Error("disable when off")
	}
	code, m = write(s, &techB, "twoFactor.enable", `{"code":"123456"}`, 0)
	if code != 200 || len(data(m)["recoveryCodes"].([]any)) != 8 || data(m)["recoveryCodesLeft"].(float64) != 8 {
		t.Fatalf("enable: %d %v", code, m)
	}
	if code, _ := write(s, &techB, "twoFactor.enable", `{"code":"654321"}`, 0); code != 409 {
		t.Error("enable twice")
	}
	if _, m := post(s, &techB, "twoFactor.get", `{}`); data(m)["enabled"] != true || data(m)["setupKey"] != nil {
		t.Errorf("enabled status: %v", m)
	}
	if _, m := post(s, &techB, "preferences.get", `{}`); data(m)["twoFactorEnabled"] != true {
		t.Error("preferences.twoFactorEnabled")
	}
	if code, m := write(s, &techB, "twoFactor.disable", `{"code":"000000"}`, 0); code != 200 || data(m)["enabled"] != false {
		t.Fatalf("disable: %d %v", code, m)
	}
}

// TestClientUserMembershipSync: a client user who has signed in keeps the membership in step (FR-A17): disabling ends
// its access now, activating opens it again, and removing the user ends it; the customer of a user is fixed, and a
// removal needs the current version.
func TestClientUserMembershipSync(t *testing.T) {
	s := server(t)
	cust := seed.ID("cust-b").String()
	membership, user := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from)
		SELECT $1, tenant_id, user_id, organization_id, 'client', 'member', valid_from FROM identity.memberships WHERE id = $2`, membership, seed.ID("customer-b"))
	email := "signed-in-" + uuid.NewString()[:8] + "@example.com"
	owner(t, `INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, client_role, status, invited_at, invited_by_membership_id)
		VALUES ($1,$2,$3,$4,$5,'member','active',$6,$7)`, user, seed.ID("tenant-a"), cust, membership, email, clock.Add(-time.Hour), seed.ID("customer-b"))
	save := func(customer, status string, v int) (int, map[string]any) {
		return write(s, &hq, "clientUsers.save", `{"id":"`+user+`","customerId":"`+customer+`","email":"`+email+`","clientRole":"member","status":"`+status+`","reason":"admin change"}`, v)
	}
	access := func() (*time.Time, int) {
		var until *time.Time
		var scope int
		ownerScan(t, `SELECT valid_until, scope_version FROM identity.memberships WHERE id = $1`, []any{membership}, &until, &scope)
		return until, scope
	}
	if code, m := save(seed.ID("cust-a").String(), "active", 1); code != 422 || m["fieldErrors"].(map[string]any)["customerId"] != "error.immutable" {
		t.Errorf("move a user to another customer: %d %v", code, m)
	}
	if code, m := save(cust, "disabled", 1); code != 200 || data(m)["status"] != "disabled" {
		t.Fatalf("disable: %d %v", code, m)
	}
	if until, scope := access(); until == nil || !until.Equal(clock) || scope != 2 {
		t.Errorf("membership after disabling: %v %d", until, scope)
	}
	if code, m := save(cust, "active", 2); code != 200 || data(m)["status"] != "active" {
		t.Fatalf("activate again: %d %v", code, m)
	}
	if until, _ := access(); until != nil {
		t.Errorf("membership after activating: %v", until)
	}
	if code, m := write(s, &hq, "clientUsers.remove", `{"id":"`+user+`","reason":"left the company"}`, 2); code != 409 || m["messageKey"] != "error.versionConflict" {
		t.Errorf("remove on a stale version: %d %v", code, m)
	}
	if code, m := write(s, &hq, "clientUsers.remove", `{"id":"`+user+`","reason":"left the company"}`, 3); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, m)
	}
	if until, _ := access(); until == nil || !until.Equal(clock) {
		t.Errorf("membership after removal: %v", until)
	}
}

// TestClientUsersByOwner (IR417): an owner manages the customer's users like HQ: invites members and owners, changes a
// pending invite, a user's role and access with a reason, and removes users; never the own account, never an accepted
// user's e-mail, never another customer's user, and the customer keeps an active owner. A member is refused.
func TestClientUsersByOwner(t *testing.T) {
	s := server(t)
	cust := seed.ID("cust-b").String()
	email := "owner-" + uuid.NewString()[:8] + "@example.com"
	save := func(body string, v int) (int, map[string]any) {
		return write(s, &customerB, "clientUsers.save", body, v)
	}
	code, m := save(`{"customerId":"`+cust+`","email":"`+email+`","clientRole":"owner"}`, 0)
	if code != 200 || data(m)["clientRole"] != "owner" || data(m)["status"] != "invited" {
		t.Fatalf("owner invites an owner: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	email2 := "fixed-" + email
	if code, m := save(`{"id":"`+id+`","customerId":"`+cust+`","email":"`+email2+`","clientRole":"member","reason":"typo"}`, 1); code != 200 || data(m)["email"] != email2 || data(m)["clientRole"] != "member" {
		t.Fatalf("change the invite: %d %v", code, m)
	}
	if code, m := save(`{"id":"`+id+`","customerId":"`+cust+`","email":"`+email2+`","clientRole":"member","status":"active","reason":"x"}`, 2); code != 409 || m["messageKey"] != "errors.invite_pending" {
		t.Errorf("activate an invite: %d %v", code, m)
	}
	// the owner's own row
	_, m = post(s, &customerB, "clientUsers.list", `{"limit":100}`)
	var self map[string]any
	for _, it := range items(m) {
		if it["membershipId"] == seed.ID("customer-b").String() {
			self = it
		}
	}
	sv := int(self["version"].(float64))
	if code, m := save(`{"id":"`+self["id"].(string)+`","customerId":"`+cust+`","email":"`+self["email"].(string)+`","clientRole":"owner","status":"disabled","reason":"x"}`, sv); code != 409 || m["messageKey"] != "errors.own_account" {
		t.Errorf("disable the own account: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "clientUsers.remove", `{"id":"`+self["id"].(string)+`","reason":"x"}`, sv); code != 409 || m["messageKey"] != "errors.own_account" {
		t.Errorf("remove the own account: %d %v", code, m)
	}
	// an accepted member: role and access change, the e-mail stays
	membership, user := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from)
		SELECT $1, tenant_id, user_id, organization_id, 'client', 'member', valid_from FROM identity.memberships WHERE id = $2`, membership, seed.ID("customer-b"))
	memberEmail := "member-" + uuid.NewString()[:8] + "@example.com"
	owner(t, `INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, client_role, status, invited_at, invited_by_membership_id)
		VALUES ($1,$2,$3,$4,$5,'member','active',$6,$7)`, user, seed.ID("tenant-a"), cust, membership, memberEmail, clock.Add(-time.Hour), seed.ID("customer-b"))
	if code, m := save(`{"id":"`+user+`","customerId":"`+cust+`","email":"x@example.com","clientRole":"member","reason":"x"}`, 1); code != 422 || fields(m) != "email=error.immutable" {
		t.Errorf("change an accepted e-mail: %d %s", code, fields(m))
	}
	if code, m := save(`{"id":"`+user+`","customerId":"`+cust+`","email":"`+memberEmail+`","clientRole":"owner","status":"disabled","reason":"on leave"}`, 1); code != 200 || data(m)["clientRole"] != "owner" || data(m)["status"] != "disabled" {
		t.Fatalf("promote and disable: %d %v", code, m)
	}
	if code, _ := write(s, &customerA, "clientUsers.remove", `{"id":"`+user+`","reason":"x"}`, 2); code != 404 {
		t.Error("another customer's owner removes")
	}
	if code, m := write(s, &customerB, "clientUsers.remove", `{"id":"`+user+`","reason":"left"}`, 2); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, m)
	}
	if code, _ := write(s, &customerB, "clientUsers.remove", `{"id":"`+id+`","reason":"withdrawn"}`, 2); code != 200 {
		t.Error("remove the invite")
	}
}

// TestClientInviteAccepted (IR417): when HQ creates the client membership for an invited e-mail, the invite becomes
// that account, and an owner invite makes an owner membership.
func TestClientInviteAccepted(t *testing.T) {
	s := server(t)
	user := newUser(t)
	cust := seed.ID("cust-b").String()
	code, m := write(s, &customerB, "clientUsers.save", `{"customerId":"`+cust+`","email":"`+user+`@ac.local","clientRole":"owner"}`, 0)
	if code != 200 {
		t.Fatalf("invite: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	from := clock.Add(-time.Hour).Format(time.RFC3339)
	code, m = write(s, &hq, "members.save", `{"userId":"`+user+`","organizationId":"`+seed.ID("org-customer-b").String()+`","role":"client","permissions":[],"scopes":[],"validFrom":"`+from+`","validUntil":null,"reason":"onboarding"}`, 0)
	if code != 200 || data(m)["clientRole"] != "owner" {
		t.Fatalf("members.save: %d %v", code, m)
	}
	t.Cleanup(func() { // later tests count customer B's owners: this one ends before them
		owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, data(m)["id"], clock.Add(-30*time.Minute))
	})
	var status, membership string
	ownerScan(t, `SELECT status, COALESCE(membership_id::text, '') FROM identity.client_users WHERE id = $1`, []any{id}, &status, &membership)
	if status != "active" || membership != data(m)["id"] {
		t.Errorf("accepted invite: %s %s", status, membership)
	}
}

// TestDisabledClientStaysClosed (IR420): HQ changing the membership of a client user that is disabled keeps the
// membership closed.
func TestDisabledClientStaysClosed(t *testing.T) {
	s := server(t)
	user := newUser(t)
	cust := seed.ID("cust-b").String()
	from := clock.Add(-time.Hour).Format(time.RFC3339)
	code, m := write(s, &hq, "members.save", `{"userId":"`+user+`","organizationId":"`+seed.ID("org-customer-b").String()+`","role":"client","permissions":[],"scopes":[],"validFrom":"`+from+`","validUntil":null,"reason":"onboarding"}`, 0)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, m)
	}
	membership := data(m)["id"].(string)
	var id string
	var v int
	ownerScan(t, `SELECT id::text, version FROM identity.client_users WHERE membership_id = $1`, []any{membership}, &id, &v)
	if code, m := write(s, &customerB, "clientUsers.save", `{"id":"`+id+`","customerId":"`+cust+`","email":"`+user+`@ac.local","clientRole":"member","status":"disabled","reason":"left"}`, v); code != 200 {
		t.Fatalf("disable: %d %v", code, m)
	}
	var mv int
	ownerScan(t, `SELECT version FROM identity.memberships WHERE id = $1`, []any{membership}, &mv)
	later := clock.Add(90 * 24 * time.Hour).Format(time.RFC3339)
	if code, m := write(s, &hq, "members.save", `{"id":"`+membership+`","userId":"`+user+`","organizationId":"`+seed.ID("org-customer-b").String()+`","role":"client","permissions":[],"scopes":[],"validFrom":"`+from+`","validUntil":"`+later+`","reason":"contract"}`, mv); code != 200 {
		t.Fatalf("HQ change: %d %v", code, m)
	}
	var end time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &end)
	if !end.Equal(clock) {
		t.Errorf("disabled client user's membership after HQ's change: %v, want still closed at %v", end, clock)
	}
}
