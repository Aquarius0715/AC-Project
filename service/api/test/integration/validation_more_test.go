package integration

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestCommandCreateValidation covers the commands.create input rules, the technician reason and job checks, and the
// unit's pendingCommands list while a command is open.
func TestCommandCreateValidation(t *testing.T) {
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
	_, res := post(srv, &customerA, "units.get", `{"id":"`+unit+`"}`)

	v := ver(res)
	for name, c := range map[string]struct {
		actor       *actor
		body, field string
	}{
		"unit missing": {&customerA, `{"action":{"kind":"set_power","power":true},` +
			`"expectedUnitVersion":` + itoa(v) + `}`, "unitId"},
		"version missing": {&customerA, `{"unitId":"` + unit + `","action":{"kind":"set_power",` +
			`"power":true}}`, "expectedUnitVersion"},
		"reason too long": {&techInt, `{"unitId":"` + unit + `","jobId":"` + uuid.NewString() +
			`","action":{"kind":"set_power","power":true},"reason":"` + strings.Repeat("r", 1001) + `","expectedUnitVersion":` +
			itoa(v) + `}`, "reason"},
	} {
		if code, resp := write(
			srv,
			c.actor,
			"commands.create",
			c.body,
			0,
		); code != 422 ||
			resp["fieldErrors"].(map[string]any)[c.field] == nil {
			t.Errorf("%s: %d %v", name, code, resp)
		}
	}
	// a technician's command needs a job it holds now and a reason
	job := assignedNow(t, srv)

	var jobUnit string
	ownerScan(t, `SELECT unit_id::text FROM maintenance.jobs WHERE id = $1`, []any{job}, &jobUnit)

	_, res = post(srv, &techInt, "units.get", `{"id":"`+jobUnit+`"}`)
	if code, resp := write(
		srv,
		&techInt,
		"commands.create",
		`{"unitId":"`+jobUnit+`","jobId":"`+job+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
			ver(res),
		)+`}`,
		0,
	); code != 422 ||
		resp["fieldErrors"].(map[string]any)["reason"] != apperr.KeyRequired {
		t.Errorf("technician without a reason: %d %v", code, resp)
	}

	if code, _ := write(
		srv,
		&techInt,
		"commands.create",
		`{"unitId":"`+jobUnit+`","jobId":"`+uuid.NewString()+`","action":{"kind":"set_power","power":true},`+
			`"reason":"check","expectedUnitVersion":`+itoa(
			ver(res),
		)+`}`,
		0,
	); code != 404 {
		t.Errorf("technician with an unknown job: %d", code)
	}
	// pendingCommands of units.get lists the open command
	code, res := write(
		srv,
		&customerA,
		"commands.create",
		`{"unitId":"`+unit+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
			v,
		)+`}`,
		0,
	)
	if code != 200 {
		t.Fatalf("command: %d %v", code, res)
	}

	id := data(res)["id"]
	_, res = post(srv, &customerA, "units.get", `{"id":"`+unit+`"}`)

	pending := data(res)["pendingCommands"].([]any)
	if len(pending) != 1 || pending[0].(map[string]any)["id"] != id ||
		(pending[0].(map[string]any)["status"] != "requested" && pending[0].(map[string]any)["status"] != "sent") {
		t.Fatalf("pendingCommands: %v", pending)
	}

	owner(
		t,
		`UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`,
		unit,
	)
}

// TestRestrictionInputValidation covers the input rules of the restriction actions, which run before any state or
// permission check.
func TestRestrictionInputValidation(t *testing.T) {
	srv := server(t)

	nilID, dup := uuid.Nil.String(), uuid.NewString()
	for name, c := range map[string]struct{ op, body, field string }{
		"release without id": {"restrictions.release", `{"restrictionId":"` + nilID + `"}`, "restrictionId"},
		"override without id": {
			"restrictions.override", `{"restrictionId":"` + nilID + `","reason":"hardship"}`,
			"restrictionId",
		},
		"defer without until": {"restrictions.defer", `{"restrictionId":"` + nilID + `","reason":"plan"}`, "until"},
		"reconcile duplicates": {"restrictions.reconcile", `{"restrictionId":"` + nilID + `","unitIds":["` + dup + `","` +
			dup + `"]}`, "unitIds"},
		"retry bad phase": {"restrictions.retry", `{"restrictionId":"` + dup + `","unitIds":["` + dup +
			`"],"phase":"again","confirmedRulesVersion":"rules-1"}`, "phase"},
		"retry blank rules": {"restrictions.retry", `{"restrictionId":"` + dup + `","unitIds":["` + dup + `","` + dup +
			`"],"phase":"apply","confirmedRulesVersion":"  "}`, "confirmedRulesVersion"},
		"retry without id": {"restrictions.retry", `{"restrictionId":"` + nilID + `","unitIds":[],"phase":"release",` +
			`"confirmedRulesVersion":"rules-1"}`, "restrictionId"},
	} {
		if code, res := write(
			srv,
			&restrMgr,
			c.op,
			c.body,
			1,
		); code != 422 ||
			res["fieldErrors"].(map[string]any)[c.field] == nil {
			t.Errorf("%s: %d %v", name, code, res)
		}
	}
}
