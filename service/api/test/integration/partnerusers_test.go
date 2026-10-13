package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/notify"
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
	// resend: not sooner than 5 minutes after the last invitation e-mail (IR423), then queued again (IR418), pending
	// invites of the own company
	if code, m := write(s, &contrA, "partnerUsers.resendInvite", `{"id":"`+id+`"}`, 0); code != 429 || m["messageKey"] != "errors.invite_recently_sent" || m["retryAfterSeconds"] != float64(300) {
		t.Errorf("resend right after the invite: %d %v", code, m)
	}
	owner(t, `UPDATE identity.partner_users SET last_invite_sent_at = last_invite_sent_at - interval '5 minutes' WHERE id = $1`, id)
	if code, m := write(s, &contrA, "partnerUsers.resendInvite", `{"id":"`+id+`"}`, 0); code != 200 || data(m)["deliveryState"] != "queued" || data(m)["target"].(map[string]any)["kind"] != "partner_user" {
		t.Fatalf("resend: %d %v", code, m)
	}
	if code, _ := write(s, &contrA, "partnerUsers.resendInvite", `{"id":"`+tech["id"].(string)+`"}`, 0); code != 409 {
		t.Error("resend to an active account")
	}
	if code, _ := write(s, &contrB, "partnerUsers.resendInvite", `{"id":"`+id+`"}`, 0); code != 404 {
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
	// the seed technician holds an active assignment: closing that account is refused (IR421)
	var booked string
	var bv int
	ownerScan(t, `SELECT id::text, version FROM identity.partner_users WHERE membership_id = $1`, []any{seed.ID("tech-external-a")}, &booked, &bv)
	if code, m := write(s, &contrA, "partnerUsers.save", `{"id":"`+booked+`","email":"tech-external-a@ac.local","partnerRole":"technician","status":"disabled","reason":"on leave"}`, bv); code != 409 || m["messageKey"] != "errors.technician_booked" {
		t.Errorf("disable a booked technician: %d %v", code, m)
	}
	if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+booked+`","reason":"left"}`, bv); code != 409 || m["messageKey"] != "errors.technician_booked" {
		t.Errorf("remove a booked technician: %d %v", code, m)
	}
	// a technician without assignments
	user := newUser(t)
	code, m := write(s, &hq, "members.save", `{"userId":"`+user+`","organizationId":"`+seed.ID("org-contractor-a").String()+`","role":"technician","employment":"external",
		"permissions":[],"scopes":[],"validFrom":"`+clock.Add(-time.Hour).Format(time.RFC3339)+`","validUntil":"`+clock.Add(30*24*time.Hour).Format(time.RFC3339)+`","reason":"contract"}`, 0)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, m)
	}
	membership := uuid.MustParse(data(m)["id"].(string))
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
	code, m = save(email, "technician", "disabled", v)
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

// TestPartnerUserInputs: the inputs of partnerUsers.remove and resendInvite name their field, an invite takes no reason,
// a removal needs the current version, and removing an account ends its membership now.
func TestPartnerUserInputs(t *testing.T) {
	s := server(t)
	for name, c := range map[string]struct {
		op, body, want string
	}{
		"remove without id":   {"partnerUsers.remove", `{"id":"00000000-0000-0000-0000-000000000000","reason":"x"}`, "id=error.required"},
		"remove long reason":  {"partnerUsers.remove", `{"id":"` + uuid.NewString() + `","reason":"` + strings.Repeat("x", 1001) + `"}`, "reason=error.length"},
		"resend without id":   {"partnerUsers.resendInvite", `{"id":"00000000-0000-0000-0000-000000000000"}`, "id=error.required"},
		"reason on an invite": {"partnerUsers.save", `{"email":"x-` + uuid.NewString()[:8] + `@example.com","partnerRole":"technician","reason":"why"}`, "reason=error.notAllowed"},
	} {
		code, m := write(s, &contrA, c.op, c.body, 1)
		if c.op == "partnerUsers.resendInvite" || c.op == "partnerUsers.save" {
			code, m = write(s, &contrA, c.op, c.body, 0)
		}
		refusal(t, name, code, m, c.want)
	}
	// an account of the company (a membership of its own), removed: the membership ends now
	user := newUser(t)
	membership, id := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until)
		VALUES ($1, $2, $3, $4, 'technician', 'external', $5, $6)`, membership, seed.ID("tenant-a"), user, seed.ID("org-contractor-a"), clock.Add(-time.Hour), clock.Add(30*24*time.Hour))
	owner(t, `INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, partner_role, status, invited_at, invited_by_membership_id)
		VALUES ($1, $2, $3, $4, $5, 'technician', 'active', $6, $7)`, id, seed.ID("tenant-a"), seed.ID("org-contractor-a"), membership, user+"@ac.local", clock, seed.ID("contractor-a"))
	if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+id+`","reason":"left the company"}`, 2); code != 409 || m["messageKey"] != "error.versionConflict" {
		t.Errorf("remove on a stale version: %d %v", code, m)
	}
	if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+id+`","reason":"left the company"}`, 1); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, m)
	}
	var until time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &until)
	if !until.Equal(clock) {
		t.Errorf("membership after removal: %v, want %v", until, clock)
	}
}

