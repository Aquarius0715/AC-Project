package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// rejectCommand reports a requested or sent command as rejected by the device. In one process it is the demo trigger
// command_fail (the wiring of demo.go); with the split services the shared demo server's clock is ahead of the
// assignment the test made, so the device path is applied as ackCommand does: the Command fails and the run follows.
func rejectCommand(t *testing.T, srv *apiserver.Server, commandID string, when time.Time) {
	t.Helper()

	if !clusterMode() {
		code, resp := write(
			srv,
			customerB(),
			"demo.trigger",
			`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+when.Format(
				time.RFC3339,
			)+
				`","eventType":"command_fail","commandId":"`+commandID+`","deviceId":"`+deviceOf(t, commandID)+`"}`,
			0,
		)
		if code != 200 {
			t.Fatalf("command_fail: %d %v", code, resp)
		}

		return
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	id := uuid.MustParse(commandID)

	beginErr := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(
			ctx,
			`UPDATE control.commands SET status = 'failed', failure_code = $2 WHERE id = $1 AND status IN ('requested','sent')`,
			id,
			codeDeviceRejected,
		)
		if execErr != nil {
			return apperr.Wrap(execErr, "update control.commands")
		}

		return control.RunCommandFailed(ctx, tx, id, codeDeviceRejected, when)
	})
	if beginErr != nil {
		t.Fatal(beginErr)
	}
}

// A device that rejects the start or the end command of a test run ends the run as start_failed / end_failed with the
// device's code (implementation-contracts.md, the DiagnosticRun table), and the unit is free again (D05).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDiagnosticRunDeviceRejection(t *testing.T) {
	srv := serverWith(t, !clusterMode())
	unit := seed.ID("unit-non-rto").String()
	clean := func() {
		owner(
			t,
			`UPDATE control.diagnostic_runs SET state = 'completed' WHERE unit_id = $1 AND state IN ('awaiting_start','running',
				'end_requested')`,
			unit,
		)
		owner(
			t,
			`UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`,
			unit,
		)
	}
	clean()
	t.Cleanup(clean)

	job := assignedNow(t, srv)
	_, resp := post(srv, hq(), "units.get", `{"id":"`+unit+`"}`)
	unitVersion := ver(t, resp)
	_, resp = post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)
	jobVersion := ver(t, resp)

	create := func() (string, string) {
		code, reply := write(
			srv,
			techInt(),
			"diagnosticRuns.create",
			`{"jobId":"`+job+`","unitId":"`+unit+`","startAction":`+actionPowerOn+`,"endAction":`+actionPowerOff+
				`,"durationMinutes":1,"reason":"check cooling","expectedUnitVersion":`+itoa(
				unitVersion,
			)+
				`,"expectedJobVersion":`+itoa(
				jobVersion,
			)+`}`,
			0,
		)
		if code != 200 {
			t.Fatalf("create: %d %v", code, reply)
		}

		return asStr(t, data(reply)["id"]), asStr(t, data(reply)["startCommandId"])
	}
	state := func(run string) map[string]any {
		_, reply := post(srv, hq(), "diagnosticRuns.get", `{"diagnosticRunId":"`+run+`"}`)

		return data(reply)
	}

	// the start command is rejected
	run, start := create()
	rejectCommand(t, srv, start, clock().Add(2*time.Second))

	if got := state(
		run,
	); got["state"] != "start_failed" ||
		got["failureCode"] != codeDeviceRejected {
		t.Fatalf("a rejected start: %v", got)
	}

	// the unit takes the next run
	run, start = create()
	if !ackCommand(t, start, clock().Add(3*time.Second)) {
		t.Fatal("start ack")
	}

	_, err := schedTick(context.Background(), srv, clock().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	end := state(run)
	if end["state"] != "end_requested" || end["endCommandId"] == nil {
		t.Fatalf("end requested: %v", end)
	}

	// the end command is rejected
	rejectCommand(t, srv, asStr(t, end["endCommandId"]), clock().Add(2*time.Minute+2*time.Second))

	if got := state(
		run,
	); got["state"] != stateEndFailed ||
		got["failureCode"] != codeDeviceRejected {
		t.Fatalf("a rejected end: %v", got)
	}
}
