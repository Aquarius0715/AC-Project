package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestDeviceOperationLifecycle covers IR67 on the equipment scheduler and demo.trigger(operation): queued → running
// one second after creation (check connecting), results only for running operations, TIMEOUT at expiresAt, the
// start recheck of REV19-018 and the firmware OFFLINE start failure.
func TestDeviceOperationLifecycle(t *testing.T) {
	s := serverWith(t, true)
	ctx := context.Background()
	u := boundUnit(t, s, "online")
	var dev string
	ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{u}, &dev)
	version := func() int {
		var v int
		ownerScan(t, `SELECT version FROM devices.devices WHERE id = $1`, []any{dev}, &v)
		return v
	}
	conn := func() (string, string) {
		var d, un string
		ownerScan(t, `SELECT d.connection, un.connection FROM devices.devices d JOIN assets.units un ON un.id = d.unit_id WHERE d.id = $1`, []any{dev}, &d, &un)
		return d, un
	}
	op := func(id string) (string, string) {
		var status, code string
		ownerScan(t, `SELECT status, COALESCE(failure_code, '') FROM devices.device_operations WHERE id = $1`, []any{id}, &status, &code)
		return status, code
	}
	tick := func(at time.Time) {
		t.Helper()
		if _, err := schedTick(ctx, s, at); err != nil {
			t.Fatal(err)
		}
	}
	result := func(id, r string, at time.Time) (int, map[string]any) {
		return write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+at.Format(time.RFC3339Nano)+
			`","eventType":"operation","operationId":"`+id+`","result":"`+r+`"}`, 0)
	}
	var base time.Time
	start := func(kind string) string {
		t.Helper()
		body := `{"id":"` + dev + `"}`
		if kind == "updateFirmware" {
			body = `{"deviceId":"` + dev + `","firmwareVersion":"v2"}`
		}
		code, m := write(s, &hq, "devices."+kind, body, version())
		if code != 200 || data(m)["status"] != "queued" {
			t.Fatalf("%s: %d %v", kind, code, m)
		}
		base, _ = time.Parse(time.RFC3339Nano, data(m)["createdAt"].(string)) // the server's business time (a shared demo cluster may run ahead of clock)
		return data(m)["id"].(string)
	}
	// check: queued until the tick one second after creation, then running and connecting
	check := start("check")
	tick(base.Add(500 * time.Millisecond))
	if st, _ := op(check); st != "queued" {
		t.Fatalf("started before one second: %s", st)
	}
	if code, m := result(check, "succeeded", base); code != 409 || m["messageKey"] != "errors.operation_not_running" {
		t.Fatalf("result for a queued operation: %d %v", code, m)
	}
	tick(base.Add(time.Second))
	if st, _ := op(check); st != "running" {
		t.Fatalf("not running at +1 s: %s", st)
	}
	if d, un := conn(); d != "connecting" || un != "connecting" {
		t.Fatalf("check sets connecting: %s / %s", d, un)
	}
	if code, m := result(check, "succeeded", base.Add(3*time.Second)); code != 200 {
		t.Fatalf("check result: %d %v", code, m)
	}
	if d, un := conn(); d != "online" || un != "online" {
		t.Fatalf("check success → online: %s / %s", d, un)
	}
	var seen time.Time
	ownerScan(t, `SELECT last_seen_at FROM devices.devices WHERE id = $1`, []any{dev}, &seen)
	if !seen.Equal(base.Add(3 * time.Second)) {
		t.Errorf("lastSeenAt from the result: %v", seen)
	}
	if code, _ := result(check, "failed", base.Add(4*time.Second)); code != 409 {
		t.Error("a finished operation takes no result")
	}
	// a running check that never answers times out at expiresAt and leaves the device in error
	timeout := start("check")
	tick(base.Add(time.Second))
	tick(base.Add(60 * time.Second))
	if st, code := op(timeout); st != "failed" || code != "TIMEOUT" {
		t.Fatalf("timeout: %s %s", st, code)
	}
	if d, un := conn(); d != "error" || un != "error" {
		t.Fatalf("check TIMEOUT → error: %s / %s", d, un)
	}
	// a failed check result (the device answers) → error; a check is accepted in any connection state
	failed := start("check")
	tick(base.Add(time.Second))
	if code, _ := result(failed, "failed", base.Add(2*time.Second)); code != 200 {
		t.Fatal("check failure result")
	}
	if st, code := op(failed); st != "failed" || code != "UNAVAILABLE" {
		t.Fatalf("failed check: %s %s", st, code)
	}
	ok := start("check")
	tick(base.Add(time.Second))
	result(ok, "succeeded", base.Add(2*time.Second))
	// firmware: the device went offline before the start tick → failed OFFLINE, connection and version kept
	fw := start("updateFirmware")
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE id = $1`, dev)
	v := version()
	tick(base.Add(time.Second))
	if st, code := op(fw); st != "failed" || code != "OFFLINE" {
		t.Fatalf("firmware start offline: %s %s", st, code)
	}
	var firmware string
	ownerScan(t, `SELECT firmware_version FROM devices.devices WHERE id = $1`, []any{dev}, &firmware)
	if d, _ := conn(); d != "offline" || version() != v || firmware != "1.0" {
		t.Fatalf("OFFLINE start keeps the device: %s v%d %s", d, version(), firmware)
	}
	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE id = $1`, dev)
	fw = start("updateFirmware")
	tick(base.Add(time.Second))
	if d, _ := conn(); d != "online" {
		t.Errorf("a running firmware update keeps the connection: %s", d)
	}
	if code, _ := result(fw, "succeeded", base.Add(5*time.Second)); code != 200 {
		t.Fatal("firmware result")
	}
	ownerScan(t, `SELECT firmware_version FROM devices.devices WHERE id = $1`, []any{dev}, &firmware)
	if firmware != "v2" {
		t.Fatalf("firmware installed: %s", firmware)
	}
	code, m := write(s, &hq, "devices.updateFirmware", `{"deviceId":"`+dev+`","firmwareVersion":"v1"}`, version())
	if code != 200 {
		t.Fatalf("firmware v1: %d %v", code, m)
	}
	back := data(m)["id"].(string)
	tick(base.Add(time.Second))
	result(back, "failed", base.Add(5*time.Second))
	ownerScan(t, `SELECT firmware_version FROM devices.devices WHERE id = $1`, []any{dev}, &firmware)
	if st, code := op(back); st != "failed" || code != "UNAVAILABLE" || firmware != "v2" {
		t.Fatalf("failed firmware keeps the old version: %s %s %s", st, code, firmware)
	}
	// REV19-018: control work patched in after creation fails the start with CONFLICT; an undelivered restriction
	// Command does not
	cmd := func(source, delivery string) string {
		id := uuid.NewString()
		owner(t, `INSERT INTO control.commands (id, tenant_id, unit_id, device_id, actor_membership_id, source, action, status, delivery, requested_at, expires_at, correlation_id)
			VALUES ($1,$2,$3,$4,$5,$6,'{"kind":"set_power","power":false}','requested',$7,$8,$9,'t')`, id, seed.ID("tenant-a"), u, dev, seed.ID("hq-operator"), source, delivery, base, base.Add(10*time.Minute))
		return id
	}
	c1 := start("check")
	restriction := cmd("restriction", "not_sent") // D03: a restriction Command for a unit with an open operation waits undelivered
	tick(base.Add(time.Second))
	if st, _ := op(c1); st != "running" {
		t.Fatalf("undelivered restriction Command does not block the start: %s", st)
	}
	result(c1, "succeeded", base.Add(2*time.Second))
	owner(t, `UPDATE control.commands SET status = 'cancelled' WHERE id = $1`, restriction)
	c2 := start("check")
	normal := cmd("ui", "sent")
	v = version()
	tick(base.Add(time.Second))
	if st, code := op(c2); st != "failed" || code != "CONFLICT" {
		t.Fatalf("recheck conflict: %s %s", st, code)
	}
	if d, _ := conn(); d != "online" || version() != v {
		t.Errorf("CONFLICT start keeps the device: %s v%d", d, version())
	}
	owner(t, `UPDATE control.commands SET status = 'cancelled' WHERE id = $1`, normal)
	c3 := start("check")
	other := uuid.NewString()
	owner(t, `INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, expires_at, created_at) VALUES ($1,$2,$3,$4,'calibrate','queued',$5,$6)`,
		other, seed.ID("tenant-a"), dev, u, base.Add(10*time.Minute), base.Add(time.Minute))
	tick(base.Add(time.Second))
	if st, code := op(c3); st != "failed" || code != "CONFLICT" {
		t.Fatalf("another open operation: %s %s", st, code)
	}
	owner(t, `DELETE FROM devices.device_operations WHERE id = $1`, other)
	// validation
	for _, b := range []string{`"operationId":"` + check + `","result":"done"`, `"result":"succeeded"`} {
		if code, _ := write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock.Format(time.RFC3339)+`","eventType":"operation",`+b+`}`, 0); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := result(uuid.NewString(), "succeeded", clock); code != 404 {
		t.Error("unknown operation")
	}
}

