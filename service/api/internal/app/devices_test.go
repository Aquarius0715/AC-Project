package app

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

var techInt = actor{"tok-ti", "tech-internal-a"}

// newUnit creates a unit for customer-b's home (fresh per test run).
func newUnit(t *testing.T, s *Server, name string) string {
	t.Helper()
	code, m := write(s, &hq, "units.save", `{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","propertyId":"`+seed.ID("property-home-b").String()+`","spaceId":null,"displayName":"`+name+` `+uuid.NewString()[:6]+`","modelId":"`+seed.ID("ventilation-demo").String()+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`, 0)
	if code != 200 {
		t.Fatalf("unit: %d %v", code, m)
	}
	return data(m)["id"].(string)
}

// assign creates a job on unit with an assignment for membership whose window is [from, until).
func assign(t *testing.T, unit, membership string, from, until time.Time, status string) string {
	job := uuid.NewString()
	if status == "active" { // isolate from active assignments other tests left in the same window
		owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND scheduled && tstzrange($2, $3)`,
			seed.ID(membership), from, until)
	}
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at)
		VALUES ($1,$2,$3,$4,'reactive','assigned','client_request',tstzrange($5,$6),$6)`, job, seed.ID("tenant-a"), unit, seed.ID("org-customer-b"), from, until)
	owner(t, `INSERT INTO maintenance.assignments (tenant_id, job_id, technician_membership_id, valid_from, valid_until, scheduled, status)
		VALUES ($1,$2,$3,$4,$5,tstzrange($4,$5),$6)`, seed.ID("tenant-a"), job, seed.ID(membership), from, until, status)
	return job
}

func TestDevicesRegisterListGet(t *testing.T) {
	s := server(t)
	unit := newUnit(t, s, "Dev AC")
	serial := "dev-" + uuid.NewString()[:8]
	body := func(serial, unit, sensors, job string) string {
		j := ""
		if job != "" {
			j = `,"jobId":"` + job + `"`
		}
		return `{"serial":"` + serial + `","sensorTypes":` + sensors + `,"unitId":"` + unit + `"` + j + `}`
	}
	code, m := write(s, &hq, "devices.register", body("  "+serial+" ", unit, `["temperature","power"]`, ""), 0)
	if code != 200 {
		t.Fatalf("register: %d %v", code, m)
	}
	d := data(m)
	if d["serial"] != strings.ToUpper(serial) || len(d["sensors"].([]any)) != 2 || d["unitId"] != nil || d["targetUnitId"] != unit || d["activeOperation"] != nil {
		t.Fatalf("device (unbound until bind, D05): %v", d)
	}
	dev := d["id"].(string)
	if code, _ := post(s, &customerB, "devices.get", `{"id":"`+dev+`"}`); code != 404 {
		t.Error("unbound devices are visible only to the creator and HQ")
	}
	code, m = write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+unit+`","reason":"installed"}`, 1)
	if code != 200 || data(m)["unitId"] != unit || data(m)["bindingId"] == nil {
		t.Fatalf("bind: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "devices.register", body(strings.ToUpper(serial), newUnit(t, s, "Dup"), `[]`, ""), 0); code != 409 {
		t.Error("duplicate serial")
	}
	if code, _ := write(s, &hq, "devices.register", body("X-"+uuid.NewString()[:6], newUnit(t, s, "Cap"), `["vibration"]`, ""), 0); code != 422 {
		t.Error("sensor not in capability (IR43)")
	}
	for _, b := range []string{body("a", unit, `[]`, ""), body("bad serial!", unit, `[]`, ""), body("OK-123", unit, `["power","power"]`, ""), `{"serial":"OK-1","unitId":"` + unit + `"}`} {
		if code, _ := write(s, &hq, "devices.register", b, 0); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := write(s, &hq, "devices.register", body("UNK-"+uuid.NewString()[:6], uuid.NewString(), `[]`, ""), 0); code != 404 {
		t.Error("unknown unit")
	}
	// reads: HQ sees it, customer-b (owner of the unit) sees it, customer-a does not
	if code, m := post(s, &hq, "devices.get", `{"id":"`+dev+`"}`); code != 200 || data(m)["calibrationRefs"] == nil {
		t.Fatalf("get: %d", code)
	}
	if code, _ := post(s, &customerB, "devices.get", `{"id":"`+dev+`"}`); code != 200 {
		t.Error("customer-b reads its device")
	}
	if code, _ := post(s, &customerA, "devices.get", `{"id":"`+dev+`"}`); code != 404 {
		t.Error("customer-a must not see customer-b's device")
	}
	_, m = post(s, &hq, "devices.list", `{"filters":{"unitId":"`+unit+`","status":"unknown"}}`)
	if len(items(m)) != 1 {
		t.Fatalf("list by unit: %v", m)
	}
	for _, b := range []string{`{"filters":{"status":"x"}}`, `{"filters":{"y":1}}`} {
		if code, _ := post(s, &hq, "devices.list", b); code != 422 {
			t.Errorf("%s", b)
		}
	}
	_, m = post(s, &customerA, "devices.list", `{"limit":100}`)
	for _, it := range items(m) {
		if it["id"] == dev {
			t.Fatal("client list leaks another customer's device")
		}
	}
}

// IR94 technician write table via devices.register.
func TestTechnicianWriteTable(t *testing.T) {
	s := server(t)
	// assignments of one technician must not overlap (assignments_no_overlap): clear earlier runs' test assignments
	owner(t, `DELETE FROM maintenance.assignments WHERE technician_membership_id = ANY($1) AND job_id IN
		(SELECT id FROM maintenance.jobs WHERE customer_org_id = $2)`, []any{seed.ID("tech-internal-a"), seed.ID("tech-external-a")}, seed.ID("org-customer-b"))
	reg := func(unit, job string) (int, map[string]any) {
		j := ""
		if job != "" {
			j = `,"jobId":"` + job + `"`
		}
		return write(s, &techInt, "devices.register", `{"serial":"T-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+unit+`"`+j+`}`, 0)
	}
	u1 := newUnit(t, s, "Tech AC")
	if code, m := reg(u1, ""); code != 422 || m["fieldErrors"].(map[string]any)["jobId"] == nil {
		t.Fatalf("jobId required: %d %v", code, m)
	}
	if code, _ := reg(u1, uuid.NewString()); code != 404 {
		t.Error("never assigned → NOT_FOUND")
	}
	future := assign(t, u1, "tech-internal-a", clock.Add(time.Hour), clock.Add(3*time.Hour), "active")
	if code, m := reg(u1, future); code != 403 || m["messageKey"] != "errors.assignment_not_started" {
		t.Fatalf("before window: %d %v", code, m)
	}
	u2 := newUnit(t, s, "Tech AC2")
	ended := assign(t, u2, "tech-internal-a", clock.Add(-3*time.Hour), clock.Add(-time.Hour), "active")
	if code, m := reg(u2, ended); code != 403 || m["messageKey"] != "errors.assignment_ended" {
		t.Fatalf("after window: %d %v", code, m)
	}
	u3 := newUnit(t, s, "Tech AC3")
	revoked := assign(t, u3, "tech-internal-a", clock.Add(-time.Hour), clock.Add(time.Hour), "revoked")
	if code, m := reg(u3, revoked); code != 403 || m["messageKey"] != "errors.assignment_ended" {
		t.Fatalf("revoked after start: %d %v", code, m)
	}
	u4 := newUnit(t, s, "Tech AC4")
	revokedEarly := assign(t, u4, "tech-internal-a", clock.Add(time.Hour), clock.Add(2*time.Hour), "revoked")
	if code, _ := reg(u4, revokedEarly); code != 404 {
		t.Errorf("revoked before start → NOT_FOUND: %d", code)
	}
	u5 := newUnit(t, s, "Tech AC5")
	active := assign(t, u5, "tech-internal-a", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	if code, _ := reg(u1, active); code != 404 {
		t.Error("job for another unit → NOT_FOUND")
	}
	if code, m := reg(u5, active); code != 200 {
		t.Fatalf("inside window: %d %v", code, m)
	}
	other := assign(t, newUnit(t, s, "Other"), "tech-external-a", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	if code, _ := reg(u5, other); code != 404 {
		t.Error("another technician's assignment → NOT_FOUND")
	}
}

func TestDeviceBindAndOperations(t *testing.T) {
	s := server(t)
	u1, u2 := newUnit(t, s, "Op AC"), newUnit(t, s, "Op AC2")
	_, m := write(s, &hq, "devices.register", `{"serial":"OP-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+u1+`"}`, 0)
	dev := data(m)["id"].(string)
	sensor := data(m)["sensors"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := write(s, &hq, "devices.check", `{"id":"`+dev+`"}`, 1); code != 409 {
		t.Error("operations need a bound device")
	}
	if code, _ := write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u1+`","reason":""}`, 1); code != 422 {
		t.Error("bind reason required")
	}
	_, m = write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u1+`","reason":"install"}`, 1)
	v := ver(m)
	if code, _ := write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u1+`","reason":"again"}`, v); code != 409 {
		t.Error("already bound to this unit")
	}
	// calibrate (synchronous history), check (queued), exclusion
	code, m := write(s, &hq, "devices.calibrate", `{"deviceId":"`+dev+`","sensorId":"`+sensor+`","metric":"temperature","unit":"°C","referenceValue":25,"measuredValue":25.4,"calibratedAt":"2026-09-14T00:30:00Z"}`, v)
	if code != 200 || data(m)["isDemo"] != true || data(m)["unitIdAtOccurrence"] != u1 {
		t.Fatalf("calibrate: %d %v", code, m)
	}
	v++
	for name, b := range map[string]string{
		"future":         `{"deviceId":"` + dev + `","sensorId":"` + sensor + `","metric":"temperature","unit":"°C","referenceValue":1,"measuredValue":1,"calibratedAt":"2030-01-01T00:00:00Z"}`,
		"unit mismatch":  `{"deviceId":"` + dev + `","sensorId":"` + sensor + `","metric":"temperature","unit":"%","referenceValue":1,"measuredValue":1,"calibratedAt":"2026-09-14T00:30:00Z"}`,
		"other sensor":   `{"deviceId":"` + dev + `","sensorId":"` + uuid.NewString() + `","metric":"temperature","unit":"°C","referenceValue":1,"measuredValue":1,"calibratedAt":"2026-09-14T00:30:00Z"}`,
		"metric differs": `{"deviceId":"` + dev + `","sensorId":"` + sensor + `","metric":"humidity","unit":"%","referenceValue":1,"measuredValue":1,"calibratedAt":"2026-09-14T00:30:00Z"}`,
	} {
		if code, _ := write(s, &hq, "devices.calibrate", b, v); code != 422 {
			t.Errorf("calibrate %s: %d", name, code)
		}
	}
	code, m = write(s, &hq, "devices.check", `{"id":"`+dev+`"}`, v)
	if code != 200 || data(m)["status"] != "queued" || data(m)["kind"] != "check" {
		t.Fatalf("check: %d %v", code, m)
	}
	v++
	if code, m := post(s, &hq, "devices.get", `{"id":"`+dev+`"}`); code != 200 || data(m)["activeOperation"] == nil {
		t.Fatal("activeOperation shown")
	}
	if code, _ := write(s, &hq, "devices.check", `{"id":"`+dev+`"}`, v); code != 409 {
		t.Error("D05: one operation at a time")
	}
	if code, _ := write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u2+`","reason":"move"}`, v); code != 409 {
		t.Error("cannot rebind while an operation runs")
	}
	// firmware: offline → OFFLINE, unsupported → VALIDATION
	if code, m := write(s, &hq, "devices.updateFirmware", `{"deviceId":"`+dev+`","firmwareVersion":"v2"}`, v); code != 409 || m["code"] != "OFFLINE" {
		t.Fatalf("offline firmware: %d %v", code, m)
	}
	owner(t, `UPDATE devices.device_operations SET status = 'succeeded' WHERE device_id = $1`, dev)
	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE id = $1`, dev)
	if code, _ := write(s, &hq, "devices.updateFirmware", `{"deviceId":"`+dev+`","firmwareVersion":"v9"}`, v); code != 422 {
		t.Error("unsupported firmware")
	}
	if code, _ := write(s, &hq, "devices.updateFirmware", `{"deviceId":"`+dev+`","firmwareVersion":"v1"}`, v); code != 422 {
		t.Error("current firmware is not a target")
	}
	if code, m := write(s, &hq, "devices.updateFirmware", `{"deviceId":"`+dev+`","firmwareVersion":"v2"}`, v); code != 200 || data(m)["targetVersion"] != "v2" {
		t.Fatalf("firmware: %d %v", code, m)
	}
	v++
	owner(t, `UPDATE devices.device_operations SET status = 'succeeded' WHERE device_id = $1`, dev)
	// rebind to another unit: old binding ends, new sensors
	code, m = write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u2+`","reason":"moved"}`, v)
	if code != 200 || data(m)["unitId"] != u2 {
		t.Fatalf("rebind: %d %v", code, m)
	}
	v++
	// tamper blocks rebinding
	owner(t, `UPDATE devices.devices SET tamper = 'detected' WHERE id = $1`, dev)
	if code, m := write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u1+`","reason":"back"}`, v); code != 409 || m["messageKey"] != "error.tamperUnresolved" {
		t.Fatalf("tamper: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+u1+`","reason":"back"}`, v-1); code != 409 {
		t.Error("stale version")
	}
	if code, _ := write(s, &hq, "devices.check", `{"id":"`+uuid.NewString()+`"}`, 1); code != 404 {
		t.Error("unknown device")
	}
}
