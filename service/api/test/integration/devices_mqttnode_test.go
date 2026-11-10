package integration_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestDeviceMqttNode: a device is registered with the node_id its ESP32 sends (IR635) — trimmed, exact, optional;
// a malformed one is refused with its field; another device cannot take it; devices.setMqttNode changes or clears it
// with the expected version and the register keeps the write in the audit.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDeviceMqttNode(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Node AC")
	node := "Node_" + uuid.NewString()[:8]
	register := func(serial, unitID, mqtt string) (int, map[string]any) {
		body := `{"serial":"` + serial + `","sensorTypes":["temperature"],"unitId":"` + unitID + `"`
		if mqtt != "" {
			body += `,"mqttNode":` + mqtt
		}

		return write(srv, hq(), "devices.register", body+`}`, 0)
	}

	code, resp := register("NODE-"+strings.ToUpper(uuid.NewString()[:8]), unit, `"  `+node+` "`)
	if code != 200 || data(resp)["mqttNode"] != node {
		t.Fatalf("register with a node: %d %v", code, resp)
	}

	dev := asStr(t, data(resp)["id"])

	_, got := post(srv, hq(), "devices.get", `{"id":"`+dev+`"}`)
	if data(got)["mqttNode"] != node {
		t.Errorf("devices.get: %v", data(got)["mqttNode"])
	}

	other := newUnit(t, srv, "Node AC 2")

	if code, res := register(
		"NODE-"+strings.ToUpper(uuid.NewString()[:8]),
		other,
		`"`+node+`"`,
	); code != 409 {
		t.Errorf("a node_id another device has: %d %v", code, res)
	}

	for _, bad := range []string{`"node 1"`, `"node/1"`, `"` + strings.Repeat("n", 65) + `"`, `"ノード"`} {
		code, res := register("BAD-"+strings.ToUpper(uuid.NewString()[:8]), other, bad)
		refusal(t, bad, code, res, "mqttNode=error.mqttNodeFormat")
	}

	code, resp = register("NONE-"+strings.ToUpper(uuid.NewString()[:8]), other, `""`)
	if code != 200 || data(resp)["mqttNode"] != nil {
		t.Fatalf("an empty node_id is none: %d %v", code, resp)
	}

	free := asStr(t, data(resp)["id"])

	// set, move, clear
	renamed := "Renamed_" + uuid.NewString()[:8]

	code, resp = write(
		srv,
		hq(),
		"devices.setMqttNode",
		`{"deviceId":"`+dev+`","mqttNode":"`+renamed+`"}`,
		1,
	)
	if code != 200 || data(resp)["mqttNode"] != renamed || asNum(t, data(resp)["version"]) != 2 {
		t.Fatalf("set: %d %v", code, resp)
	}

	if code, _ := write(
		srv,
		hq(),
		"devices.setMqttNode",
		`{"deviceId":"`+free+`","mqttNode":"`+renamed+`"}`,
		1,
	); code != 409 {
		t.Errorf("a node_id another device has: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"devices.setMqttNode",
		`{"deviceId":"`+dev+`","mqttNode":"x"}`,
		1,
	); code != 409 {
		t.Errorf("a stale version: %d", code)
	}

	code, res := write(
		srv,
		hq(),
		"devices.setMqttNode",
		`{"deviceId":"`+dev+`","mqttNode":"bad node"}`,
		2,
	)
	refusal(t, "bad node", code, res, "mqttNode=error.mqttNodeFormat")

	code, resp = write(
		srv,
		hq(),
		"devices.setMqttNode",
		`{"deviceId":"`+dev+`","mqttNode":null}`,
		2,
	)
	if code != 200 || data(resp)["mqttNode"] != nil || asNum(t, data(resp)["version"]) != 3 {
		t.Fatalf("clear: %d %v", code, resp)
	}

	// the cleared node_id is free again
	if code, _ := write(
		srv,
		hq(),
		"devices.setMqttNode",
		`{"deviceId":"`+free+`","mqttNode":"`+node+`"}`,
		1,
	); code != 200 {
		t.Errorf("a cleared node_id was not free: %d", code)
	}

	var audits int

	ownerScan(
		t,
		`SELECT count(*) FROM audit.audit_log WHERE action = 'devices.setMqttNode' AND target_id = $1`,
		[]any{dev},
		&audits,
	)

	if audits != 2 {
		t.Errorf("%d audit rows for the device, want 2", audits)
	}

	// only HQ writes it
	if code, _ := write(
		srv,
		customerB(),
		"devices.setMqttNode",
		`{"deviceId":"`+dev+`","mqttNode":"z"}`,
		3,
	); code != 403 {
		t.Errorf("customer-b: %d", code)
	}
}

