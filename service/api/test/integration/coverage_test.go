package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestUnitsCoverage(t *testing.T) {
	s := server(t)
	// warranty dates relative to the test clock: Bedroom AC expiring (30 d), Living room AC under warranty (200 d)
	owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, seed.ID("unit-online-rto"), clock.Add(30*24*time.Hour))
	owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, seed.ID("unit-non-rto"), clock.Add(200*24*time.Hour))
	// the seed contracts would cover these units: end them for this test (restored afterwards)
	owner(t, `UPDATE billing.contracts SET term = tstzrange('2025-01-01','2025-02-01') WHERE id = ANY($1)`, []uuid.UUID{seed.ID("contract-rto-a"), seed.ID("contract-general-a")})
	t.Cleanup(func() {
		owner(t, `UPDATE billing.contracts SET term = tstzrange('2026-01-01','2027-01-01') WHERE id = ANY($1)`, []uuid.UUID{seed.ID("contract-rto-a"), seed.ID("contract-general-a")})
	})
	// contract covering the Lobby AC
	k := uuid.NewString()
	owner(t, `INSERT INTO billing.contracts (id, version, tenant_id, customer_id, customer_org_id, plan_type, term, price_minor, currency, restriction_eligible, created_by)
		VALUES ($1,1,$2,$3,$4,'rto',tstzrange($5,$6),12000,'MYR',true,$7)`, k, seed.ID("tenant-a"), seed.ID("cust-a"), seed.ID("org-customer-a"),
		clock.Add(-24*time.Hour), clock.Add(365*24*time.Hour), seed.ID("hq-operator"))
	owner(t, `INSERT INTO billing.contract_units (tenant_id, contract_id, contract_version, unit_id) VALUES ($1,$2,1,$3)`, seed.ID("tenant-a"), k, seed.ID("unit-limited"))

	byUnit := func(body string) map[string]map[string]any {
		code, m := post(s, &hq, "units.coverage", body)
		if code != 200 {
			t.Fatalf("%s: %d %v", body, code, m)
		}
		out := map[string]map[string]any{}
		for _, it := range items(m) {
			out[it["unitId"].(string)] = it
		}
		return out
	}
	all := byUnit(`{"filters":{"customerId":"` + seed.ID("cust-a").String() + `"},"limit":100}`)
	if all[seed.ID("unit-online-rto").String()]["status"] != "expiring" || all[seed.ID("unit-non-rto").String()]["status"] != "under_warranty" {
		t.Fatalf("warranty statuses: %v", all)
	}
	if lobby := all[seed.ID("unit-limited").String()]; lobby["status"] != "contract" || len(lobby["contractIds"].([]any)) == 0 {
		t.Fatalf("contract coverage: %v", lobby)
	}
	// a job completed under warranty whose accepted report lists replaced parts is claimable until a claim is filed (IR209)
	job := completedJob(t, s)
	owner(t, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, parts, accepted_at) VALUES ($1, 1, $2, $3, $4, 'accepted', $5, $6)`,
		uuid.New(), seed.ID("tenant-a"), job, seed.ID("user-tech-internal-a"), `[{"name":"Fan motor","quantity":1,"catalogCode":"FM-25","source":"hq_warehouse","lotSerial":null,"replacesComponentKey":"blower_motor","oldPartDisposal":"returned","receiptAttachmentId":null}]`, clock.Add(-time.Hour))
	claimable := func() bool {
		for _, j := range byUnit(`{"filters":{"customerId":"` + seed.ID("cust-a").String() + `"},"limit":100}`)[seed.ID("unit-non-rto").String()]["claimableJobIds"].([]any) {
			if j == job {
				return true
			}
		}
		return false
	}
	if !claimable() {
		t.Fatal("completed under warranty with parts → claimable")
	}
	owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, seed.ID("unit-non-rto"), clock.Add(-2*time.Hour))
	if claimable() {
		t.Fatal("a warranty that ended before completion is not claimable")
	}
	owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, seed.ID("unit-non-rto"), clock.Add(200*24*time.Hour))
	if code, m := write(s, &hq, "jobs.recordWarrantyClaim", `{"jobId":"`+job+`","partLabel":"Fan motor FM-25 ×1","amountMinor":21000,"currency":"MYR","reason":"replaced under warranty"}`, 1); code != 200 {
		t.Fatalf("claim: %d %v", code, m)
	}
	if claimable() {
		t.Fatal("a filed claim ends the claimable state")
	}

	exp := byUnit(`{"filters":{"coverage":"expiring"},"limit":100}`)
	for _, it := range exp {
		if it["status"] != "expiring" {
			t.Fatal("coverage filter")
		}
	}
	within := byUnit(`{"filters":{"expiringWithinDays":60},"limit":100}`)
	if _, ok := within[seed.ID("unit-online-rto").String()]; !ok {
		t.Fatal("expiringWithinDays 60 includes the 30-day unit")
	}
	if _, ok := within[seed.ID("unit-non-rto").String()]; ok {
		t.Fatal("expiringWithinDays 60 excludes the 200-day unit")
	}
	if got := byUnit(`{"filters":{"search":"bedroom"},"limit":100}`); len(got) == 0 {
		t.Fatal("search by name")
	}
	for _, b := range []string{`{"sort":{"field":"name","direction":"desc"},"limit":100}`, `{"sort":{"field":"id","direction":"asc"},"limit":100}`, `{"sort":{"field":"dueAt","direction":"desc"},"limit":2}`} {
		byUnit(b)
	}
	for _, b := range []string{`{"filters":{"coverage":"x"}}`, `{"filters":{"expiringWithinDays":0}}`, `{"sort":{"field":"status","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "units.coverage", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerA, "units.coverage", `{}`); code != 403 {
		t.Error("client cannot read coverage")
	}
}
