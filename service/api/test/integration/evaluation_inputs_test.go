package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestEvaluationInputValidation covers the D02 / IR16 input rules of automations.simulate: ids, the tick, the unit
// list and every fact shape.
func TestEvaluationInputValidation(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	at := clock.Format(time.RFC3339)
	fact := func(unit, metric, value, unitName, quality string) string {
		return `{"unitId":"` + unit + `","metric":"` + metric + `","value":` + value + `,"unit":"` + unitName + `","observedAt":"` + at + `","quality":"` + quality + `"}`
	}
	body := func(fields map[string]string) string {
		f := map[string]string{"eventId": `"` + uuid.NewString() + `"`, "occurredAt": `"` + at + `"`, "unitIds": `["` + u + `"]`, "facts": "[]", "phase": `"condition"`}
		for k, v := range fields {
			if v == "" {
				delete(f, k)
			} else {
				f[k] = v
			}
		}
		parts := []string{}
		for k, v := range f {
			parts = append(parts, `"`+k+`":`+v)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = `"` + uuid.NewString() + `"`
	}
	for name, c := range map[string]struct {
		fields map[string]string
		field  string
	}{
		"eventId missing":    {map[string]string{"eventId": ""}, "eventId"},
		"occurredAt missing": {map[string]string{"occurredAt": ""}, "occurredAt"},
		"phase unknown":      {map[string]string{"phase": `"nightly"`}, "phase"},
		"no units":           {map[string]string{"unitIds": "[]"}, "unitIds"},
		"too many units":     {map[string]string{"unitIds": "[" + strings.Join(many, ",") + "]"}, "unitIds"},
		"duplicate units":    {map[string]string{"unitIds": `["` + u + `","` + u + `"]`}, "unitIds"},
		"number as text":     {map[string]string{"facts": "[" + fact(u, "temperature", `"hot"`, "°C", "valid") + "]"}, "facts"},
		"boolean as number":  {map[string]string{"facts": "[" + fact(u, "occupied", "1", "boolean", "valid") + "]"}, "facts"},
		"location event":     {map[string]string{"facts": "[" + fact(u, "location", `"home"`, "event", "valid") + "]"}, "facts"},
		"unknown metric":     {map[string]string{"facts": "[" + fact(u, "noise", "1", "dB", "valid") + "]"}, "facts"},
		"wrong unit":         {map[string]string{"facts": "[" + fact(u, "temperature", "25", "K", "valid") + "]"}, "facts"},
		"bad quality":        {map[string]string{"facts": "[" + fact(u, "temperature", "25", "°C", "good") + "]"}, "facts"},
		"fact of other unit": {map[string]string{"facts": "[" + fact(uuid.NewString(), "temperature", "25", "°C", "valid") + "]"}, "facts"},
		"duplicate fact":     {map[string]string{"facts": "[" + fact(u, "temperature", "25", "°C", "valid") + "," + fact(u, "temperature", "26", "°C", "valid") + "]"}, "facts"},
	} {
		code, m := post(s, &customerB, "automations.simulate", body(c.fields))
		if code != 422 || m["fieldErrors"].(map[string]any)[c.field] == nil {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	if code, m := post(s, &customerB, "automations.simulate", body(map[string]string{"facts": "[" + fact(u, "temperature", "null", "°C", "missing") + "," + fact(u, "location", `"arrival"`, "event", "valid") + "]"})); code != 200 {
		t.Fatalf("null and location facts are valid: %d %v", code, m)
	}
	if code, m := post(s, &customerB, "automations.simulate", body(map[string]string{"occurredAt": `"` + clock.Add(time.Minute).Format(time.RFC3339) + `"`})); code != 422 || m["fieldErrors"].(map[string]any)["occurredAt"] != "errors.not_current_tick" {
		t.Fatalf("simulate outside the current tick: %d %v", code, m)
	}
}

// TestEvaluationFireReplay covers the D02 / IR21 idempotency of automations.fire: a repeated eventId returns the
// stored result, a reused eventId with other input is CONFLICT, another eventId with the same input in the same tick
// reuses the result, other input in the same tick is CONFLICT, and a past tick is VALIDATION.
func TestEvaluationFireReplay(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)
	u := boundUnit(t, s, "online")
	if code, m := write(s, &customerB, "automations.save", `{"name":"replay","unitIds":["`+u+`"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,"kind":"event","condition":{"type":"occupancy","occupied":true},"action":{"kind":"set_temperature","celsius":24}}`, 0); code != 200 {
		t.Fatalf("rule: %d %v", code, m)
	}
	at := clock.Format(time.RFC3339)
	input := func(event, occupied, occurred string) string {
		return `{"eventId":"` + event + `","occurredAt":"` + occurred + `","unitIds":["` + u + `"],"facts":[{"unitId":"` + u + `","metric":"occupied","value":` + occupied + `,"unit":"boolean","observedAt":"` + at + `","quality":"valid"}],"phase":"condition"}`
	}
	decision := func(m map[string]any) map[string]any { return data(m)["results"].([]any)[0].(map[string]any) }
	e1 := uuid.NewString()
	code, first := write(s, &customerB, "automations.fire", input(e1, "true", at), 0)
	if code != 200 || decision(first)["decision"] != "requested" || decision(first)["commandId"] == nil {
		t.Fatalf("first fire: %d %v", code, first)
	}
	cmd := decision(first)["commandId"]
	if code, m := write(s, &customerB, "automations.fire", input(e1, "true", at), 0); code != 200 || decision(m)["commandId"] != cmd {
		t.Fatalf("repeated eventId returns the stored result: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "automations.fire", input(e1, "false", at), 0); code != 409 || m["messageKey"] != "errors.event_reused" {
		t.Fatalf("eventId reused with other input: %d %v", code, m)
	}
	e2 := uuid.NewString()
	if code, m := write(s, &customerB, "automations.fire", input(e2, "true", at), 0); code != 200 || data(m)["eventId"] != e2 || decision(m)["commandId"] != cmd {
		t.Fatalf("same input in the same tick reuses the result: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "automations.fire", input(uuid.NewString(), "false", at), 0); code != 409 || m["messageKey"] != "errors.tick_already_evaluated" {
		t.Fatalf("other input in the same tick: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "automations.fire", input(uuid.NewString(), "true", clock.Add(-time.Minute).Format(time.RFC3339)), 0); code != 422 || m["fieldErrors"].(map[string]any)["occurredAt"] != "errors.not_current_tick" {
		t.Fatalf("a past tick: %d %v", code, m)
	}
	var commands int
	ownerScan(t, `SELECT count(*) FROM control.commands WHERE unit_id = $1 AND source = 'automation'`, []any{u}, &commands)
	if commands != 1 {
		t.Fatalf("one command for the tick, got %d", commands)
	}
}
