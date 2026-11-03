package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const refusedFactsInvalid = "facts=" + apperr.KeyInvalid

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAutomationEvaluation(t *testing.T) {
	srv := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)
	unitID, off, sched := boundUnit(
		t,
		srv,
		"online",
	), boundUnit(
		t,
		srv,
		"offline",
	), boundUnit(
		t,
		srv,
		"online",
	)
	rule := func(units, cond, action string, priority int) string {
		_, res := write(
			srv,
			customerB(),
			"automations.save",
			`{"name":"r","unitIds":[`+units+`],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":`+itoa(
				priority,
			)+
				`,"kind":"event","condition":`+cond+`,"action":`+action+`}`,
			0,
		)

		return asStr(t, data(res)["id"])
	}
	occ := rule(
		`"`+unitID+`","`+off+`"`,
		`{"type":"occupancy","occupied":true}`,
		`{"kind":"set_temperature","celsius":24}`,
		10,
	)
	pattern := rule(
		`"`+unitID+`"`,
		`{"type":"pattern","localTime":"09:00"}`,
		`{"kind":"set_power","power":false}`,
		90,
	)
	stamp := clock().Format(time.RFC3339)
	fact := func(unit, metric, value, unitName string, observed time.Time) string {
		return `{"unitId":"` + unit + `","metric":"` + metric + `","value":` + value + `,"unit":"` + unitName +
			`","observedAt":"` + observed.Format(
			time.RFC3339,
		) + `","quality":"valid"}`
	}
	input := func(units, facts, phase string) string {
		return `{"eventId":"` + uuid.NewString() + `","occurredAt":"` + stamp + `","unitIds":[` + units + `],"facts":[` +
			facts +
			`],"phase":"` + phase + `"}`
	}
	decision := func(m map[string]any, unit string) map[string]any {
		for _, r := range asArr(t, data(m)["results"]) {
			if x := asObj(t, r); x["unitId"] == unit {
				return x
			}
		}

		return nil
	}
	occupied := fact(unitID, "occupied", "true", "boolean", clock())

	code, resp := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+unitID+`"`, occupied, "condition"),
	)
	if code != 200 || decision(resp, unitID)["decision"] != decisionSelected ||
		decision(resp, unitID)["ruleId"] != pattern {
		t.Fatalf("priority: %d %v", code, resp)
	}

	owner(t, `UPDATE control.automations SET enabled = false WHERE id = $1`, pattern)

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+unitID+`"`, occupied, "condition"),
	); decision(res, unitID)["ruleId"] != occ {
		t.Errorf("disabled rule skipped: %v", res)
	}
	// HQ automation policies outrank customer rules
	hqPolicy := uuid.NewString()
	owner(
		t,
		`INSERT INTO control.automation_policies (id, tenant_id, name, owner_membership_id, created_by_user_id, timezone,
			enabled, priority, condition, action)
		VALUES ($1,$2,'HQ occupancy',$3,$4,'Asia/Kuala_Lumpur',true,0,'{"type":"occupancy","occupied":true}',
		'{"kind":"set_temperature","celsius":26}')`,
		hqPolicy,
		seed.ID("tenant-a"),
		seed.ID("hq-operator"),
		seed.ID("user-hq-operator"),
	)
	owner(
		t,
		`INSERT INTO control.automation_policy_units (tenant_id, policy_id, unit_id) VALUES ($1,$2,$3)`,
		seed.ID("tenant-a"),
		hqPolicy,
		unitID,
	)
	t.Cleanup(
		func() { owner(t, `DELETE FROM control.automation_policies WHERE id = $1`, hqPolicy) },
	)

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+unitID+`"`, occupied, "condition"),
	); decision(res, unitID)["ruleId"] != hqPolicy {
		t.Errorf("HQ first: %v", res)
	}
	// data quality and other exclusions
	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(
			`"`+unitID+`"`,
			fact(unitID, "occupied", "true", "boolean", clock().Add(-10*time.Minute)),
			"condition",
		),
	); decision(res, unitID)["reason"] != "stale" &&
		decision(res, unitID)["reason"] != reasonDisabled {
		t.Errorf("stale: %v", res)
	}

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+unitID+`"`, fact(unitID, "occupied", "false", "boolean", clock()), "condition"),
	); decision(res, unitID)["decision"] != decisionSuppressed {
		t.Errorf("no match: %v", res)
	}

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+off+`"`, fact(off, "occupied", "true", "boolean", clock()), "condition"),
	); decision(res, off)["reason"] != reasonOffline {
		t.Errorf("offline: %v", res)
	}

	nobody := newUnit(t, srv, "No rules")
	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+nobody+`"`, "", "condition"),
	); decision(res, nobody)["reason"] != reasonNoMatch {
		t.Errorf("no candidates: %v", res)
	}
	// schedules are due only at their phase and tick (Monday 09:00 KL = clock)
	_, resp = write(
		srv,
		customerB(),
		"automations.save",
		scheduleRule(
			sched,
			map[string]string{
				fieldEnabled:    jsonTrue,
				fieldWeekdays:   jsonMondays,
				fieldStartLocal: `"09:00"`,
				fieldEndLocal:   jsonTenOClock,
			},
		),
		0,
	)

	sid := asStr(t, data(resp)["id"])
	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+sched+`"`, "", "schedule_start"),
	); decision(res, sched)["ruleId"] != sid {
		t.Errorf("schedule start: %v", res)
	}

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(`"`+sched+`"`, "", "schedule_end"),
	); decision(res, sched)["reason"] != reasonNoMatch {
		t.Errorf("schedule end not due: %v", res)
	}
	// validation and scope
	for body, want := range map[string]string{
		`{"eventId":"` + uuid.NewString() + `","occurredAt":"` + clock().Add(time.Hour).Format(time.RFC3339) +
			`","unitIds":["` + unitID + `"],"facts":[],"phase":"condition"}`: "occurredAt=errors.not_current_tick",
		input(`"`+unitID+`","`+unitID+`"`, "", "condition"): refusedUnitIDsInvalid,
		// a boolean fact of 1
		input(`"`+unitID+`"`, fact(unitID, "occupied", "1", "boolean", clock()), "condition"): refusedFactsInvalid,
		// a unit outside the input
		input(`"`+unitID+`"`, fact(off, "occupied", "true", "boolean", clock()), "condition"): refusedFactsInvalid,
		// a unit symbol of another metric
		input(`"`+unitID+`"`, fact(unitID, "humidity", "50", "kPa", clock()), "condition"): refusedFactsInvalid,
		input(`"`+unitID+`"`, "", "midnight"):                                              "phase=error.invalid",
	} {
		code, m := post(srv, customerB(), "automations.simulate", body)
		refusal(t, body, code, m, want)
	}

	if code, _ := post(
		srv,
		customerA(),
		"automations.simulate",
		input(`"`+unitID+`"`, "", "condition"),
	); code != 404 {
		t.Error("another customer's unit")
	}

	if code, _ := post(
		srv,
		techB(),
		"automations.simulate",
		input(`"`+unitID+`"`, "", "condition"),
	); code != 403 {
		t.Error("technician simulate")
	}
	// fire: one Command for the winner; replays reuse the result
	owner(t, `DELETE FROM control.automation_policies WHERE id = $1`, hqPolicy)
	owner(
		t,
		`UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`,
		unitID,
	)
	body := input(`"`+unitID+`"`, occupied, "condition")

	code, resp = write(srv, customerB(), "automations.fire", body, 0)
	if code != 200 || decision(resp, unitID)["decision"] != decisionRequested ||
		decision(resp, unitID)["commandId"] == nil {
		t.Fatalf("fire: %d %v", code, resp)
	}

	cmd := asStr(t, decision(resp, unitID)["commandId"])

	var source, actor string
	ownerScan(
		t,
		`SELECT source, actor_membership_id::text FROM control.commands WHERE id = $1`,
		[]any{cmd},
		&source,
		&actor,
	)

	if source != kindAutomation || actor != seed.ID("customer-b").String() {
		t.Errorf("command: %s %s", source, actor)
	}

	if code, res := write(
		srv,
		customerB(),
		"automations.fire",
		body,
		0,
	); code != 200 ||
		decision(res, unitID)["commandId"] != cmd {
		t.Error("same event replays the result")
	}

	if code, _ := write(
		srv,
		customerB(),
		"automations.fire",
		body[:len(body)-len(`"condition"}`)]+`"schedule_start"}`,
		0,
	); code != 409 {
		t.Error("another phase in the same tick")
	}

	if code, _ := write(
		srv,
		customerB(),
		"automations.fire",
		input(`"`+unitID+`"`, fact(unitID, "occupied", "false", "boolean", clock()), "condition"),
		0,
	); code != 409 {
		t.Error("different facts in the same tick")
	}

	var total int
	ownerScan(
		t,
		`SELECT count(*) FROM control.commands WHERE unit_id = $1 AND source = 'automation'`,
		[]any{unitID},
		&total,
	)

	if total != 1 {
		t.Errorf("commands created: %d", total)
	}
}
