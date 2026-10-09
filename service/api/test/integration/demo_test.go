package integration

import (
	"context"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestDemoOperations(t *testing.T) {
	plain := server(t)
	trig := func(s *apiserver.Server, body string) (int, map[string]any) {
		return write(s, &customerB, "demo.trigger", body, 0)
	}
	if code, m := write(plain, &customerB, "demo.advanceClock", `{"to":"`+clock.Add(time.Hour).Format(time.RFC3339)+`"}`, 0); code != 503 || m["messageKey"] != "errors.demo_only" {
		t.Fatalf("production demo ops: %d %v", code, m)
	}
	if code, m := write(plain, &customerB, "demoSession.signIn", `{"demoActorId":"customer-a"}`, 0); code != 503 || m["messageKey"] != "errors.session_via_bff" {
		t.Errorf("demo sign-in: %d %v", code, m)
	}
	s := serverWith(t, true)
	u := boundUnit(t, s, "online")
	var device string
	ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{u}, &device)
	sensor := uuid.NewString()
	owner(t, `INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds) VALUES ($1,$2,$3,'temperature','°C',NULL,120)`, sensor, seed.ID("tenant-a"), device)
	teleAt := func(value, unitName string, sens string, observed time.Time) string {
		return `{"scenarioId":"t","eventId":"` + uuid.NewString() + `","occurredAt":"` + clock.Format(time.RFC3339) + `","eventType":"telemetry","measurement":{"unitId":"` + u +
			`","sensorId":"` + sens + `","metric":"temperature","value":` + value + `,"unit":"` + unitName + `","observedAt":"` + observed.Format(time.RFC3339) +
			`","receivedAt":"` + clock.Format(time.RFC3339) + `","origin":"measured","quality":"valid","eventId":"` + uuid.NewString() + `"}}`
	}
	tele := func(value, unitName string, sens string) string {
		return teleAt(value, unitName, sens, clock.Add(-10*time.Second))
	}
	if code, m := trig(s, tele("25", "°C", sensor)); code != 200 || data(m)["type"] != "telemetry" || data(m)["generation"].(float64) != 1 {
		t.Fatalf("telemetry: %d %v", code, m)
	}
	// IR12 normalization: causes in priority order, a cause keeps a null input suspect, otherwise null is missing
	trig(s, tele("250", "°C", sensor))
	trig(s, tele("25", "K", sensor))
	trig(s, tele("null", "°C", sensor))
	trig(s, tele("null", "kelvin-degrees-with-a-very-long-unit-name", sensor))
	trig(s, teleAt("24", "°C", sensor, clock.Add(time.Minute)))
	type row struct {
		quality       string
		seq           int64
		value         *float64
		reason, rawUn *string
	}
	var rows []row
	func() {
		conn, err := pgx.Connect(context.Background(), "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable")
		if err != nil {
			t.Skip(err)
		}
		defer conn.Close(context.Background())
		r, err := conn.Query(context.Background(), `SELECT quality, sequence, value, quality_reason, raw_unit FROM monitoring.measurements WHERE sensor_id = $1 ORDER BY sequence`, sensor)
		if err != nil {
			t.Fatal(err)
		}
		for r.Next() {
			var x row
			_ = r.Scan(&x.quality, &x.seq, &x.value, &x.reason, &x.rawUn)
			rows = append(rows, x)
		}
	}()
	str := func(p *string) string {
		if p == nil {
			return "-"
		}
		return *p
	}
	want := []struct {
		quality, reason, raw string
		value                bool
	}{
		{"valid", "-", "-", true}, {"suspect", "out_of_range", "-", false}, {"suspect", "unit_mismatch", "K", false}, {"missing", "-", "-", false},
		{"suspect", "unit_mismatch", "kelvin-degrees-with-a-very-long-", false}, {"suspect", "invalid_time", "-", true},
	}
	if len(rows) != len(want) || rows[2].seq != 3 {
		t.Fatalf("telemetry rows: %v", rows)
	}
	for i, w := range want {
		if r := rows[i]; r.quality != w.quality || str(r.reason) != w.reason || str(r.rawUn) != w.raw || (r.value != nil) != w.value {
			t.Errorf("row %d: %s %s %s %v, want %v", i, r.quality, str(r.reason), str(r.rawUn), r.value, w)
		}
	}
	if code, _ := trig(s, tele("25", "°C", uuid.NewString())); code != 404 {
		t.Error("unknown sensor")
	}
	if code, _ := trig(s, `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock.Format(time.RFC3339)+`","eventType":"network","connected":false}`); code != 422 {
		t.Error("unsupported trigger type")
	}
	// command acknowledgement through the demo trigger
	_, g := post(s, &customerB, "units.get", `{"id":"`+u+`"}`)
	code, m := write(s, &customerB, "commands.create", `{"unitId":"`+u+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(ver(g))+`}`, 0)
	if code != 200 {
		t.Fatalf("command: %d %v", code, m)
	}
	cmd := data(m)["id"].(string)
	keepObserved(t, cmd)
	if code, _ := trig(s, `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock.Add(time.Second).Format(time.RFC3339)+`","eventType":"command_ack","commandId":"`+cmd+`","sequence":1}`); code != 200 {
		t.Fatal("ack trigger")
	}
	if _, m := post(s, &customerB, "commands.get", `{"id":"`+cmd+`"}`); data(m)["status"] != "acknowledged" {
		t.Errorf("acknowledged: %v", m)
	}
	// the acknowledged setting becomes the observed state without a unit version change
	_, g2 := post(s, &customerB, "units.get", `{"id":"`+u+`"}`)
	if obs := data(g2)["observedState"].(map[string]any); obs["power"] != true || obs["observedAt"] != clock.Add(time.Second).Format(time.RFC3339Nano) {
		t.Errorf("observedState after ack: %v", obs)
	}
	if ver(g2) != ver(g) {
		t.Errorf("unit version changed by ack: %d → %d", ver(g), ver(g2))
	}
	// the clock only moves forward; deadlines are processed by the jump
	if code, _ := write(s, &customerB, "demo.advanceClock", `{"to":"`+clock.Add(-time.Hour).Format(time.RFC3339)+`"}`, 0); code != 422 {
		t.Error("backwards jump")
	}
	if code, m := write(s, &customerB, "demo.advanceClock", `{"to":"`+clock.Add(time.Hour).Format(time.RFC3339)+`"}`, 0); code != 200 || data(m)["generation"].(float64) != 1 {
		t.Fatalf("advance: %d %v", code, m)
	}
	// rows created after the jump carry the scenario clock (platform.app_now, IR157)
	code, m = write(s, &customerB, "clientUsers.save", `{"customerId":"`+seed.ID("cust-b").String()+`","email":"clock-`+uuid.NewString()[:8]+`@example.com","clientRole":"member"}`, 0)
	if code != 200 || data(m)["createdAt"] != clock.Add(time.Hour).Format(time.RFC3339) {
		t.Errorf("created after the jump: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "demo.reset", `{"confirmation":true}`, 0); code != 503 || m["messageKey"] != "errors.demo_reset_offline" {
		t.Errorf("reset: %d %v", code, m)
	}
}
