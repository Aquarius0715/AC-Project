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

// Sensor is one measurement a module reports: the metric, its unit and the value range it simulates.
type Sensor struct {
	ID     string
	Metric string
	Unit   string
	Base   float64
	Swing  float64
}

// Sensors are the two a simulated AC module reports (the same metrics as the capability register's sensors).
var Sensors = []Sensor{
	{ID: "temperature", Metric: "temperature", Unit: "°C", Base: 24, Swing: 2},
	{ID: "power", Metric: "power", Unit: "kW", Base: 1.2, Swing: 0.4},
}

// Measurement is one reading in a telemetry batch.
type Measurement struct {
	SensorID   string    `json:"sensorId"`
	Metric     string    `json:"metric"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	ObservedAt time.Time `json:"observedAt"`
	Sequence   int64     `json:"sequence"`
}

// Batch is the telemetry message: all readings of one module taken at one time.
type Batch struct {
	DeviceID     string        `json:"deviceId"`
	Measurements []Measurement `json:"measurements"`
}

// Heartbeat is the liveness message sent every 60 seconds.
type Heartbeat struct {
	DeviceID string    `json:"deviceId"`
	At       time.Time `json:"at"`
	Firmware string    `json:"firmware"`
}

// Module is one simulated device: its identity, its firmware and the next sequence number of its readings.
type Module struct {
	TenantID string
	DeviceID string
	Firmware string
	rng      *rand.Rand
	sequence int64
}

// NewModule starts a module with its own random source, so each device drifts independently.
func NewModule(tenantID, deviceID, firmware string, seed uint64) *Module {
	return &Module{TenantID: tenantID, DeviceID: deviceID, Firmware: firmware, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
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
func (m *Module) Telemetry(at time.Time) ([]byte, error) {
	b := Batch{DeviceID: m.DeviceID, Measurements: make([]Measurement, 0, len(Sensors))}
	for _, s := range Sensors {
		m.sequence++
		v := s.Base + (m.rng.Float64()*2-1)*s.Swing
		b.Measurements = append(b.Measurements, Measurement{
			SensorID: s.ID, Metric: s.Metric, Value: math.Round(v*100) / 100, Unit: s.Unit, ObservedAt: at.UTC(), Sequence: m.sequence,
		})
	}
	return json.Marshal(b)
}

// Beat returns the heartbeat at the given time.
func (m *Module) Beat(at time.Time) ([]byte, error) {
	return json.Marshal(Heartbeat{DeviceID: m.DeviceID, At: at.UTC(), Firmware: m.Firmware})
}
