package iotbridge

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// LiveCommandTopic is a node's command topic, as its firmware subscribes to it.
func LiveCommandTopic(node string) string { return "smarthome/ac/" + node + "/command" }

// The settings firmware 0.4 takes (device-firmware.md §3.2): a whole-degree setpoint from 16 to 30 °C, and the modes
// and fan levels of the platform (the firmware's auto mode and auto fan are no platform values, so the bridge never
// sends them).
const (
	liveCelsiusMin = 16
	liveCelsiusMax = 30
)

// The firmware's words for the power, in its commands and its state.
const (
	livePowerOn  = "ON"
	livePowerOff = "OFF"
)

// The platform's modes and fan levels, which are also the firmware's words for them.
func liveModes() []string     { return []string{"cool", "dry", "fan"} }
func liveFanLevels() []string { return []string{"low", "mid", "high"} }

// LiveAction is the command a node gets for a unit action.
type LiveAction struct {
	Payload string
	// Answers is true for a JSON command: the node answers it on command/result (firmware 0.4). A text command is
	// answered by nobody, and the bridge acknowledges it once the broker took it.
	Answers bool
	// State is what a text command sets beyond its own action (COOL24), reported for the node, which reports nothing;
	// nil otherwise.
	State *devicesim.ReportedState
}

// liveUnitAction is the part of a stored UnitAction the nodes' firmware can carry out.
type liveUnitAction struct {
	Kind     string   `json:"kind"`
	Power    *bool    `json:"power"`
	Celsius  *float64 `json:"celsius"`
	Mode     *string  `json:"mode"`
	FanLevel *string  `json:"fanLevel"`
}

// liveCommandBody is firmware 0.4's JSON command: the command's ID, which the node's result names, and the one setting
// of the unit action.
type liveCommandBody struct {
	CommandID string  `json:"commandId"`
	Power     *string `json:"power,omitempty"`
	Temp      *int    `json:"temp,omitempty"`
	Mode      *string `json:"mode,omitempty"`
	Fan       *string `json:"fan,omitempty"`
}

// LiveCommand is the command a node gets for a requested unit action. A node that answers its commands (answers,
// LiveFirmware) gets firmware 0.4's JSON: set_power → power ON/OFF, set_temperature → temp (16–30 °C, whole degrees),
// set_mode → mode, set_fan → fan. Any other node gets the first firmware's text, which carries the power only: power
// on is COOL24 (on, cooling, 24 °C, fan auto; ON resumes whatever the board sent last, after a restart the infrared
// library's default), power off is OFF. An action the node's firmware does not take is not sent (ok false).
func LiveCommand(waiting Waiting, answers bool) (LiveAction, bool, error) {
	var (
		action liveUnitAction
		none   LiveAction
	)

	err := json.Unmarshal(waiting.Action, &action)
	if err != nil {
		return none, false, fmt.Errorf("live command action: %w", err)
	}

	if !answers {
		return liveTextCommand(action)
	}

	body := liveCommandBody{
		CommandID: waiting.ID.String(),
		Power:     nil,
		Temp:      nil,
		Mode:      nil,
		Fan:       nil,
	}

	switch {
	case action.Kind == "set_power" && action.Power != nil:
		power := livePowerOff
		if *action.Power {
			power = livePowerOn
		}

		body.Power = &power
	case action.Kind == "set_temperature" && action.Celsius != nil:
		celsius := *action.Celsius
		if celsius != math.Trunc(celsius) || celsius < liveCelsiusMin || celsius > liveCelsiusMax {
			return none, false, nil
		}

		temp := int(celsius)
		body.Temp = &temp
	case action.Kind == "set_mode" && action.Mode != nil && slices.Contains(liveModes(), *action.Mode):
		body.Mode = action.Mode
	case action.Kind == "set_fan" && action.FanLevel != nil && slices.Contains(liveFanLevels(), *action.FanLevel):
		body.Fan = action.FanLevel
	default:
		return none, false, nil
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return none, false, fmt.Errorf("live command %s: %w", waiting.ID, err)
	}

	return LiveAction{Payload: string(raw), Answers: true, State: nil}, true, nil
}

