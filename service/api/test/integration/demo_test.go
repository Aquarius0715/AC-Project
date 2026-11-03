package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDemoOperations(t *testing.T) {
	plain := server(t)
	trig := func(s *apiserver.Server, body string) (int, map[string]any) {
		return write(s, customerB(), "demo.trigger", body, 0)
	}

	if code, resp := write(
		plain,
		customerB(),
		"demo.advanceClock",
		`{"to":"`+clock().Add(time.Hour).Format(time.RFC3339)+`"}`,
		0,
	); code != 503 ||
		resp["messageKey"] != "errors.demo_only" {
		t.Fatalf("production demo ops: %d %v", code, resp)
	}
	// sessions belong to the BFF and the identity provider in API mode: every demoSession operation says so
	for operation, body := range map[string]string{
		opDemoSessionSignIn:           `{"demoActorId":"customer-a"}`,
		opDemoSessionSwitchMembership: `{"demoMembershipId":"m-1"}`, "demoSession.signOut": `{}`, opDemoSessionExtend: `{}`,
	} {
		if code, resp := write(
			plain,
			customerB(),
			operation,
			body,
			0,
		); code != 503 ||
			resp["messageKey"] != "errors.session_via_bff" {
			t.Errorf("%s: %d %v", operation, code, resp)
		}
	}

	srv := serverWith(t, true)
	unit := boundUnit(t, srv, "online")

	var device string
	ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{unit}, &device)

	sensor := uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds) VALUES ($1,$2,
			$3,'temperature','°C',NULL,120)`,
		sensor,
		seed.ID("tenant-a"),
		device,
	)

	teleAt := func(value, unitName string, sens string, observed time.Time) string {
		return `{"scenarioId":"t","eventId":"` + uuid.NewString() + `","occurredAt":"` + clock().Format(
			time.RFC3339,
		) +
			`","eventType":"telemetry","measurement":{"unitId":"` + unit +
			`","sensorId":"` + sens + `","metric":"temperature",` +
			`"value":` + value + `,"unit":"` + unitName + `","observedAt":"` + observed.Format(
			time.RFC3339,
		) +
			`","receivedAt":"` + clock().Format(
			time.RFC3339,
		) +
			`","origin":"measured","quality":"valid","eventId":"` + uuid.NewString() + `"}}`
	}

	tele := func(value, unitName string, sens string) string {
		return teleAt(value, unitName, sens, clock().Add(-10*time.Second))
	}
	if code, m := trig(
		srv,
		tele("25", "°C", sensor),
	); code != 200 || data(m)["type"] != "telemetry" ||
		asNum(t, data(m)["generation"]) != 1 {
		t.Fatalf("telemetry: %d %v", code, m)
	}
	// IR12 normalization: causes in priority order, a cause keeps a null input suspect, otherwise null is missing
	trig(srv, tele("250", "°C", sensor))
	trig(srv, tele("25", "K", sensor))
	trig(srv, tele("null", "°C", sensor))
	trig(srv, tele("null", "kelvin-degrees-with-a-very-long-unit-name", sensor))
	trig(srv, teleAt("24", "°C", sensor, clock().Add(time.Minute)))

	type row struct {
		quality       string
		seq           int64
		value         *float64
		reason, rawUn *string
	}

	var rows []row

	func() {
		conn, err := pgx.Connect(context.Background(), testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}
		defer func() { _ = conn.Close(context.Background()) }()

		measurementRows, err := conn.Query(
			context.Background(),
			`SELECT quality, sequence, value, quality_reason, raw_unit FROM monitoring.measurements WHERE sensor_id = $1 ORDER BY
				sequence`,
			sensor,
		)
		if err != nil {
			t.Fatal(err)
		}

		for measurementRows.Next() {
			var scanned row

			err := measurementRows.Scan(&scanned.quality, &scanned.seq, &scanned.value, &scanned.reason, &scanned.rawUn)
			if err != nil {
				t.Fatal(err)
			}

			rows = append(rows, scanned)
		}

		rowsErr := measurementRows.Err()
		if rowsErr != nil {
			t.Fatal(rowsErr)
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
		{qualityValid, "-", "-", true},
		{qualitySuspect, "out_of_range", "-", false},
		{qualitySuspect, "unit_mismatch", "K", false},
		{"missing", "-", "-", false},
		{qualitySuspect, "unit_mismatch", "kelvin-degrees-with-a-very-long-", false},
		{qualitySuspect, "invalid_time", "-", true},
	}
	if len(rows) != len(want) || rows[2].seq != 3 {
		t.Fatalf("telemetry rows: %v", rows)
	}

	for idx, wanted := range want {
		if got := rows[idx]; got.quality != wanted.quality || str(got.reason) != wanted.reason ||
			str(got.rawUn) != wanted.raw ||
			(got.value != nil) != wanted.value {
			t.Errorf(
				"row %d: %s %s %s %v, want %v",
				idx,
				got.quality,
				str(got.reason),
				str(got.rawUn),
				got.value,
				wanted,
			)
		}
	}

	if code, _ := trig(srv, tele("25", "°C", uuid.NewString())); code != 404 {
		t.Error("unknown sensor")
	}

	if code, resp := trig(
		srv,
		`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock().Format(
			time.RFC3339,
		)+`","eventType":"network","bogus":false}`,
	); code != 422 ||
		fields(resp) != refusedMalformedInput {
		t.Errorf("a trigger with a field no type has: %d %s", code, fields(resp))
	}
	// a trigger of the browser demo's types, shaped by the contract, is refused as unsupported (IR391)
	for _, extra := range []string{
		`"connected":false`, `"enabled":true`, `"membershipId":"` + uuid.NewString() + `","qualification":"demo_indoor"`,
		`"operation":"units.list","outcome":"TIMEOUT","retryAfterSeconds":null,"delayMs":null,"remainingCalls":1`,
	} {
		code, resp := trig(
			srv,
			`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock().Format(
				time.RFC3339,
			)+`","eventType":"network",`+extra+`}`,
		)
		refusal(t, "contract-shaped "+extra, code, resp, "eventType=errors.trigger_unsupported")
	}
	// the trigger's own rules (IR36): a known event type, its IDs, and a measurement of the sensor's metric and unit
	now := clock().Format(time.RFC3339)
	for name, testCase := range map[string]struct{ body, field, key string }{
		"another type": {
			`{"scenarioId":"t",` +
				`"eventId":"` + uuid.NewString() + `","occurredAt":"` + now + `","eventType":"network"}`, "eventType",
			"errors.trigger_unsupported",
		},
		"no event ID": {`{"scenarioId":"t",` +
			`"occurredAt":"` + now + `","eventType":"network"}`, fieldEventID, apperr.KeyRequired},
		"no time": {`{"scenarioId":"t",` +
			`"eventId":"` + uuid.NewString() + `","eventType":"network"}`, fieldOccurredAt, apperr.KeyRequired},
		"command without ID": {
			`{"scenarioId":"t",` +
				`"eventId":"` + uuid.NewString() + `","occurredAt":"` + now + `","eventType":"command_ack"}`, "commandId",
			apperr.KeyRequired,
		},
		"telemetry without one": {
			`{"scenarioId":"t",` +
				`"eventId":"` + uuid.NewString() + `","occurredAt":"` + now + `","eventType":"telemetry"}`, "measurement",
			apperr.KeyRequired,
		},
		"observation, no unit": {
			`{"scenarioId":"t",` +
				`"eventId":"` + uuid.NewString() + `","occurredAt":"` + now + `","eventType":"restriction_observation"}`,
			fieldUnitID,
			apperr.KeyRequired,
		},
		"device without one": {`{"scenarioId":"t",` +
			`"eventId":"` + uuid.NewString() + `","occurredAt":"` + now + `","eventType":"device","kind":"tamper",` +
			`"sequence":1}`, fieldDeviceID, apperr.KeyRequired},
	} {
		if code, m := trig(
			srv,
			testCase.body,
		); code != 422 ||
			asObj(t, m["fieldErrors"])[testCase.field] != testCase.key {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}

	if code, resp := trig(
		srv,
		strings.Replace(
			tele("55", "%", sensor),
			`"metric":"temperature"`,
			`"metric":"humidity"`,
			1,
		),
	); code != 422 ||
		asObj(t, resp["fieldErrors"])["measurement"] != "errors.sensor_mismatch" {
		t.Errorf("another metric than the sensor's: %d %v", code, resp)
	}
	// IR77: a measurement's own sequence is kept; without one it is the sensor's latest + 1
	if code, resp := trig(
		srv,
		strings.Replace(
			tele("26", "°C", sensor),
			`"quality":"valid"`,
			`"quality":"valid","sequence":100`,
			1,
		),
	); code != 200 {
		t.Fatalf("own sequence: %d %v", code, resp)
	}

	var top int64
	ownerScan(
		t,
		`SELECT max(sequence) FROM monitoring.measurements WHERE sensor_id = $1`,
		[]any{sensor},
		&top,
	)

	if top != 100 {
		t.Errorf("own sequence kept: %d", top)
	}
	// command acknowledgement through the demo trigger
	_, detail := post(srv, customerB(), "units.get", `{"id":"`+unit+`"}`)

	code, res := write(
		srv,
		customerB(),
		"commands.create",
		`{"unitId":"`+unit+`","action":{"kind":"set_power","power":true},"expectedUnitVersion":`+itoa(
			ver(t, detail),
		)+`}`,
		0,
	)
	if code != 200 {
		t.Fatalf("command: %d %v", code, res)
	}

	cmd := asStr(t, data(res)["id"])
	keepObserved(t, cmd)
	// a requested command is marked sent by the device's sent event (commands.create sends at once, so set it back)
	owner(
		t,
		`UPDATE control.commands SET status = 'requested', delivery = 'not_sent', sent_at = NULL WHERE id = $1`,
		cmd,
	)

	sentAt := clock().Add(500 * time.Millisecond)
	if code, resp := trig(
		srv,
		`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+sentAt.Format(
			time.RFC3339Nano,
		)+`","eventType":"command_sent","commandId":"`+cmd+`","sequence":1}`,
	); code != 200 {
		t.Fatalf("sent trigger: %d %v", code, resp)
	}

	if _, resp := post(
		srv,
		customerB(),
		"commands.get",
		`{"id":"`+cmd+`"}`,
	); data(resp)["status"] != statusSent || data(resp)["delivery"] != statusSent ||
		data(resp)["sentAt"] != sentAt.Format(time.RFC3339Nano) {
		t.Errorf("sent: %v", data(resp))
	}

	if code, _ := trig(
		srv,
		`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+clock().Add(time.Second).
			Format(time.RFC3339)+
			`","eventType":"command_ack","commandId":"`+cmd+`","sequence":2}`,
	); code != 200 {
		t.Fatal("ack trigger")
	}

	if _, resp := post(
		srv,
		customerB(),
		"commands.get",
		`{"id":"`+cmd+`"}`,
	); data(resp)["status"] != statusAcknowledged {
		t.Errorf("acknowledged: %v", resp)
	}
	// the request was audited pending; the acknowledgement appends its result under the same correlation ID (IR90, IR604)
	if trail := commandTrail(
		t,
		srv,
		res,
	); len(trail) != 2 || trail[0] != "pending customer-b" ||
		trail[1] != "success system-iot " {
		t.Errorf("the command's audit trail: %v", trail)
	}
	// the acknowledged setting becomes the observed state without a unit version change
	_, laterDetail := post(srv, customerB(), "units.get", `{"id":"`+unit+`"}`)
	if obs := asObj(t, data(laterDetail)["observedState"]); obs["power"] != true ||
		obs["observedAt"] != clock().Add(time.Second).Format(time.RFC3339Nano) {
		t.Errorf("observedState after ack: %v", obs)
	}

	if ver(t, laterDetail) != ver(t, detail) {
		t.Errorf("unit version changed by ack: %d → %d", ver(t, detail), ver(t, laterDetail))
	}
	// the clock only moves forward; deadlines are processed by the jump
	if code, resp := write(
		srv,
		customerB(),
		"demo.advanceClock",
		`{"to":"`+clock().Add(-time.Hour).Format(time.RFC3339)+`"}`,
		0,
	); code != 422 ||
		fields(resp) != "to=errors.clock_backwards" {
		t.Errorf("backwards jump: %d %s", code, fields(resp))
	}

	if code, resp := write(
		srv,
		customerB(),
		"demo.advanceClock",
		`{"to":"`+clock().Add(time.Hour).Format(time.RFC3339)+`"}`,
		0,
	); code != 200 ||
		asNum(t, data(resp)["generation"]) != 1 {
		t.Fatalf("advance: %d %v", code, resp)
	}
	// rows created after the jump carry the scenario clock (platform.app_now, IR157)
	code, res = write(
		srv,
		customerB(),
		"clientUsers.save",
		`{"customerId":"`+seed.ID("cust-b").
			String()+
			`","email":"clock-`+uuid.NewString()[:8]+`@example.com","clientRole":"member","displayName":"Clock Member"}`,
		0,
	)
	if code != 200 || data(res)["createdAt"] != clock().Add(time.Hour).Format(time.RFC3339) {
		t.Errorf("created after the jump: %d %v", code, res)
	}

	if code, resp := write(
		srv,
		customerB(),
		"demo.reset",
		`{"confirmation":true}`,
		0,
	); code != 503 ||
		resp["messageKey"] != "errors.demo_reset_offline" {
		t.Errorf("reset: %d %v", code, resp)
	}
}

// commandTrail lists the audit entries of a commands.create response's correlation ID, oldest first, as "result actor"
// with a final entry's reason (IR604).
func commandTrail(t *testing.T, srv *apiserver.Server, created map[string]any) []string {
	t.Helper()
	drainEvents(t)

	corr := asStr(t, asObj(t, created["meta"])["correlationId"])
	day := `"from":"` + clock().Add(-24*time.Hour).
		Format(time.RFC3339) +
		`","to":"` + clock().Add(24*time.Hour).
		Format(time.RFC3339) +
		`"`
	_, res := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"correlationId":"`+corr+`"},"limit":10}`,
	)
	out := []string{}

	for _, entry := range items(t, res) {
		if entry["action"] != opCommandsCreate {
			continue
		}

		line := asStr(t, entry["result"]) + " " +
			map[bool]string{
				true:  membershipCustomerB,
				false: asStr(t, entry["actorId"]),
			}[entry["actorId"] == seed.ID("user-customer-b").String()]
		if entry["result"] != statusPending {
			reason, _ := entry["reason"].(string)
			line += " " + reason
		}

		out = append([]string{line}, out...) // the list is newest first
	}

	return out
}
