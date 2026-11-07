package iotbridge

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// LiveNode is the node a topic carries: the level before the last, sensors (smarthome/ac/<node>/sensors); "" for any
// other topic under the subscription.
func LiveNode(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 || parts[len(parts)-1] != "sensors" {
		return ""
	}

	return parts[len(parts)-2]
}

// LiveTriggers converts one node message received at `when` on the scenario clock into readings of the node's device
// (IR635: the device registered for the node_id, with its unit and sensors). Each reading is stamped at the start of
// its scenario minute (the slot energy counts, D07), and its event ID is the node, field and minute, so the API keeps
// one reading per sensor and minute. The same readings then go to the unit's alert policies as facts (a demo
// automation trigger, IR529), and the device's heartbeat comes last: a state report with no settings (the nodes report
// none), which the API takes as heard only (IR528) and which keeps the unit's last seen time current. A message with
// nothing measured gives no triggers.
func LiveTriggers(dev NodeDevice, payload []byte, when time.Time) ([]Trigger, error) {
	var msg map[string]any

	err := json.Unmarshal(payload, &msg)
	if err != nil {
		return nil, fmt.Errorf("node %s: %w", dev.Node, err)
	}

	when = when.UTC()
	slot := when.Truncate(time.Minute)

	out := liveReadings(dev, msg, slot, when)
	if len(out) == 0 {
		return nil, nil
	}
	// the same readings as facts of the unit's alert policies (IR529): a rule over a threshold alerts and notifies
	facts := make([]Fact, 0, len(out))

	for _, tr := range out {
		measurement := tr.Measurement
		facts = append(
			facts,
			Fact{
				UnitID:     measurement.UnitID,
				Metric:     measurement.Metric,
				Value:      *measurement.Value,
				Unit:       measurement.Unit,
				ObservedAt: slot,
				Quality:    qualityValid,
			},
		)
	}

	out = append(
		out,
		liveEvaluation(
			fmt.Sprintf("live/%s/automation/%s", dev.Node, slot.Format(time.RFC3339)),
			slot,
			[]uuid.UUID{dev.UnitID},
			facts,
		),
		liveHeartbeat(dev.DeviceID, slot),
	)

	return out, nil
}

// liveReadings are the readings of one node message: each sensor of the device whose metric the bridge reads and whose
// field the node measured (its gate is not zero), at the minute slot, with the node, field and minute as its event ID.
func liveReadings(dev NodeDevice, msg map[string]any, slot, when time.Time) []Trigger {
	num := func(k string) (float64, bool) {
		v, ok := msg[k].(float64)

		return v, ok
	}

	fields := NodeFields()

	var out []Trigger

	for _, sensor := range dev.Sensors {
		field, ok := fields[sensor.Metric]
		if !ok {
			continue
		}

		gate, ok := num(field.Gate)

		raw, has := num(field.Field)
		if !ok || gate == 0 || !has {
			continue
		}

		value := raw * field.Scale
		event := uuid.NewSHA1(
			seed.Namespace(),
			[]byte(fmt.Sprintf("live/%s/%s/%s", dev.Node, field.Field, slot.Format(time.RFC3339))),
		)
		out = append(out, Trigger{
			ScenarioID: Scenario,
			EventID:    event,
			OccurredAt: slot,
			EventType:  "telemetry",
			Measurement: &Measurement{
				UnitID:     dev.UnitID,
				SensorID:   sensor.ID,
				Metric:     sensor.Metric,
				Value:      &value,
				Unit:       sensor.Unit,
				ObservedAt: slot,
				ReceivedAt: when,
				Origin:     "measured",
				Quality:    qualityValid,
				EventID:    event, Sequence: nil,
			}, CommandID: nil, DeviceID: nil, State: nil, Event: nil, BindingID: nil, Sequence: nil, Kind: "", Recovery: nil,
		})
	}

	return out
}

// liveHeartbeat is a live module's heartbeat of the minute slot: a state report with no settings (the nodes report
// none), which the API takes as heard only (IR528). It is one a minute for the module, whichever node is heard first:
// every node of the module sends the same body.
func liveHeartbeat(dev uuid.UUID, slot time.Time) Trigger {
	return Trigger{
		ScenarioID: Scenario,
		EventID: uuid.NewSHA1(
			seed.Namespace(),
			[]byte(fmt.Sprintf("%s/state/%s", dev, slot.Format(time.RFC3339))),
		),
		OccurredAt: slot,
		EventType:  eventState,
		DeviceID:   &dev,
		State: &devicesim.ReportedState{
			Power:    nil,
			Celsius:  nil,
			Mode:     nil,
			FanLevel: nil,
		},
		CommandID:   nil,
		Measurement: nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil,
	}
}

