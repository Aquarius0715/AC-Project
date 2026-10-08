package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func seedID(s string) uuid.UUID { return seed.ID(s) }

func count(m map[string]any, k string) int {
	return int(data(m)["counts"].(map[string]any)[k].(float64))
}

func TestSummariesCustomer(t *testing.T) {
	s := server(t)
	code, m := post(s, &customerA, "summaries.get", `{"kind":"customer","filters":{}}`)
	if code != 200 || data(m)["kind"] != "customer" {
		t.Fatalf("customer: %d %v", code, m)
	}
	_, all := post(s, &customerA, "units.list", `{"limit":1}`)
	_, on := post(s, &customerA, "units.list", `{"filters":{"status":"online"},"limit":1}`)
	if count(m, "total") != int(data(all)["total"].(float64)) || count(m, "online") != int(data(on)["total"].(float64)) {
		t.Errorf("counts differ from units.list: %v / %v / %v", m, data(all)["total"], data(on)["total"])
	}
	if count(m, "total") != count(m, "online")+count(m, "offline")+count(m, "unknown") || count(m, "total") != count(m, "powerOn")+count(m, "powerOff")+count(m, "powerUnknown") {
		t.Errorf("partitions: %v", m)
	}
	if count(m, "offerCount") != 0 || count(m, "assignedCount") != 0 {
		t.Error("job counts are zero for customers")
	}
	var alerts int
	ownerScan(t, `SELECT count(*) FROM monitoring.alerts a JOIN assets.units u ON u.id = a.unit_id WHERE u.customer_org_id = (SELECT organization_id FROM identity.memberships WHERE id = $1)
		AND NOT u.archived AND a.status IN ('open','acknowledged') AND a.severity IN ('critical','warning')`, []any{seedID("customer-a")}, &alerts)
	if count(m, "alertCount") != alerts {
		t.Errorf("alertCount %d want %d", count(m, "alertCount"), alerts)
	}
	unit := seedID("unit-online-rto").String()
	if _, m := post(s, &customerA, "summaries.get", `{"kind":"customer","filters":{"unitId":"`+unit+`"}}`); count(m, "total") != 1 {
		t.Errorf("unit filter: %v", m)
	}
	if _, m := post(s, &customerA, "summaries.get", `{"kind":"customer","filters":{"unitIds":[]}}`); count(m, "total") != 0 {
		t.Error("empty unitIds")
	}
	for _, b := range []string{`{"kind":"customer","filters":{"status":"assigned"}}`, `{"kind":"customer","filters":{"overdueOnly":true}}`, `{"kind":"shop","filters":{}}`} {
		if code, _ := post(s, &customerA, "summaries.get", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerA, "summaries.get", `{"kind":"partner","filters":{}}`); code != 403 {
		t.Error("client asks for partner summary")
	}
	if code, _ := post(s, &hq, "summaries.get", `{"kind":"customer","filters":{}}`); code != 403 {
		t.Error("HQ summaries.get")
	}
}

func TestSummariesTechnicianPartner(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	job := assign(t, u, "tech-external-b", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	owner(t, `UPDATE maintenance.assignments SET created_at = $2 WHERE job_id = $1`, job, clock.Add(-2*time.Hour))
	owner(t, `UPDATE maintenance.jobs j SET assignment_id = a.id, scheduled_slot = a.scheduled FROM maintenance.assignments a WHERE a.job_id = j.id AND j.id = $1`, job)
	owner(t, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at, decision, decided_at, access_valid_from, access_valid_until)
		SELECT tenant_id, id, $2, 'terms-1', requested_slot, $3, $4, 'accept', $3, $3, $4 FROM maintenance.jobs WHERE id = $1`, job, seedID("org-contractor-b"), clock.Add(-3*time.Hour), clock.Add(24*time.Hour))
	owner(t, `UPDATE maintenance.jobs SET contractor_org_id = $2 WHERE id = $1`, job, seedID("org-contractor-b"))
	code, m := post(s, &techB, "summaries.get", `{"kind":"technician","filters":{}}`)
	if code != 200 || count(m, "assignedCount") < 1 || count(m, "scheduledCount") < 1 || count(m, "total") < 1 || count(m, "powerOn") != 0 || count(m, "alertCount") != 0 {
		t.Fatalf("technician: %d %v", code, m)
	}
	_, m2 := post(s, &techB, "summaries.get", `{"kind":"technician","filters":{"unitIds":["`+u+`"],"status":"assigned"}}`)
	if count(m2, "assignedCount") != 1 || count(m2, "total") != 1 || count(m2, "online") != 1 {
		t.Errorf("technician filtered: %v", m2)
	}
	_, list := post(s, &techB, "jobs.list", `{"filters":{"unitIds":["`+u+`"],"status":"assigned"}}`)
	if int(data(list)["total"].(float64)) != count(m2, "assignedCount") {
		t.Error("summary and list disagree")
	}
	if code, _ := post(s, &techB, "summaries.get", `{"kind":"technician","filters":{"status":"assigned","statuses":["assigned"]}}`); code != 422 {
		t.Error("status with statuses")
	}
	if code, _ := post(s, &techB, "summaries.get", `{"kind":"technician","filters":{"statuses":[]}}`); code != 422 {
		t.Error("empty statuses")
	}
	code, m = post(s, &contrB, "summaries.get", `{"kind":"partner","filters":{"unitIds":["`+u+`"]}}`)
	if code != 200 || count(m, "activeCount") != 1 || count(m, "assignedCount") != 1 || count(m, "total") != 0 {
		t.Fatalf("partner: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.jobs SET due_at = $2 WHERE id = $1`, job, clock.Add(-time.Minute))
	if _, m := post(s, &contrB, "summaries.get", `{"kind":"partner","filters":{"unitIds":["`+u+`"],"overdueOnly":true}}`); count(m, "overdueCount") != 1 {
		t.Errorf("overdue: %v", m)
	}
	if code, _ := post(s, &contrB, "summaries.get", `{"kind":"technician","filters":{}}`); code != 403 {
		t.Error("contractor asks for technician summary")
	}
}
