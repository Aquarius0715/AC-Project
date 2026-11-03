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

var (
	modes = []string{"cool", "dry", "fan"}
	fans  = []string{"low", "mid", "high"}
)

// ParseAction decodes a UnitAction strictly: exactly the fields of its kind (error key "error.invalid").
func ParseAction(raw json.RawMessage) (UnitAction, bool) {
	var unitAction UnitAction

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if len(raw) == 0 || dec.Decode(&unitAction) != nil {
		return unitAction, false
	}

	set := func(b bool) int {
		if b {
			return 1
		}

		return 0
	}

	n := set(
		unitAction.Power != nil,
	) + set(
		unitAction.Celsius != nil,
	) + set(
		unitAction.Mode != nil,
	) + set(
		unitAction.FanLevel != nil,
	) + set(
		unitAction.Level != nil,
	)
	switch unitAction.Kind {
	case "set_power":
		return unitAction, n == 1 && unitAction.Power != nil
	case "set_temperature":
		return unitAction, n == 1 && unitAction.Celsius != nil &&
			!math.IsNaN(*unitAction.Celsius) &&
			!math.IsInf(*unitAction.Celsius, 0)
	case "set_mode":
		return unitAction, n == 1 && unitAction.Mode != nil &&
			slices.Contains(modes, *unitAction.Mode)
	case "set_fan":
		return unitAction, n == 1 && unitAction.FanLevel != nil &&
			slices.Contains(fans, *unitAction.FanLevel)
	case "ventilate":
		return unitAction, n == 1 && unitAction.Level != nil &&
			slices.Contains(fans, *unitAction.Level)
	}

	return unitAction, false
}

// Supports reports whether a unit with these capabilities accepts the action (DD-C03: temperature on min/max/step,
// modes and fan levels from the model's lists, ventilation only on ventilation models).
func (a UnitAction) Supports(caps Caps) bool {
	switch a.Kind {
	case "set_power":
		return caps.Control
	case "set_temperature":
		if !caps.Control || !caps.HasTemperature || *a.Celsius < caps.TempMin ||
			*a.Celsius > caps.TempMax {
			return false
		}

		if caps.TempStep > 0 {
			k := (*a.Celsius - caps.TempMin) / caps.TempStep

			return math.Abs(k-math.Round(k)) < 1e-9
		}

		return true
	case "set_mode":
		return caps.Control && caps.ModeControl && slices.Contains(caps.Modes, *a.Mode)
	case "set_fan":
		return caps.Control && caps.FanControl && slices.Contains(caps.FanLevels, *a.FanLevel)
	case "ventilate":
		return caps.Ventilation && slices.Contains(caps.VentilationLevels, *a.Level)
	}

	return false
}
