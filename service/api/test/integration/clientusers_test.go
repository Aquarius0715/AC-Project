package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestClientUsers(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	email := "member-" + uuid.NewString()[:8] + "@example.com"
	save := func(a *actor, body string, v int) (int, map[string]any) {
		return write(srv, a, "clientUsers.save", body, v)
	}
	invite := `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member","displayName":"New Member"}`
	want := map[string]string{ // the field each case is refused for
		caseBadEmail:    "email=error.invalid",
		caseBadRole:     "clientRole=error.invalid",
		caseBadStatus:   "status=error.invalid",
		caseBlankReason: refusedReasonLength,
	}

	for name, b := range map[string]string{
		caseBadEmail: `{"customerId":"` + cust + `","email":"not-an-email","clientRole":"member","displayName":"N"}`,
		caseBadRole:  `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"admin","displayName":"N"}`,
		caseBadStatus: `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member","status":"invited",` +
			`"displayName":"N"}`,
		caseBlankReason: `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member","reason":" ",` +
			`"displayName":"N"}`,
	} {
		code, m := save(hq(), b, 0)
		refusal(t, name, code, m, want[name])
	}

	if code, res := save(
		hq(),
		`{"customerId":"`+cust+`","email":"`+email+`","clientRole":"member","status":"active","displayName":"N"}`,
		0,
	); code != 422 ||
		fields(res) != refusedStatusNotAllowed {
		t.Errorf("status on invite: %d %s", code, fields(res))
	}

	if code, _ := save(customerA(), invite, 0); code != 404 {
		t.Error("client invites into another customer")
	}

	code, resp := save(customerB(), invite, 0)
	if code != 200 || data(resp)["status"] != statusInvited || data(resp)["membershipId"] == nil ||
		data(resp)["invitedByMembershipId"] != seed.ID("customer-b").String() {
		t.Fatalf("invite: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])

	if code, res := save(
		hq(),
		strings.Replace(invite, email, strings.ToUpper(email[:1])+email[1:], 1),
		0,
	); code != 422 ||
		fields(res) != refusedEmailTaken {
		t.Errorf("duplicate email (case-insensitive): %d %s", code, fields(res))
	}

	_, resp = post(srv, customerB(), "clientUsers.list", `{"limit":100}`)

	var seedOwner map[string]any

	for _, it := range items(t, resp) {
		if it["membershipId"] == seed.ID("customer-b").String() {
			seedOwner = it
		}
	}

	if seedOwner == nil || seedOwner["clientRole"] != clientRoleOwner ||
		seedOwner["status"] != statusActive {
		t.Fatalf("seed owner listed: %v", resp)
	}

	if _, res := post(
		srv,
		hq(),
		"clientUsers.list",
		`{"filters":{"customerId":"`+cust+`","status":"invited","clientRole":"member"},"limit":100}`,
	); len(
		items(t, res),
	) == 0 {
		t.Error("HQ filtered list")
	}

	if code, res := post(
		srv,
		hq(),
		"clientUsers.list",
		`{"filters":{"status":"gone"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad status filter: %d %s", code, fields(res))
	}

	if _, res := post(
		srv,
		customerA(),
		"clientUsers.list",
		`{"limit":100}`,
	); len(items(t, res)) == 0 ||
		items(t, res)[0]["customerId"] != seed.ID("cust-a").String() {
		t.Error("client list scoped to own customer")
	}
	// resend: not sooner than 5 minutes after the last invitation e-mail (IR423), then queued again (IR418)
	if code, res := write(
		srv,
		customerB(),
		"clientUsers.resendInvite",
		`{"id":"`+id+`"}`,
		0,
	); code != 429 || res["messageKey"] != "errors.invite_recently_sent" ||
		res["retryAfterSeconds"] != float64(300) {
		t.Errorf("resend right after the invite: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE identity.client_users SET last_invite_sent_at = last_invite_sent_at - interval '5 minutes' WHERE id = $1`,
		id,
	)

	if code, res := write(
		srv,
		customerB(),
		"clientUsers.resendInvite",
		`{"id":"`+id+`"}`,
		0,
	); code != 200 || data(res)["deliveryState"] != statusQueued ||
		data(res)["templateKey"] != "invite" {
		t.Fatalf("resend: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		customerB(),
		"clientUsers.resendInvite",
		`{"id":"`+asStr(t, seedOwner["id"])+`"}`,
		0,
	); code != 409 {
		t.Error("resend to an active user")
	}

	if code, _ := write(
		srv,
		customerA(),
		"clientUsers.resendInvite",
		`{"id":"`+id+`"}`,
		0,
	); code != 404 {
		t.Error("resend another customer's invite")
	}
	// updates are HQ only; the last active owner is protected
	upd := func(target map[string]any, role, status string, v int) (int, map[string]any) {
		return save(
			hq(),
			`{"id":"`+asStr(
				t,
				target["id"],
			)+`","customerId":"`+cust+`","email":"`+asStr(t, target["email"])+`","clientRole":"`+
				role+`","status":"`+status+`","reason":"admin change"}`,
			v,
		)
	}

	ov := int(asNum(t, seedOwner["version"]))
	if code, _ := upd(seedOwner, "member", "active", ov); code != 409 {
		t.Error("demote the last owner")
	}

	if code, _ := upd(seedOwner, "owner", "disabled", ov); code != 409 {
		t.Error("disable the last owner")
	}

	if code, _ := write(
		srv,
		hq(),
		"clientUsers.remove",
		`{"id":"`+asStr(t, seedOwner["id"])+`","reason":"leaving"}`,
		ov,
	); code != 409 {
		t.Error("remove the last owner")
	}

	if code, res := save(
		hq(),
		`{"id":"`+id+`","customerId":"`+cust+`","email":"`+email+`","clientRole":"member"}`,
		1,
	); code != 422 ||
		fields(res) != refusedReasonRequired {
		t.Errorf("update without reason: %d %s", code, fields(res))
	}

	if code, _ := upd(
		map[string]any{"id": id, fieldEmail: email},
		"member",
		"active",
		1,
	); code != 409 {
		t.Error("activate a pending invite")
	}

	if code, _ := upd(
		map[string]any{"id": id, fieldEmail: email},
		"member",
		"disabled",
		5,
	); code != 409 {
		t.Error("stale client user version")
	}

	code, resp = upd(map[string]any{"id": id, fieldEmail: email}, "owner", "disabled", 1)
	if code != 200 || data(resp)["status"] != statusDisabled ||
		data(resp)["clientRole"] != clientRoleOwner ||
		ver(t, resp) != 2 {
		t.Fatalf("update: %d %v", code, resp)
	}

	if code, res := write(
		srv,
		hq(),
		"clientUsers.remove",
		`{"id":"`+id+`","reason":"no longer with the company"}`,
		2,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"clientUsers.remove",
		`{"id":"`+id+`","reason":"again"}`,
		2,
	); code != 404 {
		t.Error("remove twice")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestTwoFactor(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`DELETE FROM identity.two_factor WHERE user_id = (SELECT user_id FROM identity.memberships WHERE id = $1)`,
		seed.ID("tech-external-b"),
	)

	code, resp := post(srv, techB(), "twoFactor.get", `{}`)
	if code != 200 || data(resp)["enabled"] != false || data(resp)["setupKey"] == nil ||
		data(resp)["recoveryCodesLeft"] != nil {
		t.Fatalf("get: %d %v", code, resp)
	}

	want := map[string]string{ // the field each case is refused for
		"{\"code\":\"12345\"}":  refusedCodeInvalid,
		"{\"code\":\"abcdef\"}": refusedCodeInvalid,
		"{}":                    refusedCodeInvalid,
	}

	for _, b := range []string{`{"code":"12345"}`, `{"code":"abcdef"}`, `{}`} {
		code, m := write(srv, techB(), "twoFactor.enable", b, 0)
		refusal(t, b, code, m, want[b])
	}

	if code, _ := write(srv, techB(), "twoFactor.disable", `{"code":"123456"}`, 0); code != 409 {
		t.Error("disable when off")
	}

	code, resp = write(srv, techB(), "twoFactor.enable", `{"code":"123456"}`, 0)
	if code != 200 || len(asArr(t, data(resp)["recoveryCodes"])) != 8 ||
		asNum(t, data(resp)["recoveryCodesLeft"]) != 8 {
		t.Fatalf("enable: %d %v", code, resp)
	}

	if code, _ := write(srv, techB(), "twoFactor.enable", `{"code":"654321"}`, 0); code != 409 {
		t.Error("enable twice")
	}

	if _, res := post(
		srv,
		techB(),
		"twoFactor.get",
		`{}`,
	); data(res)["enabled"] != true ||
		data(res)["setupKey"] != nil {
		t.Errorf("enabled status: %v", res)
	}

	if _, m := post(srv, techB(), "preferences.get", `{}`); data(m)["twoFactorEnabled"] != true {
		t.Error("preferences.twoFactorEnabled")
	}

	if code, res := write(
		srv,
		techB(),
		"twoFactor.disable",
		`{"code":"000000"}`,
		0,
	); code != 200 ||
		data(res)["enabled"] != false {
		t.Fatalf("disable: %d %v", code, res)
	}
}

// TestClientUserMembershipSync: a client user who has signed in keeps the membership in step (FR-A17): disabling ends
// its access now, activating opens it again, and removing the user ends it; the customer of a user is fixed, and a
// removal needs the current version.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestClientUserMembershipSync(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	membership, user := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from)
		SELECT $1, tenant_id, user_id, organization_id, 'client', 'member', valid_from FROM identity.memberships WHERE id =
		$2`,
		membership,
		seed.ID("customer-b"),
	)

	email := "signed-in-" + uuid.NewString()[:8] + "@example.com"
	owner(
		t,
		`INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, client_role, status, invited_at,
			invited_by_membership_id)
		VALUES ($1,$2,$3,$4,$5,'member','active',$6,$7)`,
		user,
		seed.ID("tenant-a"),
		cust,
		membership,
		email,
		clock().Add(-time.Hour),
		seed.ID("customer-b"),
	)

	save := func(customer, status string, v int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"clientUsers.save",
			`{"id":"`+user+`","customerId":"`+customer+`","email":"`+email+`","clientRole":"member",`+
				`"status":"`+status+`","reason":"admin change"}`,
			v,
		)
	}
	access := func() (*time.Time, int) {
		var (
			until *time.Time
			scope int
		)
		ownerScan(
			t,
			`SELECT valid_until, scope_version FROM identity.memberships WHERE id = $1`,
			[]any{membership},
			&until,
			&scope,
		)

		return until, scope
	}

	if code, res := save(
		seed.ID("cust-a").String(),
		"active",
		1,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["customerId"] != apperr.KeyImmutable {
		t.Errorf("move a user to another customer: %d %v", code, res)
	}
	// the e-mail of a signed-in user is the sign-in identity's: HQ cannot change it either (IR422)
	if code, res := write(
		srv,
		hq(),
		"clientUsers.save",
		`{"id":"`+user+`","customerId":"`+cust+`","email":"other-`+email+`","clientRole":"member","reason":"admin change"}`,
		1,
	); code != 422 ||
		fields(res) != refusedEmailImmutable {
		t.Errorf("HQ changes an accepted user's e-mail: %d %v", code, res)
	}

	if code, m := save(cust, "disabled", 1); code != 200 || data(m)["status"] != statusDisabled {
		t.Fatalf("disable: %d %v", code, m)
	}

	if until, scope := access(); until == nil || !until.Equal(clock()) || scope != 2 {
		t.Errorf("membership after disabling: %v %d", until, scope)
	}

	if code, m := save(cust, "active", 2); code != 200 || data(m)["status"] != statusActive {
		t.Fatalf("activate again: %d %v", code, m)
	}

	if until, _ := access(); until != nil {
		t.Errorf("membership after activating: %v", until)
	}
	// the same address in capitals is no change: the stored spelling stays
	if code, res := write(
		srv,
		hq(),
		"clientUsers.save",
		`{"id":"`+user+`","customerId":"`+cust+`","email":"`+strings.ToUpper(
			email,
		)+`","clientRole":"member","reason":"admin change"}`,
		3,
	); code != 200 ||
		data(res)["email"] != email {
		t.Fatalf("same e-mail in capitals: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"clientUsers.remove",
		`{"id":"`+user+`","reason":"left the company"}`,
		3,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("remove on a stale version: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"clientUsers.remove",
		`{"id":"`+user+`","reason":"left the company"}`,
		4,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, res)
	}

	if until, _ := access(); until == nil || !until.Equal(clock()) {
		t.Errorf("membership after removal: %v", until)
	}
}

// TestClientUsersByOwner (IR417): an owner manages the customer's users like HQ: invites members and owners, changes a
// pending invite, a user's role and access with a reason, and removes users; never the own account, never an accepted
// user's e-mail, never another customer's user, and the customer keeps an active owner. A member is refused.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestClientUsersByOwner(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	email := "owner-" + uuid.NewString()[:8] + "@example.com"
	save := func(body string, v int) (int, map[string]any) {
		return write(srv, customerB(), "clientUsers.save", body, v)
	}

	code, resp := save(
		`{"customerId":"`+cust+`","email":"`+email+`","clientRole":"owner","displayName":"New Owner"}`,
		0,
	)
	if code != 200 || data(resp)["clientRole"] != clientRoleOwner ||
		data(resp)["status"] != statusInvited {
		t.Fatalf("owner invites an owner: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])
	// the account exists from the start (IR461): its e-mail is fixed, its role can change
	if code, m := save(
		`{"id":"`+id+`","customerId":"`+cust+`","email":"fixed-`+email+`","clientRole":"member","reason":"typo"}`,
		1,
	); code != 422 ||
		fields(m) != refusedEmailImmutable {
		t.Errorf("change the e-mail: %d %s", code, fields(m))
	}

	email2 := email
	if code, m := save(
		`{"id":"`+id+`","customerId":"`+cust+`","email":"`+email2+`","clientRole":"member","reason":"not an owner"}`,
		1,
	); code != 200 ||
		data(m)["clientRole"] != clientRoleMember {
		t.Fatalf("change the role: %d %v", code, m)
	}

	if code, m := save(
		`{"id":"`+id+`","customerId":"`+cust+`","email":"`+email2+`","clientRole":"member","status":"active","reason":"x"}`,
		2,
	); code != 409 ||
		m["messageKey"] != "errors.invite_pending" {
		t.Errorf("activate an invite: %d %v", code, m)
	}
	// the owner's own row
	_, resp = post(srv, customerB(), "clientUsers.list", `{"limit":100}`)

	var self map[string]any

	for _, it := range items(t, resp) {
		if it["membershipId"] == seed.ID("customer-b").String() {
			self = it
		}
	}

	sv := int(asNum(t, self["version"]))
	if code, res := save(
		`{"id":"`+asStr(
			t,
			self["id"],
		)+`","customerId":"`+cust+`","email":"`+asStr(
			t,
			self["email"],
		)+`","clientRole":"owner",`+
			`"status":"disabled","reason":"x"}`,
		sv,
	); code != 409 ||
		res["messageKey"] != keyOwnAccount {
		t.Errorf("disable the own account: %d %v", code, res)
	}

	if code, res := write(
		srv,
		customerB(),
		"clientUsers.remove",
		`{"id":"`+asStr(t, self["id"])+`","reason":"x"}`,
		sv,
	); code != 409 ||
		res["messageKey"] != keyOwnAccount {
		t.Errorf("remove the own account: %d %v", code, res)
	}
	// an accepted member: role and access change, the e-mail stays
	membership, user := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from)
		SELECT $1, tenant_id, user_id, organization_id, 'client', 'member', valid_from FROM identity.memberships WHERE id =
		$2`,
		membership,
		seed.ID("customer-b"),
	)

	memberEmail := "member-" + uuid.NewString()[:8] + "@example.com"
	owner(
		t,
		`INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, client_role, status, invited_at,
			invited_by_membership_id)
		VALUES ($1,$2,$3,$4,$5,'member','active',$6,$7)`,
		user,
		seed.ID("tenant-a"),
		cust,
		membership,
		memberEmail,
		clock().Add(-time.Hour),
		seed.ID("customer-b"),
	)

	if code, m := save(
		`{"id":"`+user+`","customerId":"`+cust+`","email":"x@example.com","clientRole":"member","reason":"x"}`,
		1,
	); code != 422 ||
		fields(m) != refusedEmailImmutable {
		t.Errorf("change an accepted e-mail: %d %s", code, fields(m))
	}

	if code, res := save(
		`{"id":"`+user+`","customerId":"`+cust+`","email":"`+memberEmail+`","clientRole":"owner","status":"disabled",`+
			`"reason":"on leave"}`,
		1,
	); code != 200 || data(res)["clientRole"] != clientRoleOwner ||
		data(res)["status"] != statusDisabled {
		t.Fatalf("promote and disable: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		customerA(),
		"clientUsers.remove",
		`{"id":"`+user+`","reason":"x"}`,
		2,
	); code != 404 {
		t.Error("another customer's owner removes")
	}

	if code, res := write(
		srv,
		customerB(),
		"clientUsers.remove",
		`{"id":"`+user+`","reason":"left"}`,
		2,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		customerB(),
		"clientUsers.remove",
		`{"id":"`+id+`","reason":"withdrawn"}`,
		2,
	); code != 200 {
		t.Error("remove the invite")
	}
}

// TestClientAccountCreated (IR417, IR461): an owner adds an owner: the person and an owner client membership of the
// customer are created at once, with the client permissions and the customer's scope, and the user is listed as
// invited.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestClientAccountCreated(t *testing.T) {
	srv := server(t)
	cust := seed.ID("cust-b").String()
	email := "added-owner-" + uuid.NewString()[:8] + "@example.com"

	code, res := write(
		srv,
		customerB(),
		"clientUsers.save",
		`{"customerId":"`+cust+`","email":"`+email+`","clientRole":"owner","displayName":"Added Owner"}`,
		0,
	)
	if code != 200 || data(res)["status"] != statusInvited || data(res)["membershipId"] == nil {
		t.Fatalf("add: %d %v", code, res)
	}

	membership := asStr(t, data(res)["membershipId"])

	t.Cleanup(func() { // later tests count customer B's owners: this one ends before them
		owner(
			t,
			`UPDATE identity.memberships SET valid_from = $2, valid_until = $3 WHERE id = $1`,
			membership,
			clock().Add(-time.Hour),
			clock().Add(-30*time.Minute),
		)
	})

	var (
		role, clientRole, org string
		perms, scopes         int
	)
	ownerScan(
		t,
		`SELECT m.role, m.client_role, m.organization_id::text, (SELECT count(*) FROM identity.membership_permissions p WHERE
			p.membership_id = m.id),
		(SELECT count(*) FROM identity.membership_scopes s WHERE s.membership_id = m.id AND s.kind = 'organization' AND
		s.ref_id = m.organization_id) FROM identity.memberships m WHERE m.id = $1`,
		[]any{membership},
		&role,
		&clientRole,
		&org,
		&perms,
		&scopes,
	)

	if role != roleClient || clientRole != clientRoleOwner ||
		org != seed.ID("org-customer-b").String() ||
		perms != 1 ||
		scopes != 1 {
		t.Errorf("membership: %s %s %s perms %d scopes %d", role, clientRole, org, perms, scopes)
	}

	if code, resp := write(
		srv,
		customerB(),
		"clientUsers.save",
		`{"customerId":"`+cust+`","email":"`+strings.ToUpper(
			email,
		)+`","clientRole":"member","displayName":"Again"}`,
		0,
	); code != 422 ||
		fields(resp) != refusedEmailTaken {
		t.Errorf("the same e-mail again: %d %s", code, fields(resp))
	}
}

