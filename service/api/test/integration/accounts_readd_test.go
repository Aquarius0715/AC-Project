package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

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
func TestPersonAddedAgain(t *testing.T) {
	fake := &fakeIdP{}
	s := serverCfg(t, func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains })
	drain := func() {
		t.Helper()
		if err := s.DrainEvents(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	email := "again-" + uuid.NewString()[:8] + "@example.com"
	add := func(a *actor, role, name string) (int, map[string]any) {
		access := ""
		if role == "technician" {
			access = `,"accessUntil":"` + clock.Add(30*24*time.Hour).Format(time.RFC3339) + `"`
		}
		return write(s, a, "partnerUsers.save", `{"email":"`+email+`","partnerRole":"`+role+`","displayName":"`+name+`"`+access+`}`, 0)
	}
	// removed: the membership ends now. The test clock stands still, so the adding was "now" too and the end is a second
	// later; the removal is moved to when it would be, after the adding.
	remove := func(id, membership string) {
		t.Helper()
		if code, m := write(s, &contrA, "partnerUsers.remove", `{"id":"`+id+`","reason":"added with the wrong role"}`, 1); code != 200 {
			t.Fatalf("remove: %d %v", code, m)
		}
		drain()
		owner(t, `UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '1 hour', valid_until = $2 WHERE id = $1`, membership, clock)
	}
	updatedTo := func(membership string) bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		return slices.ContainsFunc(fake.updated, func(a idp.Account) bool { return a.Email == email && a.Membership == membership })
	}

	code, m := add(&contrA, "technician", "First Name")
	if code != 200 {
		t.Fatalf("add: %d %v", code, m)
	}
	first, firstMembership := data(m)["id"].(string), data(m)["membershipId"].(string)
	drain()
	var user string
	ownerScan(t, `SELECT user_id::text FROM identity.memberships WHERE id = $1`, []any{firstMembership}, &user)
	owner(t, `UPDATE identity.users SET last_sign_in_at = $2 WHERE id = $1`, user, clock) // as if they had signed in
	for name, a := range map[string]*actor{"the same company": &contrA, "another company": &contrB} {
		if code, m := add(a, "contractor", "X"); code != 422 || fields(m) != "email=errors.email_taken" {
			t.Errorf("%s while the person is a user: %d %s", name, code, fields(m))
		}
	}

	remove(first, firstMembership)
	code, m = add(&contrA, "contractor", "Second Name")
	if code != 200 || data(m)["status"] != "invited" || data(m)["membershipId"] == firstMembership {
		t.Fatalf("added again: %d %v", code, m)
	}
	again, membership := data(m)["id"].(string), data(m)["membershipId"].(string)
	var same, signIn string
	var name string
	var signedIn *time.Time
	ownerScan(t, `SELECT m.user_id::text, u.display_name, u.last_sign_in_at FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id WHERE m.id = $1`,
		[]any{membership}, &same, &name, &signedIn)
	if same != user || name != "Second Name" || signedIn != nil {
		t.Errorf("the person: %s (first %s) %q %v", same, user, name, signedIn)
	}
	drain()
	ownerScan(t, `SELECT coalesce(sign_in_membership_id::text, '') FROM identity.users WHERE id = $1`, []any{user}, &signIn)
	if !updatedTo(membership) || signIn != membership {
		t.Errorf("the account signs in with %q, updated %+v", signIn, fake.updated)
	}

	// HQ gives the removed person a membership: the account signs in with it
	remove(again, membership)
	code, m = write(s, &hq, "members.save", `{"userId":"`+user+`","organizationId":"`+seed.ID("org-contractor-b").String()+`","role":"technician","employment":"external",`+
		`"permissions":["alert.read"],"scopes":[{"kind":"unit","id":"`+seed.ID("unit-non-rto").String()+`"}],"validFrom":"`+clock.Format(time.RFC3339)+
		`","validUntil":"`+clock.Add(30*24*time.Hour).Format(time.RFC3339)+`","reason":"joins contractor B"}`, 0)
	if code != 200 {
		t.Fatalf("HQ: %d %v", code, m)
	}
	hqMembership := data(m)["id"].(string)
	drain()
	var listed string
	ownerScan(t, `SELECT status FROM identity.partner_users WHERE membership_id = $1`, []any{hqMembership}, &listed)
	if !updatedTo(hqMembership) || listed != "invited" {
		t.Errorf("HQ's membership: listed %q, updated %+v", listed, fake.updated)
	}
}

