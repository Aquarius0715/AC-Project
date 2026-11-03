package integration_test

import (
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const fieldEndsNextDay = "endsNextDay"

func scheduleRule(u string, extra map[string]string) string {
	f := map[string]string{
		fieldName:        `"Weekday cooling"`,
		fieldUnitIDs:     `["` + u + `"]`,
		fieldTimezone:    jsonKualaLumpur,
		fieldEnabled:     jsonFalse,
		fieldPriority:    "50",
		fieldKind:        `"schedule"`,
		fieldWeekdays:    "[1,2,3,4,5]",
		fieldStartLocal:  `"08:00"`,
		fieldEndLocal:    `"18:00"`,
		fieldEndsNextDay: jsonFalse,
		fieldStartAction: actionPowerOn,
		"endAction":      actionPowerOff,
	}

	for key, value := range extra {
		if value == "" {
			delete(f, key)

			continue
		}

		f[key] = value
	}

	parts := make([]string, 0, len(f))
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}

	return "{" + strings.Join(parts, ",") + "}"
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAutomationSchedules(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Auto AC")
	// the field each case is refused for
	want := map[string]string{
		"bad time":             "startLocal=error.invalid",
		caseBadZone:            refusedTimezoneInvalid,
		caseBlankName:          refusedNameLength,
		"duplicate weekday":    "weekdays=error.invalid",
		"event fields":         refusedKindInvalid,
		"no end action":        "endAction=error.invalid",
		caseNoWeekdays:         "weekdays=error.required",
		"over 24 hours":        "endsNextDay=errors.over_24_hours",
		"overnight not marked": "endsNextDay=errors.overnight_required",
		casePriorityOver100:    refusedPriorityRange,
		"same start and end":   "endLocal=errors.same_as_start",
		"unsupported action":   "unitIds=error.unsupportedAction",
		caseWeekdayEight:       "weekdays=error.invalid",
	}

	for name, extra := range map[string]map[string]string{
		caseNoWeekdays:         {fieldWeekdays: "[]"},
		caseWeekdayEight:       {fieldWeekdays: "[8]"},
		"duplicate weekday":    {fieldWeekdays: "[1,1]"},
		"same start and end":   {fieldEndLocal: `"08:00"`},
		"overnight not marked": {fieldStartLocal: `"22:00"`, fieldEndLocal: `"06:00"`},
		"over 24 hours":        {fieldEndsNextDay: jsonTrue},
		"no end action":        {"endAction": ""},
		"bad time":             {fieldStartLocal: `"8:00"`},
		caseBadZone:            {fieldTimezone: `"Moon/Base"`},
		casePriorityOver100:    {fieldPriority: "101"},
		caseBlankName:          {fieldName: jsonBlank},
		"event fields":         {fieldCondition: `{"type":"occupancy","occupied":true}`},
		"unsupported action":   {fieldStartAction: `{"kind":"set_temperature","celsius":40}`},
	} {
		code, m := write(srv, customerB(), "automations.save", scheduleRule(unit, extra), 0)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		srv,
		customerA(),
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

	for _, localTime := range []string{`"02:30"`, `"01:30"`} {
		code, res := write(
			srv,
			customerB(),
			"automations.save",
			scheduleRule(
				unit,
				map[string]string{
					fieldTimezone:   `"America/New_York"`,
					fieldWeekdays:   "[7]",
					fieldStartLocal: localTime,
					fieldEndLocal:   `"05:00"`,
				},
			),
			0,
		)
		refusal(t, localTime, code, res, want[localTime])
	}

	code, resp := write(
		srv,
		customerB(),
		"automations.save",
		scheduleRule(
			unit,
			map[string]string{
				fieldStartLocal:  `"22:00"`,
				fieldEndLocal:    `"06:00"`,
				fieldEndsNextDay: jsonTrue,
			},
		),
		0,
	)
	if code != 200 || data(resp)["enabled"] != false || data(resp)["kind"] != "schedule" ||
		data(resp)["ownerMembershipId"] != seed.ID("customer-b").String() {
		t.Fatalf("save: %d %v", code, resp)
	}

	automationID := asStr(t, data(resp)["id"])
	code, resp = post(
		srv,
		customerB(),
		"automations.nextRuns",
		`{"automationId":"`+automationID+`","count":8}`,
	)

	list := asArr(t, resp["data"])
	if code != 200 || len(list) != 8 {
		t.Fatalf("nextRuns: %d %v", code, resp)
	}

	first := asObj(t, list[0])
	// clock 2026-09-14T01:00Z is Monday 09:00 in KL: the first run is Monday 22:00 KL = 14:00Z start
	if first["phase"] != "schedule_start" || first["at"] != "2026-09-14T14:00:00Z" ||
		asObj(t, list[1])["at"] != "2026-09-14T22:00:00Z" {
		t.Errorf("first runs: %v", list[:2])
	}

	if code, res := post(
		srv,
		customerB(),
		"automations.nextRuns",
		`{"automationId":"`+automationID+`","count":5}`,
	); code != 422 ||
		fields(res) != "count=error.invalid" {
		t.Errorf("count must be 8: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		customerA(),
		"automations.nextRuns",
		`{"automationId":"`+automationID+`","count":8}`,
	); code != 404 {
		t.Error("another customer's rule")
	}
	// update: version, kind fixed, enabling clears the stop reason
	owner(
		t,
		`UPDATE control.automations SET disabled_reason = 'capability_changed' WHERE id = $1`,
		automationID,
	)

	if code, _ := write(
		srv,
		customerB(),
		"automations.save",
		scheduleRule(unit, map[string]string{"id": `"` + automationID + `"`}),
		2,
	); code != 409 {
		t.Error("stale rule")
	}

	if code, res := write(
		srv,
		customerB(),
		"automations.save",
		`{"id":"`+automationID+`","name":"x",`+
			`"unitIds":["`+unit+`"],"timezone":"UTC","enabled":false,"priority":1,"kind":"event",`+
			`"condition":{"type":"occupancy","occupied":true},"action":{"kind":"set_power","power":false}}`,
		1,
	); code != 422 ||
		fields(res) != "kind=error.immutable" {
		t.Errorf("kind change: %d %s", code, fields(res))
	}

	code, resp = write(
		srv,
		customerB(),
		"automations.save",
		scheduleRule(
			unit,
			map[string]string{"id": `"` + automationID + `"`, fieldName: `"Edited"`},
		),
		1,
	)
	if code != 200 || ver(t, resp) != 2 || data(resp)["disabledReason"] != "capability_changed" {
		t.Fatalf("edit while disabled keeps the reason: %d %v", code, resp)
	}

	code, resp = write(
		srv,
		customerB(),
		"automations.save",
		scheduleRule(
			unit,
			map[string]string{"id": `"` + automationID + `"`, fieldEnabled: jsonTrue},
		),
		2,
	)
	if code != 200 || data(resp)["enabled"] != true || data(resp)["disabledReason"] != nil {
		t.Fatalf("enable: %d %v", code, resp)
	}

	if _, res := post(
		srv,
		customerB(),
		"automations.list",
		`{"filters":{"unitId":"`+unit+`","kind":"schedule","enabled":true}}`,
	); len(
		items(t, res),
	) != 1 {
		t.Errorf("list: %v", res)
	}

	if code, res := post(
		srv,
		customerA(),
		"automations.list",
		`{"filters":{"unitId":"`+unit+`"}}`,
	); code != 200 ||
		len(items(t, res)) != 0 {
		t.Error("another customer's list")
	}

	if code, res := post(
		srv,
		customerB(),
		"automations.list",
		`{"filters":{"kind":"cron"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad kind filter: %d %s", code, fields(res))
	}

	if code, _ := post(srv, hq(), "automations.list", `{}`); code != 403 {
		t.Error("HQ lists customer rules")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAutomationEvents(t *testing.T) {
	srv := server(t)
	u := newUnit(t, srv, "Event AC")
	rule := func(cond string) string {
		return `{"name":"When home",` +
			`"unitIds":["` + u + `"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,` +
			`"kind":"event","condition":` + cond + `,"action":{"kind":"set_temperature","celsius":24}}`
	}
	want := map[string]string{ // the field each case is refused for
		"{\"type\":\"location\",\"event\":\"leave\"}":                                        refusedConditionInvalid,
		"{\"type\":\"occupancy\"}":                                                           refusedConditionInvalid,
		"{\"type\":\"pattern\",\"localTime\":\"25:00\"}":                                     refusedConditionInvalid,
		"{\"type\":\"tariff\",\"operator\":\"gt\",\"value\":1,\"unit\":\"MYR_per_kWh\"}":     refusedConditionInvalid,
		"{\"type\":\"weather\",\"metric\":\"temperature\",\"operator\":\"eq\",\"value\":30}": refusedConditionInvalid,
	}

	for _, condition := range []string{
		`{"type":"occupancy"}`, `{"type":"pattern",` +
			`"localTime":"25:00"}`, `{"type":"weather","metric":"temperature",` +
			`"operator":"eq","value":30}`,
		`{"type":"tariff","operator":"gt","value":1,"unit":"MYR_per_kWh"}`, `{"type":"location","event":"leave"}`,
	} {
		code, m := write(srv, customerB(), "automations.save", rule(condition), 0)
		refusal(t, condition, code, m, want[condition])
	}

	if code, resp := write(
		srv,
		customerB(),
		"automations.save",
		rule(`{"type":"weather","metric":"temperature","operator":"gte","value":30}`),
		0,
	); code != 200 ||
		data(resp)["condition"] == nil {
		t.Fatalf("weather rule: %d %v", code, resp)
	} else if code, res := post(
		srv,
		customerB(),
		"automations.nextRuns",
		`{"automationId":"`+asStr(t, data(resp)["id"])+`","count":8}`,
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
		customerB(),
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
		clock(),
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
		customerB(),
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
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAutomationOnArchivedUnit(t *testing.T) {
	srv := server(t)

	unit := newUnit(t, srv, "Archived AC") // customer B's
	if code, res := write(
		srv,
		hq(),
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

	code, m := write(srv, customerB(), "automations.save", scheduleRule(unit, nil), 0)
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
