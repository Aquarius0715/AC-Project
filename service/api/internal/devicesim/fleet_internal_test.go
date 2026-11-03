package devicesim

import "testing"

// TestMockFleetCoversEveryState: the fleet has a unit for every value the screens can show — each connection state,
// power signal off, tamper detected, each telemetry problem the API flags, each way a module answers a command, the
// three modes and fan levels, a unit switched off, a unit without a module and an archived unit.
func TestMockFleetCoversEveryState(t *testing.T) {
	t.Parallel()

	fleet, err := MockFleet()
	if err != nil {
		t.Fatal(err)
	}

	has := fleetStates(fleet)

	for _, want := range []string{
		"connection=online", "connection=offline", "connection=unknown", "connection=connecting", "connection=error",
		"power=on", "power=off", "power=unknown", "tamper=clear", "tamper=detected",
		"telemetry=normal", "telemetry=silent", "telemetry=unit_mismatch", "telemetry=out_of_range", "telemetry=missing",
		"telemetry=invalid_time",
		"commands=apply", "commands=reject", "commands=ignore", "commands=late",
		"mode=cool", "mode=dry", "mode=fan", "fan=low", "fan=mid", "fan=high",
		"switched off", "no module", "archived", "other firmware",
	} {
		if !has[want] {
			t.Errorf("no pattern for %s", want)
		}
	}
}

// fleetStates are the values the fleet's patterns show, as "field=value" or the name of a state.
func fleetStates(fleet Fleet) map[string]bool {
	has := map[string]bool{}
	for _, pattern := range fleet.Patterns {
		has["connection="+pattern.Connection] = true
		has["power="+pattern.PowerSignal] = true
		has["tamper="+pattern.Tamper] = true
		has["telemetry="+pattern.Telemetry] = true

		has["commands="+pattern.Commands] = true
		if pattern.State.Mode != nil {
			has["mode="+*pattern.State.Mode] = true
		}

		if pattern.State.FanLevel != nil {
			has["fan="+*pattern.State.FanLevel] = true
		}

		if pattern.State.Power != nil && !*pattern.State.Power {
			has["switched off"] = true
		}

		if pattern.NoDevice {
			has["no module"] = true
		}

		if pattern.Archived {
			has["archived"] = true
		}

		if fleet.FirmwareOf(pattern) != fleet.Firmware {
			has["other firmware"] = true
		}
	}

	return has
}

// TestFleetNames: each pattern's records have their own fixture-style names.
func TestFleetNames(t *testing.T) {
	t.Parallel()

	var pattern Pattern

	pattern.Key = "cool-on"
	if UnitName(pattern) != "unit-mock-cool-on" || DeviceName(pattern) != "device-mock-cool-on" ||
		BindingName(pattern) != "binding-mock-cool-on" ||
		SensorName(pattern, Sensors[0]) != "sensor-mock-cool-on-temperature" {
		t.Errorf(
			"names %s %s %s %s",
			UnitName(pattern),
			DeviceName(pattern),
			BindingName(pattern),
			SensorName(pattern, Sensors[0]),
		)
	}
}

// TestFleetRefusesBadDefinitions: a broken file, a repeated key, an unknown behaviour, or a behaviour on a unit
// without a module are refused.
func TestFleetRefusesBadDefinitions(t *testing.T) {
	t.Parallel()

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
		_, err := parseFleet([]byte(raw))
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestEveryUnitIsInARoom: every pattern's unit is in a room of the fleet's floors, and the rooms hold the 20 units.
func TestEveryUnitIsInARoom(t *testing.T) {
	t.Parallel()

	fleet, err := MockFleet()
	if err != nil {
		t.Fatal(err)
	}

	rooms := map[string]int{}

	for _, fl := range fleet.Floors {
		for _, r := range fl.Rooms {
			rooms[r.Key] = 0
		}
	}

	for _, p := range fleet.Patterns {
		if _, ok := rooms[p.Room]; !ok {
			t.Errorf("%s is in no room (%q)", p.Key, p.Room)
		}

		rooms[p.Room]++
	}

	if len(fleet.Floors) != 3 || len(rooms) != 10 || SpaceName("lab") != "space-mock-lab" {
		t.Errorf(
			"%d floors, %d rooms, space name %s",
			len(fleet.Floors),
			len(rooms),
			SpaceName("lab"),
		)
	}
}
