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
	srv := server(t)
	_, res := write(
		srv,
		&customerA,
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	job := data(res)["id"].(string)
	owner(
		t,
		`UPDATE maintenance.jobs SET status = 'on_hold', hold_reason = 'parts' WHERE id = $1`,
		job,
	)
	_, res = post(srv, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	version := ver(res)

	broken := injectAt(t, "UPDATE maintenance.jobs SET status = 'cancelled'")

	code, res := write(
		broken,
		&hq,
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"customer withdrew"}`,
		version,
	)
	if code < 500 {
		t.Fatalf("cancel with a failing status write: %d %v, want a server error", code, res)
	}

	_, res = post(srv, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	if data(res)["status"] != "on_hold" || ver(res) != version {
		t.Errorf("a failed cancel changed the job: %v", res)
	}
}
