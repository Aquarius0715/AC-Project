// Package devicesim builds the MQTT messages of a simulated IoT module, as the backend design describes them
// (backend-architecture §9): telemetry batches every minute, a heartbeat every 60 seconds, on
// t/<tenantId>/d/<deviceId>/<kind>. The broker connection lives in cmd/devicesim; this package only shapes the messages.
package devicesim

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// Sensor is one measurement a module reports: the metric, its unit, the value range it simulates and its measurement
// boundary (power is measured at the air conditioner's input, the boundary energy counts, IR11).
type Sensor struct {
	ID       string
	Metric   string
	Unit     string
	Base     float64
	Swing    float64
	Boundary string
}

// Sensors are the ones a simulated AC module can report (the metrics and units of the capability register's sensors).
// Temperature and power come first; a module reports the subset its device has (Only).
var Sensors = []Sensor{
	{ID: "temperature", Metric: "temperature", Unit: "°C", Base: 24, Swing: 2},
	{ID: "power", Metric: "power", Unit: "kW", Base: 1.2, Swing: 0.4, Boundary: "ac_input_electricity"},
	{ID: "humidity", Metric: "humidity", Unit: "%", Base: 60, Swing: 8},
	{ID: "co2", Metric: "co2", Unit: "ppm", Base: 650, Swing: 250},
	{ID: "pm25", Metric: "pm25", Unit: "µg/m³", Base: 14, Swing: 9},
}

// Measurement is one reading in a telemetry batch.
type Measurement struct {
	SensorID   string    `json:"sensorId"`
	Metric     string    `json:"metric"`
	Value      *float64  `json:"value"` // nil: the sensor gave no value (the API stores it missing)
	Unit       string    `json:"unit"`
	ObservedAt time.Time `json:"observedAt"`
	Sequence   int64     `json:"sequence"`
}

// Batch is the telemetry message: all readings of one module taken at one time.
type Batch struct {
	DeviceID     string        `json:"deviceId"`
	Measurements []Measurement `json:"measurements"`
}

// Heartbeat is the liveness message sent every 60 seconds. A module that reports its air conditioner's state carries it
// (the API keeps the unit's observed state fresh from it).
type Heartbeat struct {
	DeviceID string         `json:"deviceId"`
	At       time.Time      `json:"at"`
	Firmware string         `json:"firmware"`
	State    *ReportedState `json:"state,omitempty"`
}

// ReportedState is the air conditioner's state as the module sees it: power, setpoint, mode and fan level.
type ReportedState struct {
	Power    *bool    `json:"power"`
	Celsius  *float64 `json:"celsius"`
	Mode     *string  `json:"mode"`
	FanLevel *string  `json:"fanLevel"`
}

// Module is one simulated device: its identity, its firmware and the next sequence number of its readings.
type Module struct {
	TenantID string
	DeviceID string
	Firmware string
	rng      *rand.Rand
	sequence int64
	setpoint float64
	// behaviour of the pattern the module plays (mock fleet); empty is normal readings and applied commands
	telemetry, commands string
	powerOff            bool
	sensors             []Sensor       // the sensors the device has (all of Sensors unless Only narrows them)
	state               *ReportedState // reported with each heartbeat when set (Reports)
}

// NewModule starts a module with its own random source, so each device drifts independently.
func NewModule(tenantID, deviceID, firmware string, seed uint64) *Module {
	return &Module{TenantID: tenantID, DeviceID: deviceID, Firmware: firmware, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), setpoint: 24}
}

// Only narrows the module to the sensors its device has, by sensor name; unknown names are ignored.
func (m *Module) Only(names ...string) *Module {
	keep := map[string]bool{}
	for _, n := range names {
		keep[n] = true
	}
	var out []Sensor
	for _, s := range Sensors {
		if keep[s.ID] {
			out = append(out, s)
		}
	}
	m.sensors = out
	return m
}

// Reports makes the module report its air conditioner's state with each heartbeat, starting from the given one; a
// set-temperature command it applies changes the reported setpoint.
func (m *Module) Reports(power *bool, celsius *float64, mode, fanLevel *string) *Module {
	m.state = &ReportedState{Power: power, Celsius: celsius, Mode: mode, FanLevel: fanLevel}
	if celsius != nil {
		m.setpoint = *celsius
	}
	return m
}

// Behave makes the module play a pattern of the mock fleet: how it reports (Telemetry), how it answers commands
// (Commands), and whether its air conditioner draws no power (switched off or power signal off).
func (m *Module) Behave(telemetry, commands string, powerOff bool) *Module {
	m.telemetry, m.commands, m.powerOff = telemetry, commands, powerOff
	return m
}

// Topic is the MQTT topic of one message kind of a device (backend-architecture §9).
func Topic(tenantID, deviceID, kind string) string {
	return fmt.Sprintf("t/%s/d/%s/%s", tenantID, deviceID, kind)
}

