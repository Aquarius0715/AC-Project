package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestTelemetry(t *testing.T) {
	srv := server(t)
	online, nonRTO, other := seed.ID("unit-online-rto").
		String(),
		seed.ID("unit-non-rto").
			String(),
		seed.ID("unit-other-customer").
			String()
	from, to := clock().Add(-24*time.Hour).Format(time.RFC3339), clock().Format(time.RFC3339)
	rng := `"from":"` + from + `","to":"` + to + `"`

	// series by unit: one unit → allergen observation (available); seeded reading id = fixture event id
	code, res := post(
		srv,
		customerA(),
		"telemetry.series",
		`{`+rng+`,"unitIds":["`+online+`"],"metric":"co2","query":{}}`,
	)
	if code != 200 || len(items(t, res)) != 1 || asNum(t, items(t, res)[0]["value"]) != 800 ||
		items(t, res)[0]["id"] != seed.ID("m-sensor-online-co2-1").String() ||
		items(t, res)[0]["id"] != items(t, res)[0]["eventId"] ||
		items(t, res)[0]["isDemo"] != true {
		t.Fatalf("series: %d %v", code, res)
	}

	if a := asObj(t, data(res)["allergenObservation"]); a["availability"] != "available" ||
		a["substance"] == nil {
		t.Fatalf("allergen: %v", a)
	}

	_, res = post(
		srv,
		customerA(),
		"telemetry.series",
		`{`+rng+`,"unitIds":["`+nonRTO+`"],"metric":"co2","query":{}}`,
	)
	if a := asObj(t, data(res)["allergenObservation"]); a["availability"] != "unsupported" ||
		a["substance"] != nil ||
		len(items(t, res)) != 0 {
		t.Fatalf("unsupported allergen: %v", res)
	}

	_, res = post(
		srv,
		customerA(),
		"telemetry.series",
		`{`+rng+`,"unitIds":["`+seed.ID("unit-limited").
			String()+
			`"],"metric":"temperature","query":{}}`,
	)
	if a := asObj(t, data(res)["allergenObservation"]); a["availability"] != "not_measured" ||
		len(items(t, res)) != 1 {
		t.Fatalf("not measured: %v", res)
	}
	// several units: no allergen, ordered by observedAt
	_, res = post(
		srv,
		customerA(),
		"telemetry.series",
		`{`+rng+`,"unitIds":["`+online+`","`+nonRTO+`","`+seed.ID("unit-offline-rto").
			String()+
			`"],"metric":"temperature","query":{"limit":2}}`,
	)
	if data(res)["allergenObservation"] != nil || len(items(t, res)) != 2 ||
		asNum(t, data(res)["total"]) != 3 ||
		asNum(t, items(t, res)[0]["value"]) != 29 ||
		data(res)["nextCursor"] == nil {
		t.Fatalf("multi-unit: %v", res)
	}

	_, res = post(
		srv,
		customerA(),
		"telemetry.series",
		`{`+rng+`,"unitIds":["`+online+`","`+nonRTO+`"],"metric":"temperature","query":{"sort":{"field":"observedAt",`+
			`"direction":"desc"}}}`,
	)
	if len(items(t, res)) != 2 {
		t.Fatal("desc sort")
	}
	// by space and by sensor
	_, res = post(
		srv,
		customerA(),
		"telemetry.series",
		`{`+rng+`,"spaceId":"`+seed.ID("room-1").String()+`","metric":"humidity","query":{}}`,
	)
	if len(items(t, res)) != 1 || data(res)["allergenObservation"] == nil {
		t.Fatalf("space: %v", res)
	}

	_, res = post(
		srv,
		hq(),
		"telemetry.series",
		`{`+rng+`,"unitIds":["`+online+`"],"metric":"temperature","sensorId":"`+uuid.NewString()+`","query":{}}`,
	)
	if len(items(t, res)) != 0 {
		t.Fatal("sensor filter")
	}
	// scope and validation
	refused := map[string]string{ // the field each VALIDATION case is refused for
		"bad metric":          "metric=error.invalid",
		"bad sort":            "query.sort.field=error.invalid",
		"both targets":        "unitIds=error.exactlyOneTarget",
		"duplicate units":     "unitIds=error.invalid",
		"empty unitIds":       "unitIds=error.invalid",
		"filters not allowed": "x=error.notAllowed",
		"missing from":        "from=error.required",
		"missing to":          "to=error.required",
		"no target":           "unitIds=error.exactlyOneTarget",
		"range > 35 days":     "to=error.rangeTooLong",
		"reversed range":      "to=error.range",
	}

	for name, tc := range map[string]struct {
		a    *actor
		body string
		code int
	}{
		"other customer's unit": {customerA(), `{` + rng + `,"unitIds":["` + other + `"],"metric":"co2","query":{}}`, 404},
		"other customer's space": {customerA(), `{` + rng + `,"spaceId":"` + seed.ID("room-b-1").String() +
			`","metric":"co2","query":{}}`, 404},
		"technician in scope":     {techInt(), `{` + rng + `,"unitIds":["` + online + `"],"metric":"co2","query":{}}`, 200},
		"technician out of scope": {techInt(), `{` + rng + `,"unitIds":["` + other + `"],"metric":"co2","query":{}}`, 404},
		"external, no assignment": {techB(), `{` + rng + `,"unitIds":["` + other + `"],"metric":"co2","query":{}}`, 404},
		"both targets": {hq(), `{` + rng + `,"unitIds":["` + online + `"],"spaceId":"` +
			seed.ID("room-1").String() + `","metric":"co2","query":{}}`, 422},
		"no target":     {hq(), `{` + rng + `,"metric":"co2","query":{}}`, 422},
		"empty unitIds": {hq(), `{` + rng + `,"unitIds":[],"metric":"co2","query":{}}`, 422},
		"duplicate units": {hq(), `{` + rng + `,"unitIds":["` + online + `","` + online + `"],"metric":"co2",` +
			`"query":{}}`, 422},
		"bad metric": {hq(), `{` + rng + `,"unitIds":["` + online + `"],"metric":"noise","query":{}}`, 422},
		"reversed range": {hq(), `{"from":"` + to + `","to":"` + from + `","unitIds":["` + online + `"],"metric":"co2",` +
			`"query":{}}`, 422},
		"range > 35 days": {hq(), `{"from":"2026-08-01T00:00:00Z",` +
			`"to":"` + to + `","unitIds":["` + online + `"],"metric":"co2","query":{}}`, 422},
		"filters not allowed": {hq(), `{` + rng + `,"unitIds":["` + online + `"],"metric":"co2",` +
			`"query":{"filters":{"x":1}}}`, 422},
		"bad sort": {hq(), `{` + rng + `,"unitIds":["` + online + `"],"metric":"co2",` +
			`"query":{"sort":{"field":"value","direction":"asc"}}}`, 422},
		"missing from": {hq(), `{"to":"` + to + `","unitIds":["` + online + `"],"metric":"co2","query":{}}`, 422},
		"missing to": {hq(), `{"from":"` + from + `","unitIds":["` + online + `"],"metric":"co2",` +
			`"query":{}}`, 422},
	} {
		code, m := post(srv, tc.a, "telemetry.series", tc.body)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}

	// summary: latest per unit and metric
	code, res = post(
		srv,
		customerA(),
		"telemetry.summary",
		`{`+rng+`,"unitIds":["`+online+`","`+nonRTO+`"]}`,
	)
	if code != 200 || len(asArr(t, data(res)["measurements"])) != 8 || data(res)["energy"] != nil ||
		data(res)["asOf"] != to {
		t.Fatalf("summary: %d %v", code, res)
	}

	_, res = post(
		srv,
		customerA(),
		"telemetry.summary",
		`{`+rng+`,"unitIds":["`+online+`"],"metric":"power"}`,
	)
	if len(asArr(t, data(res)["measurements"])) != 1 {
		t.Fatal("summary metric")
	}

	for b, want := range map[string]string{
		`{` + rng + `,"unitIds":[]}`:                                             "unitIds=error.invalid",
		`{` + rng + `,"unitIds":["` + online + `"],"metric":"x"}`:                "metric=error.invalid",
		`{"from":"` + to + `","to":"` + from + `","unitIds":["` + online + `"]}`: "to=error.range",
	} {
		code, m := post(srv, customerA(), "telemetry.summary", b)
		refusal(t, b, code, m, want)
	}

	if code, _ := post(
		srv,
		customerA(),
		"telemetry.summary",
		`{`+rng+`,"unitIds":["`+other+`"]}`,
	); code != 404 {
		t.Error("summary scope")
	}
}

