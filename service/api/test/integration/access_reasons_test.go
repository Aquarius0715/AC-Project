package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestTechnicianUnitAccessReasons covers the IR94 answers to a technician read of a unit's device history without
// jobId (devices.events → TechnicianUnit): an external technician without a delegation is NOT_FOUND; an internal
// technician with the unit in scope but no assignment, an assignment that has not started, or one whose window
// ended is FORBIDDEN with the reason; inside the window the history is readable.
func TestTechnicianUnitAccessReasons(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Access AC")
	_, m := write(s, &hq, "devices.register", `{"serial":"AX-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+u+`"}`, 0)
	dev := data(m)["id"].(string)
	write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u+`","reason":"install"}`, 1)
	events := func(a *actor) (int, string) {
		code, m := post(s, a, "devices.events", `{"id":"`+dev+`","query":{}}`)
		key, _ := m["messageKey"].(string)
		return code, key
	}
	if code, _ := events(&techA); code != 404 {
		t.Errorf("external technician without a delegation: %d", code)
	}
	owner(t, `DELETE FROM maintenance.assignments WHERE technician_membership_id = $1 AND job_id IN (SELECT id FROM maintenance.jobs WHERE unit_id = $2)`, seed.ID("tech-internal-a"), u)
	owner(t, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'unit',$3) ON CONFLICT DO NOTHING`, seed.ID("tenant-a"), seed.ID("tech-internal-a"), u)
	if code, key := events(&techInt); code != 403 || key != "errors.assignment_required" {
		t.Errorf("internal technician in scope without an assignment: %d %s", code, key)
	}
	job := assign(t, u, "tech-internal-a", clock.Add(time.Hour), clock.Add(2*time.Hour), "active")
	if code, key := events(&techInt); code != 403 || key != "errors.assignment_not_started" {
		t.Errorf("assignment starting later: %d %s", code, key)
	}
	revokeOthers := func(from, to time.Time) { // other tests' assignments must not overlap (assignments_no_overlap)
		owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND scheduled && tstzrange($2, $3) AND job_id <> $4`,
			seed.ID("tech-internal-a"), from, to, job)
	}
	revokeOthers(clock.Add(-2*time.Hour), clock.Add(-time.Hour))
	owner(t, `UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1`, job, clock.Add(-2*time.Hour), clock.Add(-time.Hour))
	if code, key := events(&techInt); code != 403 || key != "errors.assignment_ended" {
		t.Errorf("assignment window ended: %d %s", code, key)
	}
	revokeOthers(clock.Add(-time.Hour), clock.Add(time.Hour))
	owner(t, `UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1`, job, clock.Add(-time.Hour), clock.Add(time.Hour))
	if code, key := events(&techInt); code != 200 {
		t.Errorf("inside the window: %d %s", code, key)
	}
}
