package integration

import (
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func scheduleRule(u string, extra map[string]string) string {
	f := map[string]string{
		"name":        `"Weekday cooling"`,
		"unitIds":     `["` + u + `"]`,
		"timezone":    `"Asia/Kuala_Lumpur"`,
		"enabled":     "false",
		"priority":    "50",
		"kind":        `"schedule"`,
		"weekdays":    "[1,2,3,4,5]",
		"startLocal":  `"08:00"`,
		"endLocal":    `"18:00"`,
		"endsNextDay": "false",
		"startAction": `{"kind":"set_power","power":true}`,
		"endAction":   `{"kind":"set_power","power":false}`,
	}

	for key, v := range extra {
		if v == "" {
			delete(f, key)

			continue
		}

		f[key] = v
	}

	parts := []string{}
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}

	return "{" + strings.Join(parts, ",") + "}"
}

func TestAutomationSchedules(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Auto AC")
	// the field each case is refused for
	want := map[string]string{
		"bad time":             "startLocal=error.invalid",
		"bad zone":             "timezone=error.invalid",
		"blank name":           "name=error.length",
		"duplicate weekday":    "weekdays=error.invalid",
		"event fields":         "kind=error.invalid",
		"no end action":        "endAction=error.invalid",
		"no weekdays":          "weekdays=error.required",
		"over 24 hours":        "endsNextDay=errors.over_24_hours",
		"overnight not marked": "endsNextDay=errors.overnight_required",
		"priority 101":         "priority=error.range",
		"same start and end":   "endLocal=errors.same_as_start",
		"unsupported action":   "unitIds=error.unsupportedAction",
		"weekday 8":            "weekdays=error.invalid",
	}

	for name, e := range map[string]map[string]string{
		"no weekdays":          {"weekdays": "[]"},
		"weekday 8":            {"weekdays": "[8]"},
		"duplicate weekday":    {"weekdays": "[1,1]"},
		"same start and end":   {"endLocal": `"08:00"`},
		"overnight not marked": {"startLocal": `"22:00"`, "endLocal": `"06:00"`},
		"over 24 hours":        {"endsNextDay": "true"},
		"no end action":        {"endAction": ""},
		"bad time":             {"startLocal": `"8:00"`},
		"bad zone":             {"timezone": `"Moon/Base"`},
		"priority 101":         {"priority": "101"},
		"blank name":           {"name": `" "`},
		"event fields":         {"condition": `{"type":"occupancy","occupied":true}`},
		"unsupported action":   {"startAction": `{"kind":"set_temperature","celsius":40}`},
	} {
		code, m := write(srv, &customerB, "automations.save", scheduleRule(unit, e), 0)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		srv,
		&customerA,
		"automations.save",
		scheduleRule(unit, nil),
		0,
	); code != 404 {
		t.Error("another customer's unit")
	}
	// nonexistent local time (America/New_York 2027-03-14 02:30) and ambiguous (2026-11-01 01:30)
	want = map[string]string{ // the field each case is refused for
		"\"01:30\"": "startLocal=errors.local_time_invalid",
		"\"02:30\"": "startLocal=errors.local_time_invalid",
	}

	for _, hm := range []string{`"02:30"`, `"01:30"`} {
		code, res := write(
			srv,
			&customerB,
			"automations.save",
			scheduleRule(
				unit,
				map[string]string{
					"timezone":   `"America/New_York"`,
					"weekdays":   "[7]",
					"startLocal": hm,
					"endLocal":   `"05:00"`,
				},
			),
			0,
		)
		refusal(t, hm, code, res, want[hm])
	}

	code, resp := write(
		srv,
		&customerB,
		"automations.save",
		scheduleRule(
			unit,
			map[string]string{
				"startLocal":  `"22:00"`,
				"endLocal":    `"06:00"`,
				"endsNextDay": "true",
			},
		),
		0,
	)
	if code != 200 || data(resp)["enabled"] != false || data(resp)["kind"] != "schedule" ||
		data(resp)["ownerMembershipId"] != seed.ID("customer-b").String() {
		t.Fatalf("save: %d %v", code, resp)
	}

	id := data(resp)["id"].(string)
	code, resp = post(
		srv,
		&customerB,
		"automations.nextRuns",
		`{"automationId":"`+id+`","count":8}`,
	)

	list := resp["data"].([]any)
	if code != 200 || len(list) != 8 {
		t.Fatalf("nextRuns: %d %v", code, resp)
	}

	first := list[0].(map[string]any)
	// clock 2026-09-14T01:00Z is Monday 09:00 in KL: the first run is Monday 22:00 KL = 14:00Z start
	if first["phase"] != "schedule_start" || first["at"] != "2026-09-14T14:00:00Z" ||
		list[1].(map[string]any)["at"] != "2026-09-14T22:00:00Z" {
		t.Errorf("first runs: %v", list[:2])
	}

	if code, res := post(
		srv,
		&customerB,
		"automations.nextRuns",
		`{"automationId":"`+id+`","count":5}`,
	); code != 422 ||
		fields(res) != "count=error.invalid" {
		t.Errorf("count must be 8: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		&customerA,
		"automations.nextRuns",
		`{"automationId":"`+id+`","count":8}`,
	); code != 404 {
		t.Error("another customer's rule")
	}
	// update: version, kind fixed, enabling clears the stop reason
	owner(
		t,
		`UPDATE control.automations SET disabled_reason = 'capability_changed' WHERE id = $1`,
		id,
	)

	if code, _ := write(
		srv,
		&customerB,
		"automations.save",
		scheduleRule(unit, map[string]string{"id": `"` + id + `"`}),
		2,
	); code != 409 {
		t.Error("stale rule")
	}

	if code, res := write(
		srv,
		&customerB,
		"automations.save",
		`{"id":"`+id+`","name":"x",`+
			`"unitIds":["`+unit+`"],"timezone":"UTC","enabled":false,"priority":1,"kind":"event",`+
			`"condition":{"type":"occupancy","occupied":true},"action":{"kind":"set_power","power":false}}`,
		1,
	); code != 422 ||
		fields(res) != "kind=error.immutable" {
		t.Errorf("kind change: %d %s", code, fields(res))
	}

	code, resp = write(
		srv,
		&customerB,
		"automations.save",
		scheduleRule(unit, map[string]string{"id": `"` + id + `"`, "name": `"Edited"`}),
		1,
	)
	if code != 200 || ver(resp) != 2 || data(resp)["disabledReason"] != "capability_changed" {
		t.Fatalf("edit while disabled keeps the reason: %d %v", code, resp)
	}

	code, resp = write(
		srv,
		&customerB,
		"automations.save",
		scheduleRule(unit, map[string]string{"id": `"` + id + `"`, "enabled": "true"}),
		2,
	)
	if code != 200 || data(resp)["enabled"] != true || data(resp)["disabledReason"] != nil {
		t.Fatalf("enable: %d %v", code, resp)
	}

	if _, res := post(
		srv,
		&customerB,
		"automations.list",
		`{"filters":{"unitId":"`+unit+`","kind":"schedule","enabled":true}}`,
	); len(
		items(res),
	) != 1 {
		t.Errorf("list: %v", res)
	}

	if code, res := post(
		srv,
		&customerA,
		"automations.list",
		`{"filters":{"unitId":"`+unit+`"}}`,
	); code != 200 ||
		len(items(res)) != 0 {
		t.Error("another customer's list")
	}

	if code, res := post(
		srv,
		&customerB,
		"automations.list",
		`{"filters":{"kind":"cron"}}`,
	); code != 422 ||
		fields(res) != "filters=error.invalid" {
		t.Errorf("bad kind filter: %d %s", code, fields(res))
	}

	if code, _ := post(srv, &hq, "automations.list", `{}`); code != 403 {
		t.Error("HQ lists customer rules")
	}
}

func TestAutomationEvents(t *testing.T) {
	srv := server(t)
	u := newUnit(t, srv, "Event AC")
	rule := func(cond string) string {
		return `{"name":"When home",` +
			`"unitIds":["` + u + `"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,` +
			`"kind":"event","condition":` + cond + `,"action":{"kind":"set_temperature","celsius":24}}`
	}
	want := map[string]string{ // the field each case is refused for
		"{\"type\":\"location\",\"event\":\"leave\"}":                                        "condition=error.invalid",
		"{\"type\":\"occupancy\"}":                                                           "condition=error.invalid",
		"{\"type\":\"pattern\",\"localTime\":\"25:00\"}":                                     "condition=error.invalid",
		"{\"type\":\"tariff\",\"operator\":\"gt\",\"value\":1,\"unit\":\"MYR_per_kWh\"}":     "condition=error.invalid",
		"{\"type\":\"weather\",\"metric\":\"temperature\",\"operator\":\"eq\",\"value\":30}": "condition=error.invalid",
	}

	for _, c := range []string{
		`{"type":"occupancy"}`, `{"type":"pattern",` +
			`"localTime":"25:00"}`, `{"type":"weather","metric":"temperature",` +
			`"operator":"eq","value":30}`,
		`{"type":"tariff","operator":"gt","value":1,"unit":"MYR_per_kWh"}`, `{"type":"location","event":"leave"}`,
	} {
		code, m := write(srv, &customerB, "automations.save", rule(c), 0)
		refusal(t, c, code, m, want[c])
	}

	if code, resp := write(
		srv,
		&customerB,
		"automations.save",
		rule(`{"type":"weather","metric":"temperature","operator":"gte","value":30}`),
		0,
	); code != 200 ||
		data(resp)["condition"] == nil {
		t.Fatalf("weather rule: %d %v", code, resp)
	} else if code, res := post(
		srv,
		&customerB,
		"automations.nextRuns",
		`{"automationId":"`+data(resp)["id"].(string)+`","count":8}`,
	); code != 422 ||
		fields(res) != "automationId=errors.not_schedule" {
		t.Errorf("event rules have no next runs: %d %s", code, fields(res))
	}

	owner(
		t,
		`UPDATE identity.consents SET granted = false WHERE membership_id = $1`,
		seed.ID("customer-b"),
	)

	if code, res := write(
		srv,
		&customerB,
		"automations.save",
		rule(`{"type":"location","event":"arrival"}`),
		0,
	); code != 422 ||
		fields(res) != "condition=errors.consent_required" {
		t.Errorf("location without consent: %d %s", code, fields(res))
	}

	owner(
		t,
		`UPDATE identity.consents SET granted = true, granted_at = $2 WHERE membership_id = $1`,
		seed.ID("customer-b"),
		clock,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.consents SET granted = false WHERE membership_id = $1`,
			seed.ID("customer-b"),
		)
	})

	if code, res := write(
		srv,
		&customerB,
		"automations.save",
		rule(`{"type":"location","event":"arrival"}`),
		0,
	); code != 200 {
		t.Fatalf("location rule: %d %v", code, res)
	}
}

// TestAutomationOnArchivedUnit: an automation may not name an archived unit. Archiving takes the unit out of its
// client's scope, so the save is NOT_FOUND and stores nothing; the 409 errors.unit_archived answers only while a cached
// scope still holds the unit (IR329). IR381 pins this answer, which had no test.
func TestAutomationOnArchivedUnit(t *testing.T) {
	srv := server(t)

	unit := newUnit(t, srv, "Archived AC") // customer B's
	if code, res := write(
		srv,
		&hq,
		"units.archive",
		`{"id":"`+unit+`","reason":"decommission"}`,
		1,
	); code != 200 {
		t.Fatalf("archive: %d %v", code, res)
	}

	var before int
	ownerScan(
		t,
		`SELECT count(*) FROM control.automation_units WHERE unit_id = $1`,
		[]any{unit},
		&before,
	)

	code, m := write(srv, &customerB, "automations.save", scheduleRule(unit, nil), 0)
	if code != 404 || m["messageKey"] != apperr.KeyNotFound {
		t.Fatalf("an archived unit: %d %v", code, m)
	}

	var after int
	ownerScan(
		t,
		`SELECT count(*) FROM control.automation_units WHERE unit_id = $1`,
		[]any{unit},
		&after,
	)

	if after != before {
		t.Errorf("the refused save linked the unit: %d → %d", before, after)
	}
}