func TestVentilation(t *testing.T) {
	srv := server(t)
	room1, room2, roomB := seed.ID("room-1").
		String(),
		seed.ID("room-2").
			String(),
		seed.ID("room-b-1").
			String()
	logv := func(a *actor, body string) (int, map[string]any) { return write(srv, a, "ventilation.log", body, 0) }

	code, res := logv(
		customerA(),
		`{"spaceId":"`+room1+`","method":"window_opened","durationMinutes":15}`,
	)

	co2, _ := data(res)["co2AtLog"].(map[string]any)
	if code != 200 || co2 == nil || asNum(t, co2["value"]) != 800 || co2["unit"] != "ppm" ||
		data(res)["loggedAt"] != clock().Format(time.RFC3339) {
		t.Fatalf("log: %d %v", code, res)
	}

	first := asStr(t, data(res)["id"])

	code, res = logv(
		customerA(),
		`{"spaceId":"`+room2+`","unitId":"`+seed.ID("unit-non-rto").
			String()+
			`","method":"ventilation_fan","durationMinutes":240}`,
	)
	if code != 200 || data(res)["co2AtLog"] != nil {
		t.Fatalf("no co2 sensor → null: %d %v", code, res)
	}

	refused := map[string]string{ // the field each VALIDATION case is refused for
		"bad method":        "method=error.invalid",
		"duration 0":        "durationMinutes=error.range",
		"duration 241":      "durationMinutes=error.range",
		"missing space":     "spaceId=error.required",
		"unit not in space": "unitId=error.unitNotInSpace",
	}

	for name, tc := range map[string]struct {
		a    *actor
		body string
		code int
	}{
		"duration 0":    {customerA(), `{"spaceId":"` + room1 + `","method":"other","durationMinutes":0}`, 422},
		"duration 241":  {customerA(), `{"spaceId":"` + room1 + `","method":"other","durationMinutes":241}`, 422},
		"bad method":    {customerA(), `{"spaceId":"` + room1 + `","method":"aircon","durationMinutes":5}`, 422},
		"missing space": {customerA(), `{"method":"other","durationMinutes":5}`, 422},
		"unit not in space": {customerA(), `{"spaceId":"` + room1 + `","unitId":"` + seed.ID("unit-non-rto").String() +
			`","method":"other","durationMinutes":5}`, 422},
		"other customer's room": {customerA(), `{"spaceId":"` + roomB + `","method":"other","durationMinutes":5}`, 404},
		"unknown room": {customerA(), `{"spaceId":"` + uuid.NewString() + `","method":"other",` +
			`"durationMinutes":5}`, 404},
		"hq cannot log": {hq(), `{"spaceId":"` + room1 + `","method":"other","durationMinutes":5}`, 403},
	} {
		code, m := logv(tc.a, tc.body)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}

	logv(customerB(), `{"spaceId":"`+roomB+`","method":"door_opened","durationMinutes":5}`)

	ids := func(m map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, it := range items(t, m) {
			out[asStr(t, it["id"])] = true
			if it["spaceId"] == roomB && m != nil {
				out["roomB"] = true
			}
		}

		return out
	}

	_, res = post(
		srv,
		customerA(),
		"ventilation.list",
		`{"filters":{"spaceId":"`+room1+`"},"limit":100}`,
	)
	if !ids(res)[first] {
		t.Fatal("list by space")
	}

	_, res = post(srv, customerA(), "ventilation.list", `{"limit":100}`)
	if ids(res)["roomB"] {
		t.Fatal("customer-a sees customer-b's log")
	}

	_, res = post(srv, techInt(), "ventilation.list", `{"limit":100}`)

	got := ids(res)
	if got["roomB"] || !got[first] {
		t.Fatalf("technician scope: %v", got)
	}

	if _, res = post(srv, techB(), "ventilation.list", `{"limit":100}`); len(items(t, res)) != 0 {
		t.Fatalf("external technician without an Assignment: %v", ids(res))
	}

	_, res = post(
		srv,
		hq(),
		"ventilation.list",
		`{"filters":{"unitId":"`+seed.ID("unit-non-rto").
			String()+
			`","from":"`+clock().Add(-time.Minute).
			Format(time.RFC3339)+
			`","to":"`+clock().Add(time.Minute).
			Format(time.RFC3339)+
			`"},"limit":100}`,
	)
	for _, it := range items(t, res) {
		if it["unitId"] != seed.ID("unit-non-rto").String() {
			t.Fatal("unit filter")
		}
	}

	if len(items(t, res)) == 0 {
		t.Fatal("hq list by unit")
	}

	want := map[string]string{ // the field each case is refused for
		"{\"filters\":{\"from\":\"2026-09-14T01:00:00Z\",\"to\":\"2026-09-14T00:00:00Z\"}}": "filters.to=error.range",
		"{\"filters\":{\"x\":1}}":                                 "x=error.notAllowed",
		"{\"sort\":{\"field\":\"method\",\"direction\":\"asc\"}}": "sort.field=error.invalid",
	}

	for _, b := range []string{`{"filters":{"x":1}}`, `{"filters":{"from":"` + clock().Format(time.RFC3339) + `","to":"` +
		clock().Add(-time.Hour).Format(time.RFC3339) + `"}}`, `{"sort":{"field":"method","direction":"asc"}}`} {
		code, m := post(srv, hq(), "ventilation.list", b)
		refusal(t, b, code, m, want[b])
	}

	if code, _ := post(
		srv,
		hq(),
		"ventilation.list",
		`{"sort":{"field":"createdAt","direction":"asc"}}`,
	); code != 200 {
		t.Error("sort createdAt")
	}

	_ = strings.TrimSpace
}

