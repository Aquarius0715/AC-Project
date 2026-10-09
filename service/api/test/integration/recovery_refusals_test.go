package integration

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
func TestRestrictionRecoveryRefusals(t *testing.T) {
	s := serverWith(t, true)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	_, sess := post(s, &customerB, "session.get", `{}`) // the business time of the server (a shared demo cluster may run ahead of clock)
	now, _ := time.Parse(time.RFC3339Nano, sess["meta"].(map[string]any)["snapshotAt"].(string))
	at := now.Format(time.RFC3339)
	if code, m := write(s, &customerB, "automations.save", `{"name":"cool","unitIds":["`+u+`"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,"kind":"event","condition":{"type":"occupancy","occupied":true},"action":{"kind":"set_temperature","celsius":20}}`, 0); code != 200 {
		t.Fatalf("rule: %d %v", code, m)
	}
	reason := func() string {
		t.Helper()
		code, m := post(s, &customerB, "automations.simulate", `{"eventId":"`+uuid.NewString()+`","occurredAt":"`+at+`","unitIds":["`+u+`"],"facts":[{"unitId":"`+u+`","metric":"occupied","value":true,"unit":"boolean","observedAt":"`+at+`","quality":"valid"}],"phase":"condition"}`)
		if code != 200 {
			t.Fatalf("simulate: %d %v", code, m)
		}
		r, _ := data(m)["results"].([]any)[0].(map[string]any)["reason"].(string)
		return r
	}
	id, m := executed(t, s, k, inv, []string{u})
	ackCommand(t, cmdOf(m, u, "applyCommandIds"), clock.Add(time.Second))
	get := func() map[string]any { _, m := post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`); return m }
	if data(get())["state"] != "applied" {
		t.Fatal("applied")
	}
	if r := reason(); r != "restricted" {
		t.Errorf("a setpoint below the restriction minimum: %q", r)
	}
	_, m = write(s, &restrMgr, "restrictions.cancel", `{"restrictionId":"`+id+`","reason":"plan agreed"}`, ver(get()))
	ackCommand(t, cmdOf(m, u, "releaseCommandIds"), clock.Add(2*time.Second))
	if data(get())["state"] != "released" {
		t.Fatal("released")
	}
	observe := func(observed string) {
		t.Helper()
		body := `{"scenarioId":"t","eventId":"` + uuid.NewString() + `","occurredAt":"` + at + `","eventType":"restriction_observation","restrictionId":"` + id + `","unitId":"` + u + `","observed":{"restrictionId":"` + observed +
			`","rulesVersion":"rules-1","policy":{"kind":"temperature_limit","minimumCoolingSetpoint":24},"observedAt":"` + at + `"}}`
		if code, m := write(s, &customerB, "demo.trigger", body, 0); code != 200 {
			t.Fatalf("observation: %d %v", code, m)
		}
	}
	caseState := func() string {
		cases := data(get())["recoveryCases"].([]any)
		return cases[len(cases)-1].(map[string]any)["state"].(string)
	}
	observe(id)
	if caseState() != "pending" {
		t.Fatalf("recovery case: %v", get())
	}
	if r := reason(); r != "reconciliation_required" {
		t.Errorf("control while a case is open: %q", r)
	}
	reconcile := func() (int, map[string]any) {
		return write(s, &restrMgr, "restrictions.reconcile", `{"restrictionId":"`+id+`","unitIds":["`+u+`"]}`, ver(get()))
	}
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE unit_id = $1`, u)
	if code, m := reconcile(); code != 409 || m["code"] != "OFFLINE" || m["messageKey"] != "errors.device_offline" {
		t.Errorf("device offline: %d %v", code, m)
	}
	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, u)
	owner(t, `UPDATE assets.units SET last_seen_at = $2 WHERE id = $1`, u, now.Add(-31*time.Second))
	if code, m := reconcile(); code != 504 || m["messageKey"] != "errors.observation_stale" {
		t.Errorf("observation older than 30 s: %d %v", code, m)
	}
	observe(uuid.NewString()) // the device now reports a restriction that is neither this one nor its successor
	if code, m := reconcile(); code != 409 || m["messageKey"] != "errors.observation_mismatch" {
		t.Errorf("another restriction observed: %d %v", code, m)
	}
	observe(id)
	var dev string
	ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{u}, &dev)
	op := uuid.NewString()
	owner(t, `INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, expires_at) VALUES ($1,$2,$3,$4,'check','queued',$5)`, op, seed.ID("tenant-a"), dev, u, now.Add(time.Minute))
	if code, m := reconcile(); code != 409 || m["messageKey"] != "errors.unit_busy" {
		t.Errorf("remove command not deliverable during a device operation: %d %v", code, m)
	}
	owner(t, `DELETE FROM devices.device_operations WHERE id = $1`, op)
	code, m := reconcile()
	cases := data(m)["recoveryCases"].([]any)
	if code != 200 || cases[len(cases)-1].(map[string]any)["state"] != "removing" {
		t.Fatalf("reconcile: %d %v", code, m)
	}
	cmds := cases[len(cases)-1].(map[string]any)["commandIds"].([]any)
	cmd := cmds[len(cmds)-1].(string)
	if code, m := write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock.Add(3*time.Second).Format(time.RFC3339)+`","eventType":"command_fail","commandId":"`+cmd+`"}`, 0); code != 200 {
		t.Fatalf("command_fail: %d %v", code, m)
	}
	if caseState() != "pending" {
		t.Fatalf("a failed remove returns the case to pending: %v", get())
	}
	code, m = reconcile()
	cases = data(m)["recoveryCases"].([]any)
	if code != 200 || cases[len(cases)-1].(map[string]any)["state"] != "removing" || len(cases[len(cases)-1].(map[string]any)["commandIds"].([]any)) != 2 {
		t.Fatalf("reconcile again: %d %v", code, m)
	}
	cmds = cases[len(cases)-1].(map[string]any)["commandIds"].([]any)
	ackCommand(t, cmds[len(cmds)-1].(string), clock.Add(5*time.Second))
	if caseState() != "resolved" {
		t.Fatalf("resolved after the acknowledgement: %v", get())
	}
	if r := reason(); r != "" && r != "no_match" {
		t.Errorf("control after the case is resolved: %q", r)
	}
}
