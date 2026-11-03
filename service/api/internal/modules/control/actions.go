package control

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
)

// Caps is the part of a unit's Capability that decides which UnitActions it accepts.
type Caps struct {
	Control           bool
	ModeControl       bool
	FanControl        bool
	TempMin, TempMax  float64
	TempStep          float64
	HasTemperature    bool
	Modes             []string
	FanLevels         []string
	Ventilation       bool
	VentilationLevels []string
}

// UnitAction is UnitAction of service-contracts.ts.
type UnitAction struct {
	Kind     string   `json:"kind"`
	Power    *bool    `json:"power,omitempty"`
	Celsius  *float64 `json:"celsius,omitempty"`
	Mode     *string  `json:"mode,omitempty"`
	FanLevel *string  `json:"fanLevel,omitempty"`
	Level    *string  `json:"level,omitempty"`
}

// The UnitAction kinds (DD-C03).
const (
	actionSetPower       = "set_power"
	actionSetTemperature = "set_temperature"
	actionSetMode        = "set_mode"
	actionSetFan         = "set_fan"
	actionVentilate      = "ventilate"
)

// stepTolerance is how far a setpoint may lie off the model's temperature step grid (floating-point error).
const stepTolerance = 1e-9

// ParseAction decodes a UnitAction strictly: exactly the fields of its kind (error key "error.invalid").
func ParseAction(raw json.RawMessage) (UnitAction, bool) {
	var unitAction UnitAction

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if len(raw) == 0 || dec.Decode(&unitAction) != nil {
		return unitAction, false
	}

	return unitAction, unitAction.fieldCount() == 1 && unitAction.kindFieldValid()
}

// Supports reports whether a unit with these capabilities accepts the action (DD-C03: temperature on min/max/step,
// modes and fan levels from the model's lists, ventilation only on ventilation models).
func (a UnitAction) Supports(caps Caps) bool {
	switch a.Kind {
	case actionSetPower:
		return caps.Control
	case actionSetTemperature:
		return caps.acceptsTemperature(a.Celsius)
	case actionSetMode:
		return caps.Control && offers(caps.ModeControl, caps.Modes, a.Mode)
	case actionSetFan:
		return caps.Control && offers(caps.FanControl, caps.FanLevels, a.FanLevel)
	case actionVentilate:
		return offers(caps.Ventilation, caps.VentilationLevels, a.Level)
	}

	return false
}

// fieldCount is how many of the kind-specific fields the action carries.
func (a UnitAction) fieldCount() int {
	count := 0

	for _, present := range []bool{a.Power != nil, a.Celsius != nil, a.Mode != nil, a.FanLevel != nil, a.Level != nil} {
		if present {
			count++
		}
	}

	return count
}

// kindFieldValid reports whether the action carries the field of its kind with a value the kind allows.
func (a UnitAction) kindFieldValid() bool {
	switch a.Kind {
	case actionSetPower:
		return a.Power != nil
	case actionSetTemperature:
		return a.Celsius != nil && finite(*a.Celsius)
	case actionSetMode:
		return a.Mode != nil && isMode(*a.Mode)
	case actionSetFan:
		return a.FanLevel != nil && isFanLevel(*a.FanLevel)
	case actionVentilate:
		return a.Level != nil && isFanLevel(*a.Level)
	}

	return false
}

// acceptsTemperature applies DD-C03 to a setpoint: temperature control, within min/max and on the step grid from min.
// The setpoint is read only once the unit has temperature control.
func (caps Caps) acceptsTemperature(celsius *float64) bool {
	if !caps.Control || !caps.HasTemperature || *celsius < caps.TempMin || *celsius > caps.TempMax {
		return false
	}

	if caps.TempStep > 0 {
		steps := (*celsius - caps.TempMin) / caps.TempStep

		return math.Abs(steps-math.Round(steps)) < stepTolerance
	}

	return true
}

// offers reports whether a capability is on and lists the value (read only when the capability is on).
func offers(enabled bool, values []string, value *string) bool {
	return enabled && slices.Contains(values, *value)
}

// isMode reports whether an action may set the mode.
func isMode(mode string) bool {
	switch mode {
	case "cool", "dry", "fan":
		return true
	}

	return false
}

// isFanLevel reports whether an action may set the fan or ventilation level.
func isFanLevel(level string) bool {
	switch level {
	case "low", "mid", "high":
		return true
	}

	return false
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
