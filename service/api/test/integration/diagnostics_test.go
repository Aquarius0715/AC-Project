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
)

func ackCommand(t *testing.T, id string, when time.Time) bool {
	t.Helper()

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	keepObserved(t, id)
	// the gateway sends a requested command and reports it sent (command_sent) before the device answers
	_, execErr := conn.Exec(
		ctx,
		`UPDATE control.commands SET status = 'sent', delivery = 'sent', sent_at = $2 WHERE id = $1 AND status = 'requested'`,
		uuid.MustParse(id),
		when.Add(-time.Second),
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	var ok bool

	beginFuncErr := pgx.BeginFunc(
		ctx,
		conn,
		func(tx pgx.Tx) error {
			ok, err = control.Acknowledge(ctx, tx, uuid.MustParse(id), when)

			return apperr.Wrap(err, "control.Acknowledge")
		},
	)
	if beginFuncErr != nil {
		t.Fatal(beginFuncErr)
	}

	drainEvents(
		t,
	) // the device path is not an operation: apply the resulting events as the consumers would (IR184)

	return ok
}

func TestDiagnosticRuns(t *testing.T) {
	srv := server(t)
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
		owner(
			t,
			`UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id IN (SELECT restriction_id FROM
				restrictions.restriction_units WHERE unit_id = $1)`,
			unit,
		)
	}
	clean()
	t.Cleanup(clean)
	job := assignedNow(t, srv)
	_, resp := post(srv, hq(), "units.get", `{"id":"`+unit+`"}`)
	uv := ver(t, resp)
	_, resp = post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)
	jv := ver(t, resp)
	run := func(a *actor, start, end string, mins, u, j int) (int, map[string]any) {
		return write(
			srv,
			a,
			"diagnosticRuns.create",
			`{"jobId":"`+job+`","unitId":"`+unit+`","startAction":`+start+`,"endAction":`+end+`,"durationMinutes":`+itoa(
				mins,
			)+
				`,"reason":"check cooling","expectedUnitVersion":`+itoa(
				u,
			)+`,"expectedJobVersion":`+itoa(
				j,
			)+`}`,
			0,
		)
	}
	on, off := `{"kind":"set_power","power":true}`, `{"kind":"set_power","power":false}`
	refused := map[string]string{ // the field each VALIDATION case is refused for
		"16 minutes":     "durationMinutes=error.range",
		"bad end action": "endAction=error.invalid",
		"unsupported":    "startAction=error.unsupportedAction",
	}

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
		code, m := run(techInt(), tc.start, tc.end, tc.mins, tc.u, tc.j)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}

	if code, res := run(
		techInt(),
		on,
		`{"kind":"ventilate","level":"low"}`,
		5,
		uv,
		jv,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["endAction"] != apperr.KeyUnsupportedAction {
		t.Errorf("an end action the unit does not support: %d %v", code, res)
	}

	if code, _ := run(hq(), on, off, 5, uv, jv); code != 403 {
		t.Error("HQ creates runs")
	}

	code, resp := run(techInt(), on, off, 5, uv, jv)
	if code != 200 || data(resp)["state"] != "awaiting_start" {
		t.Fatalf("create: %d %v", code, resp)
	}

	r1 := asStr(t, data(resp)["id"])

	start := asStr(t, data(resp)["startCommandId"])

	if code, res := write(
		srv,
		techInt(),
		"commands.create",
		`{"unitId":"`+unit+`","jobId":"`+job+`","action":`+off+`,"reason":"x","expectedUnitVersion":`+itoa(
			uv,
		)+`}`,
		0,
	); code != 409 ||
		res["messageKey"] != "errors.unit_busy" {
		t.Errorf("busy during run: %d %v", code, res)
	}

	if !ackCommand(t, start, clock().Add(5*time.Second)) {
		t.Fatal("start ack")
	}

	_, resp = post(srv, techInt(), "diagnosticRuns.get", `{"diagnosticRunId":"`+r1+`"}`)
	if data(resp)["state"] != "running" ||
		data(resp)["endAt"] != clock().Add(5*time.Second+5*time.Minute).Format(time.RFC3339) {
		t.Fatalf("running: %v", resp)
	}

	ctx := context.Background()

	_, err := schedTick(ctx, srv, clock().Add(6*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	_, resp = post(srv, hq(), "diagnosticRuns.get", `{"diagnosticRunId":"`+r1+`"}`)
	if data(resp)["state"] != "end_requested" || data(resp)["endCommandId"] == nil {
		t.Fatalf("end requested: %v", resp)
	}

	if !ackCommand(
		t,
		asStr(t, data(resp)["endCommandId"]),
		clock().Add(6*time.Minute+3*time.Second),
	) {
		t.Fatal("end ack")
	}

	if _, res := post(
		srv,
		hq(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+r1+`"}`,
	); data(res)["state"] != "completed" {
		t.Fatalf("completed: %v", res)
	}
	// start never acknowledged → start_failed
	code, resp = run(techInt(), on, off, 5, uv, jv)
	if code != 200 {
		t.Fatalf("second run: %d %v", code, resp)
	}

	r2 := asStr(t, data(resp)["id"])

	_, err = schedTick(
		ctx,
		srv,
		clock().Add(36*time.Second),
	)
	if err != nil { // 30 s window + the 5 s grace (IR453)
		t.Fatal(err)
	}

	if _, res := post(
		srv,
		hq(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+r2+`"}`,
	); data(res)["state"] != "start_failed" ||
		data(res)["failureCode"] != "TIMEOUT" {
		t.Fatalf("start failed: %v", res)
	}
	// a restriction at the end blocks the end action
	code, resp = run(techInt(), off, on, 1, uv, jv)
	if code != 200 {
		t.Fatalf("third run: %d %v", code, resp)
	}

	r3 := asStr(t, data(resp)["id"])
	ackCommand(t, asStr(t, data(resp)["startCommandId"]), clock().Add(time.Second))

	rid := uuid.NewString()
	owner(
		t,
		`INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version,
			notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,1,$4,'r',$5,$6,'demo','{"kind":"power_off"}','applied')`,
		rid,
		seed.ID("tenant-a"),
		uuid.New(),
		seed.ID("cust-a"),
		clock(),
		clock().Add(25*time.Hour),
	)
	owner(
		t,
		`INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state) VALUES
			($1,$2,$3,'applied','none')`,
		seed.ID("tenant-a"),
		rid,
		unit,
	)

	if code, _ := run(techInt(), on, off, 5, uv, jv); code != 403 && code != 409 {
		t.Errorf("start action prohibited or busy: %d", code)
	}

	_, err = schedTick(ctx, srv, clock().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if _, res := post(
		srv,
		hq(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+r3+`"}`,
	); data(res)["state"] != "end_blocked" {
		t.Fatalf("end blocked: %v", res)
	}
	// the device offline at the end: the end action fails with OFFLINE and sends nothing
	owner(t, `UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id = $1`, rid)

	code, resp = run(techInt(), off, on, 1, uv, jv)
	if code != 200 {
		t.Fatalf("fourth run: %d %v", code, resp)
	}

	r4 := asStr(t, data(resp)["id"])
	ackCommand(t, asStr(t, data(resp)["startCommandId"]), clock().Add(time.Second))
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE unit_id = $1`, unit)

	_, err = schedTick(ctx, srv, clock().Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, unit)

	if _, res := post(
		srv,
		hq(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+r4+`"}`,
	); data(res)["state"] != "end_failed" || data(res)["failureCode"] != "OFFLINE" ||
		data(res)["endCommandId"] != nil {
		t.Fatalf("end offline: %v", res)
	}
	// the retry of the end (Figma Technician 425:474, IR351): once the device is back, the technician sends the end action
	// again as an ordinary command of the job (commands.create); the run stays end_failed
	_, resp = post(srv, hq(), "units.get", `{"id":"`+unit+`"}`)
	if code, res := write(
		srv,
		techInt(),
		"commands.create",
		`{"unitId":"`+unit+`","jobId":"`+job+`","action":`+on+`,"reason":"End of test run sent again: check cooling",`+
			`"expectedUnitVersion":`+itoa(
			ver(t, resp),
		)+`}`,
		0,
	); code != 200 || data(res)["source"] != "ui" || data(res)["jobId"] != job ||
		data(res)["diagnosticRunId"] != nil {
		t.Errorf("retry the end: %d %v", code, res)
	}

	if _, res := post(
		srv,
		hq(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+r4+`"}`,
	); data(res)["state"] != "end_failed" {
		t.Errorf("the run after the retry: %v", data(res)["state"])
	}
	// reads
	if _, res := post(
		srv,
		techInt(),
		"diagnosticRuns.list",
		`{"unitId":"`+unit+`","jobId":"`+job+`","query":{}}`,
	); len(
		items(t, res),
	) != 4 {
		t.Errorf("list: %d", len(items(t, res)))
	}
	// pages of two: the cursor binds unit and job, so the second page reads the third and fourth runs (IR223 fix)
	_, resp = post(
		srv,
		techInt(),
		"diagnosticRuns.list",
		`{"unitId":"`+unit+`","jobId":"`+job+`","query":{"limit":2}}`,
	)

	next, _ := data(resp)["nextCursor"].(string)
	if code, res := post(
		srv,
		techInt(),
		"diagnosticRuns.list",
		`{"unitId":"`+unit+`","jobId":"`+job+`","query":{"limit":2,"cursor":"`+next+`"}}`,
	); next == "" || code != 200 ||
		len(items(t, res)) != 2 {
		t.Errorf("second page: %d %v", code, res)
	}
	// a revoked Assignment keeps the runs of the job visible (IR139, IR187)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE job_id = $1`, job)

	if _, res := post(
		srv,
		techInt(),
		"diagnosticRuns.list",
		`{"unitId":"`+unit+`","jobId":"`+job+`","query":{}}`,
	); len(
		items(t, res),
	) != 4 {
		t.Errorf("list after revoke: %d", len(items(t, res)))
	}

	if code, _ := post(
		srv,
		techInt(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+r1+`"}`,
	); code != 200 {
		t.Errorf("get after revoke: %d", code)
	}

	if code, res := post(
		srv,
		techB(),
		"diagnosticRuns.list",
		`{"unitId":"`+unit+`","query":{}}`,
	); code != 200 ||
		len(items(t, res)) != 0 {
		t.Error("other technician lists runs")
	}

	if code, _ := post(
		srv,
		techB(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+r1+`"}`,
	); code != 404 &&
		code != 403 {
		t.Errorf("other technician get: %d", code)
	}

	if code, _ := post(
		srv,
		hq(),
		"diagnosticRuns.get",
		`{"diagnosticRunId":"`+uuid.NewString()+`"}`,
	); code != 404 {
		t.Error("unknown run")
	}

	if code, res := post(
		srv,
		hq(),
		"diagnosticRuns.list",
		`{"query":{}}`,
	); code != 422 ||
		fields(res) != "unitId=error.required" {
		t.Errorf("unitId required: %d %s", code, fields(res))
	}
}
