// Package iotbridge turns the messages of the simulated modules (internal/devicesim) into the demo trigger the Core API
// accepts (POST /v1/demo/trigger, eventType telemetry). The links are the demo fixture's devices (demoSeed.devices in
// docs/04-agentic-sdlc/fixture-contract.json), with the same IDs the seed derives (seed.ID).
package iotbridge

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

var (
	// errNotDemoLink refuses a batch from a device the links do not know.
	errNotDemoLink = errors.New("is not a demo link")
	// errNoSensor refuses a reading of a sensor the device does not have.
	errNoSensor = errors.New("has no sensor")
)

// Scenario labels the triggers the bridge sends, so the demo audit can tell them from the browser's own triggers.
const Scenario = "iot-demo"

// Link ties one fixture device to its unit and to its sensors, by the sensor names the simulator uses.
type Link struct {
	Device  string            // fixture ID, e.g. device-online-rto
	Unit    string            // fixture ID of the unit the device is bound to
	Sensors map[string]string // simulator sensor name → fixture sensor ID
	// the device's connection in the fixture: a module that is not online sends nothing (IR613)
	Connection string
}

// Links are the demo fixture's five devices with the sensors each has there (device-no-sensor has none: its module only
// beats) and their connection.
var Links = []Link{
	{Device: "device-online-rto", Unit: "unit-online-rto", Sensors: map[string]string{
		sensorTemperature: "sensor-online-temperature",
		sensorPower:       "sensor-online-power",
		sensorHumidity:    "sensor-online-humidity",
		"co2":             "sensor-online-co2",
		"pm25":            "sensor-online-pm25",
	}, Connection: connectionOnline},
	{Device: "device-tamper", Unit: "unit-non-rto", Sensors: map[string]string{
		sensorTemperature: "sensor-tamper-temperature", sensorPower: "sensor-tamper-power",
		sensorHumidity: "sensor-tamper-humidity",
	}, Connection: connectionOnline},
	{
		Device: "device-offline-rto",
		Unit:   "unit-offline-rto",
		Sensors: map[string]string{
			sensorTemperature: "sensor-offline-temperature",
			sensorPower:       "sensor-offline-power",
		},
		Connection: "offline",
	},
	{
		Device: "device-limited",
		Unit:   "unit-limited",
		Sensors: map[string]string{
			sensorTemperature: "sensor-limited-temperature",
			sensorPower:       "sensor-limited-power",
		},
		Connection: connectionOnline,
	},
	{
		Device:     "device-no-sensor",
		Unit:       "unit-other-customer",
		Sensors:    map[string]string{},
		Connection: connectionOnline,
	},
}

// target is one link resolved to the IDs the API uses.
type target struct {
	unit    uuid.UUID
	sensors map[string]uuid.UUID
}

// Resolver maps a simulated device (by its UUID in the topic) to its unit and sensors.
type Resolver struct {
	byDevice map[uuid.UUID]target
}

// NewResolver builds the lookup from the links, deriving every ID the way the seed does.
func NewResolver(links []Link) *Resolver {
	resolver := &Resolver{byDevice: map[uuid.UUID]target{}}

	for _, link := range links {
		t := target{unit: seed.ID(link.Unit), sensors: map[string]uuid.UUID{}}
		for name, fixture := range link.Sensors {
			t.sensors[name] = seed.ID(fixture)
		}

		resolver.byDevice[seed.ID(link.Device)] = t
	}

	return resolver
}

// DeviceID is the UUID a fixture device has in the API (and in the simulator's topics).
func DeviceID(fixtureDevice string) uuid.UUID { return seed.ID(fixtureDevice) }

