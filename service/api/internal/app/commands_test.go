package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/modules/control"
	"github.com/pradita/ac-project/service/core/scheduler"
	"github.com/pradita/ac-project/service/core/seed"
)

func TestCommands(t *testing.T) {
	s := server(t)
	unit := seed.ID("unit-online-rto").String()
	owner(t, `UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`, unit)
	owner(t, `UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id IN (SELECT restriction_id FROM restrictions.restriction_units WHERE unit_id = $1)`, unit)
	_, m := post(s, &customerA, "units.get", `{"id":"`+unit+`"}`)
	v := ver(m)
	cmd := func(a *actor, action, extra string, uv int) (int, map[string]any) {
		return write(s, a, "commands.create", `{"unitId":"`+unit+`","action":`+action+`,"expectedUnitVersion":`+itoa(uv)+extra+`}`, 0)
	}
	for name, tc := range map[string]struct {
		a      *actor
		action string
		extra  string
		code   int
	}{
		"bad action":         {&customerA, `{"kind":"set_temperature"}`, "", 422},
		"out of range":       {&customerA, `{"kind":"set_temperature","celsius":35}`, "", 422},
		"client reason":      {&customerA, `{"kind":"set_power","power":true}`, `,"reason":"x"`, 422},
		"hq without reason":  {&hq, `{"kind":"set_power","power":true}`, "", 422},
		"other customer":     {&customerB, `{"kind":"set_power","power":true}`, "", 404},
		"technician no job":  {&techInt, `{"kind":"set_power","power":true}`, `,"reason":"test"`, 422},
		"stale unit version": {&customerA, `{"kind":"set_power","power":true}`, "", 409},
	} {
		uv := v
		if name == "stale unit version" {
			uv = v + 5
		}
		if code, _ := cmd(tc.a, tc.action, tc.extra, uv); code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}
	code, m := cmd(&customerA, `{"kind":"set_temperature","celsius":24}`, "", v)
	if code != 200 || data(m)["status"] != "sent" || data(m)["delivery"] != "sent" || data(m)["expiresAt"] != clock.Add(30*time.Second).Format(time.RFC3339) {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, m := cmd(&hq, `{"kind":"set_power","power":false}`, `,"reason":"check"`, v); code != 409 || m["messageKey"] != "errors.unit_busy" {
		t.Errorf("busy: %d %v", code, m)
	}
	if code, _ := post(s, &customerA, "commands.get", `{"id":"`+id+`"}`); code != 200 {
		t.Error("client get")
	}
	if code, _ := post(s, &customerB, "commands.get", `{"id":"`+id+`"}`); code != 404 {
		t.Error("other customer get")
	}
	if code, _ := post(s, &techB, "commands.get", `{"id":"`+id+`"}`); code != 404 {
		t.Error("out-of-scope technician get")
	}
	if code, _ := post(s, &hq, "commands.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown command")
	}
	// acknowledgement before expiry; late acknowledgement is only recorded
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	var ok bool
	keepObserved(t, id)
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		ok, err = control.Acknowledge(ctx, tx, uuid.MustParse(id), clock.Add(5*time.Second))
		return err
	}); err != nil || !ok {
		t.Fatalf("ack: %v %v", ok, err)
	}
	if _, m := post(s, &hq, "commands.get", `{"id":"`+id+`"}`); data(m)["status"] != "acknowledged" {
		t.Fatal("acknowledged")
	}
	// expiry by the worker
	code, m = cmd(&hq, `{"kind":"set_mode","mode":"dry"}`, `,"reason":"check"`, v)
	if code != 200 {
		t.Fatalf("second command: %d %v", code, m)
	}
	id2 := data(m)["id"].(string)
	if _, err := scheduler.Tick(ctx, s.DB, clock.Add(31*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, m := post(s, &hq, "commands.get", `{"id":"`+id2+`"}`); data(m)["status"] != "expired" || data(m)["failureCode"] != "TIMEOUT" {
		t.Fatalf("expired: %v", m)
	}
	keepObserved(t, id2)
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		ok, err = control.Acknowledge(ctx, tx, uuid.MustParse(id2), clock.Add(40*time.Second))
		return err
	}); err != nil || ok {
		t.Fatalf("late ack must not succeed: %v %v", ok, err)
	}
	// restriction table and offline delivery
	rid := uuid.NewString()
	owner(t, `INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version, notice_at, execute_after, reason, policy, state)
		VALUES ($1,$2,$3,1,$4,'r',$5,$6,'demo','{"kind":"temperature_limit","minimumCoolingSetpoint":26}','applied')`, rid, seed.ID("tenant-a"), uuid.New(), seed.ID("cust-a"), clock, clock.Add(25*time.Hour))
	owner(t, `INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state) VALUES ($1,$2,$3,'applied','none')`, seed.ID("tenant-a"), rid, unit)
	t.Cleanup(func() { owner(t, `UPDATE restrictions.restrictions SET state = 'cancelled' WHERE id = $1`, rid) })
	if code, m := cmd(&customerA, `{"kind":"set_temperature","celsius":24}`, "", v); code != 403 || m["messageKey"] != "errors.restriction_active" {
		t.Errorf("restricted temperature: %d %v", code, m)
	}
	owner(t, `UPDATE devices.devices SET power_signal = 'off' WHERE id = $1`, seed.ID("device-online-rto"))
	t.Cleanup(func() {
		owner(t, `UPDATE devices.devices SET power_signal = 'on' WHERE id = $1`, seed.ID("device-online-rto"))
	})
	if code, m := cmd(&customerA, `{"kind":"set_temperature","celsius":27}`, "", v); code != 409 || m["code"] != "OFFLINE" || m["messageKey"] != "errors.device_power_lost" {
		t.Errorf("power lost: %d %v", code, m)
	}
	// IR46 table
	for _, tc := range []struct {
		action string
		policy string
		ok     bool
	}{
		{`{"kind":"set_power","power":true}`, `{"kind":"power_off"}`, false},
		{`{"kind":"set_power","power":false}`, `{"kind":"power_off"}`, true},
		{`{"kind":"set_mode","mode":"cool"}`, `{"kind":"power_off"}`, false},
		{`{"kind":"set_temperature","celsius":26}`, `{"kind":"temperature_limit","minimumCoolingSetpoint":26}`, true},
		{`{"kind":"set_temperature","celsius":25}`, `{"kind":"temperature_limit","minimumCoolingSetpoint":26}`, false},
		{`{"kind":"set_fan","fanLevel":"high"}`, `{"kind":"temperature_limit","minimumCoolingSetpoint":26}`, true},
		{`{"kind":"ventilate","level":"low"}`, ``, true},
	} {
		a, _ := control.ParseAction([]byte(tc.action))
		if got := control.Allowed(a, []byte(tc.policy)); got != tc.ok {
			t.Errorf("%s under %s: %v", tc.action, tc.policy, got)
		}
	}
}
