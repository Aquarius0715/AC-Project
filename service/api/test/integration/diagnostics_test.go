package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/scheduler"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func ackCommand(t *testing.T, id string, at time.Time) bool {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	keepObserved(t, id)
	var ok bool
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { ok, err = control.Acknowledge(ctx, tx, uuid.MustParse(id), at); return err }); err != nil {
		t.Fatal(err)
	}
	drainEvents(t) // the device path is not an operation: apply the resulting events as the consumers would (IR184)
	return ok
}

func TestDiagnosticRuns(t *testing.T) {
	s := server(t)
	unit := seed.ID("unit-non-rto").String()
	clean := func() {
		owner(t, `UPDATE control.diagnostic_runs SET state = 'completed' WHERE unit_id = $1 AND state IN ('awaiting_start','running','end_requested')`, unit)
		owner(t, `UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`, unit)
		owner(t, `UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id IN (SELECT restriction_id FROM restrictions.restriction_units WHERE unit_id = $1)`, unit)
	}
	clean()
	t.Cleanup(clean)
	job := assignedNow(t, s)
	_, m := post(s, &hq, "units.get", `{"id":"`+unit+`"}`)
	uv := ver(m)
	_, m = post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	jv := ver(m)
	run := func(a *actor, start, end string, mins, u, j int) (int, map[string]any) {
		return write(s, a, "diagnosticRuns.create", `{"jobId":"`+job+`","unitId":"`+unit+`","startAction":`+start+`,"endAction":`+end+`,"durationMinutes":`+itoa(mins)+
			`,"reason":"check cooling","expectedUnitVersion":`+itoa(u)+`,"expectedJobVersion":`+itoa(j)+`}`, 0)
	}
	on, off := `{"kind":"set_power","power":true}`, `{"kind":"set_power","power":false}`
	for name, tc := range map[string]struct {
		start, end string
		mins, u, j int
		code       int
	}{
		"16 minutes":     {on, off, 16, uv, jv, 422},
		"unsupported":    {`{"kind":"ventilate","level":"low"}`, off, 5, uv, jv, 422},
		"stale unit":     {on, off, 5, uv + 3, jv, 409},
		"stale job":      {on, off, 5, uv, jv + 3, 409},
		"bad end action": {on, `{"kind":"dance"}`, 5, uv, jv, 422},
	} {
		if code, _ := run(&techInt, tc.start, tc.end, tc.mins, tc.u, tc.j); code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}
	if code, _ := run(&hq, on, off, 5, uv, jv); code != 403 {
		t.Error("HQ creates runs")
	}
	code, m := run(&techInt, on, off, 5, uv, jv)
	if code != 200 || data(m)["state"] != "awaiting_start" {
		t.Fatalf("create: %d %v", code, m)
	}
	r1 := data(m)["id"].(string)
	start := data(m)["startCommandId"].(string)
	if code, m := write(s, &techInt, "commands.create", `{"unitId":"`+unit+`","jobId":"`+job+`","action":`+off+`,"reason":"x","expectedUnitVersion":`+itoa(uv)+`}`, 0); code != 409 || m["messageKey"] != "errors.unit_busy" {
		t.Errorf("busy during run: %d %v", code, m)
	}
	if !ackCommand(t, start, clock.Add(5*time.Second)) {
		t.Fatal("start ack")
	}
	_, m = post(s, &techInt, "diagnosticRuns.get", `{"diagnosticRunId":"`+r1+`"}`)
	if data(m)["state"] != "running" || data(m)["endAt"] != clock.Add(5*time.Second+5*time.Minute).Format(time.RFC3339) {
		t.Fatalf("running: %v", m)
	}
	ctx := context.Background()
	if _, err := scheduler.Tick(ctx, s.DB, clock.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	_, m = post(s, &hq, "diagnosticRuns.get", `{"diagnosticRunId":"`+r1+`"}`)
	if data(m)["state"] != "end_requested" || data(m)["endCommandId"] == nil {
		t.Fatalf("end requested: %v", m)
	}
	if !ackCommand(t, data(m)["endCommandId"].(string), clock.Add(6*time.Minute+3*time.Second)) {
		t.Fatal("end ack")
	}
	if _, m := post(s, &hq, "diagnosticRuns.get", `{"diagnosticRunId":"`+r1+`"}`); data(m)["state"] != "completed" {
		t.Fatalf("completed: %v", m)
	}
	// start never acknowledged → start_failed
	code, m = run(&techInt, on, off, 5, uv, jv)
	if code != 200 {
		t.Fatalf("second run: %d %v", code, m)
	}
	r2 := data(m)["id"].(string)
	if _, err := scheduler.Tick(ctx, s.DB, clock.Add(31*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, m := post(s, &hq, "diagnosticRuns.get", `{"diagnosticRunId":"`+r2+`"}`); data(m)["state"] != "start_failed" || data(m)["failureCode"] != "TIMEOUT" {
		t.Fatalf("start failed: %v", m)
	}
	// a restriction at the end blocks the end action
	code, m = run(&techInt, off, on, 1, uv, jv)
	if code != 200 {
		t.Fatalf("third run: %d %v", code, m)
	}
	r3 := data(m)["id"].(string)
	ackCommand(t, data(m)["startCommandId"].(string), clock.Add(time.Second))
	rid := uuid.NewString()
	owner(t, `INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version, notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,1,$4,'r',$5,$6,'demo','{"kind":"power_off"}','applied')`, rid, seed.ID("tenant-a"), uuid.New(), seed.ID("cust-a"), clock, clock.Add(25*time.Hour))
	owner(t, `INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state) VALUES ($1,$2,$3,'applied','none')`, seed.ID("tenant-a"), rid, unit)
	if code, _ := run(&techInt, on, off, 5, uv, jv); code != 403 && code != 409 {
		t.Errorf("start action prohibited or busy: %d", code)
	}
	if _, err := scheduler.Tick(ctx, s.DB, clock.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, m := post(s, &hq, "diagnosticRuns.get", `{"diagnosticRunId":"`+r3+`"}`); data(m)["state"] != "end_blocked" {
		t.Fatalf("end blocked: %v", m)
	}
	// reads
	if _, m := post(s, &techInt, "diagnosticRuns.list", `{"unitId":"`+unit+`","jobId":"`+job+`","query":{}}`); len(items(m)) != 3 {
		t.Errorf("list: %d", len(items(m)))
	}
	// a revoked Assignment keeps the runs of the job visible (IR139, IR187)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE job_id = $1`, job)
	if _, m := post(s, &techInt, "diagnosticRuns.list", `{"unitId":"`+unit+`","jobId":"`+job+`","query":{}}`); len(items(m)) != 3 {
		t.Errorf("list after revoke: %d", len(items(m)))
	}
	if code, _ := post(s, &techInt, "diagnosticRuns.get", `{"diagnosticRunId":"`+r1+`"}`); code != 200 {
		t.Errorf("get after revoke: %d", code)
	}
	if _, m := post(s, &techB, "diagnosticRuns.list", `{"unitId":"`+unit+`","query":{}}`); len(items(m)) != 0 {
		t.Error("other technician lists runs")
	}
	if code, _ := post(s, &techB, "diagnosticRuns.get", `{"diagnosticRunId":"`+r1+`"}`); code != 404 && code != 403 {
		t.Errorf("other technician get: %d", code)
	}
	if code, _ := post(s, &hq, "diagnosticRuns.get", `{"diagnosticRunId":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown run")
	}
	if code, _ := post(s, &hq, "diagnosticRuns.list", `{"query":{}}`); code != 422 {
		t.Error("unitId required")
	}
}
