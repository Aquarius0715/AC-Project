package app

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// D05: units.archive conflicts with active work; customer deactivation with active units / contracts / jobs.
func TestArchiveRulesD05(t *testing.T) {
	s := server(t)
	archive := func(unit string) (int, map[string]any) {
		_, g := post(s, &hq, "units.get", `{"id":"`+unit+`"}`)
		return write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":"decommission"}`, ver(g))
	}
	// open job
	u := newUnit(t, s, "Arch job")
	job := uuid.NewString()
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at)
		VALUES ($1,$2,$3,$4,'reactive','requested','client_request',tstzrange($5,$6),$6)`, job, seed.ID("tenant-a"), u, seed.ID("org-customer-b"), clock, clock.Add(time.Hour))
	if code, m := archive(u); code != 409 || m["messageKey"] != "error.unitActive" {
		t.Fatalf("open job: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.jobs SET status = 'completed' WHERE id = $1`, job)
	if code, _ := archive(u); code != 200 {
		t.Fatalf("completed job is history only: %d", code)
	}
	// active device binding
	u2 := newUnit(t, s, "Arch dev")
	_, m := write(s, &hq, "devices.register", `{"serial":"AR-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+u2+`"}`, 0)
	write(s, &hq, "devices.bind", `{"deviceId":"`+data(m)["id"].(string)+`","unitId":"`+u2+`","reason":"install"}`, 1)
	if code, _ := archive(u2); code != 409 {
		t.Errorf("bound device: %d", code)
	}
	// unexpired contract
	u3 := newUnit(t, s, "Arch contract")
	k := uuid.NewString()
	owner(t, `INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor, currency, restriction_eligible, created_by)
		VALUES ($1,1,$2,$3,$4,'general',tstzrange($5,$6),5000,'MYR',false,$7)`, k, seed.ID("tenant-a"), seed.ID("cust-b"), seed.ID("org-customer-b"),
		clock.Add(-24*time.Hour), clock.Add(24*time.Hour), seed.ID("hq-operator"))
	owner(t, `INSERT INTO billing.contract_units (tenant_id, contract_id, contract_version, unit_id) VALUES ($1,$2,1,$3)`, seed.ID("tenant-a"), k, u3)
	if code, _ := archive(u3); code != 409 {
		t.Errorf("unexpired contract: %d", code)
	}
	// customer deactivation with active units
	_, c := post(s, &hq, "customers.list", `{"filters":{"organizationId":"`+seed.ID("org-customer-b").String()+`"}}`)
	cb := items(c)[0]
	body := `{"id":"` + cb["id"].(string) + `","name":"` + cb["name"].(string) + `","organizationId":"` + seed.ID("org-customer-b").String() + `","serviceProfile":"` + cb["serviceProfile"].(string) + `","status":"inactive"}`
	if code, m := write(s, &hq, "customers.save", body, int(cb["version"].(float64))); code != 409 || m["messageKey"] != "error.customerActive" {
		t.Fatalf("deactivation with units: %d %v", code, m)
	}
	// an organization with only an unexpired contract also blocks deactivation
	org := uuid.NewString()
	_, nc := write(s, &hq, "customers.save", `{"name":"Contract only","organizationId":"`+org+`","serviceProfile":"general","status":"active"}`, 0)
	k2 := uuid.NewString()
	owner(t, `INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor, currency, restriction_eligible, created_by)
		VALUES ($1,1,$2,$3,$4,'general',tstzrange($5,$6),5000,'MYR',false,$7)`, k2, seed.ID("tenant-a"), data(nc)["id"], org, clock.Add(-time.Hour), clock.Add(time.Hour), seed.ID("hq-operator"))
	if code, _ := write(s, &hq, "customers.save", `{"id":"`+data(nc)["id"].(string)+`","name":"Contract only","organizationId":"`+org+`","serviceProfile":"general","status":"inactive"}`, 1); code != 409 {
		t.Errorf("deactivation with contract: %d", code)
	}
}
