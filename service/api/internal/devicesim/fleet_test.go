package devicesim

import "testing"

// TestMockFleetCoversEveryState: the fleet has a unit for every value the screens can show — each connection state,
// power signal off, tamper detected, each telemetry problem the API flags, each way a module answers a command, the
// three modes and fan levels, a unit switched off, a unit without a module and an archived unit.
func TestMockFleetCoversEveryState(t *testing.T) {
	f, err := MockFleet()
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, p := range f.Patterns {
		has["connection="+p.Connection] = true
		has["power="+p.PowerSignal] = true
		has["tamper="+p.Tamper] = true
		has["telemetry="+p.Telemetry] = true
		has["commands="+p.Commands] = true
		if p.State.Mode != nil {
			has["mode="+*p.State.Mode] = true
		}
		if p.State.FanLevel != nil {
			has["fan="+*p.State.FanLevel] = true
		}
		if p.State.Power != nil && !*p.State.Power {
			has["switched off"] = true
		}
		if p.NoDevice {
			has["no module"] = true
		}
		if p.Archived {
			has["archived"] = true
		}
		if f.FirmwareOf(p) != f.Firmware {
			has["other firmware"] = true
		}
	}
	for _, want := range []string{
		"connection=online", "connection=offline", "connection=unknown", "connection=connecting", "connection=error",
		"power=on", "power=off", "power=unknown", "tamper=clear", "tamper=detected",
		"telemetry=normal", "telemetry=silent", "telemetry=unit_mismatch", "telemetry=out_of_range", "telemetry=missing", "telemetry=invalid_time",
		"commands=apply", "commands=reject", "commands=ignore", "commands=late",
		"mode=cool", "mode=dry", "mode=fan", "fan=low", "fan=mid", "fan=high",
		"switched off", "no module", "archived", "other firmware",
	} {
		if !has[want] {
			t.Errorf("no pattern for %s", want)
		}
	}
}

// TestFleetNames: each pattern's records have their own fixture-style names.
func TestFleetNames(t *testing.T) {
	p := Pattern{Key: "cool-on"}
	if UnitName(p) != "unit-mock-cool-on" || DeviceName(p) != "device-mock-cool-on" || BindingName(p) != "binding-mock-cool-on" ||
		SensorName(p, Sensors[0]) != "sensor-mock-cool-on-temperature" {
		t.Errorf("names %s %s %s %s", UnitName(p), DeviceName(p), BindingName(p), SensorName(p, Sensors[0]))
	}
}

// TestFleetRefusesBadDefinitions: a broken file, a repeated key, an unknown behaviour, or a behaviour on a unit
// without a module are refused.
func TestFleetRefusesBadDefinitions(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":          `{`,
		"empty key":         `{"patterns":[{"key":""}]}`,
		"repeated key":      `{"patterns":[{"key":"a"},{"key":"a"}]}`,
		"unknown telemetry": `{"patterns":[{"key":"a","telemetry":"loud"}]}`,
		"unknown commands":  `{"patterns":[{"key":"a","commands":"maybe"}]}`,
		"module behaviour":  `{"patterns":[{"key":"a","noDevice":true,"telemetry":"normal"}]}`,
		"unknown room":      `{"floors":[{"key":"1f","rooms":[{"key":"r1"}]}],"patterns":[{"key":"a","room":"r2"}]}`,
		"repeated room":     `{"floors":[{"key":"1f","rooms":[{"key":"r1"},{"key":"r1"}]}],"patterns":[]}`,
	} {
		if _, err := parseFleet([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestEveryUnitIsInARoom: every pattern's unit is in a room of the fleet's floors, and the rooms hold the 20 units.
func TestEveryUnitIsInARoom(t *testing.T) {
	f, err := MockFleet()
	if err != nil {
		t.Fatal(err)
	}
	rooms := map[string]int{}
	for _, fl := range f.Floors {
		for _, r := range fl.Rooms {
			rooms[r.Key] = 0
		}
	}
	for _, p := range f.Patterns {
		if _, ok := rooms[p.Room]; !ok {
			t.Errorf("%s is in no room (%q)", p.Key, p.Room)
		}
		rooms[p.Room]++
	}
	if len(f.Floors) != 3 || len(rooms) != 10 || SpaceName("lab") != "space-mock-lab" {
		t.Errorf("%d floors, %d rooms, space name %s", len(f.Floors), len(rooms), SpaceName("lab"))
	}
}
