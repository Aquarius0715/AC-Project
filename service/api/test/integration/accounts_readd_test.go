package integration_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestPersonAddedAgain (IR462): a technician the contractor removed is added again with the same e-mail — as a
// contractor this time — and is the same person with a new membership: the provider account is pointed at it and on,
// the new name holds and no sign-in shows yet. While the person still holds an open membership the address stays
// errors.email_taken, for the company and for another one. HQ giving the removed person a membership points the
// account at it too.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPersonAddedAgain(t *testing.T) {
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
	drain := func() {
		t.Helper()

		err := srv.DrainEvents(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	email := "again-" + uuid.NewString()[:8] + "@example.com"
	add := func(who *actor, role, name string) (int, map[string]any) {
		access := ""
		if role == roleTechnician {
			access = `,"accessUntil":"` + clock().Add(30*24*time.Hour).Format(time.RFC3339) + `"`
		}

		return write(
			srv,
			who,
			"partnerUsers.save",
			`{"email":"`+email+`","partnerRole":"`+role+`","displayName":"`+name+`"`+access+`}`,
			0,
		)
	}
	// removed: the membership ends now. The test clock stands still, so the adding was "now" too and the end is a second
	// later; the removal is moved to when it would be, after the adding.
	remove := func(partnerUserID, membership string) {
		t.Helper()

		if code, res := write(
			srv,
			contrA(),
			"partnerUsers.remove",
			`{"id":"`+partnerUserID+`","reason":"added with the wrong role"}`,
			1,
		); code != 200 {
			t.Fatalf("remove: %d %v", code, res)
		}

		drain()
		owner(
			t,
			`UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '1 hour', valid_until = $2 WHERE id = $1`,
			membership,
			clock(),
		)
	}
	updatedTo := func(membership string) bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()

		return slices.ContainsFunc(
			fake.updated,
			func(a idp.Account) bool { return a.Email == email && a.Membership == membership },
		)
	}

	code, resp := add(contrA(), "technician", "First Name")
	if code != 200 {
		t.Fatalf("add: %d %v", code, resp)
	}

	first, firstMembership := asStr(t, data(resp)["id"]), asStr(t, data(resp)["membershipId"])

	drain()

	var user string
	ownerScan(
		t,
		`SELECT user_id::text FROM identity.memberships WHERE id = $1`,
		[]any{firstMembership},
		&user,
	)
	owner(
		t,
		`UPDATE identity.users SET last_sign_in_at = $2 WHERE id = $1`,
		user,
		clock(),
	) // as if they had signed in

	for name, a := range map[string]*actor{"the same company": contrA(), "another company": contrB()} {
		if code, res := add(
			a,
			"contractor",
			"X",
		); code != 422 ||
			fields(res) != refusedEmailTaken {
			t.Errorf("%s while the person is a user: %d %s", name, code, fields(res))
		}
	}

	remove(first, firstMembership)

	code, resp = add(contrA(), "contractor", "Second Name")
	if code != 200 || data(resp)["status"] != statusInvited ||
		data(resp)["membershipId"] == firstMembership {
		t.Fatalf("added again: %d %v", code, resp)
	}

	again, membership := asStr(t, data(resp)["id"]), asStr(t, data(resp)["membershipId"])

	var (
		same, signIn string
		name         string
		signedIn     *time.Time
	)
	ownerScan(
		t,
		`SELECT m.user_id::text, u.display_name, u.last_sign_in_at FROM identity.memberships m JOIN identity.users u ON u.id =
			m.user_id WHERE m.id = $1`,
		[]any{membership},
		&same,
		&name,
		&signedIn,
	)

	if same != user || name != "Second Name" || signedIn != nil {
		t.Errorf("the person: %s (first %s) %q %v", same, user, name, signedIn)
	}

	drain()
	ownerScan(
		t,
		`SELECT coalesce(sign_in_membership_id::text, '') FROM identity.users WHERE id = $1`,
		[]any{user},
		&signIn,
	)

	if !updatedTo(membership) || signIn != membership {
		t.Errorf("the account signs in with %q, updated %+v", signIn, fake.updated)
	}

	// HQ gives the removed person a membership: the account signs in with it
	remove(again, membership)

	code, resp = write(
		srv,
		hq(),
		"members.save",
		`{"userId":"`+user+`","organizationId":"`+seed.ID("org-contractor-b").
			String()+
			`","role":"technician","employment":"external",`+
			`"permissions":["alert.read"],"scopes":[{"kind":"unit","id":"`+seed.ID("unit-non-rto").
			String()+
			`"}],"validFrom":"`+clock().Format(
			time.RFC3339,
		)+
			`","validUntil":"`+clock().Add(30*24*time.Hour).
			Format(time.RFC3339)+
			`","reason":"joins contractor B"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("HQ: %d %v", code, resp)
	}

	hqMembership := asStr(t, data(resp)["id"])

	drain()

	var listed string
	ownerScan(
		t,
		`SELECT status FROM identity.partner_users WHERE membership_id = $1`,
		[]any{hqMembership},
		&listed,
	)

	if !updatedTo(hqMembership) || listed != statusInvited {
		t.Errorf("HQ's membership: listed %q, updated %+v", listed, fake.updated)
	}
}

// TestPersonOfAnotherTenant (IR462): an address whose person signs in with another tenant's membership is
// errors.email_taken, though their membership here has ended.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPersonOfAnotherTenant(t *testing.T) {
	srv := server(t)
	email := "foreign-" + uuid.NewString()[:8] + "@example.com"
	user, foreign, local := uuid.New(), uuid.New(), uuid.New()
	owner(
		t,
		`INSERT INTO identity.users (id, email, display_name, sign_in_membership_id) VALUES ($1, $2, 'Foreign', $3)`,
		user,
		email,
		foreign,
	)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1,
			$2,$3,$4,'client','member',$5)`,
		foreign,
		seed.ID("tenant-b"),
		user,
		seed.ID("org-customer-tb"),
		clock().Add(-48*time.Hour),
	)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until)
			VALUES ($1,$2,$3,$4,'technician','external',$5,$6)`,
		local,
		seed.ID("tenant-a"),
		user,
		seed.ID("org-contractor-a"),
		clock().Add(-48*time.Hour),
		clock().Add(-24*time.Hour),
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.memberships WHERE id = ANY($1)`, []uuid.UUID{foreign, local})
		owner(t, `DELETE FROM identity.users WHERE id = $1`, user)
	})

	code, res := write(
		srv,
		contrA(),
		"partnerUsers.save",
		`{"email":"`+email+`","partnerRole":"technician","displayName":"Foreign","accessUntil":"`+clock().Add(24*time.Hour).
			Format(time.RFC3339)+
			`"}`,
		0,
	)
	if code != 422 || fields(res) != refusedEmailTaken {
		t.Errorf("another tenant's person: %d %s", code, fields(res))
	}
}

