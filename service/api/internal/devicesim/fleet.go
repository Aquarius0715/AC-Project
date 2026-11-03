package devicesim

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
)

//go:embed fleet.json
var fleetJSON []byte

// Pattern is one state an air conditioner and its IoT module can be in, and how the simulated module behaves in it.
type Pattern struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Room        string `json:"room"` // the room of the property the unit is in (Fleet.Floors)
	Connection  string `json:"connection"`
	PowerSignal string `json:"powerSignal"`
	Tamper      string `json:"tamper"`
	Firmware    string `json:"firmware"`
	Archived    bool   `json:"archived"`
	NoDevice    bool   `json:"noDevice"` // the unit has no IoT module
	State       struct {
		Power    *bool    `json:"power"`
		Celsius  *float64 `json:"celsius"`
		Mode     *string  `json:"mode"`
		FanLevel *string  `json:"fanLevel"`
	} `json:"state"`
	// Telemetry is what the module reports: normal readings, nothing (silent), or readings the API flags
	// (unit_mismatch, out_of_range, missing, invalid_time).
	Telemetry string `json:"telemetry"`
	// Commands is how the module answers: apply, reject (unsupported), ignore (never answers) or late (after expiry).
	Commands string `json:"commands"`
}

// Fleet is the mock fleet: one property of a customer with one unit per pattern, all of one model.
type Fleet struct {
	CustomerOrgID string `json:"customerOrgId"`
	Property      struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"property"`
	ModelID           string    `json:"modelId"`
	CapabilityVersion int       `json:"capabilityVersion"`
	Firmware          string    `json:"firmware"`
	Floors            []Floor   `json:"floors"`
	Patterns          []Pattern `json:"patterns"`
}

// Floor is a floor of the fleet's property and its rooms (assets spaces: floor, then room).
type Floor struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Rooms []Room `json:"rooms"`
}

// Room is a room on a floor.
type Room struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// telemetryKinds are the telemetry behaviours a pattern may name.
func telemetryKinds() map[string]bool {
	return map[string]bool{
		"":              true,
		"normal":        true,
		"silent":        true,
		"unit_mismatch": true,
		"out_of_range":  true,
		"missing":       true,
		"invalid_time":  true,
	}
}

// commandKinds are the command behaviours a pattern may name.
func commandKinds() map[string]bool {
	return map[string]bool{
		"":       true,
		"apply":  true,
		"reject": true,
		"ignore": true,
		"late":   true,
	}
}

// errBadFleet starts every refusal of a fleet definition.
var errBadFleet = errors.New("fleet")

// MockFleet reads the embedded fleet and checks it: unique keys, known behaviours, and a module for every pattern
// that reports or answers.
func MockFleet() (Fleet, error) { return parseFleet(fleetJSON) }

func parseFleet(raw []byte) (Fleet, error) {
	var fleet Fleet

	err := json.Unmarshal(raw, &fleet)
	if err != nil {
		return fleet, fmt.Errorf("fleet: %w", err)
	}

	rooms, err := fleetRooms(fleet.Floors)
	if err != nil {
		return fleet, err
	}

	seen := map[string]bool{}

	for _, pattern := range fleet.Patterns {
		err := checkPattern(pattern, seen, rooms)
		if err != nil {
			return fleet, err
		}

		seen[pattern.Key] = true
	}

	return fleet, nil
}

// fleetRooms are the room keys of the floors; a room key that is empty, repeated or its floor's own is refused.
func fleetRooms(floors []Floor) (map[string]bool, error) {
	rooms := map[string]bool{}

	for _, floor := range floors {
		for _, room := range floor.Rooms {
			if room.Key == "" || rooms[room.Key] || room.Key == floor.Key {
				return nil, fmt.Errorf(
					"%w: room key %q is empty or repeated",
					errBadFleet,
					room.Key,
				)
			}

			rooms[room.Key] = true
		}
	}

	return rooms, nil
}

// checkPattern refuses a pattern whose key is empty or already seen, whose behaviour is unknown, whose room is none of
// the fleet's rooms (when it has any), or which has a behaviour but no module.
func checkPattern(pattern Pattern, seen, rooms map[string]bool) error {
	switch {
	case pattern.Key == "" || seen[pattern.Key]:
		return fmt.Errorf("%w: pattern key %q is empty or repeated", errBadFleet, pattern.Key)
	case !telemetryKinds()[pattern.Telemetry] || !commandKinds()[pattern.Commands]:
		return fmt.Errorf(
			"%w: pattern %s has an unknown behaviour (%q, %q)",
			errBadFleet,
			pattern.Key,
			pattern.Telemetry,
			pattern.Commands,
		)
	case len(rooms) > 0 && !rooms[pattern.Room]:
		return fmt.Errorf(
			"%w: pattern %s is in no known room (%q)",
			errBadFleet,
			pattern.Key,
			pattern.Room,
		)
	case pattern.NoDevice && (pattern.Telemetry != "" || pattern.Commands != ""):
		return fmt.Errorf("%w: pattern %s has no module but a behaviour", errBadFleet, pattern.Key)
	}

	return nil
}

// FirmwareOf is the firmware a pattern's module reports (the fleet's unless the pattern names another).
func (f Fleet) FirmwareOf(p Pattern) string {
	if p.Firmware != "" {
		return p.Firmware
	}

	return f.Firmware
}

// UnitName is the fixture-style ID of a pattern's unit; the API IDs of a pattern's records come from seed.ID of these
// names.
func UnitName(p Pattern) string { return "unit-mock-" + p.Key }

// DeviceName is the fixture-style ID of a pattern's device.
func DeviceName(p Pattern) string { return "device-mock-" + p.Key }

// BindingName is the fixture-style ID of a pattern's binding.
func BindingName(p Pattern) string { return "binding-mock-" + p.Key }

// SensorName is the fixture-style ID of one sensor of a pattern's device.
func SensorName(p Pattern, s Sensor) string { return "sensor-mock-" + p.Key + "-" + s.Metric }

// SpaceName is the fixture-style ID of a space of the fleet's property: a floor or a room.
func SpaceName(key string) string { return "space-mock-" + key }