// TelemetryTopic and HeartbeatTopic are the two topics the module publishes its routine messages on.
func (m *Module) TelemetryTopic() string { return Topic(m.TenantID, m.DeviceID, "telemetry") }
func (m *Module) HeartbeatTopic() string { return Topic(m.TenantID, m.DeviceID, "heartbeat") }

// Telemetry takes one reading of every sensor at the given time and returns it as a batch; each reading gets the
// next sequence number, so the backend can keep them in order and drop duplicates.
//
// The pattern changes the readings: silent sends nothing (nil), unit_mismatch reports the temperature in kelvin,
// out_of_range reports an impossible temperature, missing reports no values, invalid_time stamps the readings an
// hour ahead; a module whose air conditioner draws no power reports 0 kW.
func (m *Module) Telemetry(at time.Time) ([]byte, error) {
	if m.telemetry == "silent" {
		return nil, nil
	}
	// readings are taken at the start of the minute: energy counts a power sample only at a minute slot (D07)
	observed := at.UTC().Truncate(time.Minute)
	if m.telemetry == "invalid_time" {
		observed = observed.Add(time.Hour)
	}
	sensors := m.sensors
	if sensors == nil {
		sensors = Sensors
	}
	b := Batch{DeviceID: m.DeviceID, Measurements: make([]Measurement, 0, len(sensors))}
	for _, s := range sensors {
		m.sequence++
		v := math.Round((s.Base+(m.rng.Float64()*2-1)*s.Swing)*100) / 100
		unit := s.Unit
		switch {
		case s.Metric == "power" && m.powerOff:
			v = 0
		case s.Metric == "temperature" && m.telemetry == "unit_mismatch":
			unit = "K"
		case s.Metric == "temperature" && m.telemetry == "out_of_range":
			v = 250
		}
		x := Measurement{SensorID: s.ID, Metric: s.Metric, Value: &v, Unit: unit, ObservedAt: observed, Sequence: m.sequence}
		if m.telemetry == "missing" {
			x.Value = nil
		}
		b.Measurements = append(b.Measurements, x)
	}
	return json.Marshal(b)
}

// Beat returns the heartbeat at the given time.
func (m *Module) Beat(at time.Time) ([]byte, error) {
	return json.Marshal(Heartbeat{DeviceID: m.DeviceID, At: at.UTC(), Firmware: m.Firmware, State: m.state})
}

// Command is a control command the backend sends down on /cmd. ExpiresAt is when the command stops being valid; the
// backend marks a late acknowledgement as late (backend-architecture §9, common design §5). The field names are the
// simulator's own until the command contract fixes them.
type Command struct {
	CommandID string    `json:"commandId"`
	Kind      string    `json:"kind"`
	Setpoint  *float64  `json:"setpointCelsius,omitempty"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Ack is the acknowledgement sent on /cmd/ack: whether the command was applied and the state the module observes after it.
type Ack struct {
	DeviceID   string    `json:"deviceId"`
	CommandID  string    `json:"commandId"`
	ReceivedAt time.Time `json:"receivedAt"`
	Result     string    `json:"result"`
	Observed   Observed  `json:"observed"`
}

// Observed is the module's state after the command.
type Observed struct {
	SetpointCelsius float64 `json:"setpointCelsius"`
}

// CommandTopic and AckTopic are the downward and upward topics of the command exchange.
func (m *Module) CommandTopic() string { return Topic(m.TenantID, m.DeviceID, "cmd") }
func (m *Module) AckTopic() string     { return Topic(m.TenantID, m.DeviceID, "cmd/ack") }

// Acknowledge applies a command to the module and returns the acknowledgement and when to send it. A command the module
// has already passed its expiry for is not applied; the acknowledgement says so, and the module keeps its state.
//
// The pattern changes the answer: reject answers unsupported without applying, ignore sends nothing (nil), late
// applies the command but sends the answer two seconds after the command expired.
func (m *Module) Acknowledge(raw []byte, at time.Time) ([]byte, time.Time, error) {
	var c Command
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, at, fmt.Errorf("command: %w", err)
	}
	if m.commands == "ignore" {
		return nil, at, nil
	}
	result, sendAt := "applied", at
	switch {
	case !at.Before(c.ExpiresAt):
		result = "expired"
	case m.commands == "reject" || c.Kind != "set_temperature":
		result = "unsupported"
	case c.Setpoint != nil:
		m.setpoint = *c.Setpoint
		if m.state != nil {
			sp := m.setpoint
			m.state.Celsius = &sp
		}
	}
	if m.commands == "late" && result == "applied" {
		sendAt = c.ExpiresAt.Add(2 * time.Second)
	}
	raw, err := json.Marshal(Ack{DeviceID: m.DeviceID, CommandID: c.CommandID, ReceivedAt: at.UTC(), Result: result, Observed: Observed{SetpointCelsius: m.setpoint}})
	return raw, sendAt, err
}