// TestInviteMail (IR418): an invite and a resend queue an invitation e-mail to the invited address, and the
// notification worker's Deliver sends the queued mail and records it as sent.
func TestInviteMail(t *testing.T) {
	s := server(t)
	email := "mail-" + uuid.NewString()[:8] + "@example.com"
	code, m := write(s, &contrA, "partnerUsers.save", `{"email":"`+email+`","partnerRole":"technician"}`, 0)
	if code != 200 {
		t.Fatalf("invite: %d %v", code, m)
	}
	owner(t, `UPDATE identity.partner_users SET last_invite_sent_at = last_invite_sent_at - interval '5 minutes' WHERE id = $1`, data(m)["id"])
	if code, _ := write(s, &contrA, "partnerUsers.resendInvite", `{"id":"`+data(m)["id"].(string)+`"}`, 0); code != 200 {
		t.Fatal("resend")
	}
	var queued int
	var params string
	ownerScan(t, `SELECT count(*), max(params::text) FROM notify.outbound_mail WHERE to_email = $1 AND state = 'queued'`, []any{email}, &queued, &params)
	if queued != 2 || !strings.Contains(params, `"app": "technician"`) || !strings.Contains(params, `"role": "technician"`) {
		t.Fatalf("queued mail: %d %s", queued, params)
	}
	// the worker's delivery: the queued mail goes to the sender and is recorded as sent; a failing sender leaves it for
	// a later retry with the reason
	failing := &fakeMail{err: errors.New("smtp down")}
	if _, err := notify.DeliverAll(context.Background(), s.DB, failing, notify.Apps{Technician: "http://tech"}, clock, 500); err != nil {
		t.Fatal(err)
	}
	var state, reason string
	ownerScan(t, `SELECT state, COALESCE(failure_reason, '') FROM notify.outbound_mail WHERE to_email = $1 ORDER BY created_at LIMIT 1`, []any{email}, &state, &reason)
	if state != "failed" || reason != "smtp down" {
		t.Errorf("after a failing send: %s %s", state, reason)
	}
	// the retry waits a minute after the failed attempt (IR425): nothing goes out at once
	sender := &fakeMail{}
	if _, err := notify.DeliverAll(context.Background(), s.DB, sender, notify.Apps{Technician: "http://tech"}, clock.Add(59*time.Second), 500); err != nil {
		t.Fatal(err)
	}
	for _, m := range sender.sent {
		if m.to == email {
			t.Fatalf("retried before the pause: %+v", m)
		}
	}
	if _, err := notify.DeliverAll(context.Background(), s.DB, sender, notify.Apps{Technician: "http://tech"}, clock.Add(time.Minute), 500); err != nil {
		t.Fatal(err)
	}
	mine := 0
	for _, m := range sender.sent {
		if m.to == email {
			mine++
			if !strings.Contains(m.subject, "Demo Contractor A") || !strings.Contains(m.body, "http://tech/login") {
				t.Errorf("invite mail: %+v", m)
			}
		}
	}
	ownerScan(t, `SELECT count(*) FROM notify.outbound_mail WHERE to_email = $1 AND state = 'sent' AND provider_message_id IS NOT NULL`, []any{email}, &queued)
	if mine != 2 || queued != 2 {
		t.Errorf("sent %d, recorded %d", mine, queued)
	}
}

type sentMail struct{ to, subject, body string }

// fakeMail records what the worker sends, or fails every send with err.
type fakeMail struct {
	err  error
	sent []sentMail
}

func (f *fakeMail) Send(_ context.Context, to, subject, body string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.sent = append(f.sent, sentMail{to, subject, body})
	return uuid.NewString(), nil
}