// Trigger is the body of POST /v1/demo/trigger for one telemetry reading.
type Trigger struct {
	ScenarioID  string                   `json:"scenarioId"`
	EventID     uuid.UUID                `json:"eventId"`
	OccurredAt  time.Time                `json:"occurredAt"`
	EventType   string                   `json:"eventType"`
	CommandID   *uuid.UUID               `json:"commandId,omitempty"`
	Measurement *Measurement             `json:"measurement,omitempty"`
	DeviceID    *uuid.UUID               `json:"deviceId,omitempty"` // state
	State       *devicesim.ReportedState `json:"state,omitempty"`    // state
	// automation: the facts against the alert policies (IR529)
	Event *Evaluation `json:"event,omitempty"`
	// device: a fault or recovery of the device's current binding (SR20, IR532)
	BindingID *uuid.UUID `json:"bindingId,omitempty"`
	Sequence  *int64     `json:"sequence,omitempty"`
	Kind      string     `json:"kind,omitempty"`
	Recovery  *Recovery  `json:"recovery,omitempty"`
}

// Recovery is the DeviceRecovery a restored device event carries: the axis, the value it returns to and the fault it
// ends (that fault's DeviceEvent ID).
type Recovery struct {
	Axis          string    `json:"axis"`
	SourceEventID uuid.UUID `json:"sourceEventId"`
	Value         string    `json:"value"`
}

// Evaluation is the EvaluationInput a demo automation trigger carries: the facts a gateway reports, which the API
// evaluates against the units' alert policies only (IR529).
type Evaluation struct {
	EventID    uuid.UUID   `json:"eventId"`
	OccurredAt time.Time   `json:"occurredAt"`
	UnitIDs    []uuid.UUID `json:"unitIds"`
	Facts      []Fact      `json:"facts"`
	Phase      string      `json:"phase"`
}

// Fact is one reading of a unit as an evaluation takes it (the contract's Fact, a number).
type Fact struct {
	UnitID     uuid.UUID `json:"unitId"`
	Metric     string    `json:"metric"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	ObservedAt time.Time `json:"observedAt"`
	Quality    string    `json:"quality"`
}

// Measurement is the RawMeasurement the trigger carries.
type Measurement struct {
	UnitID     uuid.UUID `json:"unitId"`
	SensorID   uuid.UUID `json:"sensorId"`
	Metric     string    `json:"metric"`
	Value      *float64  `json:"value"`
	Unit       string    `json:"unit"`
	ObservedAt time.Time `json:"observedAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	Origin     string    `json:"origin"`
	Quality    string    `json:"quality"`
	Sequence   *int64    `json:"sequence,omitempty"`
	// EventID is the key the API checks for a repeat of this reading; without it every reading shares the nil ID and all
	// but the first are taken as repeats (and dropped without an error).
	EventID uuid.UUID `json:"eventId"`
}

// Triggers converts one telemetry batch. A device the links do not know, or a sensor it does not have, is an error for
// that reading; the rest of the batch still converts. Each reading's event ID is derived from the device and its
// sequence, so a message delivered twice is one event to the API.
func (r *Resolver) Triggers(batch devicesim.Batch, received time.Time) ([]Trigger, error) {
	dev, err := uuid.Parse(batch.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("device %q: %w", batch.DeviceID, err)
	}

	t, ok := r.byDevice[dev]
	if !ok {
		return nil, fmt.Errorf("device %s %w", batch.DeviceID, errNotDemoLink)
	}

	var out []Trigger

	var first error // the first reading that could not convert; the rest of the batch still converts

	for _, measurement := range batch.Measurements {
		sensor, ok := t.sensors[measurement.SensorID]
		if !ok {
			if first == nil {
				first = fmt.Errorf(
					"device %s %w %q",
					batch.DeviceID,
					errNoSensor,
					measurement.SensorID,
				)
			}

			continue
		}
		// the event ID is the reading itself (device, sensor, observed time): a simulator restart restarts its sequence, so
		// the sequence cannot tell readings apart; a redelivered reading has the same time and so the same event
		event := uuid.NewSHA1(
			seed.Namespace,
			[]byte(
				fmt.Sprintf(
					"%s/%s/%s",
					batch.DeviceID,
					measurement.SensorID,
					measurement.ObservedAt.UTC().Format(time.RFC3339Nano),
				),
			),
		)
		out = append(out, Trigger{
			ScenarioID: Scenario,
			EventID:    event,
			OccurredAt: measurement.ObservedAt,
			EventType:  "telemetry",
			Measurement: &Measurement{
				UnitID:     t.unit,
				SensorID:   sensor,
				Metric:     measurement.Metric,
				Value:      measurement.Value,
				Unit:       measurement.Unit, // nil: the API stores the reading missing
				ObservedAt: measurement.ObservedAt,
				ReceivedAt: received.UTC(),
				Origin:     "measured",
				Quality:    qualityValid,
				EventID:    event, // no sequence: the API numbers the reading after the latest (IR77)
				Sequence:   nil,
			}, CommandID: nil, DeviceID: nil, State: nil, Event: nil, BindingID: nil, Sequence: nil, Kind: "", Recovery: nil,
		})
	}

	return out, first
}

