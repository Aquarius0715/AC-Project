package app

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRestrictionRecoveryCases(t *testing.T) {
	s := serverWith(t, true)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	id, m := executed(t, s, k, inv, []string{u})
	ackCommand(t, cmdOf(m, u, "applyCommandIds"), clock.Add(time.Second))
	get := func() map[string]any { _, m := post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`); return m }
	_, m = write(s, &restrMgr, "restrictions.cancel", `{"restrictionId":"`+id+`","reason":"plan agreed"}`, ver(get()))
	ackCommand(t, cmdOf(m, u, "releaseCommandIds"), clock.Add(2*time.Second))
	if data(get())["state"] != "released" {
		t.Fatal("released")
	}
	observe := func(observed string) {
		t.Helper()
		if code, m := write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock.Format(time.RFC3339)+
			`","eventType":"restriction_observation","restrictionId":"`+id+`","unitId":"`+u+`","observed":`+observed+`}`, 0); code != 200 {
			t.Fatalf("observation: %d %v", code, m)
		}
	}
	old := `{"restrictionId":"` + id + `","rulesVersion":"rules-1","policy":{"kind":"temperature_limit","minimumCoolingSetpoint":24},"observedAt":"` + clock.Format(time.RFC3339) + `"}`
	observe(old)
	m = get()
	cases := data(m)["recoveryCases"].([]any)
	if data(m)["state"] != "released" || len(cases) != 1 || cases[0].(map[string]any)["state"] != "pending" {
		t.Fatalf("recovery case: %v", m)
	}
	var alerts int
	ownerScan(t, `SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND type = 'reconciliation_required' AND status = 'open'`, []any{u}, &alerts)
	if alerts != 1 {
		t.Errorf("reconciliation alert %d", alerts)
	}
	observe(old) // the same observation updates the open case
	if len(data(get())["recoveryCases"].([]any)) != 1 {
		t.Error("one case per unit / restriction / rules version")
	}
	if code, _ := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, nil), 0); code != 409 {
		t.Error("an unresolved case blocks new restrictions")
	}
	if _, m := post(s, &hq, "units.get", `{"id":"`+u+`"}`); data(m)["controlAvailability"].(map[string]any)["state"] != "blocked" {
		t.Errorf("control blocked while a case is open: %v", data(m)["controlAvailability"])
	}
	if _, g := post(s, &hq, "units.get", `{"id":"`+u+`"}`); true {
		if code, m := write(s, &hq, "commands.create", `{"unitId":"`+u+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(ver(g))+`,"reason":"check"}`, 0); code != 409 || m["messageKey"] != "errors.reconciliation_required" {
			t.Errorf("manual control while blocked: %d %v", code, m)
		}
	}
	reconcile := func(a *actor) (int, map[string]any) {
		return write(s, a, "restrictions.reconcile", `{"restrictionId":"`+id+`","unitIds":["`+u+`"]}`, ver(get()))
	}
	if code, _ := reconcile(&hq); code != 403 {
		t.Error("reconcile without restriction permissions")
	}
	code, m := reconcile(&overrider) // override-only may handle terminal recovery (IR03)
	c0, _ := data(m)["recoveryCases"].([]any)
	if code != 200 || len(c0) != 1 || c0[0].(map[string]any)["state"] != "removing" || len(c0[0].(map[string]any)["commandIds"].([]any)) != 1 {
		t.Fatalf("reconcile terminal: %d %v", code, m)
	}
	if code, _ := reconcile(&restrMgr); code != 409 {
		t.Error("removing case waits for its command")
	}
	ackCommand(t, c0[0].(map[string]any)["commandIds"].([]any)[0].(string), clock.Add(3*time.Second))
	if st := data(get())["recoveryCases"].([]any)[0].(map[string]any)["state"]; st != "resolved" {
		t.Errorf("resolved after acknowledgement: %v", st)
	}
	ownerScan(t, `SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND type = 'reconciliation_required' AND status = 'open'`, []any{u}, &alerts)
	if alerts != 0 {
		t.Error("reconciliation alert resolved")
	}
	// a new contradictory observation opens a new case; observing nothing resolves it without commands
	observe(old)
	observe("null")
	code, m = reconcile(&restrMgr)
	cs := data(m)["recoveryCases"].([]any)
	if code != 200 || len(cs) != 2 || cs[1].(map[string]any)["state"] != "resolved" || len(cs[1].(map[string]any)["commandIds"].([]any)) != 0 {
		t.Fatalf("null observation: %d %v", code, m)
	}
	if code, _ := reconcile(&restrMgr); code != 422 {
		t.Error("nothing left to reconcile")
	}
	if _, m := post(s, &hq, "units.get", `{"id":"`+u+`"}`); data(m)["controlAvailability"].(map[string]any)["state"] != "available" {
		t.Error("control available after every case is resolved")
	}
	if code, _ := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, nil), 0); code != 200 {
		t.Error("schedule after every case is resolved")
	}
}
