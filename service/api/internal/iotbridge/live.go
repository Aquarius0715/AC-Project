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

// LiveField is one field of a real sensor node's message and the sensor of the live module it is recorded as (DEC-86,
// IR524).
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

// LiveTriggers converts one node message received at `at` on the scenario clock into readings of the live unit and
// module (seed.LiveSite: both nodes are one module, DEC-89). Each reading is stamped at the start of its scenario
// minute (the slot energy counts, D07), and its event ID is the node, field and minute, so the API keeps one reading
// per sensor and minute. The module's heartbeat comes with them: a state report with no settings (the nodes report
// none), which the API takes as heard only (IR528) and which keeps the unit's last seen time current. A node the
// bridge does not know is an error; a message with nothing measured gives no triggers.
func LiveTriggers(node string, payload []byte, at time.Time) ([]Trigger, error) {
	fields, ok := LiveNodes[node]
	if !ok {
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
	for _, s := range seed.LiveSite.Sensors {
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
			return nil, fmt.Errorf("node %s: the live module has no sensor %s", node, f.Sensor)
		}
		value := v * f.Scale
		event := uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("live/%s/%s/%s", node, f.Field, slot.Format(time.RFC3339))))
		out = append(out, Trigger{
			ScenarioID: Scenario,
			EventID:    event,
			OccurredAt: slot,
			EventType:  "telemetry",
			Measurement: &Measurement{
				UnitID: seed.ID(seed.LiveSite.Unit), SensorID: seed.ID(s.Name), Metric: s.Metric, Value: &value, Unit: s.Unit,
				ObservedAt: slot, ReceivedAt: at, Origin: "measured", Quality: "valid", EventID: event,
			},
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	dev := seed.ID(seed.LiveSite.Device)
	out = append(out, Trigger{
		ScenarioID: Scenario,
		// one a minute for the module, whichever node is heard first: both nodes send the same body
		EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/state/%s", dev, slot.Format(time.RFC3339)))),
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

// LiveCommandNode is the node whose infrared LED sends the commands by default (REAL_MQTT_COMMAND_NODE): both nodes have
// one, and only one sends, so a command does not reach the air conditioner twice.
const LiveCommandNode = "node_1"

// LiveCommandTopic is a node's command topic, as its firmware subscribes to it.
func LiveCommandTopic(node string) string { return "smarthome/ac/" + node + "/command" }

// LiveCommand is the nodes' firmware command for a requested unit action, and the air conditioner's settings after it.
// The firmware takes ON, OFF and COOL24 (on, cooling, 24 °C, fan auto). Power on sends COOL24: ON resumes whatever the
// board sent last, after a restart the infrared library's default, which need not be cooling. Power off sends OFF and
// leaves the other settings as they were (state nil). Any other action is one the firmware does not take (ok false).
func LiveCommand(action json.RawMessage) (payload string, state *devicesim.ReportedState, ok bool, err error) {
	var a struct {
		Kind  string `json:"kind"`
		Power *bool  `json:"power"`
	}
	if err := json.Unmarshal(action, &a); err != nil {
		return "", nil, false, fmt.Errorf("live command action: %w", err)
	}
	if a.Kind != "set_power" || a.Power == nil {
		return "", nil, false, nil
	}
	if !*a.Power {
		return "OFF", nil, true, nil
	}
	on, celsius, cool := true, 24.0, "cool"
	return "COOL24", &devicesim.ReportedState{Power: &on, Celsius: &celsius, Mode: &cool}, true, nil // fan auto has no level
}

// LiveSentTriggers report a command the bridge published to the command node at `at`: sent, then acknowledged at once
// (infrared is one-way: the node answers nothing, so the acknowledgement means the broker took the command), then,
// when the command sets more than its own action (COOL24), the module's state report with those settings at stateAt
// (no later than the API's clock, which refuses a report from its future).
func LiveSentTriggers(w Waiting, at, stateAt time.Time, state *devicesim.ReportedState) []Trigger {
	at = at.UTC()
	cmd, dev := w.ID, seed.ID(seed.LiveSite.Device)
	out := []Trigger{SentTrigger(w, at), {
		ScenarioID: Scenario,
		EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/%s/command_ack", dev, cmd))),
		OccurredAt: at,
		EventType:  "command_ack",
		CommandID:  &cmd,
	}}
	if state != nil {
		out = append(out, Trigger{
			ScenarioID: Scenario,
			EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/state/%s", dev, cmd))),
			OccurredAt: stateAt.UTC(),
			EventType:  "state",
			DeviceID:   &dev,
			State:      state,
		})
	}
	return out
}

// LiveRefusedTrigger fails a command the live module cannot carry out: an action the firmware does not take, or any
// command while sending to the nodes is off (REAL_MQTT_COMMANDS).
func LiveRefusedTrigger(w Waiting, at time.Time) Trigger {
	cmd := w.ID
	return Trigger{
		ScenarioID: Scenario,
		EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/%s/command_fail", seed.ID(seed.LiveSite.Device), cmd))),
		OccurredAt: at.UTC(),
		EventType:  "command_fail",
		CommandID:  &cmd,
	}
}
