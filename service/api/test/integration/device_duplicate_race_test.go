package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// A serial or an MQTT node_id that another device takes at the same moment is refused with the specific conflict the
// check before the write answers (error.duplicateSerial, error.duplicateMqttNode), not with the generic CONFLICT: the
// check does not see a device that is not committed yet, so the unique index (devices_serial, devices_mqtt_node)
// refuses the write when the other one commits.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDeviceDuplicateTakenAtTheSameMoment(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	unit := newUnit(t, srv, "Dup AC")

	// a device of the tenant, for the tenant and the registering membership to copy
	code, resp := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"DUP-A-`+suffix+`","sensorTypes":[],"unitId":"`+unit+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("device: %d %v", code, resp)
	}

	first, firstVersion := asStr(t, data(resp)["id"]), ver(t, resp)

	t.Cleanup(func() {
		owner(
			t,
			`DELETE FROM devices.devices WHERE serial LIKE 'DUP-%' AND serial LIKE '%'||$1`,
			suffix,
		)
	})

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	for name, testCase := range map[string]struct {
		serial, node string
		send         func() (int, map[string]any)
		want         string
	}{
		"register with the same serial": {
			"DUP-B-" + suffix, "dupb" + suffix,
			func() (int, map[string]any) {
				return write(
					srv,
					hq(),
					"devices.register",
					`{"serial":"DUP-B-`+suffix+`","sensorTypes":[],"unitId":"`+unit+`"}`,
					0,
				)
			},
			apperr.KeyDuplicateSerial,
		},
		"register with the same node": {
			"DUP-C-" + suffix, "dupc" + suffix,
			func() (int, map[string]any) {
				return write(
					srv,
					hq(),
					"devices.register",
					`{"serial":"DUP-D-`+suffix+`","sensorTypes":[],"unitId":"`+unit+`","mqttNode":"dupc`+suffix+`"}`,
					0,
				)
			},
			errDuplicateMqttNodeKey,
		},
		"set the same node": {
			"DUP-E-" + suffix, "dupe" + suffix,
			func() (int, map[string]any) {
				return write(
					srv,
					hq(),
					"devices.setMqttNode",
					`{"deviceId":"`+first+`","mqttNode":"dupe`+suffix+`"}`,
					firstVersion,
				)
			},
			errDuplicateMqttNodeKey,
		},
	} {
		// the other registration, saved at the same moment: inserted, not committed
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}

		_, err = tx.Exec(
			ctx,
			`INSERT INTO devices.devices (tenant_id, serial, mqtt_node, target_unit_id, created_by_membership_id,
				firmware_version) SELECT tenant_id, $2, $3, target_unit_id, created_by_membership_id, 'v1'
				FROM devices.devices WHERE id = $1`,
			first,
			testCase.serial,
			testCase.node,
		)
		if err != nil {
			_ = tx.Rollback(ctx)

			t.Fatalf("%s: %v", name, err)
		}

		done := make(chan saveResult, 1)

		go func() {
			status, reply := testCase.send()
			done <- saveResult{status, reply}
		}()

		select {
		case got := <-done:
			_ = tx.Rollback(ctx)

			t.Fatalf("%s: did not wait for the other device: %d %v", name, got.code, got.resp)
		case <-time.After(700 * time.Millisecond):
		}

		err = tx.Commit(ctx)
		if err != nil {
			t.Fatal(err)
		}

		got := <-done
		if got.code != 409 || got.resp["messageKey"] != testCase.want {
			t.Errorf("%s: %d %v, want 409 %s", name, got.code, got.resp, testCase.want)
		}
	}
}

// errDuplicateMqttNodeKey is the message key of a node_id another device has (devices.errDuplicateMqttNode).
const errDuplicateMqttNodeKey = "error.duplicateMqttNode"
