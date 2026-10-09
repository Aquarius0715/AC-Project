package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// automationCommands returns the actions of the unit's automation Commands, oldest first.
func automationCommands(t *testing.T, unit string) []string {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(context.Background())
	rows, err := conn.Query(context.Background(), `SELECT action::text FROM control.commands WHERE unit_id = $1 AND source = 'automation' ORDER BY requested_at, id`, unit)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// setWatermark moves the tenant's schedule watermark (IR214) so a test controls the window the next tick evaluates.
func setWatermark(t *testing.T, at time.Time) {
	t.Helper()
	owner(t, `INSERT INTO control.schedule_watermarks (tenant_id, evaluated_until) VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE SET evaluated_until = EXCLUDED.evaluated_until`, seed.ID("tenant-a"), at)
}

// IR54 / IR214 / AT-C04-N: the equipment scheduler fires a schedule rule at its start and end minutes — once per
// occurrence, re-authorizing the owner, audited — and a disabled or deleted rule creates nothing. clock is Monday
// 2026-09-14 09:00 Kuala Lumpur.
func TestScheduleAutomationsFire(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	tick := func(at time.Time) {
		t.Helper()
		if _, err := schedTick(ctx, s, at); err != nil {
			t.Fatal(err)
		}
	}
	u := boundUnit(t, s, "online")
	code, m := write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"weekdays": "[1]", "startLocal": `"09:30"`, "endLocal": `"10:00"`, "enabled": "true",
		"startAction": `{"kind":"set_temperature","celsius":25}`}), 0)
	if code != 200 {
		t.Fatalf("save: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if n := len(automationCommands(t, u)); n != 0 {
		t.Fatalf("saving sends no command: %d", n)
	}
	setWatermark(t, clock)
	tick(clock.Add(20 * time.Minute)) // before the start
	if n := len(automationCommands(t, u)); n != 0 {
		t.Fatalf("before the start: %d", n)
	}
	tick(clock.Add(31 * time.Minute))
	cs := automationCommands(t, u)
	var a map[string]any
	if len(cs) == 1 {
		_ = json.Unmarshal([]byte(cs[0]), &a)
	}
	if len(cs) != 1 || a["kind"] != "set_temperature" || a["celsius"].(float64) != 25 {
		t.Fatalf("start command: %v", cs)
	}
	tick(clock.Add(31 * time.Minute)) // the same window again
	setWatermark(t, clock)            // even a replayed window creates no second Command (automation_runs)
	tick(clock.Add(32 * time.Minute))
	if n := len(automationCommands(t, u)); n != 1 {
		t.Fatalf("one Command per occurrence: %d", n)
	}
	tick(clock.Add(61 * time.Minute))
	if cs = automationCommands(t, u); len(cs) != 2 || cs[1] != `{"kind": "set_power", "power": false}` {
		t.Fatalf("end command: %v", cs)
	}
	var runs, audits int
	ownerScan(t, `SELECT count(*) FROM control.automation_runs WHERE rule_id = $1 AND trigger_ref LIKE 'schedule:%'`, []any{id}, &runs)
	ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE action = 'automations.schedule' AND target_id = $1 AND actor_role_at_time = 'system'`, []any{id}, &audits)
	if runs != 2 || audits != 2 {
		t.Fatalf("runs %d, audits %d", runs, audits)
	}
	// disabled: the next Monday's start creates nothing
	code, m = write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"id": `"` + id + `"`, "weekdays": "[1]", "startLocal": `"09:30"`, "endLocal": `"10:00"`,
		"enabled": "false", "startAction": `{"kind":"set_temperature","celsius":25}`}), 1)
	if code != 200 {
		t.Fatalf("disable: %d %v", code, m)
	}
	tick(clock.Add(7*24*time.Hour + 31*time.Minute))
	if n := len(automationCommands(t, u)); n != 2 {
		t.Fatalf("disabled rule fired: %d", n)
	}
	// delete (Figma 03g–03i): version checked, the rule is gone, its Commands stay
	if code, _ := write(s, &customerB, "automations.delete", `{"id":"`+id+`"}`, 1); code != 409 {
		t.Fatalf("stale version: %d", code)
	}
	if code, _ := write(s, &customerA, "automations.delete", `{"id":"`+id+`"}`, 2); code != 404 {
		t.Fatalf("other customer: %d", code)
	}
	code, m = write(s, &customerB, "automations.delete", `{"id":"`+id+`"}`, 2)
	if code != 200 || data(m)["deleted"] != true || data(m)["id"] != id {
		t.Fatalf("delete: %d %v", code, m)
	}
	if _, m := post(s, &customerB, "automations.list", `{"filters":{"unitId":"`+u+`"}}`); len(items(m)) != 0 {
		t.Fatalf("deleted rule listed: %v", items(m))
	}
	if code, _ := write(s, &customerB, "automations.delete", `{"id":"`+id+`"}`, 3); code != 404 {
		t.Fatalf("already deleted: %d", code)
	}
	if n := len(automationCommands(t, u)); n != 2 {
		t.Fatalf("history kept: %d", n)
	}
}

// IR214: automations.nextRuns previews an unsaved schedule (the C04 editor's Test) with the save rules.
func TestScheduleDraftPreview(t *testing.T) {
	s := server(t)
	draft := func(extra string) string {
		return `{"count":8,"draft":{"timezone":"Asia/Kuala_Lumpur","weekdays":[1,3],"startLocal":"18:00","endLocal":"22:00","endsNextDay":false,` +
			`"startAction":{"kind":"set_temperature","celsius":25},"endAction":{"kind":"set_power","power":false}` + extra + `}}`
	}
	code, m := post(s, &customerA, "automations.nextRuns", draft(""))
	list, _ := m["data"].([]any)
	if code != 200 {
		t.Fatalf("draft: %d %v", code, m)
	}
	if len(list) != 8 {
		t.Fatalf("eight occurrences: %v", m)
	}
	first := list[0].(map[string]any)
	if first["automationId"] != nil || first["phase"] != "schedule_start" || first["at"] != "2026-09-14T10:00:00Z" || list[1].(map[string]any)["at"] != "2026-09-14T14:00:00Z" {
		t.Fatalf("Monday 18:00 → 22:00 Kuala Lumpur: %v", list[:2])
	}
	for name, body := range map[string]string{
		"both targets":   `{"count":8,"automationId":"` + uuid.NewString() + `","draft":{}}`,
		"no target":      `{"count":8}`,
		"bad count":      strings2(draft(""), `"count":8`, `"count":3`),
		"same times":     strings2(draft(""), `"endLocal":"22:00"`, `"endLocal":"18:00"`),
		"no weekdays":    strings2(draft(""), `"weekdays":[1,3]`, `"weekdays":[]`),
		"overnight flag": strings2(draft(""), `"endLocal":"22:00"`, `"endLocal":"06:00"`),
		"bad action":     strings2(draft(""), `{"kind":"set_power","power":false}`, `{"kind":"explode"}`),
		"bad zone":       strings2(draft(""), `"Asia/Kuala_Lumpur"`, `"Moon/Base"`),
		"nonexistent":    `{"count":8,"draft":{"timezone":"America/New_York","weekdays":[7],"startLocal":"02:30","endLocal":"05:00","endsNextDay":false,"startAction":{"kind":"set_power","power":true},"endAction":{"kind":"set_power","power":false}}}`,
	} {
		if code, _ := post(s, &customerA, "automations.nextRuns", body); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
}

// strings2 replaces the first occurrence of old in s.
func strings2(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

// IR53 / AT-C05-N ④: withdrawing location consent disables the owner's enabled location rules (consent_revoked,
// version + 1); granting again re-enables nothing, an explicit save with enabled=true does and clears the reason.
func TestConsentWithdrawalDisablesLocationRules(t *testing.T) {
	s := server(t)
	consent := func() (bool, int) {
		t.Helper()
		_, m := post(s, &customerA, "consents.get", `{"purpose":"location_automation"}`)
		return data(m)["granted"] == true, ver(m)
	}
	set := func(granted bool) {
		t.Helper()
		g, v := consent()
		if g == granted {
			return
		}
		if code, m := write(s, &customerA, "consents.update", `{"purpose":"location_automation","granted":`+map[bool]string{true: "true", false: "false"}[granted]+`}`, v); code != 200 {
			t.Fatalf("consent %v: %d %v", granted, code, m)
		}
		drainEvents(t)
	}
	unit := seed.ID("unit-online-rto").String()
	rule := func(id string, version int, enabled bool, event string) (int, map[string]any) {
		body := `{"name":"Arrive cool","unitIds":["` + unit + `"],"timezone":"Asia/Kuala_Lumpur","enabled":` + map[bool]string{true: "true", false: "false"}[enabled] +
			`,"priority":50,"kind":"event","condition":{"type":"location","event":"` + event + `"},"action":{"kind":"set_temperature","celsius":24}`
		if id != "" {
			body += `,"id":"` + id + `"`
		}
		return write(s, &customerA, "automations.save", body+"}", version)
	}
	set(true)
	code, m := rule("", 0, true, "arrival")
	if code != 200 {
		t.Fatalf("location rule: %d %v", code, m)
	}
	loc := data(m)["id"].(string)
	code, m = write(s, &customerA, "automations.save", scheduleRule(unit, map[string]string{"enabled": "true", "weekdays": "[6]"}), 0)
	if code != 200 {
		t.Fatalf("schedule rule: %d %v", code, m)
	}
	sched := data(m)["id"].(string)
	get := func(id string) map[string]any {
		t.Helper()
		_, m := post(s, &customerA, "automations.list", `{"limit":100,"filters":{"unitId":"`+unit+`"}}`)
		for _, it := range items(m) {
			if it["id"] == id {
				return it
			}
		}
		t.Fatalf("%s not listed", id)
		return nil
	}
	set(false)
	if r := get(loc); r["enabled"] != false || r["disabledReason"] != "consent_revoked" || r["version"].(float64) != 2 {
		t.Fatalf("withdrawn: %v", r)
	}
	if r := get(sched); r["enabled"] != true || r["disabledReason"] != nil {
		t.Fatalf("schedule rules are unaffected: %v", r)
	}
	set(true)
	if r := get(loc); r["enabled"] != false || r["disabledReason"] != "consent_revoked" {
		t.Fatalf("granting again re-enables nothing: %v", r)
	}
	if code, m := rule(loc, 2, true, "arrival"); code != 200 || data(m)["enabled"] != true || data(m)["disabledReason"] != nil {
		t.Fatalf("explicit enable: %d %v", code, m)
	}
	// leave the seed state: no rules, consent withdrawn
	for id, v := range map[string]int{loc: 3, sched: 1} {
		if code, _ := write(s, &customerA, "automations.delete", `{"id":"`+id+`"}`, v); code != 200 {
			t.Errorf("cleanup %s: %d", id, code)
		}
	}
	set(false)
}

// IR215: every onlyIf condition must hold when a rule runs. A rule whose own trigger held but that could not run keeps
// the reason in its run log (lastRun); events that are not for the rule leave no trace.
func TestAutomationExtraConditions(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	u := boundUnit(t, s, "online")
	base := `{"name":"Hot afternoon","unitIds":["` + u + `"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":50,"kind":"event",` +
		`"condition":{"type":"weather","metric":"temperature","operator":"gte","value":33},"action":{"kind":"set_temperature","celsius":24}`
	weather := `{"type":"weather","metric":"temperature","operator":"gte","value":30}`
	for name, extra := range map[string]string{
		"four conditions":    `[{"type":"weekday","weekdays":[1]},{"type":"occupancy","occupied":true},{"type":"weekday","weekdays":[2]},{"type":"occupancy","occupied":false}]`,
		"same type twice":    `[{"type":"occupancy","occupied":true},{"type":"occupancy","occupied":false}]`,
		"own condition type": `[` + weather + `]`,
		"no weekdays":        `[{"type":"weekday","weekdays":[]}]`,
		"weekday 8":          `[{"type":"weekday","weekdays":[8]}]`,
		"unknown type":       `[{"type":"tariff","operator":"gt","value":1}]`,
		"extra field":        `[{"type":"occupancy","occupied":true,"value":3}]`,
		"bad operator":       `[{"type":"occupancy","occupied":true},{"type":"weekday","weekdays":[1],"operator":"gt"}]`,
	} {
		if code, m := write(s, &customerB, "automations.save", base+`,"onlyIf":`+extra+`}`, 0); code != 422 || m["fieldErrors"].(map[string]any)["onlyIf"] == nil {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	if code, _ := write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"onlyIf": `[{"type":"weekday","weekdays":[1]}]`}), 0); code != 422 {
		t.Errorf("weekday on a schedule: %d", code)
	}
	code, m := write(s, &customerB, "automations.save", base+`,"onlyIf":[{"type":"weekday","weekdays":[1]},{"type":"occupancy","occupied":true}]}`, 0)
	if code != 200 || len(data(m)["onlyIf"].([]any)) != 2 || data(m)["lastRun"] != nil {
		t.Fatalf("save: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	fact := func(metric, value, unit string) string {
		return `{"unitId":"` + u + `","metric":"` + metric + `","value":` + value + `,"unit":"` + unit + `","observedAt":"` + clock.Format(time.RFC3339) + `","quality":"valid"}`
	}
	fire := func(facts ...string) map[string]any { // one evaluation per tick (D02): clear the tick and the unit's pending commands first
		t.Helper()
		owner(t, `DELETE FROM control.evaluation_events`)
		owner(t, `UPDATE control.commands SET status = 'expired' WHERE unit_id = $1 AND status IN ('requested','sent')`, u)
		code, m := write(s, &customerB, "automations.fire", `{"eventId":"`+uuid.NewString()+`","occurredAt":"`+clock.Format(time.RFC3339)+`","unitIds":["`+u+`"],"facts":[`+
			strings.Join(facts, ",")+`],"phase":"condition"}`, 0)
		if code != 200 {
			t.Fatalf("fire: %d %v", code, m)
		}
		return data(m)["results"].([]any)[0].(map[string]any)
	}
	last := func() map[string]any {
		t.Helper()
		_, m := post(s, &customerB, "automations.list", `{"limit":10,"filters":{"unitId":"`+u+`"}}`)
		for _, it := range items(m) {
			if it["id"] == id {
				lr, _ := it["lastRun"].(map[string]any)
				return lr
			}
		}
		t.Fatal("rule not listed")
		return nil
	}
	runs := func() int {
		var n int
		ownerScan(t, `SELECT count(*) FROM control.automation_runs WHERE rule_id = $1`, []any{id}, &n)
		return n
	}
	// Monday, hot and someone home: runs
	if d := fire(fact("weather_temperature", "34", "°C"), fact("occupied", "true", "boolean")); d["decision"] != "requested" || d["ruleId"] != id {
		t.Fatalf("all conditions hold: %v", d)
	}
	if lr := last(); lr == nil || lr["outcome"] != "command_created" || lr["commandId"] == nil || lr["at"] != clock.Format(time.RFC3339) {
		t.Fatalf("lastRun after a command: %v", lr)
	}
	// hot, but nobody home: the extra condition fails and is logged
	if d := fire(fact("weather_temperature", "34", "°C"), fact("occupied", "false", "boolean")); d["decision"] != "suppressed" || d["reason"] != "no_match" {
		t.Fatalf("only if someone is home: %v", d)
	}
	if lr := last(); lr["outcome"] != "skipped" || lr["reason"] != "no_match" {
		t.Fatalf("lastRun no_match: %v", lr)
	}
	// hot, occupancy unknown: missing data never matches, the skip says why
	if d := fire(fact("weather_temperature", "34", "°C")); d["reason"] != "missing_data" {
		t.Fatalf("missing occupancy: %v", d)
	}
	if lr := last(); lr["outcome"] != "skipped" || lr["reason"] != "missing_data" {
		t.Fatalf("lastRun missing_data: %v", lr)
	}
	// its own data arrived incomplete: logged; an event that is not for the rule (no weather): no trace
	if d := fire(`{"unitId":"` + u + `","metric":"weather_temperature","value":null,"unit":"°C","observedAt":"` + clock.Format(time.RFC3339) + `","quality":"missing"}`); d["reason"] != "missing_data" {
		t.Fatalf("null weather: %v", d)
	}
	n := runs()
	fire(fact("occupied", "true", "boolean"))
	if runs() != n {
		t.Fatal("an occupancy event left a run on the weather rule")
	}
	// not a selected weekday
	code, m = write(s, &customerB, "automations.save", base+`,"id":"`+id+`","onlyIf":[{"type":"weekday","weekdays":[6,7]}]}`, 1)
	if code != 200 || len(data(m)["onlyIf"].([]any)) != 1 {
		t.Fatalf("weekend only: %d %v", code, m)
	}
	if d := fire(fact("weather_temperature", "34", "°C")); d["decision"] != "suppressed" || d["reason"] != "no_match" {
		t.Fatalf("Monday is not a weekend: %v", d)
	}
	// a schedule that needs someone home skips at its start for want of occupancy data (the scheduler has none)
	code, m = write(s, &customerB, "automations.save", scheduleRule(u, map[string]string{"weekdays": "[1]", "startLocal": `"09:30"`, "endLocal": `"10:00"`, "enabled": "true",
		"onlyIf": `[{"type":"occupancy","occupied":true}]`}), 0)
	if code != 200 {
		t.Fatalf("schedule with onlyIf: %d %v", code, m)
	}
	sid := data(m)["id"].(string)
	before := len(automationCommands(t, u))
	setWatermark(t, clock)
	if _, err := schedTick(ctx, s, clock.Add(31*time.Minute)); err != nil {
		t.Fatal(err)
	}
	var outcome, reason string
	ownerScan(t, `SELECT outcome, skip_reason FROM control.automation_runs WHERE rule_id = $1`, []any{sid}, &outcome, &reason)
	if outcome != "skipped" || reason != "missing_data" || len(automationCommands(t, u)) != before {
		t.Fatalf("schedule skipped: %s %s", outcome, reason)
	}
	for rid, v := range map[string]int{id: 2, sid: 1} { // leave no rule behind
		if code, _ := write(s, &customerB, "automations.delete", `{"id":"`+rid+`"}`, v); code != 200 {
			t.Errorf("cleanup: %d", code)
		}
	}
}
