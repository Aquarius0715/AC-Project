package integration_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	testReason    = `"reason":"registered by mistake"`
	keyOrgProfile = "error.orgHasProfile"
)

// listed is the ID and version of the item the list operation answers for the filter, matching the key when given.
func listed(t *testing.T, operation, filter, key, value string) (string, int) {
	t.Helper()

	_, list := post(server(t), hq(), operation, `{"filters":{`+filter+`},"limit":100}`)
	for _, it := range items(t, list) {
		if key == "" || it[key] == value {
			return asStr(t, it["id"]), int(asNum(t, it["version"]))
		}
	}

	t.Fatalf("%s lists no %s %s", operation, key, value)

	return "", 0
}

// A customer registered by mistake is deleted as it was made (the customer, then its organization), after the
// organization lost its profile; a customer with units or other records, and an operator organization, are refused
// with the reason (DEC-103). HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCustomersAndOrganizationsDelete(t *testing.T) {
	srv := server(t)
	name := "Del Customer " + uuid.NewString()[:8]

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

	org, orgVersion := asStr(t, data(resp)["id"]), ver(t, resp)

	code, resp = write(
		srv,
		hq(),
		"customers.save",
		`{"name":"`+name+`","organizationId":"`+org+`","serviceProfile":"general","status":"active"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("customer: %d %v", code, resp)
	}

	cust, version := asStr(t, data(resp)["id"]), ver(t, resp)
	dropCustomer := func(a *actor, v int) (int, map[string]any) {
		return write(srv, a, "customers.delete", `{"id":"`+cust+`",`+testReason+`}`, v)
	}
	dropOrg := func(a *actor, id string, v int) (int, map[string]any) {
		return write(srv, a, "organizations.delete", `{"id":"`+id+`",`+testReason+`}`, v)
	}

	if code, res := dropOrg(
		hq(),
		org,
		orgVersion,
	); code != 409 ||
		res["messageKey"] != keyOrgProfile {
		t.Errorf("organization with a customer profile: %d %v", code, res["messageKey"])
	}

	for name, who := range map[string]*actor{roleClient: customerB(), roleTechnician: techInt()} {
		if code, _ := dropCustomer(who, version); code == 200 {
			t.Errorf("%s deletes a customer", name)
		}
	}

	if code, res := write(
		srv,
		hq(),
		"customers.delete",
		`{"id":"`+cust+`","reason":""}`,
		version,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("customer delete needs a reason: %d %s", code, fields(res))
	}

	if code, _ := dropCustomer(hq(), version+5); code != 409 {
		t.Error("customer delete at a stale version")
	}

	code, resp = dropCustomer(hq(), version)
	if code != 200 || data(resp)["deleted"] != true || data(resp)["id"] != cust {
		t.Fatalf("customer delete: %d %v", code, resp)
	}

	if code, _ := dropCustomer(hq(), version); code != 404 {
		t.Error("a customer is deleted once")
	}

	if code, res := dropOrg(hq(), org, orgVersion); code != 200 || data(res)["deleted"] != true {
		t.Fatalf("organization delete: %d %v", code, res)
	}

	if code, _ := dropOrg(hq(), org, orgVersion); code != 404 {
		t.Error("an organization is deleted once")
	}

	// a customer with units, jobs and contracts, and the organization of its profile, stay
	seeded, seededVersion := listed(
		t, "customers.list", `"organizationId":"`+seed.ID("org-customer-b").String()+`"`, "", "",
	)
	if code, res := write(
		srv,
		hq(),
		"customers.delete",
		`{"id":"`+seeded+`",`+testReason+`}`,
		seededVersion,
	); code != 409 || res["messageKey"] != "error.customerHasUnits" {
		t.Errorf("customer with units: %d %v", code, res["messageKey"])
	}

	seededOrg := seed.ID("org-customer-b").String()
	_, seededOrgVersion := listed(t, "organizations.list", "", "id", seededOrg)

	if code, res := dropOrg(hq(), seededOrg, seededOrgVersion); code != 409 ||
		res["messageKey"] != keyOrgProfile {
		t.Errorf("organization of a customer: %d %v", code, res["messageKey"])
	}

	operator := seed.ID("org-operator-a").String()
	_, operatorVersion := listed(t, "organizations.list", "", "id", operator)

	if code, res := dropOrg(hq(), operator, operatorVersion); code != 409 ||
		res["messageKey"] != "error.orgOperator" {
		t.Errorf("operator organization: %d %v", code, res["messageKey"])
	}
}

// A contractor company registered by mistake is deleted as it was made (the profile, then its organization); one
// with jobs, offers or accounts is refused with the reason (DEC-103). HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestContractorsDelete(t *testing.T) {
	srv := server(t)
	name := "Del Contractor " + uuid.NewString()[:8]

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

	org, orgVersion := asStr(t, data(resp)["id"]), ver(t, resp)

	code, resp = write(
		srv,
		hq(),
		"contractors.save",
		`{"organizationId":"`+org+`","registrationNo":"REG-D","serviceAreas":["KL"],"contactEmail":"d@example.com",`+
			`"insuranceValidUntil":null}`,
		0,
	)
	if code != 200 {
		t.Fatalf("contractor: %d %v", code, resp)
	}

	contractor, version := asStr(t, data(resp)["id"]), ver(t, resp)
	drop := func(a *actor, v int) (int, map[string]any) {
		return write(srv, a, "contractors.delete", `{"id":"`+contractor+`",`+testReason+`}`, v)
	}

	if code, res := write(
		srv,
		hq(),
		"organizations.delete",
		`{"id":"`+org+`",`+testReason+`}`,
		orgVersion,
	); code != 409 || res["messageKey"] != keyOrgProfile {
		t.Errorf("organization with a contractor profile: %d %v", code, res["messageKey"])
	}

	for name, who := range map[string]*actor{roleClient: customerB(), "contractor": contrA(), roleTechnician: techInt()} {
		if code, _ := drop(who, version); code == 200 {
			t.Errorf("%s deletes a contractor", name)
		}
	}

	if code, _ := drop(hq(), version+5); code != 409 {
		t.Error("contractor delete at a stale version")
	}

	code, resp = drop(hq(), version)
	if code != 200 || data(resp)["deleted"] != true || data(resp)["id"] != contractor {
		t.Fatalf("contractor delete: %d %v", code, resp)
	}

	if code, _ := drop(hq(), version); code != 404 {
		t.Error("a contractor is deleted once")
	}

	if code, res := write(
		srv,
		hq(),
		"organizations.delete",
		`{"id":"`+org+`",`+testReason+`}`,
		orgVersion,
	); code != 200 || data(res)["deleted"] != true {
		t.Fatalf("organization delete: %d %v", code, res)
	}

	// a contractor company with jobs, offers and accounts stays (its profile is made here when no test made it)
	seededOrg := seed.ID("org-contractor-a").String()
	_, _ = write(
		srv,
		hq(),
		"contractors.save",
		`{"organizationId":"`+seededOrg+`","registrationNo":"REG-A","serviceAreas":["KL"],`+
			`"contactEmail":"a@example.com","insuranceValidUntil":null}`,
		0,
	)

	seeded, seededVersion := listed(t, "contractors.list", "", "organizationId", seededOrg)
	if code, res := write(
		srv,
		hq(),
		"contractors.delete",
		`{"id":"`+seeded+`",`+testReason+`}`,
		seededVersion,
	); code != 409 || res["messageKey"] != "error.contractorInUse" {
		t.Errorf("contractor with records: %d %v", code, res["messageKey"])
	}
}
