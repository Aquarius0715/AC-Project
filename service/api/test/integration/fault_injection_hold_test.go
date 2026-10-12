package integration

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestHoldRollsBackWhenTheStatusCannotBeWritten: a job that cannot be put on hold keeps its status and reason. The
// operation fails as a server error, and the job stays in progress with no hold reason, because the database rolls the
// transaction back (jobs.hold; the status write fails).
func TestHoldRollsBackWhenTheStatusCannotBeWritten(t *testing.T) {
	s := server(t)
	_, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET status = 'in_progress' WHERE id = $1`, job)
	_, m = post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	version := ver(m)

	broken := injectAt(t, "UPDATE maintenance.jobs SET status = 'on_hold'")
	code, m := write(broken, &hq, "jobs.hold", `{"jobId":"`+job+`","reason":"waiting for parts"}`, version)
	if code < 500 {
		t.Fatalf("hold with a failing status write: %d %v, want a server error", code, m)
	}
	_, m = post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	if data(m)["status"] != "in_progress" || data(m)["holdReason"] != nil || ver(m) != version {
		t.Errorf("a failed hold changed the job: %v", m)
	}
}
