package integration

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// IR49(b) / IR152: external technicians see a unit only through their own active Assignment, inside its viewing
// window and the accepted Offer's access window; equipment reads wait for the work window. Contractors see the
// units of jobs with their own accepted Offer inside its access window.
func TestUnitScopeExternalTechnicianAndContractor(t *testing.T) {
	s := server(t)
	unit := seed.ID("unit-other-customer").String()
	listed := func(a *actor) bool {
		_, m := post(s, a, "units.list", `{"limit":100,"filters":{"unitIds":["`+unit+`"]}}`)
		return len(items(m)) == 1
	}
	if listed(&techB) || listed(&contrA) {
		t.Fatal("no Assignment / Offer yet: the unit is hidden")
	}
	// assignment created an hour ago, work window starts in an hour; no Offer yet
	job := assign(t, unit, "tech-external-b", clock.Add(time.Hour), clock.Add(3*time.Hour), "active")
	owner(t, `UPDATE maintenance.assignments SET created_at = $2 WHERE job_id = $1`, job, clock.Add(-time.Hour))
	if listed(&techB) {
		t.Fatal("external technician needs the accepted Offer's access window too")
	}
	owner(t, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at, access_valid_from, access_valid_until, decision, decided_at)
		VALUES ($1,$2,$3,'v1',tstzrange($4,$5),$6,$5,$6,$5,'accept',$6)`,
		seed.ID("tenant-a"), job, seed.ID("org-contractor-a"), clock.Add(time.Hour), clock.Add(3*time.Hour), clock.Add(-2*time.Hour))
	if !listed(&techB) {
		t.Fatal("inside the viewing window and the access window the unit is listed")
	}
	if !listed(&contrA) {
		t.Fatal("contractor with an accepted Offer in its access window sees the unit")
	}
	if listed(&contrB) {
		t.Fatal("another contractor never sees it")
	}
	if code, m := post(s, &techB, "units.get", `{"id":"`+unit+`"}`); code != 403 || m["messageKey"] != "errors.assignment_not_started" {
		t.Fatalf("before the work window: %d %v", code, m)
	}
	// equipment reads (IR49(b)): lists omit the unit, single reads answer FORBIDDEN until the work window starts
	owner(t, `UPDATE monitoring.alerts SET status = 'resolved', resolved_at = $2 WHERE unit_id = $1 AND rule_key = 'warning' AND status <> 'resolved'`, unit, clock)
	alert := newAlert(t, unit, seed.ID("org-customer-b").String(), "warning", clock.Add(-time.Minute))
	alertListed := func() bool {
		_, m := post(s, &techB, "alerts.list", `{"limit":100,"filters":{"unitId":"`+unit+`"}}`)
		for _, it := range items(m) {
			if it["id"] == alert {
				return true
			}
		}
		return false
	}
	if alertListed() {
		t.Fatal("alerts.list before the work window")
	}
	if code, m := post(s, &techB, "alerts.get", `{"id":"`+alert+`"}`); code != 403 || m["messageKey"] != "errors.assignment_not_started" {
		t.Fatalf("alerts.get before the work window: %d %v", code, m)
	}
	series := `{"from":"` + clock.Add(-time.Hour).Format(time.RFC3339) + `","to":"` + clock.Format(time.RFC3339) + `","unitIds":["` + unit + `"],"metric":"temperature","query":{}}`
	if code, m := post(s, &techB, "telemetry.series", series); code != 403 || m["messageKey"] != "errors.assignment_not_started" {
		t.Fatalf("telemetry before the work window: %d %v", code, m)
	}
	if code, _ := post(s, &contrA, "alerts.get", `{"id":"`+alert+`"}`); code != 200 {
		t.Fatalf("contractor reads the alert of its accepted job: %d", code)
	}
	if code, _ := post(s, &contrA, "units.get", `{"id":"`+unit+`"}`); code != 200 {
		t.Fatalf("contractor units.get: %d", code)
	}
	// work window running → equipment reads allowed; internal technicians never depend on assignments
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND job_id <> $2 AND scheduled && tstzrange($3, $4)`,
		seed.ID("tech-external-b"), job, clock.Add(-time.Minute), clock.Add(time.Hour))
	owner(t, `UPDATE maintenance.assignments SET scheduled = tstzrange($2, $3) WHERE job_id = $1`, job, clock.Add(-time.Minute), clock.Add(time.Hour))
	if code, m := post(s, &techB, "units.get", `{"id":"`+unit+`"}`); code != 200 {
		t.Fatalf("inside the work window: %d %v", code, m)
	}
	if !alertListed() {
		t.Fatal("alerts.list inside the work window")
	}
	if code, _ := post(s, &techB, "telemetry.series", series); code != 200 {
		t.Fatalf("telemetry inside the work window: %d", code)
	}
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE job_id = $1`, job)
	if listed(&techB) {
		t.Fatal("revoked Assignment hides the unit again")
	}
	owner(t, `UPDATE maintenance.offers SET access_valid_until = $2 WHERE job_id = $1`, job, clock.Add(-time.Minute))
	if listed(&contrA) {
		t.Fatal("ended access window hides the unit from the contractor")
	}
}