// liveEvaluation is a demo automation trigger of the facts of live units, with an event ID from key.
func liveEvaluation(key string, when time.Time, units []uuid.UUID, facts []Fact) Trigger {
	event := uuid.NewSHA1(seed.Namespace(), []byte(key))

	return Trigger{
		ScenarioID: Scenario,
		EventID:    event,
		OccurredAt: when,
		EventType:  eventAutomation,
		Event: &Evaluation{
			EventID:    event,
			OccurredAt: when,
			UnitIDs:    units,
			Facts:      facts,
			Phase:      "condition",
		}, CommandID: nil, Measurement: nil, DeviceID: nil, State: nil, BindingID: nil, Sequence: nil, Kind: "",
		Recovery: nil,
	}
}

// gapTenths is the step of the heartbeat gap the bridge sends: it is floored to a tenth of a minute.
const gapTenths = 10

// LiveGapTrigger reports, at the scenario minute slot, how long a live unit's module has gone unheard: the longest time
// in minutes since the last message of one of its nodes (heartbeat_gap). The default rule ac_offline alerts at 15
// minutes and resolves under 1 (IR120), so a node that stops, or a broker out of reach, is noticed without the nodes'
// help (IR529).
func LiveGapTrigger(unit uuid.UUID, gap time.Duration, slot time.Time) Trigger {
	slot = slot.UTC()
	minutes := math.Floor(gap.Minutes()*gapTenths) / gapTenths

	return liveEvaluation(
		"live/gap/"+unit.String()+"/"+slot.Format(time.RFC3339),
		slot,
		[]uuid.UUID{unit},
		[]Fact{
			{
				UnitID:     unit,
				Metric:     "heartbeat_gap",
				Value:      minutes,
				Unit:       "min",
				ObservedAt: slot,
				Quality:    qualityValid,
			},
		},
	)
}

// A live module goes offline once one of its nodes has gone unheard for LiveOfflineAfter and comes back once each of
// them has been heard within LiveOnlineWithin (its gap is the longest of its nodes'): a command sent while its node is
// silent never arrives, and the client's unit page disables its controls while the AC is offline (IR532).
const (
	LiveOfflineAfter = 3 * time.Minute
	LiveOnlineWithin = time.Minute
)

// ConnectionFault is devices.connectionFault's answer for a live module: its binding, its open connection fault (the
// DeviceEvent a restored names) and the newest sequence on the axis.
type ConnectionFault struct {
	BindingID *uuid.UUID `json:"bindingId"`
	FaultID   *uuid.UUID `json:"faultId"`
	Sequence  int64      `json:"sequence"`
}

// LiveConnectionTrigger is the device event that brings a live module's connection in line with its heartbeat gap:
// communication_lost once a node has been silent for LiveOfflineAfter while the API has no open fault, and restored,
// naming the open fault, once every node is heard again. ok is false when the connection already matches, or while the
// module is unbound. The sequence follows the API's newest on the axis, and the event IDs come from the binding and
// sequence or the fault, so a repeat is one event.
func LiveConnectionTrigger(
	dev uuid.UUID,
	fault ConnectionFault,
	gap time.Duration,
	when time.Time,
) (Trigger, bool) {
	if fault.BindingID == nil {
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
		}, false
	}

	seq := fault.Sequence + 1

	trigger := Trigger{
		ScenarioID:  Scenario,
		OccurredAt:  when.UTC(),
		EventType:   "device",
		DeviceID:    &dev,
		BindingID:   fault.BindingID,
		Sequence:    &seq,
		EventID:     uuid.UUID{},
		CommandID:   nil,
		Measurement: nil,
		State:       nil,
		Event:       nil,
		Kind:        "",
		Recovery:    nil,
	}
	switch {
	case fault.FaultID == nil && gap >= LiveOfflineAfter:
		trigger.Kind = "communication_lost"
		trigger.EventID = uuid.NewSHA1(
			seed.Namespace(),
			[]byte(fmt.Sprintf("live/lost/%s/%d", fault.BindingID, seq)),
		)
	case fault.FaultID != nil && gap < LiveOnlineWithin:
		trigger.Kind = "restored"
		trigger.Recovery = &Recovery{
			Axis:          "connection",
			SourceEventID: *fault.FaultID,
			Value:         connectionOnline,
		}
		trigger.EventID = uuid.NewSHA1(
			seed.Namespace(),
			[]byte("live/restored/"+fault.FaultID.String()),
		)
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
		}, false
	}

	return trigger, true
}

// LiveHeard keeps when each live node was last heard, for the heartbeat gap. A node not heard since the bridge started
// counts from its start. Times are kept on the wall clock: Go's monotonic clock stops while the machine sleeps (a
// laptop's Docker VM), and a gap measured on it stayed under 15 minutes through hours without the nodes.
type LiveHeard struct {
	mu    sync.Mutex
	start time.Time
	last  map[string]time.Time
}

