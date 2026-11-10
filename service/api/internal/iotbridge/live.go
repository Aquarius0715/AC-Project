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

// The topics a node publishes under smarthome/ac/<node>/ (device-firmware.md §2): its readings every 5 seconds, its
// answer to a command, its retained settings and its retained status (the heartbeat, or the last will).
const (
	LiveSensors = "sensors"
	LiveResult  = "command/result"
	LiveState   = "state"
	LiveStatus  = "status"
)

// LiveTopic is the node and the kind (LiveSensors, LiveResult, LiveState or LiveStatus) of a node's topic
// smarthome/ac/<node>/<kind>; ok is false for any other topic under the subscription, among them the commands the
// bridge publishes itself.
func LiveTopic(topic string) (string, string, bool) {
	parts := strings.Split(topic, "/")
	last := len(parts) - 1

	var node, kind string

	switch {
	case last >= 2 && parts[last-1]+"/"+parts[last] == LiveResult:
		node, kind = parts[last-2], LiveResult
	case last >= 1 && (parts[last] == LiveSensors || parts[last] == LiveState || parts[last] == LiveStatus):
		node, kind = parts[last-1], parts[last]
	}

	if node == "" {
		return "", "", false
	}

	return node, kind, true
}

// LiveTriggers converts one node message received at `when` on the scenario clock into readings of the node's device
// (IR635: the device registered for the node_id, with its unit and sensors). Each reading is stamped at the start of
// its scenario minute (the slot energy counts, D07), and its event ID is the node, field and minute, so the API keeps
// one reading per sensor and minute. The same readings then go to the unit's alert policies as facts (a demo
// automation trigger, IR529), and the device's heartbeat comes last: a state report with no settings (they come from
// the node's state topic, LiveStateTrigger), which the API takes as heard only (IR528) and which keeps the unit's last
// seen time current. A message with nothing measured gives no triggers.
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

// LiveReplayAfter is how old a node's reading must be to count as one it kept while it was offline: firmware 0.4 keeps
// up to 50 readings while it is cut off and sends them first when it is back (device-firmware.md §6).
const LiveReplayAfter = time.Minute

// LiveReplayedAt is when a node measured a reading it kept while offline, on the scenario clock: its timestamp (RFC
// 3339 with the node's zone, firmware 0.4) moved by the clock offset (scenario minus real), when that is more than
// LiveReplayAfter before the bridge received it (received, real time). ok is false for a reading of now, and for the
// first firmware's timestamp, which has no zone.
func LiveReplayedAt(payload []byte, received time.Time, offset time.Duration) (time.Time, bool) {
	var msg struct {
		Timestamp string `json:"timestamp"`
	}

	if json.Unmarshal(payload, &msg) != nil {
		return time.Time{}, false
	}

	at, err := time.Parse(time.RFC3339, msg.Timestamp)
	if err != nil || received.Sub(at) <= LiveReplayAfter {
		return time.Time{}, false
	}

	return at.Add(offset).UTC(), true
}

// LiveReplayTriggers convert a reading a node kept while offline into readings of the node's device at the minute it
// was measured (observed, on the scenario clock), received at `when`. They have the event IDs of that minute's
// readings, so a minute already recorded is not recorded again. They go to no alert policy and are no heartbeat: they
// tell how the room was, not how it is.
func LiveReplayTriggers(
	dev NodeDevice,
	payload []byte,
	observed, when time.Time,
) ([]Trigger, error) {
	var msg map[string]any

	err := json.Unmarshal(payload, &msg)
	if err != nil {
		return nil, fmt.Errorf("node %s: %w", dev.Node, err)
	}

	return liveReadings(dev, msg, observed.UTC().Truncate(time.Minute), when.UTC()), nil
}

// liveReadings are the readings of one node message: each sensor of the device whose metric the bridge reads and whose
// field the node measured (the field is there and its gate, if it has one, is not zero), at the minute slot, with the
// node, field and minute as its event ID.
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

		raw, has := num(field.Field)
		if !has {
			continue
		}

		if gate, ok := num(field.Gate); field.Gate != "" && (!ok || gate == 0) {
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
			OperationID: nil, Result: "",
		})
	}

	return out
}

// liveHeartbeat is a live module's heartbeat of the minute slot: a state report with no settings, which the API takes
// as heard only (IR528). It is one a minute for the module.
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
		Recovery:    nil, OperationID: nil, Result: "",
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
		Recovery: nil, OperationID: nil, Result: "",
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
			Recovery:    nil, OperationID: nil, Result: "",
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
		Recovery:    nil, OperationID: nil, Result: "",
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
			Recovery:    nil, OperationID: nil, Result: "",
		}, false
	}

	return trigger, true
}

// LiveHeard keeps when each live node was last heard, for the heartbeat gap, and when the broker last sent a node's
// last will. A node not heard since the bridge started counts from its start. Times are kept on the wall clock: Go's
// monotonic clock stops while the machine sleeps (a laptop's Docker VM), and a gap measured on it stayed under 15
// minutes through hours without the nodes.
type LiveHeard struct {
	mu    sync.Mutex
	start time.Time
	last  map[string]time.Time
	gone  map[string]time.Time
}

