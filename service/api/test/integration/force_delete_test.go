package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

const keyForceNotAllowed = "errors.force_delete_not_allowed"

// forceKit makes a customer with its organization and one account that has acted, for the forced-delete tests.
type forceKit struct {
	org, cust, member string
	orgV, custV, memV int
}

func newForceKit(t *testing.T, name string) forceKit {
	t.Helper()

	srv := server(t)

	code, resp := write(
		srv,
		hq(),
		"organizations.save",
		`{"name":"`+name+`","kind":"customer","status":"active"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("organization: %d %v", code, resp)
	}

	kit := forceKit{
		org:    asStr(t, data(resp)["id"]),
		orgV:   ver(t, resp),
		cust:   "",
		member: "",
		custV:  0,
		memV:   0,
	}

	code, resp = write(
		srv,
		hq(),
		"customers.save",
		`{"name":"`+name+`","organizationId":"`+kit.org+`","serviceProfile":"general","status":"active"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("customer: %d %v", code, resp)
	}

	kit.cust, kit.custV = asStr(t, data(resp)["id"]), ver(t, resp)

	person := newUser(t)
	from := clock().Add(-time.Hour).Format(time.RFC3339)

	code, resp = write(
		srv,
		hq(),
		"members.save",
		`{"userId":"`+person+`","organizationId":"`+kit.org+`","role":"client","permissions":[],"scopes":[],`+
			`"validFrom":"`+from+`","validUntil":null,"reason":"made for the test"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, resp)
	}

	kit.member, kit.memV = asStr(t, data(resp)["id"]), ver(t, resp)

	// the account has acted: an audit entry names it
	owner(
		t,
		`INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, membership_id, action, target_kind, target_id,
			occurred_at, correlation_id, result)
		VALUES ($1, $2, 'client', $3, 'units.save', 'unit', $4, $5, 'test', 'success')`,
		seed.ID("tenant-a"),
		person,
		kit.member,
		uuid.NewString(),
		clock(),
	)

	return kit
}

// A holder of data.forceDelete deletes a customer whose accounts acted (IR690): the customer with force, then each
// account with force, then the organization. Without the permission force is FORBIDDEN, without force the accounts
// still stop it, and what is not an account (a unit, a record) still stops the forced delete. The forced deletes are
// recorded as such. HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestForceDeleteCustomerWithAccounts(t *testing.T) {
	srv := server(t)
	kit := newForceKit(t, "Force Customer "+uuid.NewString()[:8])
	forceCustomer := `{"id":"` + kit.cust + `",` + testReason + `,"force":true}`
	forceMember := `{"id":"` + kit.member + `",` + testReason + `,"force":true}`

	// without force the account stops the delete
	if code, res := write(
		srv,
		hq(),
		"customers.delete",
		`{"id":"`+kit.cust+`",`+testReason+`}`,
		kit.custV,
	); code != 409 ||
		res["messageKey"] != "error.customerInUse" {
		t.Errorf("customer with an account: %d %v", code, res["messageKey"])
	}

	if code, res := write(
		srv,
		hq(),
		"members.delete",
		`{"id":"`+kit.member+`",`+testReason+`}`,
		kit.memV,
	); code != 409 ||
		res["messageKey"] != "error.memberUsed" {
		t.Errorf("account that acted: %d %v", code, res["messageKey"])
	}

	// force needs the permission: the restriction manager has identity.write and asset.write but not data.forceDelete
	if code, res := write(
		srv,
		restrMgr(),
		"customers.delete",
		forceCustomer,
		kit.custV,
	); code != 403 ||
		res["messageKey"] != keyForceNotAllowed {
		t.Errorf("force without the permission: %d %v", code, res["messageKey"])
	}

	if code, res := write(srv, restrMgr(), "members.delete", forceMember, kit.memV); code != 403 ||
		res["messageKey"] != keyForceNotAllowed {
		t.Errorf("force account delete without the permission: %d %v", code, res["messageKey"])
	}

	// a unit still stops a forced delete: force only leaves the accounts out
	seededCustomer, seededVersion := listed(
		t, "customers.list", `"organizationId":"`+seed.ID("org-customer-b").String()+`"`, "", "",
	)
	if code, res := write(
		srv,
		hq(),
		"customers.delete",
		`{"id":"`+seededCustomer+`",`+testReason+`,"force":true}`,
		seededVersion,
	); code != 409 || res["messageKey"] != "error.customerHasUnits" {
		t.Errorf("forced delete of a customer with units: %d %v", code, res["messageKey"])
	}

	// the permission holder deletes the customer, then the account, then the organization
	if code, res := write(
		srv,
		hq(),
		"customers.delete",
		forceCustomer,
		kit.custV,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("forced customer delete: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"organizations.delete",
		`{"id":"`+kit.org+`",`+testReason+`}`,
		kit.orgV,
	); code != 409 ||
		res["messageKey"] != "error.orgHasUsers" {
		t.Errorf("organization with an account: %d %v", code, res["messageKey"])
	}

	if code, res := write(
		srv,
		hq(),
		"members.delete",
		forceMember,
		kit.memV,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("forced account delete: %d %v", code, res)
	}

	orgDelete := `{"id":"` + kit.org + `",` + testReason + `}`
	if code, res := write(srv, hq(), "organizations.delete", orgDelete, kit.orgV); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("organization delete: %d %v", code, res)
	}

	var left, forced int

	ownerScan(
		t,
		`SELECT (SELECT count(*) FROM identity.memberships WHERE id = $1),
			(SELECT count(*) FROM audit.audit_log WHERE action IN ('members.delete', 'customers.delete')
				AND reason LIKE '[forced] %' AND target_id IN ($1::text, $2::text))`,
		[]any{kit.member, kit.cust},
		&left,
		&forced,
	)

	if left != 0 || forced != 2 {
		t.Errorf(
			"account gone and both forced deletes recorded as such: %d left, %d recorded",
			left,
			forced,
		)
	}
}

// A forced delete of an account keeps the rules that protect others (IR690): the caller's own account and a person's
// other account stop it, as they stop the plain delete. HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestForceDeleteAccountKeepsItsLimits(t *testing.T) {
	srv := server(t)
	kit := newForceKit(t, "Force Limits "+uuid.NewString()[:8])

	var self int

	ownerScan(
		t,
		`SELECT version FROM identity.memberships WHERE id = $1`,
		[]any{seed.ID("hq-operator")},
		&self,
	)

	if code, res := write(
		srv,
		hq(),
		"members.delete",
		`{"id":"`+seed.ID("hq-operator").String()+`",`+testReason+`,"force":true}`,
		self,
	); code != 409 || res["messageKey"] != "error.memberSelf" {
		t.Errorf("forced delete of the own account: %d %v", code, res["messageKey"])
	}

	// the person has a second account: the forced delete of one is refused
	var person string

	ownerScan(
		t,
		`SELECT user_id::text FROM identity.memberships WHERE id = $1`,
		[]any{kit.member},
		&person,
	)

	code, resp := write(
		srv,
		hq(),
		"members.save",
		`{"userId":"`+person+`","organizationId":"`+seed.ID("org-customer-a").
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
		`{"id":"`+kit.member+`",`+testReason+`,"force":true}`,
		kit.memV,
	); code != 409 || res["messageKey"] != "error.memberHasOthers" {
		t.Errorf(
			"forced delete of an account of a person with another: %d %v",
			code,
			res["messageKey"],
		)
	}
}

// The same for a contractor company with an account (IR690): the profile is deleted with force, then the account, then
// the organization; without force the account stops it, and without the permission force is FORBIDDEN. HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestForceDeleteContractorWithAccounts(t *testing.T) {
	srv := server(t)
	name := "Force Contractor " + uuid.NewString()[:8]

	code, resp := write(
		srv,
		hq(),
		"organizations.save",
		`{"name":"`+name+`","kind":"contractor","status":"active"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("organization: %d %v", code, resp)
	}

	org, orgV := asStr(t, data(resp)["id"]), ver(t, resp)

	code, resp = write(
		srv,
		hq(),
		"contractors.save",
		`{"organizationId":"`+org+`","registrationNo":"REG-F","serviceAreas":["KL"],"contactEmail":"f@example.com",`+
			`"insuranceValidUntil":null}`,
		0,
	)
	if code != 200 {
		t.Fatalf("contractor: %d %v", code, resp)
	}

	contractor, contractorV := asStr(t, data(resp)["id"]), ver(t, resp)
	person := newUser(t)

	code, resp = write(
		srv,
		hq(),
		"members.save",
		`{"userId":"`+person+`","organizationId":"`+org+`","role":"contractor","permissions":["partner.accept"],"scopes":[],`+
			`"validFrom":"`+clock().Add(-time.Hour).
			Format(time.RFC3339)+
			`","validUntil":null,"reason":"made for the test"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("members.save: %d %v", code, resp)
	}

	member, memberV := asStr(t, data(resp)["id"]), ver(t, resp)
	plain := `{"id":"` + contractor + `",` + testReason + `}`
	forced := `{"id":"` + contractor + `",` + testReason + `,"force":true}`

	if code, res := write(srv, hq(), "contractors.delete", plain, contractorV); code != 409 ||
		res["messageKey"] != "error.contractorInUse" {
		t.Errorf("contractor with an account: %d %v", code, res["messageKey"])
	}

	if code, _ := write(srv, restrMgr(), "contractors.delete", forced, contractorV); code == 200 {
		t.Error("a caller without data.forceDelete forces a contractor delete")
	}

	if code, res := write(
		srv,
		hq(),
		"contractors.delete",
		forced,
		contractorV,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("forced contractor delete: %d %v", code, res)
	}

	if code, res := write(
		srv, hq(), "members.delete", `{"id":"`+member+`",`+testReason+`,"force":true}`, memberV,
	); code != 200 || data(res)["deleted"] != true {
		t.Fatalf("forced account delete: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"organizations.delete",
		`{"id":"`+org+`",`+testReason+`}`,
		orgV,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("organization delete: %d %v", code, res)
	}
}
