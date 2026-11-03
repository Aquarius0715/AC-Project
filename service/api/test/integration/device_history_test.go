package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestDeviceHistory(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Hist AC")
	_, resp := write(
		srv,
		&hq,
		"devices.register",
		`{"serial":"HI-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+unit+`"}`,
		0,
	)
	dev := data(resp)["id"].(string)
	sensor := data(resp)["sensors"].([]any)[0].(map[string]any)["id"].(string)

	write(
		srv,
		&hq,
		"devices.bind",
		`{"deviceId":"`+dev+`","unitId":"`+unit+`","reason":"install"}`,
		1,
	)
	write(
		srv,
		&hq,
		"devices.calibrate",
		`{"deviceId":"`+dev+`","sensorId":"`+sensor+`","metric":"temperature","unit":"°C","referenceValue":25,`+
			`"measuredValue":25.2,"calibratedAt":"2026-09-14T00:10:00Z"}`,
		2,
	)
	write(srv, &hq, "devices.check", `{"id":"`+dev+`"}`, 3)

	ev := uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.device_events (id, tenant_id, device_id, unit_id, event_type, axis, evidence_source, sequence,
			occurred_at)
		VALUES ($1,$2,$3,$4,'tamper','tamper','tamper_signal',1,$5)`,
		ev,
		seed.ID("tenant-a"),
		dev,
		unit,
		clock,
	) // after the binding (bound at the test clock)

	for _, operation := range []string{"devices.operations", "devices.calibrations"} {
		code, m := post(srv, &hq, operation, `{"deviceId":"`+dev+`","query":{}}`)
		if code != 200 || len(items(m)) != 1 {
			t.Fatalf("%s: %d %v", operation, code, m)
		}

		if items(m)[0]["unitIdAtOccurrence"] != unit {
			t.Errorf("%s: occurrence scope missing", operation)
		}
	}

	code, resp := post(srv, &hq, "devices.events", `{"id":"`+dev+`","query":{"limit":10}}`)
	if code != 200 || len(items(resp)) != 1 || items(resp)[0]["eventType"] != "tamper" {
		t.Fatalf("events: %d %v", code, resp)
	}
	// customer-b (owner of the bound unit) reads events; customer-a gets NOT_FOUND
	if code, res := post(
		srv,
		&customerB,
		"devices.events",
		`{"id":"`+dev+`","query":{}}`,
	); code != 200 ||
		len(items(res)) != 1 {
		t.Error("customer-b sees events of its binding")
	}

	if code, _ := post(
		srv,
		&customerA,
		"devices.events",
		`{"id":"`+dev+`","query":{}}`,
	); code != 404 {
		t.Error("customer-a events")
	}

	if code, _ := post(
		srv,
		&customerB,
		"devices.operations",
		`{"deviceId":"`+dev+`","query":{}}`,
	); code != 403 {
		t.Error("clients do not read device operations")
	}

	for b, want := range map[string]string{
		`{"deviceId":"` + dev + `","query":{"filters":{"x":1}}}`: "x=error.notAllowed",
		`{"query":{}}`: "deviceId=error.invalid",
		`{"deviceId":"` + dev + `","query":{"sort":{"field":"kind","direction":"asc"}}}`: "sort.field=error.invalid",
	} {
		code, m := post(srv, &hq, "devices.operations", b)
		refusal(t, b, code, m, want)
	}
	// response notes (DeviceEvent.version)
	note := func(a *actor, msg string, v int) (int, map[string]any) {
		return write(
			srv,
			a,
			"devices.addResponseNote",
			`{"deviceId":"`+dev+`","eventId":"`+ev+`","responseNote":"`+msg+`"}`,
			v,
		)
	}
	if code, res := note(
		&hq,
		"  cover reattached  ",
		1,
	); code != 200 || len(data(res)["responseNotes"].([]any)) != 1 ||
		data(res)["responseNotes"].([]any)[0].(map[string]any)["message"] != "cover reattached" {
		t.Fatalf("note: %d %v", code, res)
	}

	if code, _ := note(&hq, "again", 1); code != 409 {
		t.Error("stale event version")
	}

	if code, m := note(&hq, "   ", 2); code != 422 || fields(m) != "responseNote=error.length" {
		t.Errorf("blank note: %d %s", code, fields(m))
	}

	if code, res := note(
		&hq,
		strings.Repeat("x", 1001),
		2,
	); code != 422 ||
		fields(res) != "responseNote=error.length" {
		t.Errorf("note too long: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		&hq,
		"devices.addResponseNote",
		`{"deviceId":"`+dev+`","eventId":"`+uuid.NewString()+`","responseNote":"x"}`,
		1,
	); code != 404 {
		t.Error("unknown event")
	}
	// technician needs an active assignment on the device's unit (IR94 without jobId)
	owner(
		t,
		`DELETE FROM maintenance.assignments WHERE technician_membership_id = $1 AND job_id IN (SELECT id FROM
			maintenance.jobs WHERE unit_id = $2)`,
		seed.ID("tech-internal-a"),
		unit,
	)

	if code, _ := note(&techInt, "tech note", 2); code != 404 && code != 403 {
		t.Errorf("technician without assignment: %d", code)
	}

	owner(
		t,
		`DELETE FROM maintenance.assignments WHERE technician_membership_id = $1 AND job_id IN (SELECT id FROM
			maintenance.jobs WHERE customer_org_id = $2)`,
		seed.ID("tech-internal-a"),
		seed.ID("org-customer-b"),
	)
	assign(t, unit, "tech-internal-a", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	owner(
		t,
		`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'unit',$3) ON CONFLICT
			DO NOTHING`,
		seed.ID("tenant-a"),
		seed.ID("tech-internal-a"),
		unit,
	)

	if code, m := note(&techInt, "tech note", 2); code != 200 {
		t.Fatalf("assigned technician note: %d %v", code, m)
	}
}

// A unit holds one device at a time (error.unitHasDevice), and only a bound device is calibrated
// (error.deviceNotBound).
func TestDeviceBindingRefusals(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Bind AC")
	register := func(prefix string) map[string]any {
		_, res := write(
			srv,
			&hq,
			"devices.register",
			`{"serial":"`+prefix+`-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+unit+`"}`,
			0,
		)

		return data(res)
	}
	first, second := register("BA"), register("BB")

	sensor := second["sensors"].([]any)[0].(map[string]any)["id"].(string)
	if code, res := write(
		srv,
		&hq,
		"devices.calibrate",
		`{"deviceId":"`+second["id"].(string)+`","sensorId":"`+sensor+`","metric":"temperature","unit":"°C",`+
			`"referenceValue":25,"measuredValue":25.2,"calibratedAt":"2026-09-14T00:10:00Z"}`,
		1,
	); code != 409 ||
		res["messageKey"] != "error.deviceNotBound" {
		t.Errorf("calibrate an unbound device: %d %v", code, res)
	}

	if code, res := write(
		srv,
		&hq,
		"devices.bind",
		`{"deviceId":"`+first["id"].(string)+`","unitId":"`+unit+`","reason":"install"}`,
		1,
	); code != 200 {
		t.Fatalf("bind: %d %v", code, res)
	}

	if code, res := write(
		srv,
		&hq,
		"devices.bind",
		`{"deviceId":"`+second["id"].(string)+`","unitId":"`+unit+`","reason":"install"}`,
		1,
	); code != 409 ||
		res["messageKey"] != "error.unitHasDevice" {
		t.Errorf("a second device on the unit: %d %v", code, res)
	}
}
