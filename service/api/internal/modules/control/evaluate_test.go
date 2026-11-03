package control

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

const kualaLumpur = "Asia/Kuala_Lumpur"

func TestCompare(t *testing.T) {
	for _, testCase := range []struct {
		op   string
		a, b float64
		want bool
	}{
		{"gt", 2, 1, true},
		{"gt", 1, 1, false},
		{opGTE, 1, 1, true},
		{opGTE, 0.5, 1, false},
		{"lt", 0.5, 1, true},
		{"lt", 1, 1, false},
		{"lte", 1, 1, true},
		{"lte", 2, 1, false},
		{"other", 1, 2, true},
	} {
		if got := compare(testCase.op, testCase.a, testCase.b); got != testCase.want {
			t.Errorf("compare(%s, %v, %v) = %v", testCase.op, testCase.a, testCase.b, got)
		}
	}
}

func TestConditionMetric(t *testing.T) {
	for cond, want := range map[string]string{
		`{"type":"occupancy",` +
			`"occupied":true}`: "occupied", `{"type":"weather","operator":"gt",` +
			`"value":30}`: "weather_temperature", `{"type":"location","event":"arrival"}`: condLocation,
		`{"type":"peak",` +
			`"active":true}`: condPeak, `{"type":"tariff"}`: condTariff, `{"type":"solar"}`: condSolar,
		`{"type":"battery"}`: condBattery, `{"type":"pattern","localTime":"09:00"}`: "", `not json`: "",
	} {
		if got := conditionMetric(json.RawMessage(cond)); got != want {
			t.Errorf("conditionMetric(%s) = %q, want %q", cond, got, want)
		}
	}
}

// facts of one unit at 2026-09-14 09:00 Kuala Lumpur (01:00Z, a Monday).
func testFacts(unit uuid.UUID, when time.Time) map[string]Fact {
	fact := func(metric, value, unitName string, observed time.Time) Fact {
		return Fact{
			UnitID:     unit,
			Metric:     metric,
			Value:      json.RawMessage(value),
			Unit:       unitName,
			ObservedAt: observed,
			Quality:    "valid",
		}
	}

	return map[string]Fact{
		unit.String() + "/peak": {
			UnitID:     uuid.UUID{},
			Metric:     "",
			Value:      nil,
			Unit:       "",
			ObservedAt: time.Time{},
			Quality:    "",
		}, // overwritten below to keep the literal readable
		unit.String() + "/tariff":   fact(condTariff, "0.6", "MYR_per_kWh", when),
		unit.String() + "/solar":    fact(condSolar, "3", "kW", when),
		unit.String() + "/battery":  fact(condBattery, "0.5", "kW", when),
		unit.String() + "/location": fact(condLocation, `"arrival"`, unitEvent, when),
		unit.String() + "/weather_temperature": fact(
			"weather_temperature",
			"31",
			"°C",
			when.Add(-10*time.Minute),
		), // within the 30-minute TTL
		unit.String() + "/occupied": fact("occupied", "true", valueBoolean, when),
	}
}

func TestMatchConditions(t *testing.T) {
	unit := uuid.New()
	when := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	facts := testFacts(unit, when)
	facts[unit.String()+"/peak"] = Fact{
		UnitID:     unit,
		Metric:     condPeak,
		Value:      json.RawMessage("true"),
		Unit:       valueBoolean,
		ObservedAt: when,
		Quality:    "valid",
	}

	matchCond := func(cond string) string { return match(json.RawMessage(cond), unit, facts, when, kualaLumpur) }
	for cond, want := range map[string]string{
		`{"type":"peak","active":true}`: "", `{"type":"peak","active":false}`: reasonNoMatch,
		`{"type":"occupancy","occupied":true}`: "", `{"type":"occupancy","occupied":false}`: reasonNoMatch,
		`{"type":"tariff","operator":"gte",` +
			`"value":0.5}`: "", `{"type":"tariff","operator":"gt",` +
			`"value":0.6}`: reasonNoMatch, `{"type":"tariff","operator":"lte","value":0.6}`: "",
		`{"type":"solar","operator":"gt",` +
			`"value":2.5}`: "", `{"type":"battery","operator":"lt",` +
			`"value":1}`: "", `{"type":"battery","operator":"gte","value":1}`: reasonNoMatch,
		`{"type":"location","event":"arrival"}`: "", `{"type":"location","event":"departure"}`: reasonNoMatch,
		`{"type":"pattern","localTime":"09:00"}`: "", `{"type":"pattern","localTime":"09:01"}`: reasonNoMatch,
		`{"type":"weather","operator":"gt","value":30}`: "", `{"type":"weather","operator":"lt","value":30}`: reasonNoMatch,
		`{"type":"unknown"}`: reasonNoMatch,
	} {
		if got := matchCond(cond); got != want {
			t.Errorf("match(%s) = %q, want %q", cond, got, want)
		}
	}

	if got := match(
		json.RawMessage(`{"type":"pattern","localTime":"09:00"}`),
		unit,
		facts,
		when,
		"Mars/Olympus",
	); got != reasonNoMatch {
		t.Errorf("unknown time zone: %q", got)
	}

	other := uuid.New()
	if got := match(
		json.RawMessage(`{"type":"peak","active":true}`),
		other,
		facts,
		when,
		kualaLumpur,
	); got != reasonMissingData {
		t.Errorf("no fact for the unit: %q", got)
	}

	if got := match(
		json.RawMessage(`{"type":"location","event":"arrival"}`),
		unit,
		facts,
		when.Add(2*time.Minute),
		kualaLumpur,
	); got != qualityStale {
		t.Errorf("a location event of another minute is stale: %q", got)
	}

	if got := match(
		json.RawMessage(`{"type":"tariff","operator":"gte","value":0.5}`),
		unit,
		facts,
		when.Add(10*time.Minute),
		kualaLumpur,
	); got != qualityStale {
		t.Errorf("a tariff older than its TTL is stale: %q", got)
	}
}

func TestExtrasMatch(t *testing.T) {
	unit := uuid.New()
	sunday := time.Date(
		2026,
		9,
		13,
		1,
		0,
		0,
		0,
		time.UTC,
	) // Sunday 09:00 in Kuala Lumpur: ISO weekday 7

	facts := testFacts(unit, sunday)
	for raw, want := range map[string]string{
		`[{"type":"weekday",` +
			`"weekdays":[7]}]`: "", `[{"type":"weekday","weekdays":[1,` +
			`6]}]`: reasonNoMatch, `[{"type":"weekday","weekdays":[7]},{"type":"occupancy","occupied":true}]`: "",
		`[{"type":"occupancy",` +
			`"occupied":false}]`: reasonNoMatch, `[{"type":"weather","operator":"gt",` +
			`"value":30}]`: "", `[]`: "", `{"type":"weekday"}`: "",
	} {
		if got := extrasMatch(json.RawMessage(raw), unit, facts, sunday, kualaLumpur); got != want {
			t.Errorf("extrasMatch(%s) = %q, want %q", raw, got, want)
		}
	}

	if got := extrasMatch(
		json.RawMessage(`[{"type":"weekday","weekdays":[7]}]`),
		unit,
		facts,
		sunday,
		"Mars/Olympus",
	); got != reasonNoMatch {
		t.Errorf("unknown time zone: %q", got)
	}

	if got := extrasMatch(
		json.RawMessage(`[{"type":"occupancy","occupied":true}]`),
		uuid.New(),
		facts,
		sunday,
		kualaLumpur,
	); got != reasonMissingData {
		t.Errorf("missing occupancy data never matches: %q", got)
	}
}
