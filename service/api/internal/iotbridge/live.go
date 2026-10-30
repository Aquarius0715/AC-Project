package iotbridge

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// LiveField is one field of a real sensor node's message and the sensor of the node's live-site module it is recorded
// as (DEC-86, IR524; one module per node, DEC-88).
type LiveField struct {
	Field  string  // the field of the node's JSON message
	Sensor string  // fixture-style name of the live site's sensor (seed.LiveSite)
	Scale  float64 // to the sensor's unit (W → kW: 0.001)
	// Gate is the field that must be non-zero for the value to be a reading: a node sends 0 for what it does not
	// measure, so a zero temperature is no reading, while a zero power is one when the meter sees a voltage.
	Gate string
}

// LiveNodes are the real sensor nodes and the fields the bridge records from each: node_1 measures the room
// (temperature, humidity; also pressure, gas resistance and altitude, which the platform has no metric for) and
// node_2 the air conditioner's supply (power in W; also voltage, current and cumulative energy, which the platform
// derives from power instead). Both report one JSON message every 5 seconds on smarthome/ac/<node>/sensors.
var LiveNodes = map[string][]LiveField{
	"node_1": {
		{Field: "temp", Sensor: "sensor-live-temperature", Scale: 1, Gate: "temp"},
		{Field: "humidity", Sensor: "sensor-live-humidity", Scale: 1, Gate: "humidity"},
	},
	"node_2": {
		{Field: "power", Sensor: "sensor-live-power", Scale: 0.001, Gate: "voltage"},
	},
}

// LiveNode is the node a topic carries: the level before the last, sensors (smarthome/ac/<node>/sensors); "" for any
// other topic under the subscription.
func LiveNode(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 || parts[len(parts)-1] != "sensors" {
		return ""
	}
	return parts[len(parts)-2]
}

// LiveTriggers converts one node message received at `at` on the scenario clock into readings of the node's own unit
// and module (seed.LiveSite). Each reading is stamped at the start of its scenario minute (the slot energy counts,
// D07), and its event ID is the node, field and minute, so the API keeps one reading per sensor and minute. The
// module's heartbeat comes with them: a state report with no settings (the nodes report none) that keeps its unit's
// last seen time current. A node the bridge does not know is an error; a message with nothing measured gives no
// triggers.
func LiveTriggers(node string, payload []byte, at time.Time) ([]Trigger, error) {
	fields, ok := LiveNodes[node]
	site, onSite := seed.LiveNodeNamed(node)
	if !ok || !onSite {
		return nil, fmt.Errorf("node %q is not a live-site node", node)
	}
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("node %s: %w", node, err)
	}
	num := func(k string) (float64, bool) {
		v, ok := msg[k].(float64)
		return v, ok
	}
	at = at.UTC()
	slot := at.Truncate(time.Minute)
	units := map[string]seed.LiveSensor{}
	for _, s := range site.Sensors {
		units[s.Name] = s
	}
	var out []Trigger
	for _, f := range fields {
		g, ok := num(f.Gate)
		v, has := num(f.Field)
		if !ok || g == 0 || !has {
			continue
		}
		s, ok := units[f.Sensor]
		if !ok {
			return nil, fmt.Errorf("node %s: its module has no sensor %s", node, f.Sensor)
		}
		value := v * f.Scale
		event := uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("live/%s/%s/%s", node, f.Field, slot.Format(time.RFC3339))))
		out = append(out, Trigger{
			ScenarioID: Scenario,
			EventID:    event,
			OccurredAt: slot,
			EventType:  "telemetry",
			Measurement: &Measurement{
				UnitID: seed.ID(site.Unit), SensorID: seed.ID(s.Name), Metric: s.Metric, Value: &value, Unit: s.Unit,
				ObservedAt: slot, ReceivedAt: at, Origin: "measured", Quality: "valid", EventID: event,
			},
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	dev := seed.ID(site.Device)
	out = append(out, Trigger{
		ScenarioID: Scenario,
		EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/state/%s", dev, slot.Format(time.RFC3339)))), // one a minute
		OccurredAt: slot,
		EventType:  "state",
		DeviceID:   &dev,
		State:      &devicesim.ReportedState{},
	})
	return out, nil
}

// LiveGate lets one message per node and scenario minute through: the nodes report every 5 seconds, the platform keeps
// a reading a minute. A failed send gives the minute back, so the next message of that minute retries it.
type LiveGate struct {
	mu   sync.Mutex
	sent map[string]time.Time
}

// Claim reports whether node's minute slot is still open and, if so, takes it.
func (g *LiveGate) Claim(node string, slot time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sent == nil {
		g.sent = map[string]time.Time{}
	}
	if !slot.After(g.sent[node]) {
		return false
	}
	g.sent[node] = slot
	return true
}

// Release gives back a slot whose send failed.
func (g *LiveGate) Release(node string, slot time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sent[node].Equal(slot) {
		delete(g.sent, node)
	}
}