// NewLiveHeard starts counting at start.
func NewLiveHeard(start time.Time) *LiveHeard {
	return &LiveHeard{
		start: start.Round(0),
		last:  map[string]time.Time{},
		gone:  map[string]time.Time{},
		mu:    sync.Mutex{},
	}
}

// Heard notes a message of node at.
func (h *LiveHeard) Heard(node string, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if at = at.Round(0); at.After(h.last[node]) { // Round(0) drops the monotonic reading
		h.last[node] = at
	}
}

// Gone notes the node's last will received at: the broker lost the node ({"online":false} on its status). Until the
// node is heard after it, its gap is at least LiveOfflineAfter, so the module goes offline at the next report and not
// minutes later.
func (h *LiveHeard) Gone(node string, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if at = at.Round(0); at.After(h.gone[node]) {
		h.gone[node] = at
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

		d := now.Sub(last)
		if gone, ok := h.gone[node]; ok && !last.After(gone) {
			d = max(d, LiveOfflineAfter)
		}

		gap = max(gap, d)
	}

	return gap
}

// LiveFirmware keeps which nodes run firmware 0.4 (device-firmware.md): a node counts as one once it has sent a status
// with its firmware version. Such a node takes JSON commands and answers each on command/result; any other gets the
// first firmware's text commands (ON, OFF, COOL24), which it never answers. The zero value knows no node.
type LiveFirmware struct {
	mu      sync.Mutex
	answers map[string]bool
}

// NodeStatus is a node's retained status (smarthome/ac/<node>/status): its heartbeat every minute, with its firmware
// version, or the last will {"online":false} that the broker sends when it loses the node.
type NodeStatus struct {
	Online   bool   `json:"online"`
	Firmware string `json:"fw"`
}

// ParseLiveStatus reads a node's status.
func ParseLiveStatus(payload []byte) (NodeStatus, error) {
	var status NodeStatus

	err := json.Unmarshal(payload, &status)
	if err != nil {
		return status, fmt.Errorf("status: %w", err)
	}

	return status, nil
}

// Noted takes a node's status: one with a firmware version marks the node as one that answers its commands.
func (f *LiveFirmware) Noted(node string, status NodeStatus) {
	if status.Firmware == "" {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.answers == nil {
		f.answers = map[string]bool{}
	}

	f.answers[node] = true
}

// Answers reports whether the node takes JSON commands and answers them.
func (f *LiveFirmware) Answers(node string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.answers[node]
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

// liveMinutesKept is how far before a claimed minute LiveMinutes still remembers the minutes it let through.
const liveMinutesKept = 48 * time.Hour

// LiveMinutes lets one message per node and minute through, in any order: the readings a node kept while offline come
// 50 ms apart and are worked off side by side (the live client does not keep their order), so a later minute may come
// first, which LiveGate would take as the end of the earlier ones. A failed send gives the minute back. Minutes more
// than liveMinutesKept before the one claimed are forgotten. The zero value has let nothing through.
type LiveMinutes struct {
	mu      sync.Mutex
	claimed map[string]map[time.Time]bool
}

// Claim reports whether node's minute slot is still open and, if so, takes it.
func (m *LiveMinutes) Claim(node string, slot time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.claimed == nil {
		m.claimed = map[string]map[time.Time]bool{}
	}

	minutes := m.claimed[node]
	if minutes == nil {
		minutes = map[time.Time]bool{}
		m.claimed[node] = minutes
	}

	if minutes[slot] {
		return false
	}

	for minute := range minutes {
		if minute.Before(slot.Add(-liveMinutesKept)) {
			delete(minutes, minute)
		}
	}

	minutes[slot] = true

	return true
}

// Release gives back a slot whose send failed.
func (m *LiveMinutes) Release(node string, slot time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.claimed[node], slot)
}

// LiveCheck is a running connection check of a live node's device, as devices.mqttChecks answers it.
type LiveCheck struct {
	OperationID uuid.UUID `json:"operationId"`
	DeviceID    uuid.UUID `json:"deviceId"`
	Node        string    `json:"node"`
}

// LiveCheckTrigger answers a connection check of a live node's device at `when` (IR736): real nodes answer none, so a
// check would time out and leave the device in error while the node is heard. The bridge answers from the node's
// heartbeat gap: succeeded while the node counts as online (unheard for less than LiveOfflineAfter), which sets the
// device online; failed otherwise, which sets it in error until the minute's report takes it offline
// (LiveConnectionTrigger) and the node's return brings it back. The event ID comes from the operation, so a repeat is
// one event.
func LiveCheckTrigger(check LiveCheck, gap time.Duration, when time.Time) Trigger {
	operation := check.OperationID
	result := "failed"

	if gap < LiveOfflineAfter {
		result = "succeeded"
	}

	return Trigger{
		ScenarioID:  Scenario,
		EventID:     uuid.NewSHA1(seed.Namespace(), []byte("live/check/"+operation.String())),
		OccurredAt:  when.UTC(),
		EventType:   "operation",
		CommandID:   nil,
		Measurement: nil,
		DeviceID:    nil,
		State:       nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil,
		OperationID: &operation,
		Result:      result,
	}
}
