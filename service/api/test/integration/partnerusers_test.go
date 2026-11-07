package integration_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestPartnerUsers (FR-P11, IR461): a contractor lists its company's accounts, adds contractors and technicians (the
// account is created at once and listed as invited until its first sign-in), resends the set-up e-mail and removes the
// account; another company's accounts are NOT_FOUND, and other roles are refused.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerUsers(t *testing.T) {
	srv := server(t)
	email := "tech-" + uuid.NewString()[:8] + "@example.com"
	save := func(a *actor, body string, v int) (int, map[string]any) {
		return write(srv, a, "partnerUsers.save", body, v)
	}
	access := `,"displayName":"New Tech","accessUntil":"` + clock().Add(30*24*time.Hour).
		Format(time.RFC3339) +
		`"`
	invite := `{"email":"` + email + `","partnerRole":"technician"` + access + `}`
	want := map[string]string{ // the field each case is refused for
		caseBadEmail:    "email=error.invalid",
		caseBadRole:     "partnerRole=error.invalid",
		caseBadStatus:   "status=error.invalid",
		caseBlankReason: refusedReasonLength,
	}

	for name, body := range map[string]string{
		caseBadEmail:    `{"email":"not-an-email","partnerRole":"technician"}`,
		caseBadRole:     `{"email":"` + email + `","partnerRole":"admin"}`,
		caseBadStatus:   `{"email":"` + email + `","partnerRole":"technician","status":"invited"}`,
		caseBlankReason: `{"email":"` + email + `","partnerRole":"technician","reason":" "}`,
	} {
		code, m := save(contrA(), body, 0)
		refusal(t, name, code, m, want[name])
	}

	if code, res := save(
		contrA(),
		`{"email":"`+email+`","partnerRole":"technician","status":"active"`+access+`}`,
		0,
	); code != 422 ||
		fields(res) != refusedStatusNotAllowed {
		t.Errorf("status on invite: %d %s", code, fields(res))
	}

	for _, who := range []*actor{techA(), customerA(), hq()} {
		if code, _ := post(srv, who, "partnerUsers.list", `{"limit":10}`); code != 403 {
			t.Errorf("%s lists contractor users: %d", who.membership, code)
		}

		if code, _ := save(who, invite, 0); code != 403 {
			t.Errorf("%s invites: %d", who.membership, code)
		}
	}

	code, resp := save(contrA(), invite, 0)
	if code != 200 || data(resp)["status"] != statusInvited ||
		data(resp)["partnerRole"] != roleTechnician ||
		data(resp)["membershipId"] == nil ||
		data(resp)["organizationId"] != seed.ID("org-contractor-a").String() ||
		data(resp)["invitedByMembershipId"] != seed.ID("contractor-a").String() {
		t.Fatalf("invite: %d %v", code, resp)
	}

	accountID := asStr(t, data(resp)["id"])

	if code, res := save(
		contrA(),
		strings.Replace(invite, email, strings.ToUpper(email[:1])+email[1:], 1),
		0,
	); code != 422 ||
		fields(res) != refusedEmailTaken {
		t.Errorf("duplicate email (case-insensitive): %d %s", code, fields(res))
	}

	if code, res := save(
		contrB(),
		invite,
		0,
	); code != 422 ||
		fields(res) != refusedEmailTaken { // one person, one account (IR461)
		t.Errorf("same email at company B: %d %s", code, fields(res))
	}

	_, resp = post(srv, contrA(), "partnerUsers.list", `{"limit":100}`)
	byMembership := map[string]map[string]any{}

	for _, item := range items(t, resp) {
		if item["organizationId"] != seed.ID("org-contractor-a").String() {
			t.Fatalf("another company's account listed: %v", item)
		}

		if mid, ok := item["membershipId"].(string); ok {
			byMembership[mid] = item
		}
	}

	self, tech := byMembership[seed.ID("contractor-a").String()], byMembership[seed.ID("tech-external-a").String()]
	if self == nil || self["partnerRole"] != roleContractor || self["status"] != statusActive ||
		tech == nil ||
		tech["partnerRole"] != roleTechnician ||
		tech["accessUntil"] == nil {
		t.Fatalf("seed accounts listed: %v", resp)
	}
	// the seed technician holds an assigned job: the list marks it (IR432); the contractor is not a technician
	if tech["booked"] != true || self["booked"] != false {
		t.Errorf("booked: technician %v, contractor %v", tech["booked"], self["booked"])
	}

	if _, res := post(
		srv,
		contrA(),
		"partnerUsers.list",
		`{"filters":{"status":"invited","partnerRole":"technician","search":"`+email[:12]+`"},"limit":100}`,
	); len(
		items(t, res),
	) != 1 {
		t.Errorf("filtered list: %v", res)
	}

	if code, res := post(
		srv,
		contrA(),
		"partnerUsers.list",
		`{"filters":{"partnerRole":"owner"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad role filter: %d %s", code, fields(res))
	}
	// resend: not sooner than 5 minutes after the last invitation e-mail (IR423), then queued again (IR418), pending
	// invites of the own company
	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.resendInvite",
		`{"id":"`+accountID+`"}`,
		0,
	); code != 429 || res["messageKey"] != "errors.invite_recently_sent" ||
		res["retryAfterSeconds"] != float64(300) {
		t.Errorf("resend right after the invite: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE identity.partner_users SET last_invite_sent_at = last_invite_sent_at - interval '5 minutes' WHERE id = $1`,
		accountID,
	)

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.resendInvite",
		`{"id":"`+accountID+`"}`,
		0,
	); code != 200 || data(res)["deliveryState"] != statusQueued ||
		asObj(t, data(res)["target"])["kind"] != "partner_user" {
		t.Fatalf("resend: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		contrA(),
		"partnerUsers.resendInvite",
		`{"id":"`+asStr(t, tech["id"])+`"}`,
		0,
	); code != 409 {
		t.Error("resend to an active account")
	}

	if code, _ := write(
		srv,
		contrB(),
		"partnerUsers.resendInvite",
		`{"id":"`+accountID+`"}`,
		0,
	); code != 404 {
		t.Error("resend another company's invite")
	}
	// an account not signed in yet: its e-mail and role are fixed (IR461), its status is not set by hand
	upd := func(partnerUserID, email, role, status string, version int) (int, map[string]any) {
		state := ""
		if status != "" {
			state = `,"status":"` + status + `"`
		}

		return save(
			contrA(),
			`{"id":"`+partnerUserID+`","email":"`+email+`","partnerRole":"`+role+`"`+state+`,"reason":"correction"}`,
			version,
		)
	}
	if code, res := save(
		contrA(),
		`{"id":"`+accountID+`","email":"`+email+`","partnerRole":"contractor"}`,
		1,
	); code != 422 ||
		fields(res) != refusedReasonRequired {
		t.Errorf("update without reason: %d %s", code, fields(res))
	}

	if code, res := upd(
		accountID,
		email,
		"technician",
		"active",
		1,
	); code != 409 ||
		res["messageKey"] != "errors.invite_pending" {
		t.Errorf("activate a pending invite: %d %v", code, res)
	}

	if code, _ := upd(accountID, email, "contractor", "", 5); code != 409 {
		t.Error("stale version")
	}

	if code, res := upd(
		accountID,
		"fixed-"+email,
		"technician",
		"",
		1,
	); code != 422 ||
		fields(res) != refusedEmailImmutable {
		t.Errorf("change the e-mail: %d %s", code, fields(res))
	}

	if code, res := upd(
		accountID,
		email,
		"contractor",
		"",
		1,
	); code != 422 ||
		fields(res) != "partnerRole=error.immutable" {
		t.Errorf("change the role: %d %s", code, fields(res))
	}

	if code, res := upd(
		accountID,
		email,
		"technician",
		"",
		1,
	); code != 200 || data(res)["status"] != statusInvited ||
		ver(t, res) != 2 {
		t.Fatalf("save unchanged: %d %v", code, res)
	}

	if code, _ := save(
		contrB(),
		`{"id":"`+accountID+`","email":"`+email+`","partnerRole":"technician","reason":"x"}`,
		2,
	); code != 404 {
		t.Error("company B changes company A's invite")
	}
	// the caller's own account is not changed here
	selfVersion := int(asNum(t, self["version"]))
	if code, res := upd(
		asStr(t, self["id"]),
		asStr(t, self["email"]),
		"contractor",
		"disabled",
		selfVersion,
	); code != 409 ||
		res["messageKey"] != keyOwnAccount {
		t.Errorf("disable the own account: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+asStr(t, self["id"])+`","reason":"leaving"}`,
		selfVersion,
	); code != 409 ||
		res["messageKey"] != keyOwnAccount {
		t.Errorf("remove the own account: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		contrB(),
		"partnerUsers.remove",
		`{"id":"`+accountID+`","reason":"x"}`,
		2,
	); code != 404 {
		t.Error("company B removes company A's invite")
	}

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+accountID+`","reason":"sent to the wrong person"}`,
		2,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+accountID+`","reason":"again"}`,
		2,
	); code != 404 {
		t.Error("remove twice")
	}
}

