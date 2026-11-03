package integration

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestHoldRollsBackWhenTheStatusCannotBeWritten: a job that cannot be put on hold keeps its status and reason. The
// operation fails as a server error, and the job stays in progress with no hold reason, because the database rolls the
// transaction back (jobs.hold; the status write fails).
func TestHoldRollsBackWhenTheStatusCannotBeWritten(t *testing.T) {
	srv := server(t)
	_, res := write(
		srv,
		&customerA,
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	job := data(res)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET status = 'in_progress' WHERE id = $1`, job)
	_, res = post(srv, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	version := ver(res)

	broken := injectAt(t, "UPDATE maintenance.jobs SET status = 'on_hold'")

	code, res := write(
		broken,
		&hq,
		"jobs.hold",
		`{"jobId":"`+job+`","reason":"waiting for parts"}`,
		version,
	)
	if code < 500 {
		t.Fatalf("hold with a failing status write: %d %v, want a server error", code, res)
	}

	_, res = post(srv, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	if data(res)["status"] != "in_progress" || data(res)["holdReason"] != nil ||
		ver(res) != version {
		t.Errorf("a failed hold changed the job: %v", res)
	}
}