// IR213: UnitSummary.latestMeasurements holds the latest reading of each metric at or before now (observedAt desc,
// sequence desc), whatever its origin and quality, ordered by metric. A valid reading older than its sensor's stale
// limit, or of an unknown sensor, reads as stale; telemetry.series keeps the stored quality.
func TestLatestMeasurements(t *testing.T) {
	srv := server(t)
	tenant := seed.ID("tenant-a")
	unit := newUnit(t, srv, "Latest AC")
	dev := uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version)
			VALUES ($1,$2,$3,$4,$5,'v1')`,
		dev,
		tenant,
		"LM-"+dev[:8],
		unit,
		seed.ID("hq-operator"),
	)

	sensor := map[string]string{
		"co2":         uuid.NewString(),
		"humidity":    uuid.NewString(),
		"temperature": uuid.NewString(),
		"pm25":        uuid.NewString(),
	}

	units := map[string]string{"co2": "ppm", "humidity": "%", "temperature": "°C", "pm25": "µg/m³"}
	for metric, id := range sensor {
		owner(
			t,
			`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, stale_after_seconds) VALUES ($1,$2,$3,$4,$5,
				120)`,
			id,
			tenant,
			dev,
			metric,
			units[metric],
		)
	}

	reading := func(metric, sensorID string, ago time.Duration, seq int, quality string, value any) {
		owner(
			t,
			`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
				origin, quality, received_at, event_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'measured',$9,$5,$10)`,
			tenant,
			unit,
			sensorID,
			metric,
			clock().Add(-ago),
			seq,
			value,
			units[metric],
			quality,
			uuid.NewString(),
		)
	}
	reading("co2", sensor["co2"], 10*time.Minute, 1, "valid", 700.0)
	reading("co2", sensor["co2"], 30*time.Second, 2, "valid", 1000.0)
	reading(
		"co2",
		sensor["co2"],
		-time.Minute,
		3,
		"valid",
		1500.0,
	) // future reading: ignored
	reading(
		"humidity",
		sensor["humidity"],
		2*time.Minute,
		1,
		"valid",
		55.0,
	) // exactly at the 120 s limit: still valid
	reading(
		"humidity",
		sensor["humidity"],
		20*time.Second,
		2,
		"missing",
		nil,
	) // newer null: no fallback to the older value
	reading(
		"temperature",
		sensor["temperature"],
		121*time.Second,
		1,
		"valid",
		28.0,
	) // older than 120 s → stale
	reading(
		"pm25",
		uuid.NewString(),
		10*time.Second,
		1,
		"valid",
		12.0,
	) // unknown sensor → stale

	latest := func(operation, body string) []map[string]any {
		t.Helper()

		code, resp := post(srv, customerB(), operation, body)
		if code != 200 {
			t.Fatalf("%s: %d %v", operation, code, resp)
		}

		doc := data(resp)
		if operation == "units.list" {
			doc = items(t, resp)[0]
		}

		measurements := asArr(t, doc["latestMeasurements"])

		out := make([]map[string]any, 0, len(measurements))
		for _, x := range measurements {
			out = append(out, asObj(t, x))
		}

		return out
	}
	got := latest("units.get", `{"id":"`+unit+`"}`)

	seen := make([]string, 0, len(got))
	for _, x := range got {
		seen = append(seen, asStr(t, x["metric"])+"="+asStr(t, x["quality"]))
	}

	if strings.Join(seen, ",") != "co2=valid,humidity=missing,pm25=stale,temperature=stale" ||
		asNum(t, got[0]["value"]) != 1000 ||
		got[1]["value"] != nil ||
		asNum(t, got[3]["value"]) != 28 ||
		got[0]["unitId"] != unit {
		t.Fatalf("latest: %v", got)
	}

	if l := latest(
		"units.list",
		`{"limit":10,"filters":{"unitIds":["`+unit+`"]}}`,
	); len(l) != 4 ||
		asNum(t, l[0]["value"]) != 1000 {
		t.Fatalf("units.list carries the same readings: %v", l)
	}
	// history keeps the stored quality
	_, res := post(
		srv,
		customerB(),
		"telemetry.series",
		`{"from":"`+clock().Add(-time.Hour).
			Format(time.RFC3339)+
			`","to":"`+clock().Format(
			time.RFC3339,
		)+`","unitIds":["`+unit+`"],"metric":"temperature","query":{}}`,
	)
	if len(items(t, res)) != 1 || items(t, res)[0]["quality"] != "valid" {
		t.Fatalf("series quality: %v", res)
	}
	// co2AtLog (IR110/IR213) is the unit's latest CO₂ reading when it is valid; a newer null reading gives null, never an
	// older valid value
	code, res := write(
		srv,
		hq(),
		"spaces.save",
		`{"propertyId":"`+seed.ID("property-home-b").
			String()+
			`","parentSpaceId":null,"kind":"room","name":"Vent `+uuid.NewString()[:6]+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("room: %d %v", code, res)
	}

	room := asStr(t, data(res)["id"])
	owner(t, `UPDATE assets.units SET space_id = $2 WHERE id = $1`, unit, room)

	vent := func() any {
		t.Helper()

		code, resp := write(
			srv,
			customerB(),
			"ventilation.log",
			`{"spaceId":"`+room+`","unitId":"`+unit+`","method":"window_opened","durationMinutes":15}`,
			0,
		)
		if code != 200 {
			t.Fatalf("log: %d %v", code, resp)
		}

		return data(resp)["co2AtLog"]
	}
	if co2, _ := vent().(map[string]any); co2 == nil || asNum(t, co2["value"]) != 1000 ||
		co2["observedAt"] != clock().Add(-30*time.Second).Format(time.RFC3339) {
		t.Fatalf("co2AtLog: %v", co2)
	}

	reading("co2", sensor["co2"], 10*time.Second, 4, "missing", nil)

	if co2 := vent(); co2 != nil {
		t.Fatalf("newer null reading: %v", co2)
	}
	// a unit without readings has an empty list, never null
	_, res = post(srv, customerB(), "units.get", `{"id":"`+newUnit(t, srv, "Empty AC")+`"}`)
	if l, ok := data(res)["latestMeasurements"].([]any); !ok || len(l) != 0 {
		t.Fatalf("no readings: %v", data(res)["latestMeasurements"])
	}
}

