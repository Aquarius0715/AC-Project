package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/logging"
)

// liveHandler takes the live nodes' messages (smarthome/ac/<node>/…, device-firmware.md): the readings of the devices
// registered for them, one message per node and scenario minute, and the readings a node kept while offline at the
// minute it measured them; the settings of an air conditioner's node; the nodes' answers to the commands the bridge
// sent; and their status, whose last will takes a module offline at once. It skips retained readings, which are old,
// and takes the retained settings and status, which are the nodes' current ones when the bridge connects.
type liveHandler struct {
	clock    *demoClock
	httpc    *http.Client
	endpoint string
	nodes    *iotbridge.NodeRegistry

	heard       *iotbridge.LiveHeard
	firmware    *iotbridge.LiveFirmware
	outstanding *iotbridge.LiveOutstanding
	gate        *iotbridge.LiveGate    // the minutes of the readings of now
	replays     *iotbridge.LiveMinutes // the minutes of the readings a node kept while offline

	rep    *logging.Repeats // while the API is away, once a minute
	warned sync.Map         // nodes and errors already logged

	// held are the nodes' states that came before the registered nodes were first read (the retained ones come at
	// connect): each is reported with its node's next message
	heldMu sync.Mutex
	held   map[string][]byte
}

func newLiveHandler(
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
	nodes *iotbridge.NodeRegistry,
) *liveHandler {
	return &liveHandler{
		clock:       clock,
		httpc:       httpc,
		endpoint:    endpoint,
		nodes:       nodes,
		heard:       iotbridge.NewLiveHeard(time.Now()),
		firmware:    &iotbridge.LiveFirmware{},
		outstanding: &iotbridge.LiveOutstanding{},
		gate:        &iotbridge.LiveGate{},
		replays:     &iotbridge.LiveMinutes{},
		rep:         &logging.Repeats{Logf: log.Printf, Every: 0, Now: nil},
		warned:      sync.Map{},
		heldMu:      sync.Mutex{},
		held:        map[string][]byte{},
	}
}

// minuteGate lets one message per node and minute through (iotbridge.LiveGate, iotbridge.LiveMinutes).
type minuteGate interface {
	Claim(node string, slot time.Time) bool
	Release(node string, slot time.Time)
}

// once logs msg the first time key comes up.
func (h *liveHandler) once(key, msg string) {
	if _, seen := h.warned.LoadOrStore(key, true); !seen {
		log.Print(msg)
	}
}

// handle takes one message of the live broker. Every message that is not retained is a sign of life of its node.
func (h *liveHandler) handle(ctx context.Context, msg mqtt.Message) {
	node, kind, ok := iotbridge.LiveTopic(msg.Topic())
	if !ok {
		return
	}

	now := time.Now()
	if kind == iotbridge.LiveStatus {
		h.status(node, msg, now)

		return
	}

	if msg.Retained() {
		if kind == iotbridge.LiveSensors { // a reading kept on the broker is an old one
			return
		}
	} else {
		h.heard.Heard(node, now)
	}

	dev, registered := h.nodes.Node(node)
	if !registered {
		if !h.nodes.Loaded() { // before the first read every node is unknown
			if kind == iotbridge.LiveState {
				h.hold(node, msg.Payload())
			}

			return
		}

		h.once("node/"+node, fmt.Sprintf(
			"live %s: no device is registered with MQTT node ID %s; register one in Admin's devices (logged once)",
			msg.Topic(),
			strconv.Quote(node),
		))

		return
	}

	off := h.clock.offset()
	when := now.Add(off - clockMargin).UTC()

	if held, ok := h.unhold(node); ok && kind != iotbridge.LiveState {
		h.state(ctx, dev, held, when)
	}

	switch kind {
	case iotbridge.LiveSensors:
		h.readings(ctx, dev, msg, now, off, when)
	case iotbridge.LiveState:
		h.state(ctx, dev, msg.Payload(), when)
	case iotbridge.LiveResult: // an answer at the time it came, as the modules' cmd/ack: it follows the report sent
		h.result(ctx, dev, msg, now.Add(off).UTC())
	}
}

// hold keeps a node's state until its node is known; a later state replaces it.
func (h *liveHandler) hold(node string, payload []byte) {
	h.heldMu.Lock()
	defer h.heldMu.Unlock()

	h.held[node] = payload
}