// NewLiveHeard starts counting at start.
func NewLiveHeard(start time.Time) *LiveHeard {
	return &LiveHeard{start: start.Round(0), last: map[string]time.Time{}, mu: sync.Mutex{}}
}

// Heard notes a message of node at.
func (h *LiveHeard) Heard(node string, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if at = at.Round(0); at.After(h.last[node]) { // Round(0) drops the monotonic reading
		h.last[node] = at
	}
}

// GapOf is the longest time one of the nodes has gone unheard at now: a module's gap is that of its nodes.
func (h *LiveHeard) GapOf(nodes []string, now time.Time) time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()

	now = now.Round(0)

	var gap time.Duration

	for _, node := range nodes {
		last, ok := h.last[node]
		if !ok {
			last = h.start
		}

		if d := now.Sub(last); d > gap {
			gap = d
		}
	}

	return gap
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

// LiveCommandTopic is a node's command topic, as its firmware subscribes to it.
func LiveCommandTopic(node string) string { return "smarthome/ac/" + node + "/command" }

// LiveAction is the nodes' firmware command for a unit action, and the air conditioner's settings after it (nil: the
// command leaves the other settings as they were).
type LiveAction struct {
	Payload string
	State   *devicesim.ReportedState
}

// LiveCommand is the nodes' firmware command for a requested unit action. The firmware takes ON, OFF and COOL24 (on,
// cooling, 24 °C, fan auto). Power on sends COOL24: ON resumes whatever the board sent last, after a restart the
// infrared library's default, which need not be cooling. Power off sends OFF and leaves the other settings as they
// were (State nil). Any other action is one the firmware does not take (ok false).
func LiveCommand(raw json.RawMessage) (LiveAction, bool, error) {
	var (
		action struct {
			Kind  string `json:"kind"`
			Power *bool  `json:"power"`
		}
		none LiveAction
	)

	err := json.Unmarshal(raw, &action)
	if err != nil {
		return none, false, fmt.Errorf("live command action: %w", err)
	}

	if action.Kind != "set_power" || action.Power == nil {
		return none, false, nil
	}

	if !*action.Power {
		return LiveAction{Payload: "OFF", State: nil}, true, nil
	}

	on, celsius, cool := true, 24.0, "cool"

	return LiveAction{
		Payload: "COOL24",
		State: &devicesim.ReportedState{
			Power:    &on,
			Celsius:  &celsius,
			Mode:     &cool,
			FanLevel: nil,
		}, // fan auto: no level
	}, true, nil
}

// LiveSentTriggers report a command the bridge published to the command node at `at`: sent, then acknowledged at once
// (infrared is one-way: the node answers nothing, so the acknowledgement means the broker took the command), then,
// when the command sets more than its own action (COOL24), the module's state report with those settings at stateAt
// (no later than the API's clock, which refuses a report from its future).
func LiveSentTriggers(
	waiting Waiting,
	dev uuid.UUID,
	when, stateAt time.Time,
	state *devicesim.ReportedState,
) []Trigger {
	when = when.UTC()
	cmd := waiting.ID

	out := []Trigger{
		SentTrigger(waiting, when),
		{
			ScenarioID: Scenario,
			EventID: uuid.NewSHA1(
				seed.Namespace(),
				[]byte(fmt.Sprintf("%s/%s/command_ack", dev, cmd)),
			),
			OccurredAt:  when,
			EventType:   "command_ack",
			CommandID:   &cmd,
			Measurement: nil,
			DeviceID:    nil,
			State:       nil,
			Event:       nil,
			BindingID:   nil,
			Sequence:    nil,
			Kind:        "",
			Recovery:    nil,
		},
	}
	if state != nil {
		out = append(out, Trigger{
			ScenarioID: Scenario,
			EventID: uuid.NewSHA1(
				seed.Namespace(),
				[]byte(fmt.Sprintf("%s/state/%s", dev, cmd)),
			),
			OccurredAt:  stateAt.UTC(),
			EventType:   eventState,
			DeviceID:    &dev,
			State:       state,
			CommandID:   nil,
			Measurement: nil,
			Event:       nil,
			BindingID:   nil,
			Sequence:    nil,
			Kind:        "",
			Recovery:    nil,
		})
	}

	return out
}

// LiveRefusedTrigger fails a command the live module cannot carry out: an action the firmware does not take, or any
// command while sending to the nodes is off (REAL_MQTT_COMMANDS).
func LiveRefusedTrigger(w Waiting, dev uuid.UUID, when time.Time) Trigger {
	cmd := w.ID

	return Trigger{
		ScenarioID: Scenario,
		EventID: uuid.NewSHA1(
			seed.Namespace(),
			[]byte(fmt.Sprintf("%s/%s/command_fail", dev, cmd)),
		),
		OccurredAt:  when.UTC(),
		EventType:   "command_fail",
		CommandID:   &cmd,
		Measurement: nil,
		DeviceID:    nil,
		State:       nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil,
	}
}