// TestBucketedSeries (IR605, DEC-90): with bucketMinutes the series answers the whole period in averages — every
// stretch present, the average of its valid readings with their count, no value and 0 where none is valid — and no page
// of readings; the total still counts every reading. Only 5 and 60 minutes are taken, at most 1000 stretches, and the
// unit must be readable as before.
func TestBucketedSeries(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Bucket AC") // customer B's

	from := clock().Add(-time.Hour)
	for i, v := range []float64{20, 21, 22, 23, 24} {
		reading(t, unit, "temperature", v, "°C", from.Add(time.Duration(i)*time.Minute), "valid")
	}

	reading(
		t,
		unit,
		"temperature",
		99,
		"°C",
		from.Add(5*time.Minute),
		"suspect",
	) // left out of the average
	reading(t, unit, "temperature", 25, "°C", from.Add(6*time.Minute), "valid")
	reading(
		t,
		unit,
		"temperature",
		30,
		"°C",
		from.Add(11*time.Minute),
		"stale",
	) // a stretch of invalid readings only

	body := func(extra string) string {
		return `{"from":"` + from.Format(
			time.RFC3339,
		) + `","to":"` + clock().Format(
			time.RFC3339,
		) + `","unitIds":["` + unit + `"],"metric":"temperature","query":{}` + extra + `}`
	}

	code, resp := post(srv, customerB(), "telemetry.series", body(`,"bucketMinutes":5`))
	if code != 200 {
		t.Fatalf("bucketed series: %d %v", code, resp)
	}

	doc := data(resp)

	bs, _ := doc["buckets"].([]any)
	if len(bs) != 12 || len(items(t, resp)) != 0 || doc["total"] != 8.0 ||
		doc["nextCursor"] != nil {
		t.Fatalf(
			"12 five-minute stretches, no page, every reading counted: %d buckets, %d items, total %v",
			len(bs),
			len(items(t, resp)),
			doc["total"],
		)
	}

	b := func(i int) map[string]any { return asObj(t, bs[i]) }
	if b(0)["start"] != from.UTC().Format(time.RFC3339) || b(0)["value"] != 22.0 ||
		b(0)["count"] != 5.0 {
		t.Errorf("the first stretch averages its five valid readings: %v", b(0))
	}

	if b(1)["value"] != 25.0 || b(1)["count"] != 1.0 {
		t.Errorf("a suspect reading is left out of the average: %v", b(1))
	}

	if b(2)["value"] != nil || b(2)["count"] != 0.0 || b(11)["value"] != nil ||
		b(11)["count"] != 0.0 {
		t.Errorf("stretches without a valid reading stay, empty: %v / %v", b(2), b(11))
	}

	if code, res := post(
		srv,
		customerB(),
		"telemetry.series",
		body(`,"bucketMinutes":60`),
	); code != 200 || len(asArr(t, data(res)["buckets"])) != 1 ||
		asObj(t, asArr(t, data(res)["buckets"])[0])["count"] != 6.0 {
		t.Errorf("one hourly stretch of six valid readings: %d %v", code, res)
	}

	if code, res := post(
		srv,
		customerB(),
		"telemetry.series",
		body(``),
	); code != 200 || data(res)["buckets"] != nil ||
		len(items(t, res)) == 0 {
		t.Errorf("without bucketMinutes the page of readings as before: %d %v", code, res)
	}

	if code, res := post(
		srv,
		customerB(),
		"telemetry.series",
		body(`,"bucketMinutes":7`),
	); code != 422 ||
		fields(res) != "bucketMinutes=error.invalid" {
		t.Errorf("a stretch other than 5 or 60 minutes: %d %s", code, fields(res))
	}

	long := `{"from":"` + clock().Add(-30*24*time.Hour).
		Format(time.RFC3339) +
		`","to":"` + clock().Format(
		time.RFC3339,
	) + `","unitIds":["` + unit + `"],"metric":"temperature","query":{},"bucketMinutes":5}`
	if code, res := post(
		srv,
		customerB(),
		"telemetry.series",
		long,
	); code != 422 ||
		fields(res) != "bucketMinutes=error.range" {
		t.Errorf("more than 1000 stretches: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		customerA(),
		"telemetry.series",
		body(`,"bucketMinutes":5`),
	); code != 404 {
		t.Errorf("another customer's unit in buckets: %d", code)
	}
}
