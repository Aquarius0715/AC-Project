package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
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
		"technician in scope":     {&techInt, `{` + rng + `,"unitIds":["` + online + `"],"metric":"co2","query":{}}`, 200},
		"technician out of scope": {&techInt, `{` + rng + `,"unitIds":["` + other + `"],"metric":"co2","query":{}}`, 404},
		"external, no assignment": {&techB, `{` + rng + `,"unitIds":["` + other + `"],"metric":"co2","query":{}}`, 404},
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
	_, m = post(s, &techInt, "ventilation.list", `{"limit":100}`)
	got := ids(m)
	if got["roomB"] || !got[first] {
		t.Fatalf("technician scope: %v", got)
	}
	if _, m = post(s, &techB, "ventilation.list", `{"limit":100}`); len(items(m)) != 0 {
		t.Fatalf("external technician without an Assignment: %v", ids(m))
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

// IR213: UnitSummary.latestMeasurements holds the latest reading of each metric at or before now (observedAt desc,
// sequence desc), whatever its origin and quality, ordered by metric. A valid reading older than its sensor's stale
// limit, or of an unknown sensor, reads as stale; telemetry.series keeps the stored quality.
func TestLatestMeasurements(t *testing.T) {
	s := server(t)
	tenant := seed.ID("tenant-a")
	u := newUnit(t, s, "Latest AC")
	dev := uuid.NewString()
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`,
		dev, tenant, "LM-"+dev[:8], u, seed.ID("hq-operator"))
	sensor := map[string]string{"co2": uuid.NewString(), "humidity": uuid.NewString(), "temperature": uuid.NewString(), "pm25": uuid.NewString()}
	units := map[string]string{"co2": "ppm", "humidity": "%", "temperature": "°C", "pm25": "µg/m³"}
	for metric, id := range sensor {
		owner(t, `INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, stale_after_seconds) VALUES ($1,$2,$3,$4,$5,120)`, id, tenant, dev, metric, units[metric])
	}
	reading := func(metric, sensorID string, ago time.Duration, seq int, quality string, value any) {
		owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'measured',$9,$5,$10)`, tenant, u, sensorID, metric, clock.Add(-ago), seq, value, units[metric], quality, uuid.NewString())
	}
	reading("co2", sensor["co2"], 10*time.Minute, 1, "valid", 700.0)
	reading("co2", sensor["co2"], 30*time.Second, 2, "valid", 1000.0)
	reading("co2", sensor["co2"], -time.Minute, 3, "valid", 1500.0)                  // future reading: ignored
	reading("humidity", sensor["humidity"], 2*time.Minute, 1, "valid", 55.0)         // exactly at the 120 s limit: still valid
	reading("humidity", sensor["humidity"], 20*time.Second, 2, "missing", nil)       // newer null: no fallback to the older value
	reading("temperature", sensor["temperature"], 121*time.Second, 1, "valid", 28.0) // older than 120 s → stale
	reading("pm25", uuid.NewString(), 10*time.Second, 1, "valid", 12.0)              // unknown sensor → stale
	latest := func(op, body string) []map[string]any {
		t.Helper()
		code, m := post(s, &customerB, op, body)
		if code != 200 {
			t.Fatalf("%s: %d %v", op, code, m)
		}
		d := data(m)
		if op == "units.list" {
			d = items(m)[0]
		}
		var out []map[string]any
		for _, x := range d["latestMeasurements"].([]any) {
			out = append(out, x.(map[string]any))
		}
		return out
	}
	got := latest("units.get", `{"id":"`+u+`"}`)
	var seen []string
	for _, x := range got {
		seen = append(seen, x["metric"].(string)+"="+x["quality"].(string))
	}
	if strings.Join(seen, ",") != "co2=valid,humidity=missing,pm25=stale,temperature=stale" || got[0]["value"].(float64) != 1000 || got[1]["value"] != nil ||
		got[3]["value"].(float64) != 28 || got[0]["unitId"] != u {
		t.Fatalf("latest: %v", got)
	}
	if l := latest("units.list", `{"limit":10,"filters":{"unitIds":["`+u+`"]}}`); len(l) != 4 || l[0]["value"].(float64) != 1000 {
		t.Fatalf("units.list carries the same readings: %v", l)
	}
	// history keeps the stored quality
	_, m := post(s, &customerB, "telemetry.series", `{"from":"`+clock.Add(-time.Hour).Format(time.RFC3339)+`","to":"`+clock.Format(time.RFC3339)+`","unitIds":["`+u+`"],"metric":"temperature","query":{}}`)
	if len(items(m)) != 1 || items(m)[0]["quality"] != "valid" {
		t.Fatalf("series quality: %v", m)
	}
	// co2AtLog (IR110/IR213) is the unit's latest CO₂ reading when it is valid; a newer null reading gives null, never an
	// older valid value
	code, m := write(s, &hq, "spaces.save", `{"propertyId":"`+seed.ID("property-home-b").String()+`","parentSpaceId":null,"kind":"room","name":"Vent `+uuid.NewString()[:6]+`"}`, 0)
	if code != 200 {
		t.Fatalf("room: %d %v", code, m)
	}
	room := data(m)["id"].(string)
	owner(t, `UPDATE assets.units SET space_id = $2 WHERE id = $1`, u, room)
	vent := func() any {
		t.Helper()
		code, m := write(s, &customerB, "ventilation.log", `{"spaceId":"`+room+`","unitId":"`+u+`","method":"window_opened","durationMinutes":15}`, 0)
		if code != 200 {
			t.Fatalf("log: %d %v", code, m)
		}
		return data(m)["co2AtLog"]
	}
	if co2, _ := vent().(map[string]any); co2 == nil || co2["value"].(float64) != 1000 || co2["observedAt"] != clock.Add(-30*time.Second).Format(time.RFC3339) {
		t.Fatalf("co2AtLog: %v", co2)
	}
	reading("co2", sensor["co2"], 10*time.Second, 4, "missing", nil)
	if co2 := vent(); co2 != nil {
		t.Fatalf("newer null reading: %v", co2)
	}
	// a unit without readings has an empty list, never null
	_, m = post(s, &customerB, "units.get", `{"id":"`+newUnit(t, s, "Empty AC")+`"}`)
	if l, ok := data(m)["latestMeasurements"].([]any); !ok || len(l) != 0 {
		t.Fatalf("no readings: %v", data(m)["latestMeasurements"])
	}
}