// TestPartnerUserAccess: disabling a technician's account ends its membership now and keeps its access end; enabling
// it again brings that end back; an accepted account's email and role are fixed; the company keeps an active
// contractor.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerUserAccess(t *testing.T) {
	srv := server(t)
	// the seed technician holds an active assignment: closing that account is refused (IR421)
	var (
		booked        string
		bookedVersion int
	)
	ownerScan(
		t,
		`SELECT id::text, version FROM identity.partner_users WHERE membership_id = $1`,
		[]any{seed.ID("tech-external-a")},
		&booked,
		&bookedVersion,
	)

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.save",
		`{"id":"`+booked+`","email":"tech-external-a@ac.local","partnerRole":"technician","status":"disabled","reason":"on `+
			`leave"}`,
		bookedVersion,
	); code != 409 ||
		res["messageKey"] != "errors.technician_booked" {
		t.Errorf("disable a booked technician: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+booked+`","reason":"left"}`,
		bookedVersion,
	); code != 409 ||
		res["messageKey"] != "errors.technician_booked" {
		t.Errorf("remove a booked technician: %d %v", code, res)
	}
	// a technician without assignments
	user := newUser(t)

	code, resp := write(
		srv,
		hq(),
		"members.save",
		`{"userId":"`+user+`","organizationId":"`+seed.ID("org-contractor-a").
			String()+
			`","role":"technician","employment":"external",
		"permissions":[],"scopes":[],"validFrom":"`+clock().Add(-time.Hour).
			Format(time.RFC3339)+
			`","validUntil":"`+clock().Add(30*24*time.Hour).
			Format(time.RFC3339)+
			`","reason":"contract"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, resp)
	}

	membership := uuid.MustParse(asStr(t, data(resp)["id"]))
	firstSignIn(
		t,
		srv,
		membership.String(),
	) // HQ's first membership of the person is theirs once they sign in (IR462)

	var until time.Time
	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&until,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
			membership,
			until,
		)
		owner(
			t,
			`UPDATE identity.partner_users SET status = 'active', access_until = NULL WHERE membership_id = $1`,
			membership,
		)
	})

	var (
		partnerUserID, email string
		userVersion          int
	)
	ownerScan(
		t,
		`SELECT id::text, email::text, version FROM identity.partner_users WHERE membership_id = $1`,
		[]any{membership},
		&partnerUserID,
		&email,
		&userVersion,
	)

	save := func(email, role, status string, version int) (int, map[string]any) {
		return write(
			srv,
			contrA(),
			"partnerUsers.save",
			`{"id":"`+partnerUserID+`","email":"`+email+`","partnerRole":"`+role+`","status":"`+status+`","reason":"on leave"}`,
			version,
		)
	}
	if code, res := save(
		"other@example.com",
		"technician",
		"active",
		userVersion,
	); code != 422 ||
		fields(res) != refusedEmailImmutable {
		t.Errorf("change an accepted email: %d %s", code, fields(res))
	}

	if code, res := save(
		email,
		"contractor",
		"active",
		userVersion,
	); code != 422 ||
		fields(res) != "partnerRole=error.immutable" {
		t.Errorf("change an accepted role: %d %s", code, fields(res))
	}
	// the same address in capitals is no change: the stored spelling stays
	code, resp = save(strings.ToUpper(email), "technician", "disabled", userVersion)
	if code != 200 || data(resp)["status"] != statusDisabled || data(resp)["email"] != email ||
		data(resp)["accessUntil"] != until.UTC().Format(time.RFC3339) {
		t.Fatalf("disable: %d %v", code, resp)
	}

	var now time.Time
	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&now,
	)

	if !now.Equal(clock()) {
		t.Errorf("membership after disabling: %v, want %v", now, clock())
	}

	code, resp = save(email, "technician", "active", userVersion+1)
	if code != 200 || data(resp)["status"] != statusActive {
		t.Fatalf("enable: %d %v", code, resp)
	}

	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&now,
	)

	if !now.Equal(until) {
		t.Errorf("membership after enabling: %v, want %v", now, until)
	}
	// an access end that has passed while disabled is not brought back
	if code, _ := save(email, "technician", "disabled", userVersion+2); code != 200 {
		t.Fatal("disable again")
	}

	owner(
		t,
		`UPDATE identity.partner_users SET access_until = $2 WHERE id = $1`,
		partnerUserID,
		clock().Add(-time.Minute),
	)

	if code, res := save(
		email,
		"technician",
		"active",
		userVersion+3,
	); code != 409 ||
		res["messageKey"] != "errors.access_expired" {
		t.Errorf("enable after the access end: %d %v", code, res)
	}
	// the last active contractor: another contractor account, with the company's other active contractor accounts
	// (the caller's own, and any another test added) switched off for the test
	other := uuid.NewString()

	var active []string
	ownerScan(
		t,
		`SELECT COALESCE(array_agg(id::text), '{}') FROM identity.partner_users WHERE organization_id = $1 AND partner_role =
			'contractor' AND status = 'active'`,
		[]any{seed.ID("org-contractor-a")},
		&active,
	)
	owner(
		t,
		`INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, partner_role, status,
			invited_at, invited_by_membership_id)
		VALUES ($1, $2, $3, $4, $5, 'contractor', 'active', $6, $7)`,
		other,
		seed.ID("tenant-a"),
		seed.ID("org-contractor-a"),
		accountMembership(
			t,
			seed.ID("org-contractor-a"),
			"contractor",
			nil,
		),
		"boss-"+other[:8]+"@example.com",
		clock(),
		seed.ID("contractor-a"),
	)
	owner(
		t,
		`UPDATE identity.partner_users SET status = 'disabled' WHERE id = ANY($1::uuid[])`,
		active,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.partner_users SET status = 'active' WHERE id = ANY($1::uuid[])`,
			active,
		)
		owner(t, `DELETE FROM identity.partner_users WHERE id = $1`, other)
	})

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+other+`","reason":"leaving"}`,
		1,
	); code != 409 ||
		res["messageKey"] != "errors.last_contractor" {
		t.Errorf("remove the last active contractor: %d %v", code, res)
	}
}

