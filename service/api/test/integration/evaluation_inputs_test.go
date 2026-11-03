package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const fieldFacts = "facts"

// TestEvaluationInputValidation covers the D02 / IR16 input rules of automations.simulate: ids, the tick, the unit
// list and every fact shape.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestEvaluationInputValidation(t *testing.T) {
	srv := server(t)
	unitID := boundUnit(t, srv, "online")
	at := clock().Format(time.RFC3339)
	fact := func(unit, metric, value, unitName, quality string) string {
		return `{"unitId":"` + unit + `","metric":"` + metric + `","value":` + value + `,"unit":"` + unitName +
			`","observedAt":"` + at + `","quality":"` + quality + `"}`
	}
	body := func(fields map[string]string) string {
		f := map[string]string{
			fieldEventID:    `"` + uuid.NewString() + `"`,
			fieldOccurredAt: `"` + at + `"`,
			fieldUnitIDs:    `["` + unitID + `"]`,
			fieldFacts:      "[]",
			fieldPhase:      `"condition"`,
		}

		for k, v := range fields {
			if v == "" {
				delete(f, k)
			} else {
				f[k] = v
			}
		}

		parts := make([]string, 0, len(f))
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
		"eventId missing":    {map[string]string{fieldEventID: ""}, fieldEventID},
		"occurredAt missing": {map[string]string{fieldOccurredAt: ""}, fieldOccurredAt},
		"phase unknown":      {map[string]string{fieldPhase: `"nightly"`}, fieldPhase},
		caseNoUnits:          {map[string]string{fieldUnitIDs: "[]"}, fieldUnitIDs},
		"too many units":     {map[string]string{fieldUnitIDs: "[" + strings.Join(many, ",") + "]"}, fieldUnitIDs},
		caseDuplicateUnits:   {map[string]string{fieldUnitIDs: `["` + unitID + `","` + unitID + `"]`}, fieldUnitIDs},
		"number as text": {
			map[string]string{fieldFacts: "[" + fact(unitID, "temperature", `"hot"`, "°C", "valid") + "]"},
			fieldFacts,
		},
		"boolean as number": {
			map[string]string{fieldFacts: "[" + fact(unitID, "occupied", "1", "boolean", "valid") + "]"},
			fieldFacts,
		},
		"location event": {
			map[string]string{fieldFacts: "[" + fact(unitID, "location", `"home"`, "event", "valid") + "]"},
			fieldFacts,
		},
		caseUnknownMetric: {map[string]string{fieldFacts: "[" + fact(unitID, "noise", "1", "dB", "valid") + "]"}, fieldFacts},
		"wrong unit": {
			map[string]string{fieldFacts: "[" + fact(unitID, "temperature", "25", "K", "valid") + "]"},
			fieldFacts,
		},
		"bad quality": {
			map[string]string{fieldFacts: "[" + fact(unitID, "temperature", "25", "°C", "good") + "]"},
			fieldFacts,
		},
		"fact of other unit": {
			map[string]string{fieldFacts: "[" + fact(uuid.NewString(), "temperature", "25", "°C", "valid") + "]"}, fieldFacts,
		},
		"duplicate fact": {
			map[string]string{fieldFacts: "[" + fact(unitID, "temperature", "25", "°C", "valid") + "," +
				fact(unitID, "temperature", "26", "°C", "valid") + "]"}, fieldFacts,
		},
	} {
		code, m := post(srv, customerB(), "automations.simulate", body(c.fields))
		if code != 422 || asObj(t, m["fieldErrors"])[c.field] == nil {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}

	if code, res := post(
		srv,
		customerB(),
		"automations.simulate",
		body(
			map[string]string{
				fieldFacts: "[" + fact(
					unitID,
					"temperature",
					"null",
					"°C",
					"missing",
				) + "," + fact(
					unitID,
					"location",
					`"arrival"`,
					"event",
					"valid",
				) + "]",
			},
		),
	); code != 200 {
		t.Fatalf("null and location facts are valid: %d %v", code, res)
	}

	if code, res := post(
		srv,
		customerB(),
		"automations.simulate",
		body(
			map[string]string{
				fieldOccurredAt: `"` + clock().Add(time.Minute).Format(time.RFC3339) + `"`,
			},
		),
	); code != 422 ||
		asObj(t, res["fieldErrors"])["occurredAt"] != "errors.not_current_tick" {
		t.Fatalf("simulate outside the current tick: %d %v", code, res)
	}
}

