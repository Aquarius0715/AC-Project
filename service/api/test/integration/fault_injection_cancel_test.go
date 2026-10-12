package integration

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestCancelRollsBackWhenTheStatusCannotBeWritten: a job whose cancellation cannot be written stays as it was. The
// operation fails as a server error, and the job stays on hold with its version unchanged, because the database
// rolls the transaction back (jobs.cancel; the status write fails). A job in progress cannot be cancelled, so the
// job starts on hold.
func TestCancelRollsBackWhenTheStatusCannotBeWritten(t *testing.T) {
	s := server(t)
	_, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET status = 'on_hold', hold_reason = 'parts' WHERE id = $1`, job)
	_, m = post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	version := ver(m)

	broken := injectAt(t, "UPDATE maintenance.jobs SET status = 'cancelled'")
	code, m := write(broken, &hq, "jobs.cancel", `{"jobId":"`+job+`","cancelReason":"customer withdrew"}`, version)
	if code < 500 {
		t.Fatalf("cancel with a failing status write: %d %v, want a server error", code, m)
	}
	_, m = post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	if data(m)["status"] != "on_hold" || ver(m) != version {
		t.Errorf("a failed cancel changed the job: %v", m)
	}
}
