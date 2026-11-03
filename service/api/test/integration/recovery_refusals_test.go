package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestRestrictionRecoveryRefusals covers the SR26 reconcile refusals of a terminal restriction (device offline,
// stale observation, another restriction observed, a device operation blocking the remove command), the return of
// a removing case to pending when its remove command fails, and the automation skip reasons an applied restriction
// (restricted) and an unresolved recovery case (reconciliation_required) give.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionRecoveryRefusals(t *testing.T) {
	srv := serverWith(t, true)
	unit := boundUnit(t, srv, "online")
	key, inv := overdueContract(t, srv, unit)
	_, sess := post(
		srv,
		customerB(),
		"session.get",
		`{}`,
	) // the business time of the server (a shared demo cluster may run ahead of clock)
	now, _ := time.Parse(time.RFC3339Nano, asStr(t, asObj(t, sess["meta"])["snapshotAt"]))
	stamp := now.Format(time.RFC3339)

	if code, resp := write(
		srv,
		customerB(),
		"automations.save",
		`{"name":"cool",`+
			`"unitIds":["`+unit+`"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,"kind":"event",`+
			`"condition":{"type":"occupancy","occupied":true},"action":{"kind":"set_temperature","celsius":20}}`,
		0,
	); code != 200 {
		t.Fatalf("rule: %d %v", code, resp)
	}

	reason := func() string {
		t.Helper()

		code, resp := post(
			srv,
			customerB(),
			"automations.simulate",
			`{"eventId":"`+uuid.NewString()+`","occurredAt":"`+stamp+`","unitIds":["`+unit+`"],"facts":[{"unitId":"`+unit+
				`","metric":"occupied","value":true,"unit":"boolean","observedAt":"`+stamp+
				`","quality":"valid"}],"phase":"condition"}`,
		)
		if code != 200 {
			t.Fatalf("simulate: %d %v", code, resp)
		}

		r, _ := data(resp)["results"].([]any)[0].(map[string]any)["reason"].(string)

		return r
	}
	restrictionID, res := executed(t, srv, key, inv, []string{unit})
	ackCommand(t, cmdOf(t, res, unit, "applyCommandIds"), clock().Add(time.Second))

	get := func() map[string]any {
		_, m := post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)

		return m
	}
	if data(get())["state"] != statusApplied {
		t.Fatal("applied")
	}

	if r := reason(); r != "restricted" {
		t.Errorf("a setpoint below the restriction minimum: %q", r)
	}

	_, res = write(
		srv,
		restrMgr(),
		"restrictions.cancel",
		`{"restrictionId":"`+restrictionID+`","reason":"plan agreed"}`,
		ver(t, get()),
	)
	ackCommand(t, cmdOf(t, res, unit, "releaseCommandIds"), clock().Add(2*time.Second))

	if data(get())["state"] != statusReleased {
		t.Fatal("released")
	}

	observe := func(observed string) {
		t.Helper()

		body := `{"scenarioId":"t",` +
			`"eventId":"` + uuid.NewString() + `","occurredAt":"` + stamp + `","eventType":"restriction_observation",` +
			`"restrictionId":"` + restrictionID + `","unitId":"` + unit + `","observed":{"restrictionId":"` + observed +
			`","rulesVersion":"rules-1","policy":{"kind":"temperature_limit","minimumCoolingSetpoint":24},` +
			`"observedAt":"` + stamp + `"}}`
		if code, m := write(srv, customerB(), "demo.trigger", body, 0); code != 200 {
			t.Fatalf("observation: %d %v", code, m)
		}
	}
	caseState := func() string {
		cases := asArr(t, data(get())["recoveryCases"])

		return asStr(t, asObj(t, cases[len(cases)-1])["state"])
	}

	observe(restrictionID)

	if caseState() != statusPending {
		t.Fatalf("recovery case: %v", get())
	}

	if r := reason(); r != "reconciliation_required" {
		t.Errorf("control while a case is open: %q", r)
	}

	reconcile := func() (int, map[string]any) {
		return write(
			srv,
			restrMgr(),
			"restrictions.reconcile",
			`{"restrictionId":"`+restrictionID+`","unitIds":["`+unit+`"]}`,
			ver(t, get()),
		)
	}

	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE unit_id = $1`, unit)

	if code, m := reconcile(); code != 409 || m["code"] != codeOffline ||
		m["messageKey"] != "errors.device_offline" {
		t.Errorf("device offline: %d %v", code, m)
	}

	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, unit)
	owner(
		t,
		`UPDATE assets.units SET last_seen_at = $2 WHERE id = $1`,
		unit,
		now.Add(-31*time.Second),
	)

	if code, m := reconcile(); code != 504 || m["messageKey"] != "errors.observation_stale" {
		t.Errorf("observation older than 30 s: %d %v", code, m)
	}

	observe(
		uuid.NewString(),
	) // the device now reports a restriction that is neither this one nor its successor

	if code, m := reconcile(); code != 409 || m["messageKey"] != "errors.observation_mismatch" {
		t.Errorf("another restriction observed: %d %v", code, m)
	}

	observe(restrictionID)

	var dev string
	ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{unit}, &dev)

	operation := uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, expires_at) VALUES ($1,$2,$3,
			$4,'check','queued',$5)`,
		operation,
		seed.ID("tenant-a"),
		dev,
		unit,
		now.Add(time.Minute),
	)

	if code, m := reconcile(); code != 409 || m["messageKey"] != keyUnitBusy {
		t.Errorf("remove command not deliverable during a device operation: %d %v", code, m)
	}

	owner(t, `DELETE FROM devices.device_operations WHERE id = $1`, operation)

	code, res := reconcile()

	cases := asArr(t, data(res)["recoveryCases"])
	if code != 200 || asObj(t, cases[len(cases)-1])["state"] != statusRemoving {
		t.Fatalf("reconcile: %d %v", code, res)
	}

	cmds := asArr(t, asObj(t, cases[len(cases)-1])["commandIds"])

	cmd := asStr(t, cmds[len(cmds)-1])
	if code, resp := write(
		srv,
		customerB(),
		"demo.trigger",
		`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock().Add(3*time.Second).
			Format(time.RFC3339)+
			`","eventType":"command_fail","commandId":"`+cmd+`"}`,
		0,
	); code != 200 {
		t.Fatalf("command_fail: %d %v", code, resp)
	}

	if caseState() != statusPending {
		t.Fatalf("a failed remove returns the case to pending: %v", get())
	}

	code, res = reconcile()

	cases = asArr(t, data(res)["recoveryCases"])
	if code != 200 || asObj(t, cases[len(cases)-1])["state"] != statusRemoving ||
		len(asArr(t, asObj(t, cases[len(cases)-1])["commandIds"])) != 2 {
		t.Fatalf("reconcile again: %d %v", code, res)
	}

	cmds = asArr(t, asObj(t, cases[len(cases)-1])["commandIds"])
	ackCommand(t, asStr(t, cmds[len(cmds)-1]), clock().Add(5*time.Second))

	if caseState() != statusResolved {
		t.Fatalf("resolved after the acknowledgement: %v", get())
	}

	if r := reason(); r != "" && r != reasonNoMatch {
		t.Errorf("control after the case is resolved: %q", r)
	}
}
