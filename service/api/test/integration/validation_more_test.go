package integration_test

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
	_, res := post(srv, customerA(), "units.get", `{"id":"`+unit+`"}`)

	v := ver(t, res)
	for name, c := range map[string]struct {
		actor       *actor
		body, field string
	}{
		"unit missing": {customerA(), `{"action":{"kind":"set_power","power":true},` +
			`"expectedUnitVersion":` + itoa(v) + `}`, fieldUnitID},
		"version missing": {customerA(), `{"unitId":"` + unit + `","action":{"kind":"set_power",` +
			`"power":true}}`, "expectedUnitVersion"},
		"reason too long": {techInt(), `{"unitId":"` + unit + `","jobId":"` + uuid.NewString() +
			`","action":{"kind":"set_power","power":true},"reason":"` + strings.Repeat("r", 1001) + `","expectedUnitVersion":` +
			itoa(v) + `}`, fieldReason},
	} {
		if code, resp := write(
			srv,
			c.actor,
			"commands.create",
			c.body,
			0,
		); code != 422 ||
			asObj(t, resp["fieldErrors"])[c.field] == nil {
			t.Errorf("%s: %d %v", name, code, resp)
		}
	}
	// a technician's command needs a job it holds now and a reason
	job := assignedNow(t, srv)

	var jobUnit string
	ownerScan(t, `SELECT unit_id::text FROM maintenance.jobs WHERE id = $1`, []any{job}, &jobUnit)

	_, res = post(srv, techInt(), "units.get", `{"id":"`+jobUnit+`"}`)
	if code, resp := write(
		srv,
		techInt(),
		"commands.create",
		`{"unitId":"`+jobUnit+`","jobId":"`+job+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
			ver(t, res),
		)+`}`,
		0,
	); code != 422 ||
		asObj(t, resp["fieldErrors"])["reason"] != apperr.KeyRequired {
		t.Errorf("technician without a reason: %d %v", code, resp)
	}

	if code, _ := write(
		srv,
		techInt(),
		"commands.create",
		`{"unitId":"`+jobUnit+`","jobId":"`+uuid.NewString()+`","action":{"kind":"set_power","power":true},`+
			`"reason":"check","expectedUnitVersion":`+itoa(
			ver(t, res),
		)+`}`,
		0,
	); code != 404 {
		t.Errorf("technician with an unknown job: %d", code)
	}
	// pendingCommands of units.get lists the open command
	code, res := write(
		srv,
		customerA(),
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
	_, res = post(srv, customerA(), "units.get", `{"id":"`+unit+`"}`)

	pending := asArr(t, data(res)["pendingCommands"])
	if len(pending) != 1 || asObj(t, pending[0])["id"] != id ||
		(asObj(t, pending[0])["status"] != statusRequested && asObj(t, pending[0])["status"] != statusSent) {
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
		"release without id": {"restrictions.release", `{"restrictionId":"` + nilID + `"}`, fieldRestrictionID},
		"override without id": {
			"restrictions.override", `{"restrictionId":"` + nilID + `","reason":"hardship"}`,
			fieldRestrictionID,
		},
		"defer without until": {"restrictions.defer", `{"restrictionId":"` + nilID + `","reason":"plan"}`, "until"},
		"reconcile duplicates": {"restrictions.reconcile", `{"restrictionId":"` + nilID + `","unitIds":["` + dup + `","` +
			dup + `"]}`, fieldUnitIDs},
		"retry bad phase": {opRestrictionsRetry, `{"restrictionId":"` + dup + `","unitIds":["` + dup +
			`"],"phase":"again","confirmedRulesVersion":"rules-1"}`, fieldPhase},
		"retry blank rules": {opRestrictionsRetry, `{"restrictionId":"` + dup + `","unitIds":["` + dup + `","` + dup +
			`"],"phase":"apply","confirmedRulesVersion":"  "}`, "confirmedRulesVersion"},
		"retry without id": {opRestrictionsRetry, `{"restrictionId":"` + nilID + `","unitIds":[],"phase":"release",` +
			`"confirmedRulesVersion":"rules-1"}`, fieldRestrictionID},
	} {
		if code, res := write(
			srv,
			restrMgr(),
			c.op,
			c.body,
			1,
		); code != 422 ||
			asObj(t, res["fieldErrors"])[c.field] == nil {
			t.Errorf("%s: %d %v", name, code, res)
		}
	}
}