// liveTextCommand is the first firmware's command for a power action: COOL24 with the settings it runs, or OFF, which
// leaves the other settings as they were.
func liveTextCommand(action liveUnitAction) (LiveAction, bool, error) {
	if action.Kind != "set_power" || action.Power == nil {
		return LiveAction{Payload: "", Answers: false, State: nil}, false, nil
	}

	if !*action.Power {
		return LiveAction{Payload: livePowerOff, Answers: false, State: nil}, true, nil
	}

	powerOn, celsius, cool := true, 24.0, "cool"

	return LiveAction{
		Payload: "COOL24",
		Answers: false,
		State: &devicesim.ReportedState{
			Power:    &powerOn,
			Celsius:  &celsius,
			Mode:     &cool,
			FanLevel: nil,
		}, // fan auto: no level
	}, true, nil
}

// liveTrigger is a demo trigger of the live site with nothing but its type, event ID and time.
func liveTrigger(eventType string, event uuid.UUID, when time.Time) Trigger {
	return Trigger{
		ScenarioID:  Scenario,
		EventID:     event,
		OccurredAt:  when.UTC(),
		EventType:   eventType,
		CommandID:   nil,
		Measurement: nil,
		DeviceID:    nil,
		State:       nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil, OperationID: nil, Result: "",
	}
}

// liveCommandTrigger is a trigger of a command of a live device: kind is command_ack or command_fail.
func liveCommandTrigger(kind string, dev, cmd uuid.UUID, when time.Time) Trigger {
	trigger := liveTrigger(
		kind,
		uuid.NewSHA1(seed.Namespace(), []byte(fmt.Sprintf("%s/%s/%s", dev, cmd, kind))),
		when,
	)
	trigger.CommandID = &cmd

	return trigger
}

// LiveSentTriggers report a command the bridge published to its node at `when`: sent. A command nobody answers (a text
// command) is also acknowledged at once, as the broker took it, and when it sets more than its own action (COOL24) the
// module's state report with those settings follows at stateAt (no later than the API's clock, which refuses a report
// from its future). A JSON command is acknowledged or failed by the node's result (LiveResultTrigger).
func LiveSentTriggers(
	waiting Waiting,
	dev uuid.UUID,
	when, stateAt time.Time,
	action LiveAction,
) []Trigger {
	out := []Trigger{SentTrigger(waiting, when)}
	if action.Answers {
		return out
	}

	out = append(out, liveCommandTrigger("command_ack", dev, waiting.ID, when))

	if action.State != nil {
		state := liveTrigger(
			eventState,
			uuid.NewSHA1(seed.Namespace(), []byte(fmt.Sprintf("%s/state/%s", dev, waiting.ID))),
			stateAt,
		)
		state.DeviceID, state.State = &dev, action.State
		out = append(out, state)
	}

	return out
}

// LiveRefusedTrigger fails a command the live module cannot carry out: an action the node's firmware does not take, a
// unit whose model takes no commands, or any command while sending to the nodes is off (REAL_MQTT_COMMANDS).
func LiveRefusedTrigger(w Waiting, dev uuid.UUID, when time.Time) Trigger {
	return liveCommandTrigger("command_fail", dev, w.ID, when)
}

// nodeResult is a node's answer to a command (smarthome/ac/<node>/command/result): the command's ID as the bridge sent
// it (null for a command without one) and what the node did with it.
type nodeResult struct {
	CommandID *string `json:"commandId"`
	Result    string  `json:"result"`
}

// LiveResultTrigger turns a node's answer to a command into the demo trigger of the command, for the node's device:
// sent (the node sent the infrared) is command_ack; invalid (a setting out of range, or a command within a second of
// the last) and unsupported (a text command it does not know) are command_fail, which the API records as
// DEVICE_REJECTED. ok is false for an answer that names no command (one somebody else sent by hand) or whose result is
// none of these.
func LiveResultTrigger(dev uuid.UUID, payload []byte, when time.Time) (Trigger, bool, error) {
	var (
		result nodeResult
		none   Trigger
	)

	err := json.Unmarshal(payload, &result)
	if err != nil {
		return none, false, fmt.Errorf("command result: %w", err)
	}

	cmd, ok := commandOf(result.CommandID)
	if !ok {
		return none, false, nil
	}

	switch result.Result {
	case "sent":
		return liveCommandTrigger("command_ack", dev, cmd, when), true, nil
	case "invalid", "unsupported":
		return liveCommandTrigger("command_fail", dev, cmd, when), true, nil
	default:
		return none, false, nil
	}
}

