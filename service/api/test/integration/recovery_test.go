package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRestrictionRecoveryCases(t *testing.T) {
	srv := serverWith(t, true)
	unit := boundUnit(t, srv, "online")
	key, inv := overdueContract(t, srv, unit)
	id, res := executed(t, srv, key, inv, []string{unit})
	ackCommand(t, cmdOf(t, res, unit, "applyCommandIds"), clock().Add(time.Second))

	get := func() map[string]any {
		_, m := post(srv, restrMgr(), "restrictions.get", `{"id":"`+id+`"}`)

		return m
	}
	_, res = write(
		srv,
		restrMgr(),
		"restrictions.cancel",
		`{"restrictionId":"`+id+`","reason":"plan agreed"}`,
		ver(t, get()),
	)
	ackCommand(t, cmdOf(t, res, unit, "releaseCommandIds"), clock().Add(2*time.Second))

	if data(get())["state"] != statusReleased {
		t.Fatal("released")
	}

	observe := func(observed string) {
		t.Helper()

		if code, resp := write(
			srv,
			customerB(),
			"demo.trigger",
			`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock().Format(
				time.RFC3339,
			)+
				`","eventType":"restriction_observation","restrictionId":"`+id+`","unitId":"`+unit+`","observed":`+observed+`}`,
			0,
		); code != 200 {
			t.Fatalf("observation: %d %v", code, resp)
		}
	}
	old := `{"restrictionId":"` + id + `","rulesVersion":"rules-1","policy":{"kind":"temperature_limit",` +
		`"minimumCoolingSetpoint":24},"observedAt":"` + clock().Format(
		time.RFC3339,
	) +
		`"}`
	observe(old)

	res = get()

	cases := asArr(t, data(res)["recoveryCases"])
	if data(res)["state"] != statusReleased || len(cases) != 1 ||
		asObj(t, cases[0])["state"] != statusPending {
		t.Fatalf("recovery case: %v", res)
	}

	var alerts int
	ownerScan(
		t,
		`SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND type = 'reconciliation_required' AND status = 'open'`,
		[]any{unit},
		&alerts,
	)

	if alerts != 1 {
		t.Errorf("reconciliation alert %d", alerts)
	}

	observe(old) // the same observation updates the open case

	if len(asArr(t, data(get())["recoveryCases"])) != 1 {
		t.Error("one case per unit / restriction / rules version")
	}

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{unit}, nil),
		0,
	); code != 409 {
		t.Error("an unresolved case blocks new restrictions")
	}

	if _, resp := post(
		srv,
		hq(),
		"units.get",
		`{"id":"`+unit+`"}`,
	); asObj(t, data(resp)["controlAvailability"])["state"] != "blocked" {
		t.Errorf("control blocked while a case is open: %v", data(resp)["controlAvailability"])
	}

	if _, g := post(srv, hq(), "units.get", `{"id":"`+unit+`"}`); true {
		if code, resp := write(
			srv,
			hq(),
			"commands.create",
			`{"unitId":"`+unit+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
				ver(t, g),
			)+`,"reason":"check"}`,
			0,
		); code != 409 ||
			resp["messageKey"] != "errors.reconciliation_required" {
			t.Errorf("manual control while blocked: %d %v", code, resp)
		}
	}

	reconcile := func(a *actor) (int, map[string]any) {
		return write(
			srv,
			a,
			"restrictions.reconcile",
			`{"restrictionId":"`+id+`","unitIds":["`+unit+`"]}`,
			ver(t, get()),
		)
	}
	if code, _ := reconcile(hq()); code != 403 {
		t.Error("reconcile without restriction permissions")
	}

	code, res := reconcile(overrider()) // override-only may handle terminal recovery (IR03)

	c0, _ := data(res)["recoveryCases"].([]any)
	if code != 200 || len(c0) != 1 || asObj(t, c0[0])["state"] != statusRemoving ||
		len(asArr(t, asObj(t, c0[0])["commandIds"])) != 1 {
		t.Fatalf("reconcile terminal: %d %v", code, res)
	}

	if code, _ := reconcile(restrMgr()); code != 409 {
		t.Error("removing case waits for its command")
	}

	ackCommand(
		t,
		asStr(t, asArr(t, asObj(t, c0[0])["commandIds"])[0]),
		clock().Add(3*time.Second),
	)

	if st := asObj(t, asArr(t, data(get())["recoveryCases"])[0])["state"]; st != statusResolved {
		t.Errorf("resolved after acknowledgement: %v", st)
	}

	ownerScan(
		t,
		`SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND type = 'reconciliation_required' AND status = 'open'`,
		[]any{unit},
		&alerts,
	)

	if alerts != 0 {
		t.Error("reconciliation alert resolved")
	}
	// a new contradictory observation opens a new case; observing nothing resolves it without commands
	observe(old)
	observe("null")

	code, res = reconcile(restrMgr())

	cs := asArr(t, data(res)["recoveryCases"])
	if code != 200 || len(cs) != 2 || asObj(t, cs[1])["state"] != statusResolved ||
		len(asArr(t, asObj(t, cs[1])["commandIds"])) != 0 {
		t.Fatalf("null observation: %d %v", code, res)
	}

	if code, m := reconcile(
		restrMgr(),
	); code != 422 ||
		fields(m) != "unitIds=errors.reconcile_not_needed" {
		t.Errorf("nothing left to reconcile: %d %s", code, fields(m))
	}

	if _, m := post(
		srv,
		hq(),
		"units.get",
		`{"id":"`+unit+`"}`,
	); asObj(t, data(m)["controlAvailability"])["state"] != stateAvailable {
		t.Error("control available after every case is resolved")
	}

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{unit}, nil),
		0,
	); code != 200 {
		t.Error("schedule after every case is resolved")
	}
}
