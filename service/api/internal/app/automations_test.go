package app

import (
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func scheduleRule(u string, extra map[string]string) string {
	f := map[string]string{"name": `"Weekday cooling"`, "unitIds": `["` + u + `"]`, "timezone": `"Asia/Kuala_Lumpur"`, "enabled": "false", "priority": "50", "kind": `"schedule"`,
		"weekdays": "[1,2,3,4,5]", "startLocal": `"08:00"`, "endLocal": `"18:00"`, "endsNextDay": "false",
		"startAction": `{"kind":"set_power","power":true}`, "endAction": `{"kind":"set_power","power":false}`}
	for k, v := range extra {
		if v == "" {
			delete(f, k)
			continue
		}
		f[k] = v
	}
	parts := []string{}
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestAutomationSchedules(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Auto AC")
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
		if code, m := write(s, &customerB, "automations.save", scheduleRule(u, e), 0); code != 422 {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	if code, _ := write(s, &customerA, "automations.save", scheduleRule(u, nil), 0); code != 404 {
		t.Error("another customer's unit")
	}
	// nonexistent local time (America/New_York 2027-03-14 02:30) and ambiguous (2026-11-01 01:30)
	for _, hm := range []string{`"02:30"`, `"01:30"`} {
		if code, _ := write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"timezone": `"America/New_York"`, "weekdays": "[7]", "startLocal": hm, "endLocal": `"05:00"`}), 0); code != 422 {
			t.Errorf("DST %s accepted", hm)
		}
	}
	code, m := write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"startLocal": `"22:00"`, "endLocal": `"06:00"`, "endsNextDay": "true"}), 0)
	if code != 200 || data(m)["enabled"] != false || data(m)["kind"] != "schedule" || data(m)["ownerMembershipId"] != seed.ID("customer-b").String() {
		t.Fatalf("save: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	code, m = post(s, &customerB, "automations.nextRuns", `{"automationId":"`+id+`","count":8}`)
	list := m["data"].([]any)
	if code != 200 || len(list) != 8 {
		t.Fatalf("nextRuns: %d %v", code, m)
	}
	first := list[0].(map[string]any)
	// clock 2026-09-14T01:00Z is Monday 09:00 in KL: the first run is Monday 22:00 KL = 14:00Z start
	if first["phase"] != "schedule_start" || first["at"] != "2026-09-14T14:00:00Z" || list[1].(map[string]any)["at"] != "2026-09-14T22:00:00Z" {
		t.Errorf("first runs: %v", list[:2])
	}
	if code, _ := post(s, &customerB, "automations.nextRuns", `{"automationId":"`+id+`","count":5}`); code != 422 {
		t.Error("count must be 8")
	}
	if code, _ := post(s, &customerA, "automations.nextRuns", `{"automationId":"`+id+`","count":8}`); code != 404 {
		t.Error("another customer's rule")
	}
	// update: version, kind fixed, enabling clears the stop reason
	owner(t, `UPDATE control.automations SET disabled_reason = 'capability_changed' WHERE id = $1`, id)
	if code, _ := write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"id": `"` + id + `"`}), 2); code != 409 {
		t.Error("stale rule")
	}
	if code, _ := write(s, &customerB, "automations.save", `{"id":"`+id+`","name":"x","unitIds":["`+u+`"],"timezone":"UTC","enabled":false,"priority":1,"kind":"event","condition":{"type":"occupancy","occupied":true},"action":{"kind":"set_power","power":false}}`, 1); code != 422 {
		t.Error("kind change")
	}
	code, m = write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"id": `"` + id + `"`, "name": `"Edited"`}), 1)
	if code != 200 || ver(m) != 2 || data(m)["disabledReason"] != "capability_changed" {
		t.Fatalf("edit while disabled keeps the reason: %d %v", code, m)
	}
	code, m = write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"id": `"` + id + `"`, "enabled": "true"}), 2)
	if code != 200 || data(m)["enabled"] != true || data(m)["disabledReason"] != nil {
		t.Fatalf("enable: %d %v", code, m)
	}
	if _, m := post(s, &customerB, "automations.list", `{"filters":{"unitId":"`+u+`","kind":"schedule","enabled":true}}`); len(items(m)) != 1 {
		t.Errorf("list: %v", m)
	}
	if _, m := post(s, &customerA, "automations.list", `{"filters":{"unitId":"`+u+`"}}`); len(items(m)) != 0 {
		t.Error("another customer's list")
	}
	if code, _ := post(s, &customerB, "automations.list", `{"filters":{"kind":"cron"}}`); code != 422 {
		t.Error("bad kind filter")
	}
	if code, _ := post(s, &hq, "automations.list", `{}`); code != 403 {
		t.Error("HQ lists customer rules")
	}
}

func TestAutomationEvents(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Event AC")
	rule := func(cond string) string {
		return `{"name":"When home","unitIds":["` + u + `"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,"kind":"event","condition":` + cond + `,"action":{"kind":"set_temperature","celsius":24}}`
	}
	for _, c := range []string{`{"type":"occupancy"}`, `{"type":"pattern","localTime":"25:00"}`, `{"type":"weather","metric":"temperature","operator":"eq","value":30}`,
		`{"type":"tariff","operator":"gt","value":1,"unit":"MYR_per_kWh"}`, `{"type":"location","event":"leave"}`} {
		if code, _ := write(s, &customerB, "automations.save", rule(c), 0); code != 422 {
			t.Errorf("%s accepted", c)
		}
	}
	if code, m := write(s, &customerB, "automations.save", rule(`{"type":"weather","metric":"temperature","operator":"gte","value":30}`), 0); code != 200 || data(m)["condition"] == nil {
		t.Fatalf("weather rule: %d %v", code, m)
	} else if code, _ := post(s, &customerB, "automations.nextRuns", `{"automationId":"`+data(m)["id"].(string)+`","count":8}`); code != 422 {
		t.Error("event rules have no next runs")
	}
	owner(t, `UPDATE identity.consents SET granted = false WHERE membership_id = $1`, seed.ID("customer-b"))
	if code, _ := write(s, &customerB, "automations.save", rule(`{"type":"location","event":"arrival"}`), 0); code != 422 {
		t.Error("location without consent")
	}
	owner(t, `UPDATE identity.consents SET granted = true, granted_at = $2 WHERE membership_id = $1`, seed.ID("customer-b"), clock)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.consents SET granted = false WHERE membership_id = $1`, seed.ID("customer-b"))
	})
	if code, m := write(s, &customerB, "automations.save", rule(`{"type":"location","event":"arrival"}`), 0); code != 200 {
		t.Fatalf("location rule: %d %v", code, m)
	}
}
