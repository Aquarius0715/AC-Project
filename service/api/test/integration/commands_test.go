package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const actionLimitTo26 = `{"kind":"temperature_limit","minimumCoolingSetpoint":26}`

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCommands(t *testing.T) {
	srv := server(t)
	unit := seed.ID("unit-online-rto").String()
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
	_, resp := post(srv, customerA(), "units.get", `{"id":"`+unit+`"}`)
	v := ver(t, resp)
	cmd := func(a *actor, action, extra string, uv int) (int, map[string]any) {
		return write(
			srv,
			a,
			"commands.create",
			`{"unitId":"`+unit+`","action":`+action+`,"expectedUnitVersion":`+itoa(uv)+extra+`}`,
			0,
		)
	}
	refused := map[string]string{ // the field each VALIDATION case is refused for
		caseBadAction:       refusedActionInvalid,
		"client reason":     "reason=error.notAllowed",
		"hq voice":          "source=error.notAllowed",
		"hq without reason": refusedReasonRequired,
		"out of range":      refusedActionUnsupported,
		"technician no job": "jobId=error.required",
		"technician voice":  "source=error.notAllowed",
		"unknown source":    "source=error.invalid",
	}

	for name, tc := range map[string]struct {
		a      *actor
		action string
		extra  string
		code   int
	}{
		caseBadAction:        {customerA(), `{"kind":"set_temperature"}`, "", 422},
		"out of range":       {customerA(), `{"kind":"set_temperature","celsius":35}`, "", 422},
		"client reason":      {customerA(), actionPowerOn, `,"reason":"x"`, 422},
		"hq without reason":  {hq(), actionPowerOn, "", 422},
		caseOtherCustomer:    {customerB(), actionPowerOn, "", 404},
		"technician no job":  {techInt(), actionPowerOn, `,"reason":"test"`, 422},
		"stale unit version": {customerA(), actionPowerOn, "", 409},
		"unknown source":     {customerA(), actionPowerOn, `,"source":"automation"`, 422},
		"hq voice":           {hq(), actionPowerOn, `,"reason":"check","source":"voice"`, 422},
		"technician voice": {techInt(), `{"kind":"set_power",` +
			`"power":true}`, `,"reason":"test",` +
			`"jobId":"` + uuid.NewString() + `","source":"voice"`, 422},
	} {
		uv := v
		if name == "stale unit version" {
			uv = v + 5
		}

		code, m := cmd(tc.a, tc.action, tc.extra, uv)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}
	// the assistant's confirmed change (IR306): the same command, labelled voice
	code, resp := cmd(
		customerA(),
		`{"kind":"set_temperature","celsius":24}`,
		`,"source":"voice"`,
		v,
	)
	if code != 200 || data(resp)["status"] != statusRequested ||
		data(resp)["delivery"] != statusNotSent ||
		data(resp)["sentAt"] != nil ||
		data(resp)["source"] != "voice" ||
		data(resp)["expiresAt"] != clock().Add(30*time.Second).Format(time.RFC3339) {
		t.Fatalf("create: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])

	if code, res := cmd(
		hq(),
		`{"kind":"set_power","power":false}`,
		`,"reason":"check"`,
		v,
	); code != 409 ||
		res["messageKey"] != keyUnitBusy {
		t.Errorf("busy: %d %v", code, res)
	}

	if code, _ := post(srv, customerA(), "commands.get", `{"id":"`+id+`"}`); code != 200 {
		t.Error("client get")
	}

	if code, _ := post(srv, customerB(), "commands.get", `{"id":"`+id+`"}`); code != 404 {
		t.Error("other customer get")
	}

	if code, _ := post(srv, techB(), "commands.get", `{"id":"`+id+`"}`); code != 404 {
		t.Error("out-of-scope technician get")
	}

	if code, _ := post(srv, hq(), "commands.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown command")
	}
	// the IoT gateway sends the requested command and reports it sent (demo trigger command_sent, the same update)
	owner(
		t,
		`UPDATE control.commands SET status = 'sent', delivery = 'sent', sent_at = $2 WHERE id = $1 AND status = 'requested'`,
		id,
		clock(),
	)
	// acknowledgement before expiry; late acknowledgement is only recorded
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var ok bool

	keepObserved(t, id)

	beginFuncErr := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		ok, err = control.Acknowledge(ctx, tx, uuid.MustParse(id), clock().Add(5*time.Second))

		return apperr.Wrap(err, "control.Acknowledge")
	})
	if beginFuncErr != nil || !ok {
		t.Fatalf("ack: %v %v", ok, beginFuncErr)
	}

	if _, m := post(
		srv,
		hq(),
		"commands.get",
		`{"id":"`+id+`"}`,
	); data(m)["status"] != statusAcknowledged {
		t.Fatal("acknowledged")
	}
	// expiry by the worker
	code, resp = cmd(hq(), `{"kind":"set_mode","mode":"dry"}`, `,"reason":"check"`, v)
	if code != 200 {
		t.Fatalf("second command: %d %v", code, resp)
	}

	id2 := asStr(t, data(resp)["id"])
	// IR453: a tick just after expiresAt leaves the command for an acknowledgement still on its way; the grace after
	// it expires the command
	_, schedTickErr := schedTick(ctx, srv, clock().Add(32*time.Second))
	if schedTickErr != nil {
		t.Fatal(schedTickErr)
	}

	if _, m := post(
		srv,
		hq(),
		"commands.get",
		`{"id":"`+id2+`"}`,
	); data(m)["status"] == statusExpired {
		t.Fatalf("expired within the grace: %v", m)
	}

	_, schedTickErr = schedTick(
		ctx,
		srv,
		clock().Add(30*time.Second+control.ExpireGrace+time.Second),
	)
	if schedTickErr != nil {
		t.Fatal(schedTickErr)
	}

	if _, res := post(
		srv,
		hq(),
		"commands.get",
		`{"id":"`+id2+`"}`,
	); data(res)["status"] != statusExpired ||
		data(res)["failureCode"] != codeTimeout {
		t.Fatalf("expired: %v", res)
	}

	keepObserved(t, id2)

	beginFuncErr = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		ok, err = control.Acknowledge(ctx, tx, uuid.MustParse(id2), clock().Add(40*time.Second))

		return apperr.Wrap(err, "control.Acknowledge")
	})
	if beginFuncErr != nil || ok {
		t.Fatalf("late ack must not succeed: %v %v", ok, beginFuncErr)
	}
	// restriction table and offline delivery
	rid := uuid.NewString()
	owner(
		t,
		`INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version,
			notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,1,$4,'r',$5,$6,'demo','{"kind":"temperature_limit","minimumCoolingSetpoint":26}','applied')`,
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
	t.Cleanup(
		func() { owner(t, `UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id = $1`, rid) },
	)

	if code, res := cmd(
		customerA(),
		`{"kind":"set_temperature","celsius":24}`,
		"",
		v,
	); code != 403 ||
		res["messageKey"] != "errors.restriction_active" {
		t.Errorf("restricted temperature: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE devices.devices SET power_signal = 'off' WHERE id = $1`,
		seed.ID("device-online-rto"),
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE devices.devices SET power_signal = 'on' WHERE id = $1`,
			seed.ID("device-online-rto"),
		)
	})

	if code, res := cmd(
		customerA(),
		`{"kind":"set_temperature","celsius":27}`,
		"",
		v,
	); code != 409 || res["code"] != codeOffline ||
		res["messageKey"] != "errors.device_power_lost" {
		t.Errorf("power lost: %d %v", code, res)
	}
	// IR46 table
	for _, tc := range []struct {
		action string
		policy string
		ok     bool
	}{
		{actionPowerOn, policyPowerOff, false},
		{actionPowerOff, policyPowerOff, true},
		{`{"kind":"set_mode","mode":"cool"}`, policyPowerOff, false},
		{`{"kind":"set_temperature","celsius":26}`, actionLimitTo26, true},
		{actionCoolTo25, actionLimitTo26, false},
		{`{"kind":"set_fan","fanLevel":"high"}`, actionLimitTo26, true},
		{actionVentilateLow, ``, true},
		{actionPowerOff, `{"kind":`, false}, // a policy that cannot be read allows nothing
	} {
		a, _ := control.ParseAction([]byte(tc.action))
		if got := control.Allowed(a, []byte(tc.policy)); got != tc.ok {
			t.Errorf("%s under %s: %v", tc.action, tc.policy, got)
		}
	}
	// a unit that does not exist is NOT_FOUND to HQ and to a technician, for a command and for the history
	missing := uuid.NewString()
	if code, _ := write(
		srv,
		hq(),
		"commands.create",
		`{"unitId":"`+missing+`","action":{"kind":"set_power","power":true},"reason":"check","expectedUnitVersion":1}`,
		0,
	); code != 404 {
		t.Errorf("HQ command for an unknown unit: %d", code)
	}

	if code, _ := write(
		srv,
		techInt(),
		"commands.create",
		`{"unitId":"`+missing+`","jobId":"`+uuid.NewString()+`","action":{"kind":"set_power","power":true},`+
			`"reason":"check","expectedUnitVersion":1}`,
		0,
	); code != 404 {
		t.Errorf("technician command for an unknown unit: %d", code)
	}

	if code, _ := post(
		srv,
		hq(),
		"commands.list",
		`{"unitId":"`+missing+`","query":{}}`,
	); code != 404 {
		t.Errorf("history of an unknown unit: %d", code)
	}
}

// IR216: commands.list is the unit's Command history, newest first: a client sees its unit's commands with the
// reason of others' withheld; a technician sees one job it holds (jobId required); HQ sees every command and reason.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCommandHistory(t *testing.T) {
	srv := server(t)
	unit := seed.ID("unit-non-rto").String()
	free := func() {
		owner(
			t,
			`UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`,
			unit,
		)
	}
	free()
	owner(
		t,
		`UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id IN (SELECT restriction_id FROM
			restrictions.restriction_units WHERE unit_id = $1)`,
		unit,
	)
	owner(
		t,
		`UPDATE control.diagnostic_runs SET state = 'completed' WHERE unit_id = $1 AND state IN ('awaiting_start','running',
			'end_requested')`,
		unit,
	)
	job := assignedNow(t, srv)
	_, resp := post(srv, hq(), "units.get", `{"id":"`+unit+`"}`)
	uv := ver(t, resp)

	code, resp := write(
		srv,
		techInt(),
		"commands.create",
		`{"unitId":"`+unit+`","jobId":"`+job+`","action":{"kind":"set_mode","mode":"cool"},"reason":"airflow check",`+
			`"expectedUnitVersion":`+itoa(
			uv,
		)+`}`,
		0,
	)
	if code != 200 || data(resp)["source"] != "ui" || data(resp)["jobId"] != job {
		t.Fatalf("technician command: %d %v", code, resp)
	}

	tech := asStr(t, data(resp)["id"])

	free()

	code, resp = write(
		srv,
		customerA(),
		"commands.create",
		`{"unitId":"`+unit+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
			uv,
		)+`}`,
		0,
	)
	if code != 200 || data(resp)["source"] != "ui" {
		t.Fatalf("client command: %d %v", code, resp)
	}

	own := asStr(t, data(resp)["id"])

	free()
	// list reads the unit's history: rows is the first page (ordering checks), byID every page — the test database
	// keeps other tests' commands on this unit, some at later business times, which can push these off page one
	list := func(a *actor, body string) (int, map[string]map[string]any, []map[string]any) {
		t.Helper()

		code, res := post(srv, a, "commands.list", body)
		rows := items(t, res)
		byID := map[string]map[string]any{}

		for page := 0; ; page++ {
			for _, it := range items(t, res) {
				byID[asStr(t, it["id"])] = it
			}

			next, _ := data(res)["nextCursor"].(string)
			if code != 200 || next == "" || page == 50 {
				break
			}

			var input map[string]any

			_ = json.Unmarshal([]byte(body), &input)

			q, _ := input["query"].(map[string]any)
			if q == nil {
				q = map[string]any{}
			}

			q["cursor"] = next
			input["query"] = q
			b := jsonBytes(t, input)
			code, res = post(srv, a, "commands.list", string(b))
		}

		return code, byID, rows
	}

	code, got, rows := list(customerA(), `{"unitId":"`+unit+`","query":{"limit":100}}`)
	if code != 200 || got[tech] == nil || got[own] == nil || got[tech]["reason"] != nil ||
		got[tech]["status"] != statusExpired {
		t.Fatalf("client history: %d %v", code, got[tech])
	}

	for i := 1; i < len(rows); i++ {
		if asStr(t, rows[i-1]["requestedAt"]) < asStr(t, rows[i]["requestedAt"]) {
			t.Fatalf("newest first: %v before %v", rows[i-1]["requestedAt"], rows[i]["requestedAt"])
		}
	}
	// the next page of a history (limit 1): the cursor binds the unit and job, not the paging fields (IR223 fix)
	_, resp = post(srv, customerA(), "commands.list", `{"unitId":"`+unit+`","query":{"limit":1}}`)
	if next, _ := data(resp)["nextCursor"].(string); next == "" {
		t.Fatal("history of one per page has a next page")
	} else if code, m := post(srv, customerA(), "commands.list", `{"unitId":"`+unit+`","query":{"limit":1,`+
		`"cursor":"`+next+`"}}`); code != 200 || len(items(t, m)) != 1 {
		t.Fatalf("second page: %d %v", code, m)
	}

	if code, res := post(
		srv,
		techInt(),
		"commands.list",
		`{"unitId":"`+unit+`","query":{}}`,
	); code != 422 ||
		fields(res) != "jobId=error.required" {
		t.Errorf("technician without a job: %d %s", code, fields(res))
	}

	code, got, _ = list(techInt(), `{"unitId":"`+unit+`","jobId":"`+job+`","query":{"limit":100}}`)
	if code != 200 || got[tech] == nil || got[tech]["reason"] != "airflow check" ||
		got[own] != nil {
		t.Fatalf("technician job history: %d %v", code, got)
	}

	if code, _, _ := list(
		techInt(),
		`{"unitId":"`+unit+`","jobId":"`+uuid.NewString()+`","query":{}}`,
	); code != 404 {
		t.Errorf("technician, a job it does not hold: %d", code)
	}

	if code, _, _ := list(customerB(), `{"unitId":"`+unit+`","query":{}}`); code != 404 {
		t.Errorf("another customer: %d", code)
	}

	if _, got, _ := list(
		hq(),
		`{"unitId":"`+unit+`","query":{"limit":100}}`,
	); got[tech]["reason"] != "airflow check" ||
		got[own] == nil {
		t.Errorf("HQ history: %v", got[tech])
	}

	for q, want := range map[string]string{`{"sort":{"field":"status",` +
		`"direction":"asc"}}`: refusedSortFieldInvalid, `{"filters":{"status":"sent"}}`: refusedStatusNotAllowed} {
		code, m := post(srv, hq(), "commands.list", `{"unitId":"`+unit+`","query":`+q+`}`)
		refusal(t, q, code, m, want)
	}

	if code, _, _ := list(
		hq(),
		`{"unitId":"`+unit+`","query":{"sort":{"field":"requestedAt","direction":"asc"}}}`,
	); code != 200 {
		t.Errorf("requestedAt asc: %d", code)
	}
}

// TestCommandExpiryAudited (IR604): a command whose window passes without an answer is failed by the scheduler, and
// its request's audit trail ends with that result under the same correlation ID: pending, then failed (TIMEOUT).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCommandExpiryAudited(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")
	_, g := post(srv, customerB(), "units.get", `{"id":"`+unit+`"}`)

	code, res := write(
		srv,
		customerB(),
		"commands.create",
		`{"unitId":"`+unit+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
			ver(t, g),
		)+`}`,
		0,
	)
	if code != 200 {
		t.Fatalf("command: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE control.commands SET expires_at = requested_at + interval '1 second' WHERE id = $1`,
		data(res)["id"],
	)
	expireAt(t, clock().Add(time.Minute)) // the window passed with no answer

	if trail := commandTrail(
		t,
		srv,
		res,
	); len(trail) != 2 || trail[0] != "pending customer-b" ||
		trail[1] != "failed system-scheduler TIMEOUT" {
		t.Errorf("the expired command's audit trail: %v", trail)
	}
}