// TestPartnerAccountCreated (IR461): a contractor adds a technician: the person, an external technician membership with
// the technicians' permissions, no unit scope and the contractor's access end are created; the identity consumer
// opens the provider account and sends the set-up e-mail; the first sign-in makes the account active; the access end
// moves within a year; removing the account turns its sign-in off.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerAccountCreated(t *testing.T) {
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
	srv := serverCfg(
		t,
		func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains },
	)
	email := "added-" + uuid.NewString()[:8] + "@example.com"

	body := func(until time.Time) string {
		return `{"email":"` + email + `","partnerRole":"technician","displayName":"Added Tech",` +
			`"accessUntil":"` + until.Format(
			time.RFC3339,
		) + `"}`
	}
	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.save",
		body(clock().Add(367*24*time.Hour)),
		0,
	); code != 422 ||
		fields(res) != "accessUntil=error.range" {
		t.Errorf("access beyond a year: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.save",
		`{"email":"`+email+`","partnerRole":"contractor","displayName":"Boss","accessUntil":"`+clock().Add(time.Hour).
			Format(time.RFC3339)+
			`"}`,
		0,
	); code != 422 ||
		fields(res) != "accessUntil=error.notAllowed" {
		t.Errorf("access end of a contractor account: %d %s", code, fields(res))
	}

	until := clock().Add(30 * 24 * time.Hour)

	code, resp := write(srv, contrA(), "partnerUsers.save", body(until), 0)
	if code != 200 || data(resp)["status"] != statusInvited || data(resp)["membershipId"] == nil ||
		data(resp)["displayName"] != "Added Tech" {
		t.Fatalf("add: %d %v", code, resp)
	}

	partnerUserID, membership := asStr(t, data(resp)["id"]), asStr(t, data(resp)["membershipId"])

	var (
		role, employment string
		end              time.Time
		perms, scopes    int
	)
	ownerScan(
		t,
		`SELECT m.role, m.employment, m.valid_until, (SELECT count(*) FROM identity.membership_permissions p WHERE
			p.membership_id = m.id),
		(SELECT count(*) FROM identity.membership_scopes s WHERE s.membership_id = m.id) FROM identity.memberships m WHERE
		m.id = $1`,
		[]any{membership},
		&role,
		&employment,
		&end,
		&perms,
		&scopes,
	)

	if role != roleTechnician || employment != "external" || !end.Equal(until) || perms != 4 ||
		scopes != 0 {
		t.Errorf("membership: %s %s %v perms %d scopes %d", role, employment, end, perms, scopes)
	}

	err := srv.DrainEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// other tests' accounts may be drained here too: look at this one only
	var created []idp.Account
	for _, a := range fake.created {
		if a.Email == email {
			created = append(created, a)
		}
	}

	if len(created) != 1 || created[0].Role != roleTechnician ||
		created[0].Membership != membership ||
		!strings.HasSuffix(created[0].RedirectURI, "/login") ||
		!slices.Contains(fake.setups, "sub-"+email) {
		t.Fatalf("provider: created %+v, set-up %v", created, fake.setups)
	}

	var sub, user string
	ownerScan(
		t,
		`SELECT u.cognito_sub, u.id::text FROM identity.users u JOIN identity.memberships m ON m.user_id = u.id WHERE m.id =
			$1`,
		[]any{membership},
		&sub,
		&user,
	)

	if sub != "sub-"+email {
		t.Errorf("subject stored: %q", sub)
	}
	// the first sign-in makes the account active
	(&auth.Authenticator{DB: srv.DB, Now: clock, Verifier: nil, Source: nil}).RecordSignIn(
		context.Background(),
		&ops.Principal{
			TenantID: seed.ID("tenant-a"),
			UserID:   uuid.MustParse(user),
			MembershipID: uuid.MustParse(
				membership,
			),
			Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
			OrgID:             uuid.UUID{},
			ScopeVersion:      0,
			Scopes:            nil,
			Employment:        "",
			Timezone:          "",
			SessionsRevokedAt: nil,
		},
		clock(),
	)

	var (
		status  string
		version int
	)
	ownerScan(
		t,
		`SELECT status, version FROM identity.partner_users WHERE id = $1`,
		[]any{partnerUserID},
		&status,
		&version,
	)

	if status != statusActive {
		t.Fatalf("after the first sign-in: %s", status)
	}
	// the access end moves (at most a year ahead)
	later := clock().Add(200 * 24 * time.Hour)
	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.save",
		`{"id":"`+partnerUserID+`","email":"`+email+`","partnerRole":"technician","accessUntil":"`+later.Format(
			time.RFC3339,
		)+`","reason":"contract extended"}`,
		version,
	); code != 200 {
		t.Fatalf("extend: %d %v", code, res)
	}

	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&end,
	)

	if !end.Equal(later) {
		t.Errorf("access after the extension: %v", end)
	}
	// removed: the membership ends and the sign-in is turned off
	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+partnerUserID+`","reason":"left the company"}`,
		version+1,
	); code != 200 {
		t.Fatalf("remove: %d %v", code, res)
	}

	err = srv.DrainEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(fake.switched, "sub-"+email+"=false") {
		t.Errorf("sign-in after the removal: %v", fake.switched)
	}
}

