package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func TestAutomationEvaluation(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)
	u, off, sched := boundUnit(t, s, "online"), boundUnit(t, s, "offline"), boundUnit(t, s, "online")
	rule := func(units, cond, action string, priority int) string {
		_, m := write(s, &customerB, "automations.save", `{"name":"r","unitIds":[`+units+`],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":`+itoa(priority)+
			`,"kind":"event","condition":`+cond+`,"action":`+action+`}`, 0)
		return data(m)["id"].(string)
	}
	occ := rule(`"`+u+`","`+off+`"`, `{"type":"occupancy","occupied":true}`, `{"kind":"set_temperature","celsius":24}`, 10)
	pattern := rule(`"`+u+`"`, `{"type":"pattern","localTime":"09:00"}`, `{"kind":"set_power","power":false}`, 90)
	at := clock.Format(time.RFC3339)
	fact := func(unit, metric, value, unitName string, observed time.Time) string {
		return `{"unitId":"` + unit + `","metric":"` + metric + `","value":` + value + `,"unit":"` + unitName + `","observedAt":"` + observed.Format(time.RFC3339) + `","quality":"valid"}`
	}
	input := func(units, facts, phase string) string {
		return `{"eventId":"` + uuid.NewString() + `","occurredAt":"` + at + `","unitIds":[` + units + `],"facts":[` + facts + `],"phase":"` + phase + `"}`
	}
	decision := func(m map[string]any, unit string) map[string]any {
		for _, r := range data(m)["results"].([]any) {
			if x := r.(map[string]any); x["unitId"] == unit {
				return x
			}
		}
		return nil
	}
	occupied := fact(u, "occupied", "true", "boolean", clock)
	code, m := post(s, &customerB, "automations.simulate", input(`"`+u+`"`, occupied, "condition"))
	if code != 200 || decision(m, u)["decision"] != "selected" || decision(m, u)["ruleId"] != pattern {
		t.Fatalf("priority: %d %v", code, m)
	}
	owner(t, `UPDATE control.automations SET enabled = false WHERE id = $1`, pattern)
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+u+`"`, occupied, "condition")); decision(m, u)["ruleId"] != occ {
		t.Errorf("disabled rule skipped: %v", m)
	}
	// HQ automation policies outrank customer rules
	hqPolicy := uuid.NewString()
	owner(t, `INSERT INTO control.automation_policies (id, tenant_id, name, owner_membership_id, created_by_user_id, timezone, enabled, priority, condition, action)
		VALUES ($1,$2,'HQ occupancy',$3,$4,'Asia/Kuala_Lumpur',true,0,'{"type":"occupancy","occupied":true}','{"kind":"set_temperature","celsius":26}')`,
		hqPolicy, seed.ID("tenant-a"), seed.ID("hq-operator"), seed.ID("user-hq-operator"))
	owner(t, `INSERT INTO control.automation_policy_units (tenant_id, policy_id, unit_id) VALUES ($1,$2,$3)`, seed.ID("tenant-a"), hqPolicy, u)
	t.Cleanup(func() { owner(t, `DELETE FROM control.automation_policies WHERE id = $1`, hqPolicy) })
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+u+`"`, occupied, "condition")); decision(m, u)["ruleId"] != hqPolicy {
		t.Errorf("HQ first: %v", m)
	}
	// data quality and other exclusions
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+u+`"`, fact(u, "occupied", "true", "boolean", clock.Add(-10*time.Minute)), "condition")); decision(m, u)["reason"] != "stale" &&
		decision(m, u)["reason"] != "disabled" {
		t.Errorf("stale: %v", m)
	}
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+u+`"`, fact(u, "occupied", "false", "boolean", clock), "condition")); decision(m, u)["decision"] != "suppressed" {
		t.Errorf("no match: %v", m)
	}
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+off+`"`, fact(off, "occupied", "true", "boolean", clock), "condition")); decision(m, off)["reason"] != "offline" {
		t.Errorf("offline: %v", m)
	}
	nobody := newUnit(t, s, "No rules")
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+nobody+`"`, "", "condition")); decision(m, nobody)["reason"] != "no_match" {
		t.Errorf("no candidates: %v", m)
	}
	// schedules are due only at their phase and tick (Monday 09:00 KL = clock)
	_, m = write(s, &customerB, "automations.save", scheduleRule(sched, map[string]string{"enabled": "true", "weekdays": "[1]", "startLocal": `"09:00"`, "endLocal": `"10:00"`}), 0)
	sid := data(m)["id"].(string)
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+sched+`"`, "", "schedule_start")); decision(m, sched)["ruleId"] != sid {
		t.Errorf("schedule start: %v", m)
	}
	if _, m := post(s, &customerB, "automations.simulate", input(`"`+sched+`"`, "", "schedule_end")); decision(m, sched)["reason"] != "no_match" {
		t.Errorf("schedule end not due: %v", m)
	}
	// validation and scope
	for _, b := range []string{
		`{"eventId":"` + uuid.NewString() + `","occurredAt":"` + clock.Add(time.Hour).Format(time.RFC3339) + `","unitIds":["` + u + `"],"facts":[],"phase":"condition"}`,
		input(`"`+u+`","`+u+`"`, "", "condition"), input(`"`+u+`"`, fact(u, "occupied", "1", "boolean", clock), "condition"),
		input(`"`+u+`"`, fact(off, "occupied", "true", "boolean", clock), "condition"), input(`"`+u+`"`, fact(u, "humidity", "50", "kPa", clock), "condition"),
		input(`"`+u+`"`, "", "midnight"),
	} {
		if code, _ := post(s, &customerB, "automations.simulate", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerA, "automations.simulate", input(`"`+u+`"`, "", "condition")); code != 404 {
		t.Error("another customer's unit")
	}
	if code, _ := post(s, &techB, "automations.simulate", input(`"`+u+`"`, "", "condition")); code != 403 {
		t.Error("technician simulate")
	}
	// fire: one Command for the winner; replays reuse the result
	owner(t, `DELETE FROM control.automation_policies WHERE id = $1`, hqPolicy)
	owner(t, `UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`, u)
	body := input(`"`+u+`"`, occupied, "condition")
	code, m = write(s, &customerB, "automations.fire", body, 0)
	if code != 200 || decision(m, u)["decision"] != "requested" || decision(m, u)["commandId"] == nil {
		t.Fatalf("fire: %d %v", code, m)
	}
	cmd := decision(m, u)["commandId"].(string)
	var source, actor string
	ownerScan(t, `SELECT source, actor_membership_id::text FROM control.commands WHERE id = $1`, []any{cmd}, &source, &actor)
	if source != "automation" || actor != seed.ID("customer-b").String() {
		t.Errorf("command: %s %s", source, actor)
	}
	if code, m := write(s, &customerB, "automations.fire", body, 0); code != 200 || decision(m, u)["commandId"] != cmd {
		t.Error("same event replays the result")
	}
	if code, _ := write(s, &customerB, "automations.fire", body[:len(body)-len(`"condition"}`)]+`"schedule_start"}`, 0); code != 409 {
		t.Error("another phase in the same tick")
	}
	if code, _ := write(s, &customerB, "automations.fire", input(`"`+u+`"`, fact(u, "occupied", "false", "boolean", clock), "condition"), 0); code != 409 {
		t.Error("different facts in the same tick")
	}
	var n int
	ownerScan(t, `SELECT count(*) FROM control.commands WHERE unit_id = $1 AND source = 'automation'`, []any{u}, &n)
	if n != 1 {
		t.Errorf("commands created: %d", n)
	}
}