// TestEvaluationFireReplay covers the D02 / IR21 idempotency of automations.fire: a repeated eventId returns the
// stored result, a reused eventId with other input is CONFLICT, another eventId with the same input in the same tick
// reuses the result, other input in the same tick is CONFLICT, and a past tick is VALIDATION.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestEvaluationFireReplay(t *testing.T) {
	srv := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)

	unit := boundUnit(t, srv, "online")
	if code, res := write(
		srv,
		customerB(),
		"automations.save",
		`{"name":"replay",`+
			`"unitIds":["`+unit+`"],"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,"kind":"event",`+
			`"condition":{"type":"occupancy","occupied":true},"action":{"kind":"set_temperature","celsius":24}}`,
		0,
	); code != 200 {
		t.Fatalf("rule: %d %v", code, res)
	}

	at := clock().Format(time.RFC3339)
	input := func(event, occupied, occurred string) string {
		return `{"eventId":"` + event + `","occurredAt":"` + occurred + `","unitIds":["` + unit + `"],"facts":[{"unitId":"` +
			unit +
			`","metric":"occupied","value":` + occupied + `,"unit":"boolean","observedAt":"` + at +
			`","quality":"valid"}],"phase":"condition"}`
	}
	decision := func(m map[string]any) map[string]any { return asObj(t, asArr(t, data(m)["results"])[0]) }
	e1 := uuid.NewString()

	code, first := write(srv, customerB(), "automations.fire", input(e1, "true", at), 0)
	if code != 200 || decision(first)["decision"] != decisionRequested ||
		decision(first)["commandId"] == nil {
		t.Fatalf("first fire: %d %v", code, first)
	}

	cmd := decision(first)["commandId"]
	if code, res := write(
		srv,
		customerB(),
		"automations.fire",
		input(e1, "true", at),
		0,
	); code != 200 ||
		decision(res)["commandId"] != cmd {
		t.Fatalf("repeated eventId returns the stored result: %d %v", code, res)
	}

	if code, res := write(
		srv,
		customerB(),
		"automations.fire",
		input(e1, "false", at),
		0,
	); code != 409 ||
		res["messageKey"] != "errors.event_reused" {
		t.Fatalf("eventId reused with other input: %d %v", code, res)
	}

	e2 := uuid.NewString()
	if code, res := write(
		srv,
		customerB(),
		"automations.fire",
		input(e2, "true", at),
		0,
	); code != 200 || data(res)["eventId"] != e2 ||
		decision(res)["commandId"] != cmd {
		t.Fatalf("same input in the same tick reuses the result: %d %v", code, res)
	}

	if code, res := write(
		srv,
		customerB(),
		"automations.fire",
		input(uuid.NewString(), "false", at),
		0,
	); code != 409 ||
		res["messageKey"] != "errors.tick_already_evaluated" {
		t.Fatalf("other input in the same tick: %d %v", code, res)
	}

	if code, res := write(
		srv,
		customerB(),
		"automations.fire",
		input(uuid.NewString(), "true", clock().Add(-time.Minute).Format(time.RFC3339)),
		0,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["occurredAt"] != "errors.not_current_tick" {
		t.Fatalf("a past tick: %d %v", code, res)
	}

	var commands int
	ownerScan(
		t,
		`SELECT count(*) FROM control.commands WHERE unit_id = $1 AND source = 'automation'`,
		[]any{unit},
		&commands,
	)

	if commands != 1 {
		t.Fatalf("one command for the tick, got %d", commands)
	}
}