// firstSignIn is the person's first sign-in with the membership: a user HQ or the company added becomes active
// (IR461, IR462).
func firstSignIn(t *testing.T, srv *apiserver.Server, membership string) {
	t.Helper()

	var user string
	ownerScan(
		t,
		`SELECT user_id::text FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&user,
	)
	(&auth.Authenticator{DB: srv.DB, Now: clock, Verifier: nil, Source: nil}).RecordSignIn(
		context.Background(),
		&ops.Principal{
			TenantID: seed.ID("tenant-a"),
			UserID:   uuid.MustParse(user),
			MembershipID: uuid.MustParse(
				membership,
			),
			Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
			OrgID:             uuid.UUID{},
			ScopeVersion:      0,
			Scopes:            nil,
			Employment:        "",
			Timezone:          "",
			SessionsRevokedAt: nil,
		},
		clock(),
	)
}

// TestPartnerResendAfterAccessEnd: no set-up e-mail goes to a technician whose access ended before they signed in; once
// the contractor moves the end, it goes again (IR461).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerResendAfterAccessEnd(t *testing.T) {
	srv := server(t)
	email := "late-" + uuid.NewString()[:8] + "@example.com"

	code, resp := write(
		srv,
		contrA(),
		"partnerUsers.save",
		`{"email":"`+email+`","partnerRole":"technician","displayName":"Late Tech","accessUntil":"`+clock().Add(24*time.Hour).
			Format(time.RFC3339)+
			`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("add: %d %v", code, resp)
	}

	partnerUserID, membership := asStr(t, data(resp)["id"]), asStr(t, data(resp)["membershipId"])
	owner(
		t,
		`UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '2 days', valid_until = $2::timestamptz -
			interval '1 hour' WHERE id = $1`,
		membership,
		clock(),
	)
	owner(
		t,
		`UPDATE identity.partner_users SET last_invite_sent_at = NULL WHERE id = $1`,
		partnerUserID,
	) // past the 5-minute wait (IR423)

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.resendInvite",
		`{"id":"`+partnerUserID+`"}`,
		0,
	); code != 409 ||
		res["messageKey"] != "errors.access_expired" {
		t.Errorf("resend after the access end: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.save",
		`{"id":"`+partnerUserID+`","email":"`+email+`","partnerRole":"technician","accessUntil":"`+clock().Add(48*time.Hour).
			Format(time.RFC3339)+
			`","reason":"starts later"}`,
		1,
	); code != 200 {
		t.Fatalf("move the access end: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.resendInvite",
		`{"id":"`+partnerUserID+`"}`,
		0,
	); code != 200 {
		t.Errorf("resend after the move: %d %v", code, res)
	}
}

// TestPartnerUserInputs: the inputs of partnerUsers.remove and resendInvite name their field, an invite takes no
// reason, a removal needs the current version, and removing an account ends its membership now.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerUserInputs(t *testing.T) {
	srv := server(t)
	for name, testCase := range map[string]struct {
		op, body, want string
	}{
		"remove without id": {opPartnerUsersRemove, `{"id":"00000000-0000-0000-0000-000000000000",` +
			`"reason":"x"}`, "id=error.required"},
		"remove long reason": {opPartnerUsersRemove, `{"id":"` + uuid.NewString() + `","reason":"` +
			strings.Repeat("x", 1001) + `"}`, refusedReasonLength},
		"resend without id": {
			opPartnerUsersResendInvite, `{"id":"00000000-0000-0000-0000-000000000000"}`,
			"id=error.required",
		},
		"reason on an invite": {opPartnerUsersSave, `{"email":"x-` + uuid.NewString()[:8] + `@example.com",` +
			`"partnerRole":"contractor","displayName":"X","reason":"why"}`, "reason=error.notAllowed"},
	} {
		code, m := write(srv, contrA(), testCase.op, testCase.body, 1)
		if testCase.op == opPartnerUsersResendInvite || testCase.op == opPartnerUsersSave {
			code, m = write(srv, contrA(), testCase.op, testCase.body, 0)
		}

		refusal(t, name, code, m, testCase.want)
	}
	// an account of the company (a membership of its own), removed: the membership ends now
	user := newUser(t)
	membership, partnerUserID := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until)
		VALUES ($1, $2, $3, $4, 'technician', 'external', $5, $6)`,
		membership,
		seed.ID("tenant-a"),
		user,
		seed.ID("org-contractor-a"),
		clock().Add(-time.Hour),
		clock().Add(30*24*time.Hour),
	)
	owner(
		t,
		`INSERT INTO identity.partner_users (id, tenant_id, organization_id, membership_id, email, partner_role, status,
			invited_at, invited_by_membership_id)
		VALUES ($1, $2, $3, $4, $5, 'technician', 'active', $6, $7)`,
		partnerUserID,
		seed.ID("tenant-a"),
		seed.ID("org-contractor-a"),
		membership,
		user+"@ac.local",
		clock(),
		seed.ID("contractor-a"),
	)

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+partnerUserID+`","reason":"left the company"}`,
		2,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("remove on a stale version: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+partnerUserID+`","reason":"left the company"}`,
		1,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, res)
	}

	var until time.Time
	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&until,
	)

	if !until.Equal(clock()) {
		t.Errorf("membership after removal: %v, want %v", until, clock())
	}
}

// TestNotificationMailRetry (IR425): a failed e-mail notification keeps its reason and is retried a minute after the
// failed attempt, not before; then it is sent and recorded. (Until IR479 the invitation e-mails showed this.)
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestNotificationMailRetry(t *testing.T) {
	srv := server(t)
	membership, notificationID, name := uuid.NewString(), uuid.NewString(), "Retry AC "+uuid.NewString()[:8]
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from)
		VALUES ($1, $2, $3, $4, 'client', 'member', $5)`,
		membership,
		seed.ID("tenant-a"),
		newUser(t),
		seed.ID("org-customer-a"),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, channel,
			template_key, target, params, severity, occurred_at)
		VALUES ($1, $2, $3, 1, 'fault', 'email', 'alert', '{"kind":"unit","id":"`+seed.ID("unit-online-rto").
			String()+
			`"}', $4, 'warning', $5)`,
		notificationID,
		seed.ID("tenant-a"),
		membership,
		`{"targetName":"`+name+`","status":"open"}`,
		clock(),
	)

	state := func() (string, string) {
		var status, reason string

		ownerScan(
			t,
			`SELECT state, COALESCE(failure_reason, '') FROM notify.deliveries WHERE notification_id = $1 AND channel = 'email'`,
			[]any{notificationID},
			&status,
			&reason,
		)

		return status, reason
	}
	deliver := func(sender *fakeMail, when time.Time) int {
		t.Helper()

		_, err := notify.DeliverAll(
			context.Background(),
			srv.DB,
			sender,
			notify.Apps{Customer: "http://c", Partner: "", Technician: "", Admin: ""},
			when,
			500,
		)
		if err != nil {
			t.Fatal(err)
		}

		total := 0

		for _, m := range sender.sent {
			if strings.Contains(m.subject, name) {
				total++
			}
		}

		return total
	}
	deliver(&fakeMail{err: errSMTPDown, sent: nil}, clock())

	if st, reason := state(); st != statusFailed || reason != "smtp down" {
		t.Fatalf("after a failing send: %s %s", st, reason)
	}

	sender := &fakeMail{err: nil, sent: nil}
	if n := deliver(sender, clock().Add(59*time.Second)); n != 0 {
		t.Fatalf("retried before the pause: %d", n)
	}

	if n := deliver(sender, clock().Add(time.Minute)); n != 1 {
		t.Fatalf("retried after the pause: %d", n)
	}

	if st, _ := state(); st != statusSent {
		t.Errorf("after the retry: %s", st)
	}
}

