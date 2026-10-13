package devicesim

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed fleet.json
var fleetJSON []byte

// Pattern is one state an air conditioner and its IoT module can be in, and how the simulated module behaves in it.
type Pattern struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
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
	CustomerOrgID     string                          `json:"customerOrgId"`
	Property          struct{ ID, Kind, Name string } `json:"property"`
	ModelID           string                          `json:"modelId"`
	CapabilityVersion int                             `json:"capabilityVersion"`
	Firmware          string                          `json:"firmware"`
	Patterns          []Pattern                       `json:"patterns"`
}

// Telemetry and command behaviours a pattern may name.
var (
	telemetryKinds = map[string]bool{"": true, "normal": true, "silent": true, "unit_mismatch": true, "out_of_range": true, "missing": true, "invalid_time": true}
	commandKinds   = map[string]bool{"": true, "apply": true, "reject": true, "ignore": true, "late": true}
)

// MockFleet reads the embedded fleet and checks it: unique keys, known behaviours, and a module for every pattern
// that reports or answers.
func MockFleet() (Fleet, error) { return parseFleet(fleetJSON) }

func parseFleet(raw []byte) (Fleet, error) {
	var f Fleet
	if err := json.Unmarshal(raw, &f); err != nil {
		return f, fmt.Errorf("fleet: %w", err)
	}
	seen := map[string]bool{}
	for _, p := range f.Patterns {
		switch {
		case p.Key == "" || seen[p.Key]:
			return f, fmt.Errorf("fleet: pattern key %q is empty or repeated", p.Key)
		case !telemetryKinds[p.Telemetry] || !commandKinds[p.Commands]:
			return f, fmt.Errorf("fleet: pattern %s has an unknown behaviour (%q, %q)", p.Key, p.Telemetry, p.Commands)
		case p.NoDevice && (p.Telemetry != "" || p.Commands != ""):
			return f, fmt.Errorf("fleet: pattern %s has no module but a behaviour", p.Key)
		}
		seen[p.Key] = true
	}
	return f, nil
}

// FirmwareOf is the firmware a pattern's module reports (the fleet's unless the pattern names another).
func (f Fleet) FirmwareOf(p Pattern) string {
	if p.Firmware != "" {
		return p.Firmware
	}
	return f.Firmware
}

// Fixture-style IDs of a pattern's records; the API IDs come from seed.ID of these names.
func UnitName(p Pattern) string             { return "unit-mock-" + p.Key }
func DeviceName(p Pattern) string           { return "device-mock-" + p.Key }
func BindingName(p Pattern) string          { return "binding-mock-" + p.Key }
func SensorName(p Pattern, s Sensor) string { return "sensor-mock-" + p.Key + "-" + s.Metric }
