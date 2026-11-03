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

// TestConnectionFaultForTheGateway: the IoT gateway reads a device's connection axis with the internal service token
// (devices.connectionFault, IR532): no open fault at first; after a communication_lost the fault's DeviceEvent ID and
// sequence, which a restored names to bring the device back online; after it no open fault and the newer sequence.
// A request without the token, without a device or for an unknown one is refused.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestConnectionFaultForTheGateway(t *testing.T) {
	srv := serverWith(t, true)
	cluster := newCluster(t)
	unit := boundUnit(t, srv, "online")

	var device, binding string
	ownerScan(
		t,
		`SELECT id::text, binding_id::text FROM devices.devices WHERE unit_id = $1`,
		[]any{unit},
		&device,
		&binding,
	)

	_, sess := post(
		srv,
		customerB(),
		"session.get",
		`{}`,
	) // the business time of the server (a shared demo cluster may run ahead of clock)
	now, _ := time.Parse(time.RFC3339Nano, asStr(t, asObj(t, sess["meta"])["snapshotAt"]))

	ask := func(token, input string) (int, map[string]any) {
		req, _ := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			cluster.urls[ops.DomainEquipment]+"/internal/v1/system/queries/devices.connectionFault",
			strings.NewReader(input),
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
			Data map[string]any `json:"data"`
		}

		_ = json.NewDecoder(resp.Body).Decode(&body)

		return resp.StatusCode, body.Data
	}
	fault := func() map[string]any {
		t.Helper()

		code, f := ask("test-internal", `{"deviceId":"`+device+`"}`)
		if code != 200 {
			t.Fatalf("connection fault: %d", code)
		}

		return f
	}
	trigger := func(body string) {
		t.Helper()

		if code, res := write(
			srv,
			customerB(),
			"demo.trigger",
			`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+now.Format(
				time.RFC3339,
			)+`","eventType":"device",`+
				`"deviceId":"`+device+`","bindingId":"`+binding+`",`+body+`}`,
			0,
		); code != 200 {
			t.Fatalf("device trigger %s: %d %v", body, code, res)
		}
	}
	connection := func() string {
		var c string
		ownerScan(t, `SELECT connection FROM devices.devices WHERE id = $1`, []any{device}, &c)

		return c
	}

	if f := fault(); f["bindingId"] != binding || f["faultId"] != nil ||
		asNum(t, f["sequence"]) != 0 {
		t.Fatalf("a device without faults: %v", f)
	}

	trigger(`"sequence":10,"kind":"communication_lost"`)

	f := fault()

	id, _ := f["faultId"].(string)
	if id == "" || asNum(t, f["sequence"]) != 10 || connection() != connectionOffline {
		t.Fatalf("after the loss: %v, connection %s", f, connection())
	}

	trigger(
		`"sequence":11,"kind":"restored","recovery":{"axis":"connection","sourceEventId":"` + id + `","value":"online"}`,
	)

	if f := fault(); f["faultId"] != nil || asNum(t, f["sequence"]) != 11 ||
		connection() != connectionOnline {
		t.Fatalf("after the recovery: %v, connection %s", f, connection())
	}

	if code, _ := ask("", `{"deviceId":"`+device+`"}`); code != 401 {
		t.Errorf("without the internal token: %d", code)
	}

	if code, _ := ask("test-internal", `{}`); code != 422 {
		t.Errorf("without a device: %d", code)
	}

	if code, _ := ask("test-internal", `{"deviceId":"`+uuid.NewString()+`"}`); code != 404 {
		t.Errorf("an unknown device: %d", code)
	}
}