// unhold is the state held for a node, which it forgets.
func (h *liveHandler) unhold(node string) ([]byte, bool) {
	h.heldMu.Lock()
	defer h.heldMu.Unlock()

	payload, ok := h.held[node]
	delete(h.held, node)

	return payload, ok
}

// status takes a node's status: its heartbeat (not a retained one) marks it heard and tells its firmware; its last will
// marks it gone.
func (h *liveHandler) status(node string, msg mqtt.Message, now time.Time) {
	status, err := iotbridge.ParseLiveStatus(msg.Payload())
	if err != nil {
		h.once("status/"+node, fmt.Sprintf("live %s: %v (logged once)", msg.Topic(), err))

		return
	}

	h.firmware.Noted(node, status)

	if status.Online {
		if !msg.Retained() { // the broker replays a retained status on a reconnect: not a sign of life
			h.heard.Heard(node, now)
		}

		return
	}

	h.heard.Gone(node, now)
	log.Printf("live node %s: the broker lost it (last will); offline at the next report", node)
}

// readings records a node's readings: those of now on the minute the bridge received them, one message per minute,
// and those the node kept while offline on the minute it measured them (received at now, real time).
func (h *liveHandler) readings(
	ctx context.Context,
	dev iotbridge.NodeDevice,
	msg mqtt.Message,
	now time.Time,
	off time.Duration,
	when time.Time,
) {
	var (
		gate minuteGate = h.gate
		slot            = when.Truncate(time.Minute)
	)

	observed, replayed := iotbridge.LiveReplayedAt(msg.Payload(), now, off)
	if replayed {
		gate, slot = h.replays, observed.Truncate(time.Minute)
	}

	if !gate.Claim(dev.Node, slot) {
		return
	}

	var (
		triggers []iotbridge.Trigger
		err      error
	)

	if replayed {
		triggers, err = iotbridge.LiveReplayTriggers(dev, msg.Payload(), observed, when)
	} else {
		triggers, err = iotbridge.LiveTriggers(dev, msg.Payload(), when)
	}

	if err != nil || len(triggers) == 0 {
		gate.Release(dev.Node, slot) // the node's next message of the minute may have a reading

		if err != nil {
			h.once("convert/"+dev.Node, fmt.Sprintf("live %s: %v (logged once)", msg.Topic(), err))
		}

		return
	}

	for _, tr := range triggers {
		err := send(ctx, h.httpc, h.endpoint, tr)
		if err != nil && !recorded(err) {
			gate.Release(dev.Node, slot)
			h.rep.Failed(fmt.Sprintf("live %s: %v", dev.Node, err))

			return
		}
	}

	h.rep.OK("live readings")
}

// state reports the settings of an air conditioner's node for its device; a sensor unit's node has none to show.
func (h *liveHandler) state(
	ctx context.Context,
	dev iotbridge.NodeDevice,
	payload []byte,
	when time.Time,
) {
	if !dev.Control {
		return
	}

	trigger, err := iotbridge.LiveStateTrigger(dev, payload, when)
	if err != nil {
		h.once("state/"+dev.Node, fmt.Sprintf("live node %s: %v (logged once)", dev.Node, err))

		return
	}

	sendErr := send(ctx, h.httpc, h.endpoint, trigger)
	if sendErr != nil && !recorded(sendErr) {
		log.Printf("live node %s: state: %v", dev.Node, sendErr)
	}
}

// result passes on a node's answer to a command the bridge sent it: acknowledged or failed.
func (h *liveHandler) result(
	ctx context.Context,
	dev iotbridge.NodeDevice,
	msg mqtt.Message,
	when time.Time,
) {
	trigger, ok, err := iotbridge.LiveResultTrigger(dev.DeviceID, msg.Payload(), when)
	if err != nil {
		h.once("result/"+dev.Node, fmt.Sprintf("live %s: %v (logged once)", msg.Topic(), err))

		return
	}

	if !ok || !h.outstanding.Take(*trigger.CommandID, dev.Node, when) {
		return // a command somebody else sent the node, or one the bridge no longer waits for
	}

	log.Printf("live command %s: %s by node %s", *trigger.CommandID, trigger.EventType, dev.Node)

	sendErr := sendRetried(ctx, h.httpc, h.endpoint, trigger)
	if sendErr != nil && !recorded(sendErr) {
		log.Printf("live command %s: %s: %v", *trigger.CommandID, trigger.EventType, sendErr)
	}
}

// refused is true when the API answered with a client error: trying the same report again would be refused again.
func refused(err error) bool {
	var se *statusError

	return errors.As(err, &se) && se.code >= http.StatusBadRequest &&
		se.code < http.StatusInternalServerError
}