// TestAccountSwitchedByMembership (IR462): disabling a demo-style account — no provider subject stored — turns it off
// by the person's ID; when the account signs in with another membership, enabling this one leaves it alone.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAccountSwitchedByMembership(t *testing.T) {
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
	email := "demo-" + uuid.NewString()[:8] + "@example.com"
	user, membership := uuid.New(), uuid.New()
	owner(
		t,
		`INSERT INTO identity.users (id, email, display_name, sign_in_membership_id) VALUES ($1, $2, 'Demo Tech', $3)`,
		user,
		email,
		membership,
	)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until)
			VALUES ($1,$2,$3,$4,'technician','external',$5,$6)`,
		membership,
		seed.ID("tenant-a"),
		user,
		seed.ID("org-contractor-b"),
		clock().Add(-24*time.Hour),
		clock().Add(10*24*time.Hour),
	)

	var partnerUserID string
	ownerScan(
		t,
		`INSERT INTO identity.partner_users (tenant_id, organization_id, membership_id, email, display_name, partner_role,
			status, invited_at, invited_by_membership_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'Demo Tech', 'technician', 'active', $5, $6, $5, $5) RETURNING id::text`,
		[]any{
			seed.ID("tenant-a"),
			seed.ID("org-contractor-b"),
			membership,
			email,
			clock().Add(-24 * time.Hour),
			seed.ID("contractor-b"),
		},
		&partnerUserID,
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.partner_users WHERE id = $1`, partnerUserID)
		owner(t, `DELETE FROM identity.memberships WHERE id = $1`, membership)
		owner(t, `DELETE FROM identity.users WHERE id = $1`, user)
	})

	set := func(status string, version int) {
		t.Helper()

		if code, res := write(
			srv,
			contrB(),
			"partnerUsers.save",
			`{"id":"`+partnerUserID+`","email":"`+email+`","partnerRole":"technician",`+
				`"status":"`+status+`","reason":"on `+
				`leave"}`,
			version,
		); code != 200 {
			t.Fatalf("%s: %d %v", status, code, res)
		}

		err := srv.DrainEvents(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	set("disabled", 1)

	if !slices.Contains(fake.switched, user.String()+"=false") {
		t.Errorf("the demo account was not turned off: %v", fake.switched)
	}

	owner(
		t,
		`UPDATE identity.users SET sign_in_membership_id = $2 WHERE id = $1`,
		user,
		uuid.New(),
	) // signs in with another membership
	set("active", 2)

	if slices.Contains(fake.switched, user.String()+"=true") {
		t.Errorf("an account signing in with another membership was turned on: %v", fake.switched)
	}
}

// TestClientAddedAgain (IR462): a customer owner removes a member and adds the same address again as an owner: the same
// person, a new client membership with the new role. (Only a person whose account the provider opened for this tenant
// is taken back, hence the provider and the drained events.)
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestClientAddedAgain(t *testing.T) {
	srv := serverCfg(
		t,
		func(config *apiserver.Config) {
			config.IdentityProvider = &fakeIDP{
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
			config.Domains = ops.Domains
		},
	)
	cust := seed.ID("cust-b").String()
	email := "client-again-" + uuid.NewString()[:8] + "@example.com"
	add := func(role string) (int, map[string]any) {
		return write(
			srv,
			customerB(),
			"clientUsers.save",
			`{"customerId":"`+cust+`","email":"`+email+`","clientRole":"`+role+`","displayName":"Client Again"}`,
			0,
		)
	}

	code, res := add("member")
	if code != 200 {
		t.Fatalf("add: %d %v", code, res)
	}

	first := asStr(t, data(res)["membershipId"])

	err := srv.DrainEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if code, resp := write(
		srv,
		customerB(),
		"clientUsers.remove",
		`{"id":"`+asStr(t, data(res)["id"])+`","reason":"should be an owner"}`,
		1,
	); code != 200 {
		t.Fatalf("remove: %d %v", code, resp)
	}
	// the test clock stands still: the removal comes after the adding (see TestPersonAddedAgain)
	owner(
		t,
		`UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '1 hour', valid_until = $2 WHERE id = $1`,
		first,
		clock(),
	)

	code, res = add("owner")
	if code != 200 || data(res)["clientRole"] != clientRoleOwner {
		t.Fatalf("added again: %d %v", code, res)
	}

	again := asStr(t, data(res)["membershipId"])

	t.Cleanup(func() { // later tests count customer B's owners: this one ends before them
		owner(
			t,
			`UPDATE identity.memberships SET valid_from = $2, valid_until = $3 WHERE id = $1`,
			again,
			clock().Add(-time.Hour),
			clock().Add(-30*time.Minute),
		)
	})

	var same int
	ownerScan(
		t,
		`SELECT count(DISTINCT user_id) FROM identity.memberships WHERE id = ANY($1)`,
		[]any{[]string{first, again}},
		&same,
	)

	if same != 1 || again == first {
		t.Errorf(
			"not the same person with a new membership: %d persons, %s / %s",
			same,
			first,
			again,
		)
	}
}

// TestAccountProviderTrouble (IR461–IR463): a provider that does not answer leaves the person without an account and
// nothing recorded; once it answers, the set-up e-mail sent again opens the account. An account the provider lost is
// opened anew when the person is added again.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAccountProviderTrouble(t *testing.T) {
	fake := &fakeIDP{
		failing:    true,
		mu:         sync.Mutex{},
		subjects:   nil,
		err:        nil,
		created:    nil,
		updated:    nil,
		setups:     nil,
		switched:   nil,
		lost:       nil,
		generation: 0,
	}
	srv := serverCfg(
		t,
		func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains },
	)
	drain := func() {
		t.Helper()

		err := srv.DrainEvents(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}
	email := "trouble-" + uuid.NewString()[:8] + "@example.com"
	add := func() (string, string) {
		t.Helper()

		code, res := write(
			srv,
			contrA(),
			"partnerUsers.save",
			`{"email":"`+email+`","partnerRole":"technician","displayName":"Trouble Tech",`+
				`"accessUntil":"`+clock().Add(30*24*time.Hour).
				Format(time.RFC3339)+
				`"}`,
			0,
		)
		if code != 200 {
			t.Fatalf("add: %d %v", code, res)
		}

		return asStr(t, data(res)["id"]), asStr(t, data(res)["membershipId"])
	}
	account := func(membership string) (string, string) {
		t.Helper()

		var sub, signIn string

		ownerScan(
			t,
			`SELECT coalesce(u.cognito_sub, ''), coalesce(u.sign_in_membership_id::text, '') FROM identity.users u JOIN
				identity.memberships m ON m.user_id = u.id WHERE m.id = $1`,
			[]any{membership},
			&sub,
			&signIn,
		)

		return sub, signIn
	}
	partnerUserID, membership := add()

	drain()

	if sub, signIn := account(membership); sub != "" || signIn != "" {
		t.Errorf("recorded although the provider did not answer: %q %q", sub, signIn)
	}

	fake.mu.Lock()
	fake.failing = false
	fake.mu.Unlock()
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
	); code != 200 {
		t.Fatalf("resend: %d %v", code, res)
	}

	drain()

	if sub, signIn := account(membership); sub != "sub-"+email || signIn != membership {
		t.Errorf("after the resend: %q %q", sub, signIn)
	}

	// removed, then added again while the provider no longer knows the account: a new one is opened
	if code, res := write(
		srv,
		contrA(),
		"partnerUsers.remove",
		`{"id":"`+partnerUserID+`","reason":"left"}`,
		1,
	); code != 200 {
		t.Fatalf("remove: %d %v", code, res)
	}

	drain()
	owner(
		t,
		`UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '1 hour', valid_until = $2 WHERE id = $1`,
		membership,
		clock(),
	) // removed after the adding
	fake.mu.Lock()
	fake.lost, fake.generation = map[string]bool{"sub-" + email: true}, 2
	fake.mu.Unlock()

	_, again := add()

	drain()

	if sub, signIn := account(again); sub != "sub-"+email+"#2" || signIn != again {
		t.Errorf("the lost account: %q %q", sub, signIn)
	}
}