// TestNotificationMail (IR418): an e-mail notification goes to the recipient's address; one whose recipient's
// membership has ended is recorded as suppressed and never sent.
func TestNotificationMail(t *testing.T) {
	s := server(t)
	note := func(membership string) string {
		id := uuid.NewString()
		owner(t, `INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, channel, template_key, target, params, severity, occurred_at)
			VALUES ($1, $2, $3, 1, 'fault', 'email', 'alert', '{"kind":"unit","id":"`+seed.ID("unit-online-rto").String()+`"}', '{"targetName":"Mail AC","status":"open"}', 'warning', $4)`,
			id, seed.ID("tenant-a"), membership, clock)
		return id
	}
	member := func(until time.Time) string {
		id := uuid.NewString()
		owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from, valid_until)
			VALUES ($1, $2, $3, $4, 'client', 'member', $5, $6)`, id, seed.ID("tenant-a"), newUser(t), seed.ID("org-customer-a"), clock.Add(-time.Hour), until)
		return id
	}
	active, ended := note(member(clock.Add(time.Hour))), note(member(clock.Add(-time.Minute)))
	sender := &fakeMail{}
	if _, err := notify.DeliverAll(context.Background(), s.DB, sender, notify.Apps{Customer: "http://c"}, clock, 500); err != nil {
		t.Fatal(err)
	}
	state := func(id string) (st, reason string) {
		ownerScan(t, `SELECT state, COALESCE(failure_reason, '') FROM notify.deliveries WHERE notification_id = $1 AND channel = 'email'`, []any{id}, &st, &reason)
		return
	}
	if st, _ := state(active); st != "sent" {
		t.Errorf("active recipient: %s", st)
	}
	if st, reason := state(ended); st != "suppressed" || reason != "membership ended" {
		t.Errorf("ended recipient: %s %s", st, reason)
	}
	mail := 0
	for _, m := range sender.sent {
		if m.subject == "Alert: Mail AC" {
			mail++
		}
	}
	if mail != 1 {
		t.Errorf("sent %d alert e-mails, want 1", mail)
	}
}

// TestDisabledAccountStaysClosed (IR420): HQ extending the membership of an account its contractor disabled does not
// reopen it; the new end is kept and comes back when the contractor enables the account.
func TestDisabledAccountStaysClosed(t *testing.T) {
	s := server(t)
	user := newUser(t)
	from, until, later := clock.Add(-time.Hour).Format(time.RFC3339), clock.Add(30*24*time.Hour), clock.Add(60*24*time.Hour)
	body := func(id string, end time.Time) string {
		idPart := ""
		if id != "" {
			idPart = `"id":"` + id + `",`
		}
		return `{` + idPart + `"userId":"` + user + `","organizationId":"` + seed.ID("org-contractor-a").String() + `","role":"technician","employment":"external",
			"permissions":[],"scopes":[],"validFrom":"` + from + `","validUntil":"` + end.Format(time.RFC3339) + `","reason":"contract"}`
	}
	code, m := write(s, &hq, "members.save", body("", until), 0)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, m)
	}
	membership := data(m)["id"].(string)
	var account string
	var v int
	ownerScan(t, `SELECT id::text, version FROM identity.partner_users WHERE membership_id = $1`, []any{membership}, &account, &v)
	status := func(st string, v int) {
		t.Helper()
		if code, m := write(s, &contrA, "partnerUsers.save", `{"id":"`+account+`","email":"`+user+`@ac.local","partnerRole":"technician","status":"`+st+`","reason":"leave"}`, v); code != 200 {
			t.Fatalf("%s: %d %v", st, code, m)
		}
	}
	status("disabled", v)
	var mv int
	ownerScan(t, `SELECT version FROM identity.memberships WHERE id = $1`, []any{membership}, &mv)
	if code, m := write(s, &hq, "members.save", body(membership, later), mv); code != 200 { // HQ extends the contract
		t.Fatalf("extend: %d %v", code, m)
	}
	var end time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &end)
	if !end.Equal(clock) {
		t.Errorf("membership of a disabled account after HQ's change: %v, want still closed at %v", end, clock)
	}
	ownerScan(t, `SELECT version FROM identity.partner_users WHERE id = $1`, []any{account}, &v)
	status("active", v)
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{membership}, &end)
	if !end.Equal(later.Truncate(time.Second)) {
		t.Errorf("membership after enabling: %v, want HQ's end %v", end, later)
	}
}