// The answer of a node is taken from the broker once and has no second delivery, so passing it on to the API is tried
// a few times before it is given up.
const (
	reportTries     = 3
	reportRetryWait = 200 * time.Millisecond
)

// sendRetried is send, tried up to reportTries times while the API does not take it (not for an answer that it
// already has or refuses for good, nor after the context ends).
func sendRetried(
	ctx context.Context,
	httpc *http.Client,
	endpoint string,
	trigger iotbridge.Trigger,
) error {
	var err error

	for try := range reportTries {
		err = send(ctx, httpc, endpoint, trigger)
		if err == nil || recorded(err) || refused(err) || try == reportTries-1 {
			return err
		}

		select {
		case <-ctx.Done():
			return err
		case <-time.After(reportRetryWait):
		}
	}

	return err
}

// liveCommandSpacing keeps one node's commands apart: firmware 0.4 refuses (invalid) a command that comes within a
// second of the last one it carried out.
const liveCommandSpacing = 1100 * time.Millisecond

// liveControl carries the live units' commands to the nodes' broker (DEC-89): the command of the node's firmware, on
// the topic of the node of the commanded unit's device (IR635). Sending is on only with REAL_MQTT_COMMANDS=1; without
// it a command of a live unit fails.
type liveControl struct {
	client      mqtt.Client
	nodes       *iotbridge.NodeRegistry
	firmware    *iotbridge.LiveFirmware
	outstanding *iotbridge.LiveOutstanding
	enabled     bool
	// last is when each node last got a command; only pollCommands sends, one command at a time
	last map[string]time.Time
	// published are the commands on the broker whose "sent" report the API has not taken yet: the next poll lists them
	// as requested again and reports them, but never publishes them to the node a second time
	published map[uuid.UUID]publishedCommand
}

// publishedCommand is a command that was published to its node: what was sent, when, and when the API ends it.
type publishedCommand struct {
	command iotbridge.LiveAction
	sentAt  time.Time
	expires time.Time
}

func newLiveControl(
	client mqtt.Client,
	nodes *iotbridge.NodeRegistry,
	firmware *iotbridge.LiveFirmware,
	outstanding *iotbridge.LiveOutstanding,
	enabled bool,
) *liveControl {
	if enabled {
		log.Print(
			"live commands: on (REAL_MQTT_COMMANDS); they go to the node of the commanded unit's device",
		)
	} else {
		log.Print("live commands: off (REAL_MQTT_COMMANDS); commands of the live units fail")
	}

	return &liveControl{
		client:      client,
		nodes:       nodes,
		firmware:    firmware,
		outstanding: outstanding,
		enabled:     enabled,
		last:        map[string]time.Time{},
		published:   map[uuid.UUID]publishedCommand{},
	}
}

// node is the registered node of a device; ok is false for a device that is not a live one.
func (l *liveControl) node(device uuid.UUID) (iotbridge.NodeDevice, bool) {
	return l.nodes.Device(device)
}

// send carries one requested command of a live unit to its device's node (QoS 1), then reports it sent. A node on
// firmware 0.4 gets the JSON command and answers it on command/result (liveHandler.result); any other gets the first
// firmware's text command for the power, which the bridge acknowledges with the settings it runs with. An action the
// node's firmware does not take, a unit whose model takes no commands, or any while sending is off, fails. While the
// broker is out of reach the command stays requested, and the API ends it at its expiry.
func (l *liveControl) send(
	ctx context.Context,
	httpc *http.Client,
	endpoint string,
	dev iotbridge.NodeDevice,
	waiting iotbridge.Waiting,
	off time.Duration,
) error {
	l.forgetEnded(time.Now().Add(off))

	command, supported, err := iotbridge.LiveCommand(waiting, l.firmware.Answers(dev.Node))
	if err != nil {
		return apperr.Wrap(err, "iotbridge.LiveCommand")
	}

	if !supported || !dev.Control || !l.enabled {
		return send(
			ctx,
			httpc,
			endpoint,
			iotbridge.LiveRefusedTrigger(waiting, dev.DeviceID, time.Now().Add(off)),
		)
	}

	sent, again := l.published[waiting.ID]
	if !again {
		var publishErr error

		sent, publishErr = l.publish(ctx, dev, waiting, command)
		if publishErr != nil {
			return publishErr
		}

		l.published[waiting.ID] = sent
	}

	when := sent.sentAt.Add(off)
	for _, tr := range iotbridge.LiveSentTriggers(waiting, dev.DeviceID, when, when.Add(-clockMargin), sent.command) {
		err := send(ctx, httpc, endpoint, tr)
		if err != nil && !recorded(err) { // the API already has it: its reply was lost earlier
			return err
		}
	}

	delete(l.published, waiting.ID)

	return nil
}

