package integration_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A device that is in use is not deleted (DEC-103): it is unbound first, then deleted with its own history. HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDevicesUnbindAndDelete(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Del AC")

	code, resp := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"DEL-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+unit+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("register: %d %v", code, resp)
	}

	dev := asStr(t, data(resp)["id"])
	unbind := func(a *actor, version int) (int, map[string]any) {
		return write(
			srv,
			a,
			"devices.unbind",
			`{"deviceId":"`+dev+`","reason":"moved out"}`,
			version,
		)
	}
	remove := func(a *actor, version int) (int, map[string]any) {
		return write(
			srv,
			a,
			"devices.delete",
			`{"deviceId":"`+dev+`","reason":"registered by mistake"}`,
			version,
		)
	}

	if code, _ := unbind(hq(), 1); code != 409 {
		t.Error("unbind of a device that is not bound")
	}

	_, resp = write(
		srv,
		hq(),
		"devices.bind",
		`{"deviceId":"`+dev+`","unitId":"`+unit+`","reason":"installed"}`,
		1,
	)
	version := ver(t, resp)

	if code, res := remove(hq(), version); code != 409 || res["messageKey"] != "error.deviceBound" {
		t.Errorf("delete of a bound device: %d %v", code, res["messageKey"])
	}

	for name, who := range map[string]*actor{roleClient: customerB(), roleTechnician: techInt()} {
		if code, _ := unbind(who, version); code == 200 {
			t.Errorf("%s unbinds", name)
		}

		if code, _ := remove(who, version); code == 200 {
			t.Errorf("%s deletes", name)
		}
	}

	if code, res := write(
		srv,
		hq(),
		"devices.unbind",
		`{"deviceId":"`+dev+`","reason":""}`,
		version,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("unbind needs a reason: %d %s", code, fields(res))
	}

	if code, _ := unbind(hq(), version+5); code != 409 {
		t.Error("unbind at a stale version")
	}

	code, resp = unbind(hq(), version)
	if code != 200 || data(resp)["unitId"] != nil || data(resp)["bindingId"] != nil ||
		ver(t, resp) != version+1 {
		t.Fatalf("unbind: %d %v", code, resp)
	}

	if code, _ := remove(hq(), version); code != 409 {
		t.Error("delete at a stale version")
	}

	code, resp = remove(hq(), version+1)
	if code != 200 || data(resp)["deleted"] != true || data(resp)["id"] != dev {
		t.Fatalf("delete: %d %v", code, resp)
	}

	if code, _ := post(srv, hq(), "devices.get", `{"id":"`+dev+`"}`); code != 404 {
		t.Error("a deleted device is gone")
	}

	if code, _ := remove(hq(), version+1); code != 404 {
		t.Error("a device is deleted once")
	}

	var left int

	ownerScan(
		t,
		`SELECT (SELECT count(*) FROM devices.sensors WHERE device_id = $1) + (SELECT count(*) FROM devices.device_bindings
			WHERE device_id = $1)`,
		[]any{dev},
		&left,
	)

	if left != 0 {
		t.Errorf("the device's own records stay: %d", left)
	}

	// the unit is free for another device
	if code, _ := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"DEL-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+unit+`"}`,
		0,
	); code != 200 {
		t.Error("register again on the unit")
	}
}

// An unbound device with a live command is not deleted (error.deviceBusy), nor one still pending in a firmware campaign
// that has not ended (error.deviceInCampaign); an expired command and an aborted campaign no longer hold it.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDeviceDeleteInUse(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Busy AC")

	code, resp := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"BUSY-`+uuid.NewString()[:8]+`","sensorTypes":[],"unitId":"`+unit+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("register: %d %v", code, resp)
	}

	dev := asStr(t, data(resp)["id"])
	remove := func() (int, map[string]any) {
		return write(
			srv,
			hq(),
			"devices.delete",
			`{"deviceId":"`+dev+`","reason":"registered by mistake"}`,
			1,
		)
	}

	operation, campaign := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, expires_at) VALUES ($1,$2,$3,
			$4,'check','queued','2100-01-01T00:00:00Z')`,
		operation,
		seed.ID("tenant-a"),
		dev,
		unit,
	)

	if code, res := remove(); code != 409 || res["messageKey"] != "error.deviceBusy" {
		t.Errorf("delete with a queued command: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE devices.device_operations SET expires_at = '2000-01-01T00:00:00Z' WHERE id = $1`,
		operation,
	)
	owner(
		t,
		`INSERT INTO devices.firmware_campaigns (id, tenant_id, model_id, from_versions, target_version, checksum, waves,
			window_start_local, window_end_local, auto_pause_failure_percent, start_at, state) VALUES ($1,$2,$3,'{1.0}','v2',
			'sha256:x','[{"label":"All","percent":100}]','00:00','23:59',5,'2100-01-01T00:00:00Z','scheduled')`,
		campaign,
		seed.ID("tenant-a"),
		seed.ID("ventilation-demo"),
	)
	t.Cleanup(func() {
		owner(t, `UPDATE devices.firmware_campaigns SET state = 'aborted' WHERE id = $1`, campaign)
	})
	owner(
		t,
		`INSERT INTO devices.firmware_campaign_devices (campaign_id, device_id, tenant_id, wave, result) VALUES
			($1,$2,$3,0,'pending')`,
		campaign,
		dev,
		seed.ID("tenant-a"),
	)

	if code, res := remove(); code != 409 || res["messageKey"] != "error.deviceInCampaign" {
		t.Errorf("delete while pending in a scheduled campaign: %d %v", code, res)
	}

	owner(t, `UPDATE devices.firmware_campaigns SET state = 'aborted' WHERE id = $1`, campaign)

	if code, res := remove(); code != 200 {
		t.Errorf("delete after the command expired and the campaign was aborted: %d %v", code, res)
	}
}