type sentMail struct{ to, subject, body string }

// errSMTPDown is the failure of a mail server that does not answer.
var errSMTPDown = errors.New("smtp down")

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
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestNotificationMail(t *testing.T) {
	srv := server(t)
	note := func(membership string) string {
		notificationID := uuid.NewString()
		owner(
			t,
			`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, channel,
				template_key, target, params, severity, occurred_at)
			VALUES ($1, $2, $3, 1, 'fault', 'email', 'alert', '{"kind":"unit","id":"`+seed.ID("unit-online-rto").
				String()+
				`"}', '{"targetName":"Mail AC","status":"open"}', 'warning', $4)`,
			notificationID,
			seed.ID("tenant-a"),
			membership,
			clock(),
		)

		return notificationID
	}
	member := func(until time.Time) string {
		membershipID := uuid.NewString()
		owner(
			t,
			`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from,
				valid_until)
			VALUES ($1, $2, $3, $4, 'client', 'member', $5, $6)`,
			membershipID,
			seed.ID("tenant-a"),
			newUser(t),
			seed.ID("org-customer-a"),
			clock().Add(-time.Hour),
			until,
		)

		return membershipID
	}
	active, ended := note(member(clock().Add(time.Hour))), note(member(clock().Add(-time.Minute)))

	sender := &fakeMail{err: nil, sent: nil}

	_, err := notify.DeliverAll(
		context.Background(),
		srv.DB,
		sender,
		notify.Apps{Customer: "http://c", Partner: "", Technician: "", Admin: ""},
		clock(),
		500,
	)
	if err != nil {
		t.Fatal(err)
	}

	state := func(notificationID string) (string, string) {
		var status, reason string

		ownerScan(
			t,
			`SELECT state, COALESCE(failure_reason, '') FROM notify.deliveries WHERE notification_id = $1 AND channel = 'email'`,
			[]any{notificationID},
			&status,
			&reason,
		)

		return status, reason
	}
	if st, _ := state(active); st != statusSent {
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
	// retention (IR428): a notification older than two years goes with its delivery; a recent one stays
	owner(
		t,
		`UPDATE notify.notifications SET occurred_at = $2 WHERE id = $1`,
		active,
		clock().Add(-notify.Retention-time.Hour),
	)

	_, err = notify.SweepAll(context.Background(), srv.DB, clock())
	if err != nil {
		t.Fatal(err)
	}

	var left int
	ownerScan(
		t,
		`SELECT (SELECT count(*) FROM notify.notifications WHERE id = $1) + (SELECT count(*) FROM notify.deliveries WHERE
			notification_id = $1)`,
		[]any{active},
		&left,
	)

	if left != 0 {
		t.Errorf("an old notification and its delivery: %d rows left", left)
	}

	ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE id = $1`, []any{ended}, &left)

	if left != 1 {
		t.Error("a recent notification was swept")
	}
}

// TestDisabledAccountStaysClosed (IR420): HQ extending the membership of an account its contractor disabled does not
// reopen it; the new end is kept and comes back when the contractor enables the account.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDisabledAccountStaysClosed(t *testing.T) {
	srv := server(t)
	user := newUser(t)
	from, until, later := clock().Add(-time.Hour).
		Format(time.RFC3339),
		clock().Add(
			30*24*time.Hour,
		), clock().Add(
			60*24*time.Hour,
		)
	body := func(id string, end time.Time) string {
		idPart := ""
		if id != "" {
			idPart = `"id":"` + id + `",`
		}

		return `{` + idPart + `"userId":"` + user + `","organizationId":"` + seed.ID("org-contractor-a").
			String() +
			`","role":"technician","employment":"external",
			"permissions":[],"scopes":[],"validFrom":"` + from + `","validUntil":"` + end.Format(
			time.RFC3339,
		) + `","reason":"contract"}`
	}

	code, m := write(srv, hq(), "members.save", body("", until), 0)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, m)
	}

	membership := asStr(t, data(m)["id"])
	firstSignIn(t, srv, membership) // an active account (IR462)

	var (
		account string
		version int
	)
	ownerScan(
		t,
		`SELECT id::text, version FROM identity.partner_users WHERE membership_id = $1`,
		[]any{membership},
		&account,
		&version,
	)

	status := func(state string, userVersion int) {
		t.Helper()

		if code, res := write(
			srv,
			contrA(),
			"partnerUsers.save",
			`{"id":"`+account+`","email":"`+user+`@ac.local","partnerRole":"technician","status":"`+state+`","reason":"leave"}`,
			userVersion,
		); code != 200 {
			t.Fatalf("%s: %d %v", state, code, res)
		}
	}
	status("disabled", version)

	var membershipVersion int
	ownerScan(
		t,
		`SELECT version FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&membershipVersion,
	)

	if code, res := write(
		srv,
		hq(),
		"members.save",
		body(membership, later),
		membershipVersion,
	); code != 200 { // HQ extends the contract
		t.Fatalf("extend: %d %v", code, res)
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
			"membership of a disabled account after HQ's change: %v, want still closed at %v",
			end,
			clock(),
		)
	}

	ownerScan(
		t,
		`SELECT version FROM identity.partner_users WHERE id = $1`,
		[]any{account},
		&version,
	)
	status("active", version)
	ownerScan(
		t,
		`SELECT valid_until FROM identity.memberships WHERE id = $1`,
		[]any{membership},
		&end,
	)

	if !end.Equal(later.Truncate(time.Second)) {
		t.Errorf("membership after enabling: %v, want HQ's end %v", end, later)
	}
}

// TestContractorAddedTechnicianAssigned (IR461): a technician the contractor added has no unit scope, and the
// contractor still assigns them on its own accepted offer once HQ verified their qualification; another contractor
// cannot.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestContractorAddedTechnicianAssigned(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	email := "assign-" + uuid.NewString()[:8] + "@example.com"

	code, resp := write(
		srv,
		contrA(),
		"partnerUsers.save",
		`{"email":"`+email+`","partnerRole":"technician","displayName":"Assignable","accessUntil":"`+stamp(
			24*60,
		)+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("add: %d %v", code, resp)
	}

	tech := asStr(t, data(resp)["membershipId"])
	// HQ verified the technician's certificates (the unit's service scope needs all three)
	for _, code := range []string{"demo_indoor", "demo_outdoor", "demo_electrical"} {
		owner(
			t,
			`INSERT INTO identity.qualification_grants (tenant_id, membership_id, code, valid_from, valid_until) VALUES ($1, $2,
				$3, $4, $5)`,
			seed.ID("tenant-a"),
			tech,
			code,
			clock().Add(-time.Hour),
			clock().Add(365*24*time.Hour),
		)
	}

	_, resp = write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)

	job := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	if code, res := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		2,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+tech+`","startAt":"`+stamp(
			49,
		)+`","endAt":"`+stamp(
			51,
		)+`"}`,
		3,
	); code != 200 {
		t.Fatalf("assign the added technician: %d %v", code, res)
	}
}