// TestDeviceEvents covers demo.trigger(device) (SR20, SR23, REV18-040, AT-T12-N/E/B): independent connection, power
// and tamper axes, the tamper Alert, recoveries of the current fault only, out-of-order sequences and eventId repeats.
func TestDeviceEvents(t *testing.T) {
	s := serverWith(t, true)
	u := boundUnit(t, s, "online")
	var dev, binding string
	ownerScan(t, `SELECT id::text, binding_id::text FROM devices.devices WHERE unit_id = $1`, []any{u}, &dev, &binding)
	at := func(sec int) string { return clock.Add(time.Duration(sec) * time.Second).Format(time.RFC3339) }
	signal := func(event, kind string, seq int, recovery string, sec int) (int, map[string]any) {
		r := ""
		if recovery != "" {
			r = `,"recovery":` + recovery
		}
		return write(s, &customerB, "demo.trigger", `{"scenarioId":"t12","eventId":"`+event+`","occurredAt":"`+at(sec)+`","eventType":"device","deviceId":"`+dev+
			`","bindingId":"`+binding+`","kind":"`+kind+`","sequence":`+itoa(seq)+r+`}`, 0)
	}
	state := func() (string, string, string, string) {
		var conn, power, tamper, unitConn string
		ownerScan(t, `SELECT d.connection, d.power_signal, d.tamper, un.connection FROM devices.devices d JOIN assets.units un ON un.id = d.unit_id WHERE d.id = $1`,
			[]any{dev}, &conn, &power, &tamper, &unitConn)
		return conn, power, tamper, unitConn
	}
	eventOf := func(eventID string) (string, []uuid.UUID, *time.Time, string) {
		var id, evidence string
		var alerts []uuid.UUID
		var restored *time.Time
		ownerScan(t, `SELECT id::text, alert_ids, restored_at, evidence_source FROM devices.device_events WHERE event_id = $1`, []any{eventID}, &id, &alerts, &restored, &evidence)
		return id, alerts, restored, evidence
	}
	count := func() int {
		var n int
		ownerScan(t, `SELECT count(*) FROM devices.device_events WHERE device_id = $1`, []any{dev}, &n)
		return n
	}
	// ① missing heartbeat → offline only ② power signal lost ③ tamper signal: each axis independent (AT-T12-B)
	lost := uuid.NewString()
	if code, m := signal(lost, "communication_lost", 1, "", 1); code != 200 || data(m)["type"] != "device" {
		t.Fatalf("communication_lost: %d %v", code, m)
	}
	if c, p, tm, uc := state(); c != "offline" || p != "on" || tm != "clear" || uc != "offline" {
		t.Fatalf("offline only: %s %s %s %s", c, p, tm, uc)
	}
	lostID, alerts, _, evidence := eventOf(lost)
	if len(alerts) != 0 || evidence != "heartbeat" {
		t.Errorf("communication_lost evidence: %v %s", alerts, evidence)
	}
	power := uuid.NewString()
	signal(power, "power_lost", 1, "", 2) // sequences are per axis
	if c, p, tm, _ := state(); c != "offline" || p != "off" || tm != "clear" {
		t.Fatalf("power lost: %s %s %s", c, p, tm)
	}
	if _, _, _, evidence := eventOf(power); evidence != "power_signal" {
		t.Errorf("power evidence: %s", evidence)
	}
	tamper := uuid.NewString()
	if code, m := signal(tamper, "tamper", 1, "", 3); code != 200 {
		t.Fatalf("tamper: %d %v", code, m)
	}
	tamperID, tamperAlerts, _, evidence := eventOf(tamper)
	if c, p, tm, _ := state(); c != "offline" || p != "off" || tm != "detected" || len(tamperAlerts) != 1 || evidence != "tamper_signal" {
		t.Fatalf("tamper detected with one alert: %s %s %s %v %s", c, p, tm, tamperAlerts, evidence)
	}
	alert := tamperAlerts[0].String()
	var typ, severity, status string
	ownerScan(t, `SELECT type, severity, status FROM monitoring.alerts WHERE id = $1`, []any{alert}, &typ, &severity, &status)
	if typ != "tamper" || severity != "critical" || status != "open" {
		t.Fatalf("tamper alert: %s %s %s", typ, severity, status)
	}
	// the same eventId again: no new event; with other content CONFLICT
	n := count()
	if code, _ := signal(tamper, "tamper", 1, "", 3); code != 200 || count() != n {
		t.Error("repeated eventId is idempotent")
	}
	if code, m := signal(tamper, "tamper", 2, "", 3); code != 409 || m["messageKey"] != "errors.event_id_reused" {
		t.Errorf("eventId with other content: %d %v", code, m)
	}
	// connection recovery while the tamper alert is unacknowledged: only the connection recovers (AT-T12-E ①)
	rec := func(axis, source, value string) string {
		return `{"axis":"` + axis + `","sourceEventId":"` + source + `","value":"` + value + `"}`
	}
	back := uuid.NewString()
	if code, m := signal(back, "restored", 2, rec("connection", lostID, "online"), 10); code != 200 {
		t.Fatalf("restored: %d %v", code, m)
	}
	if c, p, tm, uc := state(); c != "online" || p != "off" || tm != "detected" || uc != "online" {
		t.Fatalf("connection restored only: %s %s %s %s", c, p, tm, uc)
	}
	if _, _, restored, _ := eventOf(lost); restored == nil || !restored.Equal(clock.Add(10*time.Second)) {
		t.Errorf("restoredAt on the fault: %v", restored)
	}
	ownerScan(t, `SELECT status FROM monitoring.alerts WHERE id = $1`, []any{alert}, &status)
	if status != "open" {
		t.Errorf("tamper alert remains: %s", status)
	}
	if code, m := signal(uuid.NewString(), "restored", 3, rec("connection", lostID, "online"), 11); code != 409 || m["messageKey"] != "errors.recovery_source_not_current" {
		t.Errorf("restoring a restored fault: %d %v", code, m)
	}
	// an old heartbeat after a new communication loss leaves the device offline (AT-T12-E ②, IR102 G1-023)
	lost5 := uuid.NewString()
	signal(lost5, "communication_lost", 5, "", 20)
	lost5ID, _, _, _ := eventOf(lost5)
	n = count()
	if code, _ := signal(uuid.NewString(), "restored", 4, rec("connection", lost5ID, "online"), 21); code != 200 {
		t.Fatal("old heartbeat is accepted")
	}
	if c, _, _, _ := state(); c != "offline" || count() != n {
		t.Fatalf("old sequence changes nothing: %s %d/%d", c, count(), n)
	}
	if _, _, restored, _ := eventOf(lost5); restored != nil {
		t.Error("old heartbeat does not restore the fault")
	}
	// tamper recovery clears the observed tamper only; the event inherits the fault's alerts (SR23), the alert stays
	clear := uuid.NewString()
	if code, m := signal(clear, "restored", 2, rec("tamper", tamperID, "clear"), 30); code != 200 {
		t.Fatalf("tamper restored: %d %v", code, m)
	}
	_, inherited, _, evidence := eventOf(clear)
	if _, _, tm, _ := state(); tm != "clear" || len(inherited) != 1 || inherited[0].String() != alert || evidence != "tamper_signal" {
		t.Fatalf("tamper recovery: %s %v %s", tm, inherited, evidence)
	}
	ownerScan(t, `SELECT status FROM monitoring.alerts WHERE id = $1`, []any{alert}, &status)
	if status != "open" {
		t.Errorf("tamper recovery does not resolve the alert: %s", status)
	}
	// the recovery is evidence a resolution of the tamper alert may cite (IR327)
	clearID, _, _, _ := eventOf(clear)
	if _, m := post(s, &hq, "alerts.evidence", `{"alertId":"`+alert+`","query":{"limit":100}}`); !slices.ContainsFunc(items(m), func(it map[string]any) bool {
		return it["id"] == clearID && it["kind"] == "device_event" && it["eventType"] == "restored" && it["observedAt"] != nil
	}) {
		t.Errorf("tamper recovery as evidence: %v", m)
	}
	// a new tamper while the alert is open reuses it; after resolution a new alert links the previous one (IR66)
	again := uuid.NewString()
	signal(again, "tamper", 3, "", 40)
	if _, a, _, _ := eventOf(again); len(a) != 1 || a[0].String() != alert {
		t.Errorf("open alert reused: %v", a)
	}
	signal(uuid.NewString(), "restored", 4, rec("tamper", func() string { id, _, _, _ := eventOf(again); return id }(), "clear"), 41)
	owner(t, `UPDATE monitoring.alerts SET status = 'resolved', resolved_at = $2 WHERE id = $1`, alert, clock.Add(42*time.Second))
	third := uuid.NewString()
	signal(third, "tamper", 5, "", 50)
	_, a, _, _ := eventOf(third)
	var previous *string
	ownerScan(t, `SELECT previous_alert_id::text FROM monitoring.alerts WHERE id = $1`, []any{a[0]}, &previous)
	if a[0].String() == alert || previous == nil || *previous != alert {
		t.Errorf("recurrence links the resolved alert: %v %v", a, previous)
	}
	// devices.events shows the events with their alerts and recovery
	code, m := post(s, &hq, "devices.events", `{"id":"`+dev+`","query":{"limit":50}}`)
	if code != 200 || len(items(m)) != count() {
		t.Fatalf("events: %d %v", code, m)
	}
	// validation and refusals
	for name, b := range map[string]string{
		"unknown kind":        `"kind":"smoke","sequence":9`,
		"restored no source":  `"kind":"restored","sequence":9`,
		"axis/value mismatch": `"kind":"restored","sequence":9,"recovery":` + rec("connection", lostID, "on"),
		"fault with recovery": `"kind":"tamper","sequence":9,"recovery":` + rec("tamper", tamperID, "clear"),
		"no sequence":         `"kind":"tamper"`,
	} {
		body := `{"scenarioId":"t","eventId":"` + uuid.NewString() + `","occurredAt":"` + at(60) + `","eventType":"device","deviceId":"` + dev + `","bindingId":"` + binding + `",` + b + `}`
		if code, _ := write(s, &customerB, "demo.trigger", body, 0); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, m := write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+at(60)+`","eventType":"device","deviceId":"`+dev+
		`","bindingId":"`+uuid.NewString()+`","kind":"tamper","sequence":9}`, 0); code != 422 || m["fieldErrors"].(map[string]any)["bindingId"] == nil {
		t.Errorf("binding mismatch: %d %v", code, m)
	}
	if code, _ := write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+at(60)+`","eventType":"device","deviceId":"`+uuid.NewString()+
		`","bindingId":null,"kind":"tamper","sequence":9}`, 0); code != 404 {
		t.Error("unknown device")
	}
}
