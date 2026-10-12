package integration

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestOtherTechnicianCannotReadAJob: a job assigned to another technician is not visible to a technician without a
// delegation: the read is NOT_FOUND, so neither the job nor its assignment reason (why the other technician could not
// do it) leaks (maintenance jobs.get; the object is out of scope, D01).
func TestOtherTechnicianCannotReadAJob(t *testing.T) {
	s := server(t)
	unit := seed.ID("unit-non-rto").String()
	from, until := clock.Add(30*time.Hour), clock.Add(32*time.Hour)
	job := assign(t, unit, "tech-external-b", from, until, "revoked")
	owner(t, `UPDATE maintenance.assignments SET reason = 'Could not reach the site' WHERE job_id = $1`, job)
	code, m := post(s, &techA, "jobs.get", `{"jobId":"`+job+`"}`)
	if code != 404 || m["messageKey"] != "error.notFound" {
		t.Fatalf("another technician reads a job: %d %v, want 404 NOT_FOUND", code, m)
	}
}