// TestPersonOfAnotherTenant (IR462): an address whose person signs in with another tenant's membership is
// errors.email_taken, though their membership here has ended.
func TestPersonOfAnotherTenant(t *testing.T) {
	s := server(t)
	email := "foreign-" + uuid.NewString()[:8] + "@example.com"
	user, foreign, local := uuid.New(), uuid.New(), uuid.New()
	owner(t, `INSERT INTO identity.users (id, email, display_name, sign_in_membership_id) VALUES ($1, $2, 'Foreign', $3)`, user, email, foreign)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1,$2,$3,$4,'client','member',$5)`,
		foreign, seed.ID("tenant-b"), user, seed.ID("org-customer-tb"), clock.Add(-48*time.Hour))
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until) VALUES ($1,$2,$3,$4,'technician','external',$5,$6)`,
		local, seed.ID("tenant-a"), user, seed.ID("org-contractor-a"), clock.Add(-48*time.Hour), clock.Add(-24*time.Hour))
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.memberships WHERE id = ANY($1)`, []uuid.UUID{foreign, local})
		owner(t, `DELETE FROM identity.users WHERE id = $1`, user)
	})
	code, m := write(s, &contrA, "partnerUsers.save", `{"email":"`+email+`","partnerRole":"technician","displayName":"Foreign","accessUntil":"`+clock.Add(24*time.Hour).Format(time.RFC3339)+`"}`, 0)
	if code != 422 || fields(m) != "email=errors.email_taken" {
		t.Errorf("another tenant's person: %d %s", code, fields(m))
	}
}

// TestAccountSwitchedByMembership (IR462): disabling a demo-style account — no provider subject stored — turns it off by
// the person's ID; when the account signs in with another membership, enabling this one leaves it alone.
func TestAccountSwitchedByMembership(t *testing.T) {
	fake := &fakeIdP{}
	s := serverCfg(t, func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains })
	email := "demo-" + uuid.NewString()[:8] + "@example.com"
	user, membership := uuid.New(), uuid.New()
	owner(t, `INSERT INTO identity.users (id, email, display_name, sign_in_membership_id) VALUES ($1, $2, 'Demo Tech', $3)`, user, email, membership)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until) VALUES ($1,$2,$3,$4,'technician','external',$5,$6)`,
		membership, seed.ID("tenant-a"), user, seed.ID("org-contractor-b"), clock.Add(-24*time.Hour), clock.Add(10*24*time.Hour))
	var id string
	ownerScan(t, `INSERT INTO identity.partner_users (tenant_id, organization_id, membership_id, email, display_name, partner_role, status, invited_at, invited_by_membership_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'Demo Tech', 'technician', 'active', $5, $6, $5, $5) RETURNING id::text`,
		[]any{seed.ID("tenant-a"), seed.ID("org-contractor-b"), membership, email, clock.Add(-24 * time.Hour), seed.ID("contractor-b")}, &id)
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.partner_users WHERE id = $1`, id)
		owner(t, `DELETE FROM identity.memberships WHERE id = $1`, membership)
		owner(t, `DELETE FROM identity.users WHERE id = $1`, user)
	})
	set := func(status string, v int) {
		t.Helper()
		if code, m := write(s, &contrB, "partnerUsers.save", `{"id":"`+id+`","email":"`+email+`","partnerRole":"technician","status":"`+status+`","reason":"on leave"}`, v); code != 200 {
			t.Fatalf("%s: %d %v", status, code, m)
		}
		if err := s.DrainEvents(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	set("disabled", 1)
	if !slices.Contains(fake.switched, user.String()+"=false") {
		t.Errorf("the demo account was not turned off: %v", fake.switched)
	}
	owner(t, `UPDATE identity.users SET sign_in_membership_id = $2 WHERE id = $1`, user, uuid.New()) // signs in with another membership
	set("active", 2)
	if slices.Contains(fake.switched, user.String()+"=true") {
		t.Errorf("an account signing in with another membership was turned on: %v", fake.switched)
	}
}

// TestClientAddedAgain (IR462): a customer owner removes a member and adds the same address again as an owner: the same
// person, a new client membership with the new role. (Only a person whose account the provider opened for this tenant
// is taken back, hence the provider and the drained events.)
func TestClientAddedAgain(t *testing.T) {
	s := serverCfg(t, func(c *apiserver.Config) { c.IdentityProvider = &fakeIdP{}; c.Domains = ops.Domains })
	cust := seed.ID("cust-b").String()
	email := "client-again-" + uuid.NewString()[:8] + "@example.com"
	add := func(role string) (int, map[string]any) {
		return write(s, &customerB, "clientUsers.save", `{"customerId":"`+cust+`","email":"`+email+`","clientRole":"`+role+`","displayName":"Client Again"}`, 0)
	}
	code, m := add("member")
	if code != 200 {
		t.Fatalf("add: %d %v", code, m)
	}
	first := data(m)["membershipId"].(string)
	if err := s.DrainEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code, m := write(s, &customerB, "clientUsers.remove", `{"id":"`+data(m)["id"].(string)+`","reason":"should be an owner"}`, 1); code != 200 {
		t.Fatalf("remove: %d %v", code, m)
	}
	// the test clock stands still: the removal comes after the adding (see TestPersonAddedAgain)
	owner(t, `UPDATE identity.memberships SET valid_from = $2::timestamptz - interval '1 hour', valid_until = $2 WHERE id = $1`, first, clock)
	code, m = add("owner")
	if code != 200 || data(m)["clientRole"] != "owner" {
		t.Fatalf("added again: %d %v", code, m)
	}
	again := data(m)["membershipId"].(string)
	t.Cleanup(func() { // later tests count customer B's owners: this one ends before them
		owner(t, `UPDATE identity.memberships SET valid_from = $2, valid_until = $3 WHERE id = $1`, again, clock.Add(-time.Hour), clock.Add(-30*time.Minute))
	})
	var same int
	ownerScan(t, `SELECT count(DISTINCT user_id) FROM identity.memberships WHERE id = ANY($1)`, []any{[]string{first, again}}, &same)
	if same != 1 || again == first {
		t.Errorf("not the same person with a new membership: %d persons, %s / %s", same, first, again)
	}
}
