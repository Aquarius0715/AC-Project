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
	var a UnitAction
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if len(raw) == 0 || dec.Decode(&a) != nil {
		return a, false
	}
	set := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	n := set(a.Power != nil) + set(a.Celsius != nil) + set(a.Mode != nil) + set(a.FanLevel != nil) + set(a.Level != nil)
	switch a.Kind {
	case "set_power":
		return a, n == 1 && a.Power != nil
	case "set_temperature":
		return a, n == 1 && a.Celsius != nil && !math.IsNaN(*a.Celsius) && !math.IsInf(*a.Celsius, 0)
	case "set_mode":
		return a, n == 1 && a.Mode != nil && slices.Contains(modes, *a.Mode)
	case "set_fan":
		return a, n == 1 && a.FanLevel != nil && slices.Contains(fans, *a.FanLevel)
	case "ventilate":
		return a, n == 1 && a.Level != nil && slices.Contains(fans, *a.Level)
	}
	return a, false
}

// Supports reports whether a unit with these capabilities accepts the action (DD-C03: temperature on min/max/step,
// modes and fan levels from the model's lists, ventilation only on ventilation models).
func (a UnitAction) Supports(c Caps) bool {
	switch a.Kind {
	case "set_power":
		return c.Control
	case "set_temperature":
		if !c.Control || !c.HasTemperature || *a.Celsius < c.TempMin || *a.Celsius > c.TempMax {
			return false
		}
		if c.TempStep > 0 {
			k := (*a.Celsius - c.TempMin) / c.TempStep
			return math.Abs(k-math.Round(k)) < 1e-9
		}
		return true
	case "set_mode":
		return c.Control && c.ModeControl && slices.Contains(c.Modes, *a.Mode)
	case "set_fan":
		return c.Control && c.FanControl && slices.Contains(c.FanLevels, *a.FanLevel)
	case "ventilate":
		return c.Ventilation && slices.Contains(c.VentilationLevels, *a.Level)
	}
	return false
}
