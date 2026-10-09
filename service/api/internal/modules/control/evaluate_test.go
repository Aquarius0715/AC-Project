package control

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

const kl = "Asia/Kuala_Lumpur"

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		op   string
		a, b float64
		want bool
	}{{"gt", 2, 1, true}, {"gt", 1, 1, false}, {"gte", 1, 1, true}, {"gte", 0.5, 1, false}, {"lt", 0.5, 1, true}, {"lt", 1, 1, false}, {"lte", 1, 1, true}, {"lte", 2, 1, false}, {"other", 1, 2, true}} {
		if got := compare(c.op, c.a, c.b); got != c.want {
			t.Errorf("compare(%s, %v, %v) = %v", c.op, c.a, c.b, got)
		}
	}
}

func TestConditionMetric(t *testing.T) {
	for cond, want := range map[string]string{
		`{"type":"occupancy","occupied":true}`: "occupied", `{"type":"weather","operator":"gt","value":30}`: "weather_temperature", `{"type":"location","event":"arrival"}`: "location",
		`{"type":"peak","active":true}`: "peak", `{"type":"tariff"}`: "tariff", `{"type":"solar"}`: "solar", `{"type":"battery"}`: "battery", `{"type":"pattern","localTime":"09:00"}`: "", `not json`: "",
	} {
		if got := conditionMetric(json.RawMessage(cond)); got != want {
			t.Errorf("conditionMetric(%s) = %q, want %q", cond, got, want)
		}
	}
}

// facts of one unit at 2026-09-14 09:00 Kuala Lumpur (01:00Z, a Monday)
func testFacts(unit uuid.UUID, at time.Time) map[string]Fact {
	f := func(metric, value, unitName string, observed time.Time) Fact {
		return Fact{UnitID: unit, Metric: metric, Value: json.RawMessage(value), Unit: unitName, ObservedAt: observed, Quality: "valid"}
	}
	return map[string]Fact{
		unit.String() + "/peak":                {}, // overwritten below to keep the literal readable
		unit.String() + "/tariff":              f("tariff", "0.6", "MYR_per_kWh", at),
		unit.String() + "/solar":               f("solar", "3", "kW", at),
		unit.String() + "/battery":             f("battery", "0.5", "kW", at),
		unit.String() + "/location":            f("location", `"arrival"`, "event", at),
		unit.String() + "/weather_temperature": f("weather_temperature", "31", "°C", at.Add(-10*time.Minute)), // within the 30-minute TTL
		unit.String() + "/occupied":            f("occupied", "true", "boolean", at),
	}
}

func TestMatchConditions(t *testing.T) {
	unit := uuid.New()
	at := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	facts := testFacts(unit, at)
	facts[unit.String()+"/peak"] = Fact{UnitID: unit, Metric: "peak", Value: json.RawMessage("true"), Unit: "boolean", ObservedAt: at, Quality: "valid"}
	m := func(cond string) string { return match(json.RawMessage(cond), unit, facts, at, kl) }
	for cond, want := range map[string]string{
		`{"type":"peak","active":true}`: "", `{"type":"peak","active":false}`: "no_match",
		`{"type":"occupancy","occupied":true}`: "", `{"type":"occupancy","occupied":false}`: "no_match",
		`{"type":"tariff","operator":"gte","value":0.5}`: "", `{"type":"tariff","operator":"gt","value":0.6}`: "no_match", `{"type":"tariff","operator":"lte","value":0.6}`: "",
		`{"type":"solar","operator":"gt","value":2.5}`: "", `{"type":"battery","operator":"lt","value":1}`: "", `{"type":"battery","operator":"gte","value":1}`: "no_match",
		`{"type":"location","event":"arrival"}`: "", `{"type":"location","event":"departure"}`: "no_match",
		`{"type":"pattern","localTime":"09:00"}`: "", `{"type":"pattern","localTime":"09:01"}`: "no_match",
		`{"type":"weather","operator":"gt","value":30}`: "", `{"type":"weather","operator":"lt","value":30}`: "no_match",
		`{"type":"unknown"}`: "no_match",
	} {
		if got := m(cond); got != want {
			t.Errorf("match(%s) = %q, want %q", cond, got, want)
		}
	}
	if got := match(json.RawMessage(`{"type":"pattern","localTime":"09:00"}`), unit, facts, at, "Mars/Olympus"); got != "no_match" {
		t.Errorf("unknown time zone: %q", got)
	}
	other := uuid.New()
	if got := match(json.RawMessage(`{"type":"peak","active":true}`), other, facts, at, kl); got != "missing_data" {
		t.Errorf("no fact for the unit: %q", got)
	}
	if got := match(json.RawMessage(`{"type":"location","event":"arrival"}`), unit, facts, at.Add(2*time.Minute), kl); got != "stale" {
		t.Errorf("a location event of another minute is stale: %q", got)
	}
	if got := match(json.RawMessage(`{"type":"tariff","operator":"gte","value":0.5}`), unit, facts, at.Add(10*time.Minute), kl); got != "stale" {
		t.Errorf("a tariff older than its TTL is stale: %q", got)
	}
}

func TestExtrasMatch(t *testing.T) {
	unit := uuid.New()
	sunday := time.Date(2026, 9, 13, 1, 0, 0, 0, time.UTC) // Sunday 09:00 in Kuala Lumpur: ISO weekday 7
	facts := testFacts(unit, sunday)
	for raw, want := range map[string]string{
		`[{"type":"weekday","weekdays":[7]}]`: "", `[{"type":"weekday","weekdays":[1,6]}]`: "no_match", `[{"type":"weekday","weekdays":[7]},{"type":"occupancy","occupied":true}]`: "",
		`[{"type":"occupancy","occupied":false}]`: "no_match", `[{"type":"weather","operator":"gt","value":30}]`: "", `[]`: "", `{"type":"weekday"}`: "",
	} {
		if got := extrasMatch(json.RawMessage(raw), unit, facts, sunday, kl); got != want {
			t.Errorf("extrasMatch(%s) = %q, want %q", raw, got, want)
		}
	}
	if got := extrasMatch(json.RawMessage(`[{"type":"weekday","weekdays":[7]}]`), unit, facts, sunday, "Mars/Olympus"); got != "no_match" {
		t.Errorf("unknown time zone: %q", got)
	}
	if got := extrasMatch(json.RawMessage(`[{"type":"occupancy","occupied":true}]`), uuid.New(), facts, sunday, kl); got != "missing_data" {
		t.Errorf("missing occupancy data never matches: %q", got)
	}
}
