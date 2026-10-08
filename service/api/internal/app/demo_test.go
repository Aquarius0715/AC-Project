package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestDemoOperations(t *testing.T) {
	plain := server(t)
	trig := func(s *Server, body string) (int, map[string]any) {
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
	tele := func(value, unitName string, sens string) string {
		return `{"scenarioId":"t","eventId":"` + uuid.NewString() + `","occurredAt":"` + clock.Format(time.RFC3339) + `","eventType":"telemetry","measurement":{"unitId":"` + u +
			`","sensorId":"` + sens + `","metric":"temperature","value":` + value + `,"unit":"` + unitName + `","observedAt":"` + clock.Add(-10*time.Second).Format(time.RFC3339) +
			`","receivedAt":"` + clock.Format(time.RFC3339) + `","origin":"measured","quality":"valid","eventId":"` + uuid.NewString() + `"}}`
	}
	if code, m := trig(s, tele("25", "°C", sensor)); code != 200 || data(m)["type"] != "telemetry" || data(m)["generation"].(float64) != 1 {
		t.Fatalf("telemetry: %d %v", code, m)
	}
	trig(s, tele("250", "°C", sensor))
	trig(s, tele("25", "K", sensor))
	rows := [][2]any{}
	func() {
		conn, err := pgx.Connect(context.Background(), "postgres://postgres:local@localhost:5432/ac?sslmode=disable")
		if err != nil {
			t.Skip(err)
		}
		defer conn.Close(context.Background())
		r, err := conn.Query(context.Background(), `SELECT quality, sequence FROM monitoring.measurements WHERE sensor_id = $1 ORDER BY sequence`, sensor)
		if err != nil {
			t.Fatal(err)
		}
		for r.Next() {
			var q string
			var seq int64
			_ = r.Scan(&q, &seq)
			rows = append(rows, [2]any{q, seq})
		}
	}()
	if len(rows) != 3 || rows[0][0] != "valid" || rows[1][0] != "suspect" || rows[2][0] != "suspect" || rows[2][1].(int64) != 3 {
		t.Errorf("telemetry rows: %v", rows)
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
	if code, _ := trig(s, `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock.Add(time.Second).Format(time.RFC3339)+`","eventType":"command_ack","commandId":"`+cmd+`","sequence":1}`); code != 200 {
		t.Fatal("ack trigger")
	}
	if _, m := post(s, &customerB, "commands.get", `{"id":"`+cmd+`"}`); data(m)["status"] != "acknowledged" {
		t.Errorf("acknowledged: %v", m)
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
