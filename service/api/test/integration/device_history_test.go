package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestDeviceHistory(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Hist AC")
	_, m := write(s, &hq, "devices.register", `{"serial":"HI-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+u+`"}`, 0)
	dev := data(m)["id"].(string)
	sensor := data(m)["sensors"].([]any)[0].(map[string]any)["id"].(string)
	write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u+`","reason":"install"}`, 1)
	write(s, &hq, "devices.calibrate", `{"deviceId":"`+dev+`","sensorId":"`+sensor+`","metric":"temperature","unit":"°C","referenceValue":25,"measuredValue":25.2,"calibratedAt":"2026-09-14T00:10:00Z"}`, 2)
	write(s, &hq, "devices.check", `{"id":"`+dev+`"}`, 3)
	ev := uuid.NewString()
	owner(t, `INSERT INTO devices.device_events (id, tenant_id, device_id, unit_id, event_type, axis, evidence_source, sequence, occurred_at)
		VALUES ($1,$2,$3,$4,'tamper','tamper','tamper_signal',1,$5)`, ev, seed.ID("tenant-a"), dev, u, clock) // after the binding (bound at the test clock)

	for _, op := range []string{"devices.operations", "devices.calibrations"} {
		code, m := post(s, &hq, op, `{"deviceId":"`+dev+`","query":{}}`)
		if code != 200 || len(items(m)) != 1 {
			t.Fatalf("%s: %d %v", op, code, m)
		}
		if items(m)[0]["unitIdAtOccurrence"] != u {
			t.Errorf("%s: occurrence scope missing", op)
		}
	}
	code, m := post(s, &hq, "devices.events", `{"id":"`+dev+`","query":{"limit":10}}`)
	if code != 200 || len(items(m)) != 1 || items(m)[0]["eventType"] != "tamper" {
		t.Fatalf("events: %d %v", code, m)
	}
	// customer-b (owner of the bound unit) reads events; customer-a gets NOT_FOUND
	if code, m := post(s, &customerB, "devices.events", `{"id":"`+dev+`","query":{}}`); code != 200 || len(items(m)) != 1 {
		t.Error("customer-b sees events of its binding")
	}
	if code, _ := post(s, &customerA, "devices.events", `{"id":"`+dev+`","query":{}}`); code != 404 {
		t.Error("customer-a events")
	}
	if code, _ := post(s, &customerB, "devices.operations", `{"deviceId":"`+dev+`","query":{}}`); code != 403 {
		t.Error("clients do not read device operations")
	}
	for _, b := range []string{`{"deviceId":"` + dev + `","query":{"filters":{"x":1}}}`, `{"query":{}}`, `{"deviceId":"` + dev + `","query":{"sort":{"field":"kind","direction":"asc"}}}`} {
		if code, _ := post(s, &hq, "devices.operations", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	// response notes (DeviceEvent.version)
	note := func(a *actor, msg string, v int) (int, map[string]any) {
		return write(s, a, "devices.addResponseNote", `{"deviceId":"`+dev+`","eventId":"`+ev+`","responseNote":"`+msg+`"}`, v)
	}
	if code, m := note(&hq, "  cover reattached  ", 1); code != 200 || len(data(m)["responseNotes"].([]any)) != 1 || data(m)["responseNotes"].([]any)[0].(map[string]any)["message"] != "cover reattached" {
		t.Fatalf("note: %d %v", code, m)
	}
	if code, _ := note(&hq, "again", 1); code != 409 {
		t.Error("stale event version")
	}
	if code, _ := note(&hq, "   ", 2); code != 422 {
		t.Error("blank note")
	}
	if code, _ := note(&hq, strings.Repeat("x", 1001), 2); code != 422 {
		t.Error("note too long")
	}
	if code, _ := write(s, &hq, "devices.addResponseNote", `{"deviceId":"`+dev+`","eventId":"`+uuid.NewString()+`","responseNote":"x"}`, 1); code != 404 {
		t.Error("unknown event")
	}
	// technician needs an active assignment on the device's unit (IR94 without jobId)
	owner(t, `DELETE FROM maintenance.assignments WHERE technician_membership_id = $1 AND job_id IN (SELECT id FROM maintenance.jobs WHERE unit_id = $2)`, seed.ID("tech-internal-a"), u)
	if code, _ := note(&techInt, "tech note", 2); code != 404 && code != 403 {
		t.Errorf("technician without assignment: %d", code)
	}
	owner(t, `DELETE FROM maintenance.assignments WHERE technician_membership_id = $1 AND job_id IN (SELECT id FROM maintenance.jobs WHERE customer_org_id = $2)`,
		seed.ID("tech-internal-a"), seed.ID("org-customer-b"))
	assign(t, u, "tech-internal-a", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	owner(t, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'unit',$3) ON CONFLICT DO NOTHING`, seed.ID("tenant-a"), seed.ID("tech-internal-a"), u)
	if code, m := note(&techInt, "tech note", 2); code != 200 {
		t.Fatalf("assigned technician note: %d %v", code, m)
	}
}