// CommandTrigger turns a module's acknowledgement into the demo trigger for the command it answers: applied is
// command_ack, unsupported is command_fail (the device rejected it). An expired acknowledgement is not forwarded: the
// backend decides a late acknowledgement by its own clock (common design §5), so the bridge does not change the
// command.
func CommandTrigger(ack devicesim.Ack, received time.Time) (Trigger, bool, error) {
	cmd, err := uuid.Parse(ack.CommandID)
	if err != nil {
		return Trigger{}, false, fmt.Errorf("command %q: %w", ack.CommandID, err)
	}

	var kind string

	switch ack.Result {
	case "applied":
		kind = "command_ack"
	case "unsupported":
		kind = "command_fail"
	default:
		return Trigger{
			ScenarioID:  "",
			EventID:     uuid.UUID{},
			OccurredAt:  time.Time{},
			EventType:   "",
			CommandID:   nil,
			Measurement: nil,
			DeviceID:    nil,
			State:       nil,
			Event:       nil,
			BindingID:   nil,
			Sequence:    nil,
			Kind:        "",
			Recovery:    nil,
		}, false, nil
	}

	return Trigger{
		ScenarioID: Scenario,
		EventID: uuid.NewSHA1(
			seed.Namespace,
			[]byte(fmt.Sprintf("%s/%s/%s", ack.DeviceID, ack.CommandID, kind)),
		),
		OccurredAt:  received.UTC(),
		EventType:   kind,
		CommandID:   &cmd,
		Measurement: nil,
		DeviceID:    nil,
		State:       nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil,
	}, true, nil
}

// FleetLinks are the mock fleet's modules (internal/devicesim/fleet.json): each pattern with a module reports its
// temperature and power sensors on its own unit.
func FleetLinks(f devicesim.Fleet) []Link {
	var out []Link

	for _, pattern := range f.Patterns {
		if pattern.NoDevice {
			continue
		}

		sensors := map[string]string{}
		for _, s := range devicesim.Sensors {
			sensors[s.ID] = devicesim.SensorName(pattern, s)
		}

		out = append(
			out,
			Link{
				Device:     devicesim.DeviceName(pattern),
				Unit:       devicesim.UnitName(pattern),
				Sensors:    sensors,
				Connection: "",
			},
		)
	}

	return out
}

// ToScenario stamps a batch received at real time now on the API's scenario clock: each reading is put at the start of
// the scenario minute it was received in (now + offset - margin), the slot energy counts (D07), so the newest reading
// is never more than about a minute old there. What the module stamped beyond its own current minute (a reading from
// the future) is kept as that many minutes ahead.
func ToScenario(
	batch devicesim.Batch,
	now time.Time,
	offset, margin time.Duration,
) devicesim.Batch {
	slot := now.Add(offset - margin).UTC().Truncate(time.Minute)
	moduleMinute := now.UTC().Truncate(time.Minute)
	out := batch

	out.Measurements = make([]devicesim.Measurement, len(batch.Measurements))
	for i, m := range batch.Measurements {
		ahead := max(m.ObservedAt.Sub(moduleMinute).Round(time.Minute), 0)

		m.ObservedAt = slot.Add(ahead)
		out.Measurements[i] = m
	}

	return out
}