// publish carries the command to its node's command topic (QoS 1) and notes it as published.
func (l *liveControl) publish(
	ctx context.Context,
	dev iotbridge.NodeDevice,
	waiting iotbridge.Waiting,
	command iotbridge.LiveAction,
) (publishedCommand, error) {
	var none publishedCommand

	if !l.client.IsConnectionOpen() {
		return none, errLiveBrokerDown
	}

	l.space(ctx, dev.Node)

	if command.Answers { // before the publish: the answer may come before Publish returns
		l.outstanding.Add(waiting.ID, dev.Node, waiting.ExpiresAt)
	}

	topic := iotbridge.LiveCommandTopic(dev.Node)

	tok := l.client.Publish(topic, 1, false, command.Payload)
	if !tok.WaitTimeout(liveCommandWait) {
		return none, errLiveCommandTimeout
	}

	publishErr := tok.Error()
	if publishErr != nil {
		return none, apperr.Wrap(publishErr, "mqtt.Token.Error")
	}

	sentAt := time.Now()
	l.last[dev.Node] = sentAt

	log.Printf(
		"live command %s: %s → %s",
		waiting.ID,
		strconv.Quote(command.Payload),
		strconv.Quote(topic),
	)

	return publishedCommand{command: command, sentAt: sentAt, expires: waiting.ExpiresAt}, nil
}

// forgetEnded drops the published commands whose report was never taken and that the API has ended by now (scenario
// clock), so the API no longer lists them.
func (l *liveControl) forgetEnded(now time.Time) {
	for id, sent := range l.published {
		if now.After(sent.expires) {
			delete(l.published, id)
		}
	}
}

// space waits until the node's last command is liveCommandSpacing old, or the context ends.
func (l *liveControl) space(ctx context.Context, node string) {
	wait := time.Until(l.last[node].Add(liveCommandSpacing))
	if wait <= 0 {
		return
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// answerChecks answers the connection checks of the live nodes' devices every commandPoll until the context ends
// (devices.mqttChecks, IR736): a check runs for at most a minute, and real nodes answer none. A read that fails is
// logged once a minute while it keeps failing.
func answerChecks(
	ctx context.Context,
	heard *iotbridge.LiveHeard,
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
	api liveAPI,
) {
	ticker := time.NewTicker(commandPoll)
	defer ticker.Stop()

	rep := &logging.Repeats{Logf: log.Printf, Every: 0, Now: nil}

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			checks, err := fetchChecks(ctx, httpc, api)
			if err != nil {
				rep.Failed(fmt.Sprintf("live connection checks: %v", err))

				continue
			}

			rep.OK("live connection checks")

			when := now.Add(clock.offset() - clockMargin)

			for _, check := range checks {
				answerCheck(
					ctx,
					httpc,
					endpoint,
					check,
					heard.GapOf([]string{check.Node}, now),
					when,
				)
			}
		}
	}
}

// answerCheck sends the answer to one check and logs it.
func answerCheck(
	ctx context.Context,
	httpc *http.Client,
	endpoint string,
	check iotbridge.LiveCheck,
	gap time.Duration,
	when time.Time,
) {
	trigger := iotbridge.LiveCheckTrigger(check, gap, when)

	err := send(ctx, httpc, endpoint, trigger)
	if err != nil && !recorded(err) {
		log.Printf("live check %s of node %s: %v", check.OperationID, check.Node, err)

		return
	}

	log.Printf(
		"live check %s of node %s: %s (unheard for %s)",
		check.OperationID,
		check.Node,
		trigger.Result,
		gap.Round(time.Second),
	)
}

// fetchChecks reads the running connection checks of the live nodes' devices (devices.mqttChecks, IR736).
func fetchChecks(
	ctx context.Context,
	client *http.Client,
	api liveAPI,
) ([]iotbridge.LiveCheck, error) {
	var data struct {
		Checks []iotbridge.LiveCheck `json:"checks"`
	}

	err := liveQuery(ctx, client, api, "mqttChecks", `{}`, &data)

	return data.Checks, err
}
