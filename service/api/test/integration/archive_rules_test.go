package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// D05: units.archive conflicts with active work; customer deactivation with active units / contracts / jobs.
func TestArchiveRulesD05(t *testing.T) {
	srv := server(t)
	archive := func(unit string) (int, map[string]any) {
		_, g := post(srv, &hq, "units.get", `{"id":"`+unit+`"}`)

		return write(srv, &hq, "units.archive", `{"id":"`+unit+`","reason":"decommission"}`, ver(g))
	}
	// open job
	unitID := newUnit(t, srv, "Arch job")
	job := uuid.NewString()
	owner(
		t,
		`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at)
		VALUES ($1,$2,$3,$4,'reactive','requested','client_request',tstzrange($5,$6),$6)`,
		job,
		seed.ID("tenant-a"),
		unitID,
		seed.ID("org-customer-b"),
		clock,
		clock.Add(time.Hour),
	)

	if code, m := archive(unitID); code != 409 || m["messageKey"] != "error.unitActive" {
		t.Fatalf("open job: %d %v", code, m)
	}

	owner(t, `UPDATE maintenance.jobs SET status = 'completed' WHERE id = $1`, job)

	if code, _ := archive(unitID); code != 200 {
		t.Fatalf("completed job is history only: %d", code)
	}
	// active device binding
	u2 := newUnit(t, srv, "Arch dev")
	_, res := write(
		srv,
		&hq,
		"devices.register",
		`{"serial":"AR-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+u2+`"}`,
		0,
	)
	write(
		srv,
		&hq,
		"devices.bind",
		`{"deviceId":"`+asStr(t, data(res)["id"])+`","unitId":"`+u2+`","reason":"install"}`,
		1,
	)

	if code, _ := archive(u2); code != 409 {
		t.Errorf("bound device: %d", code)
	}
	// unexpired contract
	u3 := newUnit(t, srv, "Arch contract")
	key := uuid.NewString()
	owner(
		t,
		`INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor,
			currency, restriction_eligible, created_by)
		VALUES ($1,1,$2,$3,$4,'general',tstzrange($5,$6),5000,'MYR',false,$7)`,
		key,
		seed.ID("tenant-a"),
		seed.ID("cust-b"),
		seed.ID("org-customer-b"),
		clock.Add(-24*time.Hour),
		clock.Add(24*time.Hour),
		seed.ID("hq-operator"),
	)
	owner(
		t,
		`INSERT INTO billing.contract_units (tenant_id, contract_id, contract_version, unit_id) VALUES ($1,$2,1,$3)`,
		seed.ID("tenant-a"),
		key,
		u3,
	)

	if code, _ := archive(u3); code != 409 {
		t.Errorf("unexpired contract: %d", code)
	}
	// customer deactivation with active units
	_, c := post(
		srv,
		&hq,
		"customers.list",
		`{"filters":{"organizationId":"`+seed.ID("org-customer-b").String()+`"}}`,
	)
	cb := items(c)[0]

	body := `{"id":"` + asStr(
		t,
		cb["id"],
	) + `","name":"` + asStr(
		t,
		cb["name"],
	) + `","organizationId":"` +
		seed.ID("org-customer-b").
			String() +
		`","serviceProfile":"` + asStr(
		t,
		cb["serviceProfile"],
	) + `","status":"inactive"}`
	if code, resp := write(
		srv,
		&hq,
		"customers.save",
		body,
		int(asNum(t, cb["version"])),
	); code != 409 ||
		resp["messageKey"] != apperr.KeyCustomerActive {
		t.Fatalf("deactivation with units: %d %v", code, resp)
	}
	// an organization with only an unexpired contract also blocks deactivation
	org := uuid.NewString()
	_, nc := write(
		srv,
		&hq,
		"customers.save",
		`{"name":"Contract only","organizationId":"`+org+`","serviceProfile":"general","status":"active"}`,
		0,
	)
	k2 := uuid.NewString()
	owner(
		t,
		`INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor,
			currency, restriction_eligible, created_by)
		VALUES ($1,1,$2,$3,$4,'general',tstzrange($5,$6),5000,'MYR',false,$7)`,
		k2,
		seed.ID("tenant-a"),
		data(nc)["id"],
		org,
		clock.Add(-time.Hour),
		clock.Add(time.Hour),
		seed.ID("hq-operator"),
	)

	if code, _ := write(
		srv,
		&hq,
		"customers.save",
		`{"id":"`+asStr(t, data(nc)["id"])+`","name":"Contract only",`+
			`"organizationId":"`+org+`","serviceProfile":"general",`+
			`"status":"inactive"}`,
		1,
	); code != 409 {
		t.Errorf("deactivation with contract: %d", code)
	}
}