// TestMqttNodesForTheGateway: the IoT gateway reads the registered nodes with the internal service token
// (devices.mqttNodes, IR635): a bound device with a node_id is listed with its unit, whether its model takes
// commands and its sensors; an unbound device and a device without a node_id are not.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMqttNodesForTheGateway(t *testing.T) {
	srv := serverWith(t, true)
	cluster := newCluster(t)
	unit := boundUnit(t, srv, "online")
	node := "Gate_" + uuid.NewString()[:8]

	var device string
	ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{unit}, &device)
	owner(t, `UPDATE devices.devices SET mqtt_node = $2 WHERE id = $1`, device, node)
	owner(
		t,
		`INSERT INTO devices.sensors (tenant_id, device_id, metric, unit) VALUES ($1, $2, 'temperature', '°C')`,
		seed.ID("tenant-a"),
		device,
	)

	ask := func(token string) (int, []any) {
		req, _ := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			cluster.urls[ops.DomainEquipment]+"/internal/v1/system/queries/devices.mqttNodes",
			strings.NewReader(`{}`),
		)
		req.Header.Set("Content-Type", "application/json")

		if token != "" {
			req.Header.Set(ops.InternalTokenHeader, token)
		}

		req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		var body struct {
			Data struct {
				Nodes []any `json:"nodes"`
			} `json:"data"`
		}

		_ = json.NewDecoder(resp.Body).Decode(&body)

		return resp.StatusCode, body.Data.Nodes
	}

	code, nodes := ask("test-internal")
	if code != 200 {
		t.Fatalf("mqttNodes: %d", code)
	}

	var found map[string]any

	for _, n := range nodes {
		if m := asObj(t, n); m["node"] == node {
			found = m
		}
	}

	if found == nil || found["deviceId"] != device || found["unitId"] != unit ||
		len(asArr(t, found["sensors"])) != 1 {
		t.Fatalf("the registered node is missing or wrong: %v", nodes)
	}

	owner(t, `UPDATE devices.devices SET binding_id = NULL, unit_id = NULL WHERE id = $1`, device)

	_, nodes = ask("test-internal")
	for _, n := range nodes {
		if asObj(t, n)["node"] == node {
			t.Error("an unbound device is listed")
		}
	}

	if code, _ := ask(""); code != 401 {
		t.Errorf("without the internal token: %d", code)
	}
}

// TestMqttChecksForTheGateway: the IoT gateway reads the running connection checks of the devices that have a node_id
// (devices.mqttChecks, IR736), and its answer (demo.trigger operation succeeded) brings a device a timed-out check left
// in error back online. A queued check, a check of a device without a node_id and a finished check are not listed;
// without the internal token the read is refused.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMqttChecksForTheGateway(t *testing.T) {
	srv := serverWith(t, true)
	cluster := newCluster(t)
	unit, other := boundUnit(t, srv, "error"), boundUnit(t, srv, "online")
	node := "Check_" + uuid.NewString()[:8]

	var device, otherDevice string
	ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{unit}, &device)
	ownerScan(
		t,
		`SELECT id::text FROM devices.devices WHERE unit_id = $1`,
		[]any{other},
		&otherDevice,
	)
	owner(t, `UPDATE devices.devices SET mqtt_node = $2 WHERE id = $1`, device, node)

	running, queued, noNode := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, operation := range []struct{ id, device, unit, status string }{
		{running, device, unit, "running"}, {queued, device, unit, "queued"}, {noNode, otherDevice, other, "running"},
	} {
		owner(
			t,
			`INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, expires_at)
			VALUES ($1,$2,$3,$4,'check',$5, now() + interval '1 day')`,
			operation.id,
			seed.ID("tenant-a"),
			operation.device,
			operation.unit,
			operation.status,
		)
	}

	ask := func(token string) (int, []map[string]any) {
		req, _ := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			cluster.urls[ops.DomainEquipment]+"/internal/v1/system/queries/devices.mqttChecks",
			strings.NewReader(`{}`),
		)
		req.Header.Set("Content-Type", "application/json")

		if token != "" {
			req.Header.Set(ops.InternalTokenHeader, token)
		}

		req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()

		var body struct {
			Data struct {
				Checks []map[string]any `json:"checks"`
			} `json:"data"`
		}

		_ = json.NewDecoder(resp.Body).Decode(&body)

		return resp.StatusCode, body.Data.Checks
	}

	mine := func(checks []map[string]any) []map[string]any {
		var out []map[string]any

		for _, check := range checks {
			if check["deviceId"] == device || check["deviceId"] == otherDevice {
				out = append(out, check)
			}
		}

		return out
	}

	code, checks := ask("test-internal")
	if got := mine(checks); code != 200 || len(got) != 1 || got[0]["operationId"] != running ||
		got[0]["node"] != node {
		t.Fatalf("mqttChecks: %d %v", code, got)
	}

	var appNow time.Time
	ownerScan(t, `SELECT platform.app_now()`, nil, &appNow)

	if code, resp := write(
		srv,
		customerB(),
		"demo.trigger",
		`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+appNow.Add(-time.Second).
			Format(time.RFC3339Nano)+
			`","eventType":"operation","operationId":"`+running+`","result":"succeeded"}`,
		0,
	); code != 200 {
		t.Fatalf("the gateway's answer: %d %v", code, resp)
	}

	var devConn, unitConn string
	ownerScan(
		t,
		`SELECT d.connection, u.connection FROM devices.devices d JOIN assets.units u ON u.id = d.unit_id
		WHERE d.id = $1`,
		[]any{device},
		&devConn,
		&unitConn,
	)

	if devConn != "online" || unitConn != "online" {
		t.Errorf("after the answered check the device is %s, its unit %s", devConn, unitConn)
	}

	if _, checks := ask("test-internal"); len(mine(checks)) != 0 {
		t.Errorf("a finished check is still listed: %v", mine(checks))
	}

	if code, _ := ask(""); code != 401 {
		t.Errorf("without the internal token: %d", code)
	}
}
