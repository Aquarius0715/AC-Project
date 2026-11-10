package integration_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// An account made by mistake is deleted with its person: one that has never acted, has no assignment or certificate
// and is the person's only membership. Anything else is refused with the reason (DEC-103). HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMembersDelete(t *testing.T) {
	srv := server(t)
	from := clock().Add(-time.Hour).Format(time.RFC3339)
	create := func(user, org string) (string, int) {
		code, resp := write(
			srv,
			hq(),
			"members.save",
			`{"userId":"`+user+`","organizationId":"`+org+`","role":"client","permissions":[],"scopes":[],`+
				`"validFrom":"`+from+`","validUntil":null,"reason":"made for the test"}`,
			0,
		)
		if code != 200 {
			t.Fatalf("members.save: %d %v", code, resp)
		}

		return asStr(t, data(resp)["id"]), ver(t, resp)
	}
	drop := func(a *actor, id string, version int) (int, map[string]any) {
		return write(
			srv,
			a,
			"members.delete",
			`{"id":"`+id+`","reason":"made by mistake"}`,
			version,
		)
	}
	customerOrg := seed.ID("org-customer-b").String()
	person := newUser(t)
	membership, version := create(person, customerOrg)

	for name, who := range map[string]*actor{roleClient: customerB(), roleTechnician: techInt()} {
		if code, _ := drop(who, membership, version); code == 200 {
			t.Errorf("%s deletes an account", name)
		}
	}

	if code, res := write(
		srv,
		hq(),
		"members.delete",
		`{"id":"`+membership+`","reason":""}`,
		version,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("account delete needs a reason: %d %s", code, fields(res))
	}

	if code, _ := drop(hq(), membership, version+5); code != 409 {
		t.Error("account delete at a stale version")
	}

	// a person with a second account: delete the person's accounts one at a time is refused
	second, _ := create(person, seed.ID("org-customer-a").String())

	if code, res := drop(
		hq(),
		membership,
		version,
	); code != 409 ||
		res["messageKey"] != "error.memberHasOthers" {
		t.Errorf("person with two accounts: %d %v", code, res["messageKey"])
	}

	owner(t, `DELETE FROM identity.consents WHERE membership_id = $1`, second)
	owner(t, `DELETE FROM identity.memberships WHERE id = $1`, second)

	// an account that has acted stays: its audit trail names it
	acted := newUser(t)
	actedMembership, actedVersion := create(acted, customerOrg)
	owner(
		t,
		`INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, membership_id, action, target_kind, target_id,
			occurred_at, correlation_id, result)
		VALUES ($1, $2, 'client', $3, 'units.save', 'unit', $4, $5, 'test', 'success')`,
		seed.ID("tenant-a"),
		acted,
		actedMembership,
		uuid.NewString(),
		clock(),
	)

	if code, res := drop(
		hq(),
		actedMembership,
		actedVersion,
	); code != 409 ||
		res["messageKey"] != "error.memberUsed" {
		t.Errorf("account that acted: %d %v", code, res["messageKey"])
	}

	// the account the caller signs in with stays
	var self int

	ownerScan(
		t,
		`SELECT version FROM identity.memberships WHERE id = $1`,
		[]any{seed.ID("hq-operator")},
		&self,
	)

	if code, res := drop(
		hq(),
		seed.ID("hq-operator").String(),
		self,
	); code != 409 ||
		res["messageKey"] != "error.memberSelf" {
		t.Errorf("own account: %d %v", code, res["messageKey"])
	}

	// the clean account goes, and its person with it
	code, resp := drop(hq(), membership, version)
	if code != 200 || data(resp)["deleted"] != true || data(resp)["id"] != membership {
		t.Fatalf("account delete: %d %v", code, resp)
	}

	if code, _ := drop(hq(), membership, version); code != 404 {
		t.Error("an account is deleted once")
	}

	var left int

	ownerScan(
		t,
		`SELECT (SELECT count(*) FROM identity.memberships WHERE id = $1) +
			(SELECT count(*) FROM identity.users WHERE id = $2)`,
		[]any{membership, person},
		&left,
	)

	if left != 0 {
		t.Errorf("the account and its person are gone: %d left", left)
	}
}

// The provider account of a deleted person is deleted too, once the event is applied (DEC-103).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMembersDeleteRemovesTheProviderAccount(t *testing.T) {
	fake := &fakeIDP{
		mu:         sync.Mutex{},
		subjects:   nil,
		err:        nil,
		created:    nil,
		updated:    nil,
		setups:     nil,
		switched:   nil,
		removed:    nil,
		failing:    false,
		lost:       nil,
		generation: 0,
	}
	srv := serverCfg(
		t,
		func(c *apiserver.Config) { c.IdentityProvider = fake; c.Domains = ops.Domains },
	)
	user := newUser(t)
	subject := "sub-" + user

	owner(t, `UPDATE identity.users SET cognito_sub = $1 WHERE id = $2`, subject, user)

	code, resp := write(
		srv,
		hq(),
		"members.save",
		`{"userId":"`+user+`","organizationId":"`+seed.ID("org-customer-b").
			String()+
			`","role":"client","permissions":[],`+
			`"scopes":[],"validFrom":"`+clock().Add(-time.Hour).
			Format(time.RFC3339)+
			`","validUntil":null,"reason":"test"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, resp)
	}

	if code, res := write(
		srv,
		hq(),
		"members.delete",
		`{"id":"`+asStr(t, data(resp)["id"])+`","reason":"made by mistake"}`,
		ver(t, resp),
	); code != 200 {
		t.Fatalf("members.delete: %d %v", code, res)
	}

	err := srv.DrainEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if !slices.Contains(fake.removed, subject) {
		t.Errorf("the provider account was not deleted: %v", fake.removed)
	}
}