// A model that units use is not deleted; a model nobody uses is, with every version (DEC-103). HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCapabilitiesDelete(t *testing.T) {
	srv := server(t)
	model := "TD-" + uuid.NewString()[:6]

	code, resp := write(srv, hq(), "capabilities.save", capBody("", model, ""), 0)
	if code != 200 {
		t.Fatalf("create: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])
	remove := func(a *actor, id string, version int) (int, map[string]any) {
		return write(
			srv,
			a,
			"capabilities.delete",
			`{"id":"`+id+`","reason":"never used"}`,
			version,
		)
	}

	if code, _ := remove(customerA(), id, 1); code == 200 {
		t.Error("a client deletes a model")
	}

	if code, _ := remove(hq(), id, 7); code != 409 {
		t.Error("delete at a stale version")
	}

	// a model with a unit on it (a unit this test made)
	unit := newUnit(t, srv, "Model AC")
	owner(
		t,
		`UPDATE assets.units SET model_id = $1, capability_version = 1 WHERE id = $2`,
		id,
		unit,
	)

	if code, res := remove(hq(), id, 1); code != 409 || res["messageKey"] != "error.modelInUse" {
		t.Errorf("delete of a model a unit uses: %d %v", code, res["messageKey"])
	}

	owner(
		t,
		`UPDATE assets.units SET model_id = $2, capability_version = 1 WHERE id = $1`,
		unit,
		seed.ID("ventilation-demo"),
	)

	code, resp = remove(hq(), id, 1)
	if code != 200 || data(resp)["deleted"] != true {
		t.Fatalf("delete: %d %v", code, resp)
	}

	if code, _ := remove(hq(), id, 1); code != 404 {
		t.Error("a model is deleted once")
	}

	_, list := post(srv, hq(), "capabilities.list", `{"limit":100}`)
	for _, it := range items(t, list) {
		if it["id"] == id {
			t.Error("the deleted model is still listed")
		}
	}
}
