package integration_test

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// storedResult is writes.getResult of the caller's last stored write of operation whose result names target in field.
func storedResult(
	t *testing.T,
	srv *apiserver.Server,
	who *actor,
	operation, field, target string,
) map[string]any {
	t.Helper()

	var key string
	ownerScan(
		t,
		`SELECT key FROM platform.idempotency_keys WHERE membership_id = $1 AND operation = $2 AND status = 'completed'
			AND response->'data'->>$3 = $4 ORDER BY created_at DESC LIMIT 1`,
		[]any{seed.ID(who.membership), operation, field, target},
		&key,
	)

	code, res := post(
		srv,
		who,
		opWritesGetResult,
		`{"operation":"`+operation+`","idempotencyKey":"`+key+`"}`,
	)
	if code != 200 {
		t.Fatalf("writes.getResult %s: %d %v", operation, code, res)
	}

	return data(res)
}

// TestWriteResultRechecksTheJob: writes.getResult returns a technician's stored write on a job while the technician
// may read the job, and once the assignment is revoked only that it succeeded, without what it answered then
// (strict review SR14, IR724).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestWriteResultRechecksTheJob(t *testing.T) {
	srv := server(t)
	job := assignedNow(t, srv)

	var version int
	ownerScan(t, `SELECT version FROM maintenance.jobs WHERE id = $1`, []any{job}, &version)

	if code, res := write(
		srv,
		techInt(),
		opJobsAcknowledgeAssignment,
		`{"jobId":"`+job+`","decision":"accept"}`,
		version,
	); code != 200 {
		t.Fatalf("the technician accepts: %d %v", code, res)
	}

	if stored := storedResult(
		t,
		srv,
		techInt(),
		opJobsAcknowledgeAssignment,
		"jobId",
		job,
	); stored["state"] != stateSucceeded ||
		asObj(t, stored["result"])["jobId"] != job {
		t.Fatalf("assigned: %v", stored)
	}

	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE job_id = $1`, job)

	if code, _ := post(srv, techInt(), "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Fatalf("the revoked technician reads the job: %d", code)
	}

	stored := storedResult(t, srv, techInt(), opJobsAcknowledgeAssignment, "jobId", job)
	if stored["state"] != stateSucceeded || stored["operation"] != opJobsAcknowledgeAssignment ||
		stored["result"] != nil {
		t.Fatalf("revoked: %v, want succeeded without the result", stored)
	}
}
