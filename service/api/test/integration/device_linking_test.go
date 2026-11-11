package integration_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// An unplanned device (no unit) registers, a replacement takes a unit over from the device on it in one write, and the
// lists find devices and units by their related names.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDeviceUnitLinking(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Link AC")

	register := func(serial string) (string, int) {
		code, resp := write(
			srv, hq(), "devices.register",
			`{"serial":"`+serial+`-`+uuid.NewString()[:6]+`","sensorTypes":[]}`, 0,
		)
		if code != 200 {
			t.Fatalf("register without unit: %d %v", code, resp)
		}

		d := data(resp)
		if d["targetUnitId"] != nil || d["unitId"] != nil {
			t.Fatalf("unplanned device has a unit: %v", d)
		}

		return asStr(t, d["id"]), int(as[float64](t, d["version"]))
	}

	first, firstVersion := register("SPARE")
	second, secondVersion := register("SPARE")

	// bound=false lists both, bound=true neither
	_, resp := post(srv, hq(), "devices.list", `{"limit":100,"filters":{"bound":false}}`)
	if !containsID(items(t, resp), first) || !containsID(items(t, resp), second) {
		t.Fatalf("unbound filter misses the spare devices: %v", resp)
	}

	// technicians cannot register an unplanned device
	code, reply := write(
		srv,
		techInt(),
		"devices.register",
		`{"serial":"TECH-`+uuid.NewString()[:6]+`","sensorTypes":[]}`,
		0,
	)
	if code == 200 {
		t.Fatalf("technician registered an unplanned device: %v", reply)
	}

	if code, reply = write(
		srv,
		hq(),
		"devices.bind",
		`{"deviceId":"`+first+`","unitId":"`+unit+`","reason":"install"}`,
		firstVersion,
	); code != 200 {
		t.Fatalf("bind: %d %v", code, reply)
	}

	// a second device is refused while the unit has one, and nothing changes
	code, reply = write(
		srv,
		hq(),
		"devices.bind",
		`{"deviceId":"`+second+`","unitId":"`+unit+`","reason":"x"}`,
		secondVersion,
	)
	if code != 409 {
		t.Fatalf("second device on a taken unit: %d %v", code, reply)
	}

	// replacing a device that is not on the unit is refused
	code, reply = write(
		srv,
		hq(),
		"devices.bind",
		`{"deviceId":"`+second+`","unitId":"`+unit+`","replaceDeviceId":"`+second+`","reason":"x"}`,
		secondVersion,
	)
	if code != 422 {
		t.Fatalf("replace itself: %d %v", code, reply)
	}

	code, reply = write(
		srv,
		hq(),
		"devices.bind",
		`{"deviceId":"`+second+`","unitId":"`+unit+`","replaceDeviceId":"`+first+`","reason":"swap"}`,
		secondVersion,
	)
	if code != 200 {
		t.Fatalf("replace: %d %v", code, reply)
	}

	if as[string](t, data(reply)["unitId"]) != unit {
		t.Fatalf("replacement is not on the unit: %v", data(reply))
	}

	_, resp = post(srv, hq(), "devices.get", `{"id":"`+first+`"}`)
	if data(resp)["unitId"] != nil {
		t.Fatalf("replaced device is still on the unit: %v", resp)
	}

	// search: by the unit's customer, and the unit by its device's serial
	_, resp = post(
		srv,
		hq(),
		"devices.list",
		`{"limit":100,"filters":{"unitId":"`+unit+`","search":"Demo Customer B"}}`,
	)
	if !containsID(items(t, resp), second) {
		t.Fatalf("search by customer misses the device: %v", resp)
	}

	_, resp = post(srv, hq(), "units.list", `{"limit":100,"filters":{"withoutDevice":true}}`)
	if containsID(items(t, resp), unit) {
		t.Fatalf("a unit with a device is a connection candidate: %v", resp)
	}

	_, resp = post(srv, hq(), "units.list", `{"limit":100,"filters":{"search":"SPARE"}}`)
	if !containsID(items(t, resp), unit) {
		t.Fatalf("unit search by device serial: %v", resp)
	}
}

// A device registered without sensorTypes starts with the sensors of its IoT model.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDeviceDefaultSensors(t *testing.T) {
	srv := server(t)

	code, resp := write(
		srv,
		hq(),
		"devices.register",
		`{"serial":"DEF-`+uuid.NewString()[:6]+`","iotModelId":"`+seed.ID("ventilation-demo").
			String()+
			`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("register: %d %v", code, resp)
	}

	if len(asArr(t, data(resp)["sensors"])) == 0 {
		t.Fatalf("no sensors from the IoT model: %v", data(resp))
	}
}

func containsID(rows []map[string]any, id string) bool {
	for _, r := range rows {
		if r["id"] == id {
			return true
		}
	}

	return false
}