// TestAccountsPerOrgPerDay (IR464): a contractor or a customer owner adds at most identity.AccountsPerOrgPerDay
// accounts to their organization in 24 hours — the next is RATE_LIMITED errors.too_many_accounts with the seconds until
// the oldest of them is a day old; HQ is not limited.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAccountsPerOrgPerDay(t *testing.T) {
	srv := server(t)
	filler := uuid.New()
	owner(
		t,
		`INSERT INTO identity.users (id, email, display_name) VALUES ($1, $2, 'Cap Filler')`,
		filler,
		"cap-"+uuid.NewString()[:8]+"@example.com",
	)

	ids := make([]uuid.UUID, 0, 2*identity.AccountsPerOrgPerDay)
	for _, org := range []string{"org-contractor-b", "org-customer-b"} {
		for idx := range identity.AccountsPerOrgPerDay { // ended memberships added 1 h to 1 h 29 min ago
			membershipID := uuid.New()
			ids = append(ids, membershipID)

			role, employment, clientRole := roleTechnician, any("external"), any(nil)
			if org == "org-customer-b" {
				role, employment, clientRole = roleClient, nil, clientRoleMember
			}

			owner(
				t,
				`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, client_role,
					valid_from, valid_until, created_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
				membershipID,
				seed.ID("tenant-a"),
				filler,
				seed.ID(org),
				role,
				employment,
				clientRole,
				clock().Add(
					-3*time.Hour,
				),
				clock().Add(-2*time.Hour),
				clock().Add(-time.Hour-time.Duration(idx)*time.Minute),
			)
		}
	}

	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.memberships WHERE id = ANY($1)`, ids)
		owner(t, `DELETE FROM identity.users WHERE id = $1`, filler)
	})
	// the wait runs until the oldest of the latest 30 is a day old: at most 23 h, as each of ours is an hour old or more
	// (other tests may have added to contractor B since, which only shortens it)
	code, res := write(
		srv,
		contrB(),
		"partnerUsers.save",
		`{"email":"cap-`+uuid.NewString()[:8]+`@example.com","partnerRole":"technician","displayName":"Cap Tech",`+
			`"accessUntil":"`+clock().Add(24*time.Hour).
			Format(time.RFC3339)+
			`"}`,
		0,
	)

	wait, _ := res["retryAfterSeconds"].(float64)
	if code != 429 || res["messageKey"] != "errors.too_many_accounts" || wait <= 0 ||
		wait > float64(23*time.Hour/time.Second) {
		t.Errorf("contractor over the cap: %d %v", code, res)
	}

	cust := seed.ID("cust-b").String()

	add := func(a *actor) (int, map[string]any) {
		return write(
			srv,
			a,
			"clientUsers.save",
			`{"customerId":"`+cust+`","email":"cap-`+uuid.NewString()[:8]+`@example.com","clientRole":"member",`+
				`"displayName":"Cap Client"}`,
			0,
		)
	}
	if code, m := add(customerB()); code != 429 || m["messageKey"] != "errors.too_many_accounts" {
		t.Errorf("customer owner over the cap: %d %v", code, m)
	}

	code, res = add(hq())
	if code != 200 {
		t.Fatalf("HQ is not limited: %d %v", code, res)
	}

	if code, resp := write(
		srv,
		hq(),
		"clientUsers.remove",
		`{"id":"`+asStr(t, data(res)["id"])+`","reason":"cap test"}`,
		1,
	); code != 200 {
		t.Errorf("remove: %d %v", code, resp)
	}
}