// TestDisabledClientStaysClosed (IR420): HQ changing the membership of a client user that is disabled keeps the
// membership closed.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDisabledClientStaysClosed(t *testing.T) {
	srv := server(t)
	user := newUser(t)
	cust := seed.ID("cust-b").String()
	from := clock().Add(-time.Hour).Format(time.RFC3339)

	code, res := write(
		srv,
		hq(),
		"members.save",
		`{"userId":"`+user+`","organizationId":"`+seed.ID("org-customer-b").
			String()+
			`","role":"client","permissions":[],"scopes":[],"validFrom":"`+from+`","validUntil":null,"reason":"onboarding"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, res)
	}

	membership := asStr(t, data(res)["id"])

	var (
		id string
		v  int
	)
	ownerScan(
		t,
		`SELECT id::text, version FROM identity.client_users WHERE membership_id = $1`,
		[]any{membership},
		&id,
		&v,
	)

	if code, resp := write(
		srv,
		customerB(),
		"clientUsers.save",
		`{"id":"`+id+`","customerId":"`+cust+`","email":"`+user+`@ac.local","clientRole":"member","status":"disabled",`+
			`"reason":"left"}`,
		v,
	); code != 200 {
		t.Fatalf("disable: %d %v", code, resp)
	}

	var mv int
	ownerScan(t, `SELECT version FROM identity.memberships WHERE id = $1`, []any{membership}, &mv)

	later := clock().Add(90 * 24 * time.Hour).Format(time.RFC3339)
	if code, resp := write(
		srv,
		hq(),
		"members.save",
		`{"id":"`+membership+`","userId":"`+user+`","organizationId":"`+seed.ID("org-customer-b").
			String()+
			`","role":"client","permissions":[],"scopes":[],`+
			`"validFrom":"`+from+`","validUntil":"`+later+`","reason":"contract"}`,
		mv,
	); code != 200 {
		t.Fatalf("HQ change: %d %v", code, resp)
	}

	var end time.Time
	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&end,
	)

	if !end.Equal(clock()) {
		t.Errorf(
			"disabled client user's membership after HQ's change: %v, want still closed at %v",
			end,
			clock(),
		)
	}
}
