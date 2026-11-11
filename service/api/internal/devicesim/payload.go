// Package devicesim builds the MQTT messages of a simulated IoT module, as the backend design describes them
// (backend-architecture §9): telemetry batches every minute, a heartbeat every 60 seconds, on
// t/<tenantId>/d/<deviceId>/<kind>. The broker connection lives in cmd/devicesim; this package only shapes the
// messages.
package devicesim

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
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

// The metrics the pattern's behaviour changes the readings of.
const (
	metricTemperature = "temperature"
	metricPower       = "power"
)

// The value range each sensor simulates: a reading is its base, give or take up to its swing.
const (
	temperatureBaseCelsius  = 24
	temperatureSwingCelsius = 2
	powerBaseKW             = 1.2
	powerSwingKW            = 0.4
	humidityBasePercent     = 60
	humiditySwingPercent    = 8
	co2BasePPM              = 650
	co2SwingPPM             = 250
	pm25BaseMicrograms      = 14 // µg/m³
	pm25SwingMicrograms     = 9
)

// Sensors are the ones a simulated AC module can report (the metrics and units of the capability register's sensors).
// Temperature and power come first; a module reports the subset its device has (Only).
func Sensors() []Sensor {
	return []Sensor{
		{
			ID:       metricTemperature,
			Metric:   metricTemperature,
			Unit:     "°C",
			Base:     temperatureBaseCelsius,
			Swing:    temperatureSwingCelsius,
			Boundary: "",
		},
		{
			ID:       metricPower,
			Metric:   metricPower,
			Unit:     "kW",
			Base:     powerBaseKW,
			Swing:    powerSwingKW,
			Boundary: "ac_input_electricity",
		},
		{
			ID:       "humidity",
			Metric:   "humidity",
			Unit:     "%",
			Base:     humidityBasePercent,
			Swing:    humiditySwingPercent,
			Boundary: "",
		},
		{ID: "co2", Metric: "co2", Unit: "ppm", Base: co2BasePPM, Swing: co2SwingPPM, Boundary: ""},
		{
			ID:       "pm25",
			Metric:   "pm25",
			Unit:     "µg/m³",
			Base:     pm25BaseMicrograms,
			Swing:    pm25SwingMicrograms,
			Boundary: "",
		},
	}
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
//
// The broker's command handler (Acknowledge, Restore, Keep) runs on another goroutine than the publishing loop (Beat),
// so the setpoint and the reported state they share are guarded by lock.
type Module struct {
	lock     sync.Mutex
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

const (
	// seedSpread is 2⁶⁴ divided by the golden ratio: it spreads a module's seed into the second word of its generator.
	seedSpread = 0x9e3779b97f4a7c15
	// initialSetpointCelsius is a module's setpoint until a command or its pattern's state sets another.
	initialSetpointCelsius = 24
)

// NewModule starts a module with its own random source, so each device drifts independently.
func NewModule(tenantID, deviceID, firmware string, seed uint64) *Module {
	return &Module{
		lock:     sync.Mutex{},
		TenantID: tenantID,
		DeviceID: deviceID,
		Firmware: firmware,
		//nolint:gosec // simulated readings: a predictable per-device generator is wanted, not a secure one
		rng:       rand.New(rand.NewPCG(seed, seed^seedSpread)),
		setpoint:  initialSetpointCelsius,
		sequence:  0,
		telemetry: "",
		commands:  "",
		powerOff:  false,
		sensors:   nil,
		state:     nil,
	}
}

// Only narrows the module to the sensors its device has, by sensor name; unknown names are ignored. A device with none
// (device-no-sensor) reports no readings, only its heartbeat (IR613).
func (m *Module) Only(names ...string) *Module {
	keep := map[string]bool{}
	for _, n := range names {
		keep[n] = true
	}

	out := []Sensor{}

	for _, s := range Sensors() {
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

// TelemetryTopic is one of the two topics the module publishes its routine messages on: its telemetry batches.
func (m *Module) TelemetryTopic() string { return Topic(m.TenantID, m.DeviceID, "telemetry") }

// HeartbeatTopic is the other routine topic: the module's heartbeats.
func (m *Module) HeartbeatTopic() string { return Topic(m.TenantID, m.DeviceID, "heartbeat") }

// Telemetry takes one reading of every sensor at the given time and returns it as a batch; each reading gets the
// next sequence number, so the backend can keep them in order and drop duplicates.
//
// The pattern changes the readings: silent sends nothing (nil), as does a module without sensors; unit_mismatch reports
// the temperature in kelvin, out_of_range reports an impossible temperature, missing reports no values, invalid_time
// stamps the readings an hour ahead; a module whose air conditioner draws no power reports 0 kW.
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
		sensors = Sensors()
	}

	if len(sensors) == 0 { // a module without sensors sends only its heartbeat
		return nil, nil
	}

	batch := Batch{DeviceID: m.DeviceID, Measurements: make([]Measurement, 0, len(sensors))}
	for _, sensor := range sensors {
		batch.Measurements = append(batch.Measurements, m.measure(sensor, observed))
	}

	out, err := json.Marshal(batch)

	return out, apperr.Wrap(err, "json.Marshal")
}

// Beat returns the heartbeat at the given time.
func (m *Module) Beat(at time.Time) ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	out, err := json.Marshal(
		Heartbeat{DeviceID: m.DeviceID, At: at.UTC(), Firmware: m.Firmware, State: m.state},
	)

	return out, apperr.Wrap(err, "json.Marshal")
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

// Ack is the acknowledgement sent on /cmd/ack: whether the command was applied and the state the module observes after
// it.
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

// CommandTopic is the downward topic of the command exchange: the module hears its commands on it.
func (m *Module) CommandTopic() string { return Topic(m.TenantID, m.DeviceID, "cmd") }

// AckTopic is the upward topic of the command exchange: the module answers on it.
func (m *Module) AckTopic() string { return Topic(m.TenantID, m.DeviceID, "cmd/ack") }

// The results an acknowledgement reports.
const (
	resultApplied     = "applied"
	resultExpired     = "expired"
	resultUnsupported = "unsupported"
)

// lateAnswerDelay is how long after a command's expiry a module that answers late (pattern late) sends its answer.
const lateAnswerDelay = 2 * time.Second

// Acknowledge applies a command to the module and returns the acknowledgement and when to send it. A command the module
// has already passed its expiry for is not applied; the acknowledgement says so, and the module keeps its state.
//
// The pattern changes the answer: reject answers unsupported without applying, ignore sends nothing (nil), late
// applies the command but sends the answer two seconds after the command expired.
func (m *Module) Acknowledge(raw []byte, when time.Time) ([]byte, time.Time, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var command Command

	err := json.Unmarshal(raw, &command)
	if err != nil {
		return nil, when, fmt.Errorf("command: %w", err)
	}

	if m.commands == "ignore" {
		return nil, when, nil
	}

	result, sendAt := resultApplied, when
	switch {
	case !when.Before(command.ExpiresAt):
		result = resultExpired
	case m.commands == "reject" || command.Kind != "set_temperature":
		result = resultUnsupported
	case command.Setpoint != nil:
		m.setpoint = *command.Setpoint
		if m.state != nil {
			sp := m.setpoint
			m.state.Celsius = &sp
		}
	}

	if m.commands == "late" && result == resultApplied {
		sendAt = command.ExpiresAt.Add(lateAnswerDelay)
	}

	raw, err = json.Marshal(
		Ack{
			DeviceID:   m.DeviceID,
			CommandID:  command.CommandID,
			ReceivedAt: when.UTC(),
			Result:     result,
			Observed:   Observed{SetpointCelsius: m.setpoint},
		},
	)

	return raw, sendAt, apperr.Wrap(err, "json.Marshal")
}

// Kept is what a module keeps across a simulator restart: the setpoint its last applied command set. The simulator
// stores it as a retained message on the module's own kept topic (the bridge does not listen there) and reads it back
// when it starts, so a restart does not put the air conditioner back to its pattern's first setpoint.
type Kept struct {
	SetpointCelsius float64 `json:"setpointCelsius"`
}

// KeptTopic is the retained topic of the module's kept state.
func (m *Module) KeptTopic() string { return Topic(m.TenantID, m.DeviceID, "sim/kept") }

// Keep returns the module's kept state.
func (m *Module) Keep() ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	out, err := json.Marshal(Kept{SetpointCelsius: m.setpoint})

	return out, apperr.Wrap(err, "json.Marshal")
}

// errOutOfRange refuses a kept setpoint outside 16–30 °C.
var errOutOfRange = errors.New("out of range")

// Restore applies a kept state read back from the broker; a setpoint outside 16–30 °C is ignored.
func (m *Module) Restore(raw []byte) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	var kept Kept

	err := json.Unmarshal(raw, &kept)
	if err != nil {
		return fmt.Errorf("kept state: %w", err)
	}

	if kept.SetpointCelsius < 16 || kept.SetpointCelsius > 30 {
		return fmt.Errorf("kept setpoint %v %w", kept.SetpointCelsius, errOutOfRange)
	}

	m.setpoint = kept.SetpointCelsius
	if m.state != nil {
		sp := m.setpoint
		m.state.Celsius = &sp
	}

	return nil
}

// hundredths rounds a simulated reading to two decimals.
const hundredths = 100

// measure takes one reading of a sensor with the next sequence number, changed as the module's pattern says.
func (m *Module) measure(sensor Sensor, observed time.Time) Measurement {
	m.sequence++
	value := math.Round((sensor.Base+(m.rng.Float64()*2-1)*sensor.Swing)*hundredths) / hundredths

	unit := sensor.Unit
	switch {
	case sensor.Metric == metricPower && m.powerOff:
		value = 0
	case sensor.Metric == metricTemperature && m.telemetry == "unit_mismatch":
		unit = "K"
	case sensor.Metric == metricTemperature && m.telemetry == "out_of_range":
		value = 250
	}

	measurement := Measurement{
		SensorID:   sensor.ID,
		Metric:     sensor.Metric,
		Value:      &value,
		Unit:       unit,
		ObservedAt: observed,
		Sequence:   m.sequence,
	}
	if m.telemetry == "missing" {
		measurement.Value = nil
	}

	return measurement
}
