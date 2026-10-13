// Package iotbridge turns the messages of the simulated modules (internal/devicesim) into the demo trigger the Core API
// accepts (POST /v1/demo/trigger, eventType telemetry). The links are the demo fixture's devices (demoSeed.devices in
// docs/04-agentic-sdlc/fixture-contract.json), with the same IDs the seed derives (seed.ID).
package iotbridge

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// Scenario labels the triggers the bridge sends, so the demo audit can tell them from the browser's own triggers.
const Scenario = "iot-demo"

// Link ties one fixture device to its unit and to its sensors, by the sensor names the simulator uses.
type Link struct {
	Device  string            // fixture ID, e.g. device-online-rto
	Unit    string            // fixture ID of the unit the device is bound to
	Sensors map[string]string // simulator sensor name → fixture sensor ID
}

// Links are the four demo devices that report measurements (the fifth, device-no-sensor, reports none).
var Links = []Link{
	{Device: "device-online-rto", Unit: "unit-online-rto", Sensors: map[string]string{"temperature": "sensor-online-temperature", "power": "sensor-online-power"}},
	{Device: "device-tamper", Unit: "unit-non-rto", Sensors: map[string]string{"temperature": "sensor-tamper-temperature", "power": "sensor-tamper-power"}},
	{Device: "device-offline-rto", Unit: "unit-offline-rto", Sensors: map[string]string{"temperature": "sensor-offline-temperature", "power": "sensor-offline-power"}},
	{Device: "device-limited", Unit: "unit-limited", Sensors: map[string]string{"temperature": "sensor-limited-temperature", "power": "sensor-limited-power"}},
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
	r := &Resolver{byDevice: map[uuid.UUID]target{}}
	for _, l := range links {
		t := target{unit: seed.ID(l.Unit), sensors: map[string]uuid.UUID{}}
		for name, fixture := range l.Sensors {
			t.sensors[name] = seed.ID(fixture)
		}
		r.byDevice[seed.ID(l.Device)] = t
	}
	return r
}

// DeviceID is the UUID a fixture device has in the API (and in the simulator's topics).
func DeviceID(fixtureDevice string) uuid.UUID { return seed.ID(fixtureDevice) }

// Trigger is the body of POST /v1/demo/trigger for one telemetry reading.
type Trigger struct {
	ScenarioID  string      `json:"scenarioId"`
	EventID     uuid.UUID   `json:"eventId"`
	OccurredAt  time.Time   `json:"occurredAt"`
	EventType   string      `json:"eventType"`
	Measurement Measurement `json:"measurement"`
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
}

// Triggers converts one telemetry batch. A device the links do not know, or a sensor it does not have, is an error for
// that reading; the rest of the batch still converts. Each reading's event ID is derived from the device and its
// sequence, so a message delivered twice is one event to the API.
func (r *Resolver) Triggers(b devicesim.Batch, received time.Time) ([]Trigger, error) {
	dev, err := uuid.Parse(b.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("device %q: %w", b.DeviceID, err)
	}
	t, ok := r.byDevice[dev]
	if !ok {
		return nil, fmt.Errorf("device %s is not a demo link", b.DeviceID)
	}
	var out []Trigger
	var first error // the first reading that could not convert; the rest of the batch still converts
	for _, m := range b.Measurements {
		sensor, ok := t.sensors[m.SensorID]
		if !ok {
			if first == nil {
				first = fmt.Errorf("device %s has no sensor %q", b.DeviceID, m.SensorID)
			}
			continue
		}
		v := m.Value
		out = append(out, Trigger{
			ScenarioID: Scenario,
			EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/%d", b.DeviceID, m.Sequence))),
			OccurredAt: m.ObservedAt,
			EventType:  "telemetry",
			Measurement: Measurement{
				UnitID: t.unit, SensorID: sensor, Metric: m.Metric, Value: &v, Unit: m.Unit,
				ObservedAt: m.ObservedAt, ReceivedAt: received.UTC(), Origin: "measured",
			},
		})
	}
	return out, first
}