// commandOf is the command a result names; ok is false for none, or an ID the bridge did not write (a UUID).
func commandOf(id *string) (uuid.UUID, bool) {
	if id == nil {
		return uuid.UUID{}, false
	}

	cmd, err := uuid.Parse(*id)

	return cmd, err == nil
}

// liveStateBody is a node's retained state (smarthome/ac/<node>/state): the settings it last sent the air conditioner.
// The mode and fan are read as raw JSON, as a board on firmware 0.4.0 sent them as numbers at start.
type liveStateBody struct {
	Power string          `json:"power"`
	Temp  *float64        `json:"temp"`
	Mode  json.RawMessage `json:"mode"`
	Fan   json.RawMessage `json:"fan"`
}

// LiveStateTrigger turns a node's state into the state report of its device at `when`: the power, the setpoint and
// the mode and fan level the platform knows. Auto mode and auto fan have no platform value and are reported as unknown
// (nil), as is a value of any other form. The node sends its state after every command it carries out and keeps it
// retained, so the bridge reads the settings when it connects. The event ID comes from the node and the time: a node
// that is set back to earlier settings reports them again.
func LiveStateTrigger(dev NodeDevice, payload []byte, when time.Time) (Trigger, error) {
	var body liveStateBody

	err := json.Unmarshal(payload, &body)
	if err != nil {
		return Trigger{}, fmt.Errorf("node %s state: %w", dev.Node, err)
	}

	when = when.UTC()
	state := devicesim.ReportedState{
		Power:    nil,
		Celsius:  nil,
		Mode:     liveWord(body.Mode, liveModes()),
		FanLevel: liveWord(body.Fan, liveFanLevels()),
	}

	switch body.Power {
	case livePowerOn, livePowerOff:
		on := body.Power == livePowerOn
		state.Power = &on
	}

	if body.Temp != nil && *body.Temp >= liveCelsiusMin && *body.Temp <= liveCelsiusMax {
		state.Celsius = body.Temp
	}

	trigger := liveTrigger(
		eventState,
		uuid.NewSHA1(
			seed.Namespace(),
			[]byte(fmt.Sprintf("live/%s/state/%s", dev.Node, when.Format(time.RFC3339Nano))),
		),
		when,
	)
	trigger.DeviceID, trigger.State = &dev.DeviceID, &state

	return trigger, nil
}

// liveWord is a setting of a node's state when it is a string among the words; nil otherwise.
func liveWord(raw json.RawMessage, words []string) *string {
	var word string
	if json.Unmarshal(raw, &word) != nil || !slices.Contains(words, word) {
		return nil
	}

	return &word
}

// liveAnswerGrace is how long after its expiry the bridge still takes a command's answer: the API decides a late
// acknowledgement by its own clock and records it (common design §5), so a late answer is passed on, not dropped.
const liveAnswerGrace = 10 * time.Minute

// LiveOutstanding keeps the JSON commands the bridge sent until their node answers, or until liveAnswerGrace after they
// expire, so that an answer counts only for a command the bridge sent to that node. The zero value is empty.
type LiveOutstanding struct {
	mu   sync.Mutex
	sent map[uuid.UUID]outstandingCommand
}

// outstandingCommand is a sent command's node and its expiry on the scenario clock.
type outstandingCommand struct {
	node    string
	expires time.Time
}

// Add notes a command sent to node, which the API ends at expires.
func (o *LiveOutstanding) Add(cmd uuid.UUID, node string, expires time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.sent == nil {
		o.sent = map[uuid.UUID]outstandingCommand{}
	}

	o.sent[cmd] = outstandingCommand{node: node, expires: expires}
}

// Take reports whether cmd is a command the bridge sent to node and still waits for at now (scenario clock), and
// forgets it; commands past their grace are forgotten on the way.
func (o *LiveOutstanding) Take(cmd uuid.UUID, node string, now time.Time) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	for id, sent := range o.sent {
		if now.After(sent.expires.Add(liveAnswerGrace)) {
			delete(o.sent, id)
		}
	}

	sent, ok := o.sent[cmd]
	if !ok || sent.node != node {
		return false
	}

	delete(o.sent, cmd)

	return true
}
