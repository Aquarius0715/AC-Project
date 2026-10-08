package app

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/seed"
)

func TestTelemetry(t *testing.T) {
	s := server(t)
	online, nonRTO, other := seed.ID("unit-online-rto").String(), seed.ID("unit-non-rto").String(), seed.ID("unit-other-customer").String()
	from, to := clock.Add(-24*time.Hour).Format(time.RFC3339), clock.Format(time.RFC3339)
	rng := `"from":"` + from + `","to":"` + to + `"`

	// series by unit: one unit → allergen observation (available); seeded reading id = fixture event id
	code, m := post(s, &customerA, "telemetry.series", `{`+rng+`,"unitIds":["`+online+`"],"metric":"co2","query":{}}`)
	if code != 200 || len(items(m)) != 1 || items(m)[0]["value"].(float64) != 800 || items(m)[0]["id"] != seed.ID("m-sensor-online-co2-1").String() ||
		items(m)[0]["id"] != items(m)[0]["eventId"] || items(m)[0]["isDemo"] != true {
		t.Fatalf("series: %d %v", code, m)
	}
	if a := data(m)["allergenObservation"].(map[string]any); a["availability"] != "available" || a["substance"] == nil {
		t.Fatalf("allergen: %v", a)
	}
	_, m = post(s, &customerA, "telemetry.series", `{`+rng+`,"unitIds":["`+nonRTO+`"],"metric":"co2","query":{}}`)
	if a := data(m)["allergenObservation"].(map[string]any); a["availability"] != "unsupported" || a["substance"] != nil || len(items(m)) != 0 {
		t.Fatalf("unsupported allergen: %v", m)
	}
	_, m = post(s, &customerA, "telemetry.series", `{`+rng+`,"unitIds":["`+seed.ID("unit-limited").String()+`"],"metric":"temperature","query":{}}`)
	if a := data(m)["allergenObservation"].(map[string]any); a["availability"] != "not_measured" || len(items(m)) != 1 {
		t.Fatalf("not measured: %v", m)
	}
	// several units: no allergen, ordered by observedAt
	_, m = post(s, &customerA, "telemetry.series", `{`+rng+`,"unitIds":["`+online+`","`+nonRTO+`","`+seed.ID("unit-offline-rto").String()+`"],"metric":"temperature","query":{"limit":2}}`)
	if data(m)["allergenObservation"] != nil || len(items(m)) != 2 || data(m)["total"].(float64) != 3 || items(m)[0]["value"].(float64) != 29 || data(m)["nextCursor"] == nil {
		t.Fatalf("multi-unit: %v", m)
	}
	_, m = post(s, &customerA, "telemetry.series", `{`+rng+`,"unitIds":["`+online+`","`+nonRTO+`"],"metric":"temperature","query":{"sort":{"field":"observedAt","direction":"desc"}}}`)
	if len(items(m)) != 2 {
		t.Fatal("desc sort")
	}
	// by space and by sensor
	_, m = post(s, &customerA, "telemetry.series", `{`+rng+`,"spaceId":"`+seed.ID("room-1").String()+`","metric":"humidity","query":{}}`)
	if len(items(m)) != 1 || data(m)["allergenObservation"] == nil {
		t.Fatalf("space: %v", m)
	}
	_, m = post(s, &hq, "telemetry.series", `{`+rng+`,"unitIds":["`+online+`"],"metric":"temperature","sensorId":"`+uuid.NewString()+`","query":{}}`)
	if len(items(m)) != 0 {
		t.Fatal("sensor filter")
	}
	// scope and validation
	for name, tc := range map[string]struct {
		a    *actor
		body string
		code int
	}{
		"other customer's unit":   {&customerA, `{` + rng + `,"unitIds":["` + other + `"],"metric":"co2","query":{}}`, 404},
		"other customer's space":  {&customerA, `{` + rng + `,"spaceId":"` + seed.ID("room-b-1").String() + `","metric":"co2","query":{}}`, 404},
		"technician in scope":     {&techB, `{` + rng + `,"unitIds":["` + other + `"],"metric":"co2","query":{}}`, 200},
		"technician out of scope": {&techB, `{` + rng + `,"unitIds":["` + online + `"],"metric":"co2","query":{}}`, 404},
		"both targets":            {&hq, `{` + rng + `,"unitIds":["` + online + `"],"spaceId":"` + seed.ID("room-1").String() + `","metric":"co2","query":{}}`, 422},
		"no target":               {&hq, `{` + rng + `,"metric":"co2","query":{}}`, 422},
		"empty unitIds":           {&hq, `{` + rng + `,"unitIds":[],"metric":"co2","query":{}}`, 422},
		"duplicate units":         {&hq, `{` + rng + `,"unitIds":["` + online + `","` + online + `"],"metric":"co2","query":{}}`, 422},
		"bad metric":              {&hq, `{` + rng + `,"unitIds":["` + online + `"],"metric":"noise","query":{}}`, 422},
		"reversed range":          {&hq, `{"from":"` + to + `","to":"` + from + `","unitIds":["` + online + `"],"metric":"co2","query":{}}`, 422},
		"range > 35 days":         {&hq, `{"from":"2026-08-01T00:00:00Z","to":"` + to + `","unitIds":["` + online + `"],"metric":"co2","query":{}}`, 422},
		"filters not allowed":     {&hq, `{` + rng + `,"unitIds":["` + online + `"],"metric":"co2","query":{"filters":{"x":1}}}`, 422},
		"bad sort":                {&hq, `{` + rng + `,"unitIds":["` + online + `"],"metric":"co2","query":{"sort":{"field":"value","direction":"asc"}}}`, 422},
		"missing from":            {&hq, `{"to":"` + to + `","unitIds":["` + online + `"],"metric":"co2","query":{}}`, 422},
		"missing to":              {&hq, `{"from":"` + from + `","unitIds":["` + online + `"],"metric":"co2","query":{}}`, 422},
	} {
		if code, _ := post(s, tc.a, "telemetry.series", tc.body); code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}

	// summary: latest per unit and metric
	code, m = post(s, &customerA, "telemetry.summary", `{`+rng+`,"unitIds":["`+online+`","`+nonRTO+`"]}`)
	if code != 200 || len(data(m)["measurements"].([]any)) != 8 || data(m)["energy"] != nil || data(m)["asOf"] != to {
		t.Fatalf("summary: %d %v", code, m)
	}
	_, m = post(s, &customerA, "telemetry.summary", `{`+rng+`,"unitIds":["`+online+`"],"metric":"power"}`)
	if len(data(m)["measurements"].([]any)) != 1 {
		t.Fatal("summary metric")
	}
	for _, b := range []string{`{` + rng + `,"unitIds":[]}`, `{` + rng + `,"unitIds":["` + online + `"],"metric":"x"}`, `{"from":"` + to + `","to":"` + from + `","unitIds":["` + online + `"]}`} {
		if code, _ := post(s, &customerA, "telemetry.summary", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerA, "telemetry.summary", `{`+rng+`,"unitIds":["`+other+`"]}`); code != 404 {
		t.Error("summary scope")
	}
}

func TestVentilation(t *testing.T) {
	s := server(t)
	room1, room2, roomB := seed.ID("room-1").String(), seed.ID("room-2").String(), seed.ID("room-b-1").String()
	logv := func(a *actor, body string) (int, map[string]any) { return write(s, a, "ventilation.log", body, 0) }

	code, m := logv(&customerA, `{"spaceId":"`+room1+`","method":"window_opened","durationMinutes":15}`)
	co2, _ := data(m)["co2AtLog"].(map[string]any)
	if code != 200 || co2 == nil || co2["value"].(float64) != 800 || co2["unit"] != "ppm" || data(m)["loggedAt"] != clock.Format(time.RFC3339) {
		t.Fatalf("log: %d %v", code, m)
	}
	first := data(m)["id"].(string)
	code, m = logv(&customerA, `{"spaceId":"`+room2+`","unitId":"`+seed.ID("unit-non-rto").String()+`","method":"ventilation_fan","durationMinutes":240}`)
	if code != 200 || data(m)["co2AtLog"] != nil {
		t.Fatalf("no co2 sensor → null: %d %v", code, m)
	}
	for name, tc := range map[string]struct {
		a    *actor
		body string
		code int
	}{
		"duration 0":            {&customerA, `{"spaceId":"` + room1 + `","method":"other","durationMinutes":0}`, 422},
		"duration 241":          {&customerA, `{"spaceId":"` + room1 + `","method":"other","durationMinutes":241}`, 422},
		"bad method":            {&customerA, `{"spaceId":"` + room1 + `","method":"aircon","durationMinutes":5}`, 422},
		"missing space":         {&customerA, `{"method":"other","durationMinutes":5}`, 422},
		"unit not in space":     {&customerA, `{"spaceId":"` + room1 + `","unitId":"` + seed.ID("unit-non-rto").String() + `","method":"other","durationMinutes":5}`, 422},
		"other customer's room": {&customerA, `{"spaceId":"` + roomB + `","method":"other","durationMinutes":5}`, 404},
		"unknown room":          {&customerA, `{"spaceId":"` + uuid.NewString() + `","method":"other","durationMinutes":5}`, 404},
		"hq cannot log":         {&hq, `{"spaceId":"` + room1 + `","method":"other","durationMinutes":5}`, 403},
	} {
		if code, _ := logv(tc.a, tc.body); code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}
	logv(&customerB, `{"spaceId":"`+roomB+`","method":"door_opened","durationMinutes":5}`)

	ids := func(m map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, it := range items(m) {
			out[it["id"].(string)] = true
			if it["spaceId"] == roomB && m != nil {
				out["roomB"] = true
			}
		}
		return out
	}
	_, m = post(s, &customerA, "ventilation.list", `{"filters":{"spaceId":"`+room1+`"},"limit":100}`)
	if !ids(m)[first] {
		t.Fatal("list by space")
	}
	_, m = post(s, &customerA, "ventilation.list", `{"limit":100}`)
	if ids(m)["roomB"] {
		t.Fatal("customer-a sees customer-b's log")
	}
	_, m = post(s, &techB, "ventilation.list", `{"limit":100}`)
	got := ids(m)
	if !got["roomB"] || got[first] {
		t.Fatalf("technician scope: %v", got)
	}
	_, m = post(s, &hq, "ventilation.list", `{"filters":{"unitId":"`+seed.ID("unit-non-rto").String()+`","from":"`+clock.Add(-time.Minute).Format(time.RFC3339)+`","to":"`+clock.Add(time.Minute).Format(time.RFC3339)+`"},"limit":100}`)
	for _, it := range items(m) {
		if it["unitId"] != seed.ID("unit-non-rto").String() {
			t.Fatal("unit filter")
		}
	}
	if len(items(m)) == 0 {
		t.Fatal("hq list by unit")
	}
	for _, b := range []string{`{"filters":{"x":1}}`, `{"filters":{"from":"` + clock.Format(time.RFC3339) + `","to":"` + clock.Add(-time.Hour).Format(time.RFC3339) + `"}}`, `{"sort":{"field":"method","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "ventilation.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &hq, "ventilation.list", `{"sort":{"field":"createdAt","direction":"asc"}}`); code != 200 {
		t.Error("sort createdAt")
	}
	_ = strings.TrimSpace
}
