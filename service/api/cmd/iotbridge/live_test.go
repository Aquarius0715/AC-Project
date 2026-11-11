package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// fakeMessage is a message of the live broker.
type fakeMessage struct {
	topic    string
	payload  string
	retained bool
}

func (*fakeMessage) Duplicate() bool   { return false }
func (*fakeMessage) Qos() byte         { return 0 }
func (m *fakeMessage) Retained() bool  { return m.retained }
func (m *fakeMessage) Topic() string   { return m.topic }
func (*fakeMessage) MessageID() uint16 { return 0 }
func (m *fakeMessage) Payload() []byte { return []byte(m.payload) }
func (*fakeMessage) Ack()              {}
func message(topic, payload string) *fakeMessage {
	return &fakeMessage{topic: topic, payload: payload, retained: false}
}

// doneToken is a publish the broker took at once.
type doneToken struct{}

func (doneToken) Wait() bool                     { return true }
func (doneToken) WaitTimeout(time.Duration) bool { return true }
func (doneToken) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)

	return done
}
func (doneToken) Error() error { return nil }

// fakeBroker is a connected live broker that keeps what is published; the client's other methods are not used.
type fakeBroker struct {
	mqtt.Client

	mu        sync.Mutex
	published map[string][]string
}

func (*fakeBroker) IsConnectionOpen() bool { return true }

//nolint:ireturn // mqtt.Client's Publish returns the Token interface, which the fake implements
func (b *fakeBroker) Publish(topic string, _ byte, _ bool, payload any) mqtt.Token {
	b.mu.Lock()
	defer b.mu.Unlock()

	text, _ := payload.(string)
	b.published[topic] = append(b.published[topic], text)

	return doneToken{}
}

// triggerLog is the demo trigger endpoint of a test: the triggers it took, in order.
type triggerLog struct {
	mu  sync.Mutex
	got []iotbridge.Trigger
}

func (l *triggerLog) types() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]string, 0, len(l.got))
	for _, tr := range l.got {
		out = append(out, tr.EventType)
	}

	return out
}

func (l *triggerLog) last() iotbridge.Trigger {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.got[len(l.got)-1]
}

func (l *triggerLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.got = nil
}

// liveFixture is a live handler and control over the seed's live site, with a recording API and broker.
type liveFixture struct {
	handler *liveHandler
	control *liveControl
	broker  *fakeBroker
	api     *triggerLog
	srv     *httptest.Server
	air     iotbridge.NodeDevice
	sensors iotbridge.NodeDevice
}

func newLiveFixture(t *testing.T) *liveFixture {
	t.Helper()

	api := &triggerLog{mu: sync.Mutex{}, got: nil}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var trigger iotbridge.Trigger

		err := json.NewDecoder(r.Body).Decode(&trigger)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)

			return
		}

		api.mu.Lock()
		api.got = append(api.got, trigger)
		api.mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	modules := seed.LiveSite().Modules()
	devices := make([]iotbridge.NodeDevice, 0, len(modules))

	for _, module := range modules {
		sensors := make([]iotbridge.NodeSensor, 0, len(module.Sensors))
		for _, s := range module.Sensors {
			sensors = append(
				sensors,
				iotbridge.NodeSensor{ID: seed.ID(s.Name), Metric: s.Metric, Unit: s.Unit},
			)
		}

		devices = append(devices, iotbridge.NodeDevice{
			Node:     module.Node,
			DeviceID: seed.ID(module.Device),
			UnitID:   seed.ID(module.Unit),
			Control:  module.Control,
			Sensors:  sensors,
		})
	}

	nodes := iotbridge.NewNodeRegistry()
	nodes.Set(devices)

	httpc := &http.Client{Timeout: 5 * time.Second, Transport: nil, CheckRedirect: nil, Jar: nil}
	clock := &demoClock{api: "", c: httpc, mu: sync.Mutex{}, off: 0}
	handler := newLiveHandler(clock, httpc, srv.URL, nodes)
	broker := &fakeBroker{Client: nil, mu: sync.Mutex{}, published: map[string][]string{}}

	return &liveFixture{
		handler: handler,
		control: newLiveControl(broker, nodes, handler.firmware, handler.outstanding, true),
		broker:  broker,
		api:     api,
		srv:     srv,
		air:     devices[0],
		sensors: devices[1],
	}
}

func (f *liveFixture) command(t *testing.T, action string) iotbridge.Waiting {
	t.Helper()

	waiting := iotbridge.Waiting{
		ID:        uuid.New(),
		DeviceID:  f.air.DeviceID,
		UnitID:    f.air.UnitID,
		Action:    json.RawMessage(action),
		ExpiresAt: time.Now().Add(time.Minute),
	}

	err := f.control.send(t.Context(), f.handler.httpc, f.srv.URL, f.air, waiting, 0)
	if err != nil {
		t.Fatal(err)
	}

	return waiting
}

// TestLiveControlJSONNode: a node that sent a firmware-0.4 status gets JSON commands, reported sent only; its answer
// acknowledges the command once, an answer to a command the bridge did not send is dropped, and an invalid one fails
// its command.
func TestLiveControlJSONNode(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	live.handler.handle(t.Context(), &fakeMessage{
		topic:    node1StatusTopic,
		payload:  `{"online":true,"fw":"0.4.0","parts":{"ir":true}}`,
		retained: true,
	})

	waiting := live.command(t, `{"kind":"set_temperature","celsius":26}`)

	topic := iotbridge.LiveCommandTopic(node1Name)
	if got := live.broker.published[topic]; len(got) != 1 ||
		got[0] != `{"commandId":"`+waiting.ID.String()+`","temp":26}` {
		t.Fatalf("published %v", live.broker.published)
	}

	if !slices.Equal(live.api.types(), []string{eventCommandSent}) {
		t.Fatalf("triggers %v, want sent only", live.api.types())
	}

	answer := `{"commandId":"` + waiting.ID.String() + `","command":"{}","result":"sent","at":"x"}`
	live.handler.handle(t.Context(), message("smarthome/ac/node_1/command/result", answer))
	live.handler.handle(t.Context(), message("smarthome/ac/node_1/command/result", answer))
	live.handler.handle(t.Context(), message(
		"smarthome/ac/node_1/command/result",
		`{"commandId":"`+uuid.NewString()+`","command":"{}","result":"sent"}`,
	))

	if !slices.Equal(live.api.types(), []string{eventCommandSent, "command_ack"}) ||
		*live.api.last().CommandID != waiting.ID {
		t.Fatalf("triggers %v, want the one acknowledgement", live.api.types())
	}

	live.api.mu.Lock()
	sentAt, ackAt := live.api.got[0].OccurredAt, live.api.got[1].OccurredAt
	live.api.mu.Unlock()

	if ackAt.Before(sentAt) { // stamped when it came, not earlier as readings are
		t.Errorf("acknowledged at %s, before it was sent at %s", ackAt, sentAt)
	}

	failed := live.command(t, `{"kind":"set_mode","mode":"dry"}`)
	live.handler.handle(t.Context(), message(
		"smarthome/ac/node_1/command/result",
		`{"commandId":"`+failed.ID.String()+`","command":"{}","result":"invalid"}`,
	))

	if last := live.api.last(); last.EventType != "command_fail" || *last.CommandID != failed.ID {
		t.Errorf("an invalid answer gave %+v", last)
	}

	if got := live.broker.published[topic]; len(got) != 2 ||
		got[1] != `{"commandId":"`+failed.ID.String()+`","mode":"dry"}` {
		t.Errorf("published %v", got)
	}
}

// TestLiveControlTextNode: a node that sent no firmware-0.4 status gets the first firmware's text for the power, which
// the bridge acknowledges with the settings it runs with; an action the text firmware does not take fails.
func TestLiveControlTextNode(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	live.command(t, `{"kind":"set_power","power":true}`)

	if got := live.broker.published[iotbridge.LiveCommandTopic(node1Name)]; len(got) != 1 ||
		got[0] != "COOL24" {
		t.Fatalf("published %v", live.broker.published)
	}

	if !slices.Equal(live.api.types(), []string{eventCommandSent, "command_ack", "state"}) {
		t.Fatalf("triggers %v", live.api.types())
	}

	live.api.reset()
	live.command(t, `{"kind":"set_fan","fanLevel":"low"}`)

	if !slices.Equal(live.api.types(), []string{"command_fail"}) ||
		len(live.broker.published) != 1 {
		t.Errorf(
			"a fan command to a text node: triggers %v, published %v",
			live.api.types(),
			live.broker.published,
		)
	}
}

// TestLiveControlSpacesCommands: a node's second command waits until the first is liveCommandSpacing old.
func TestLiveControlSpacesCommands(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	start := time.Now()

	live.command(t, `{"kind":"set_power","power":false}`)
	live.command(t, `{"kind":"set_power","power":false}`)

	if took := time.Since(start); took < liveCommandSpacing {
		t.Errorf("two commands to one node within %s", took)
	}
}

// TestLiveHandlerState: the air conditioner's node's state, retained or not, is its device's state report; a sensor
// unit's node reports none, and an unregistered node nothing.
func TestLiveHandlerState(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	live.handler.handle(t.Context(), &fakeMessage{
		topic:    "smarthome/ac/node_1/state",
		payload:  `{"power":"ON","temp":24,"mode":"cool","fan":"auto"}`,
		retained: true,
	})

	last := live.api.last()
	if last.EventType != eventState || *last.DeviceID != live.air.DeviceID || last.State == nil ||
		!*last.State.Power ||
		*last.State.Celsius != 24 ||
		*last.State.Mode != "cool" ||
		last.State.FanLevel != nil {
		t.Fatalf("state %+v", last)
	}

	live.handler.handle(t.Context(), message("smarthome/ac/node_2/state", `{"power":"ON"}`))
	live.handler.handle(t.Context(), message("smarthome/ac/node_9/state", `{"power":"ON"}`))

	if n := len(live.api.types()); n != 1 {
		t.Errorf("%d triggers, want the air conditioner's state only", n)
	}
}

// TestLiveHandlerHoldsEarlyState: a retained state that comes before the registered nodes were first read is reported
// with the node's next message once its device is known, before that message's readings; a later state replaces it.
func TestLiveHandlerHoldsEarlyState(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	live.handler.nodes = iotbridge.NewNodeRegistry() // not read yet

	for _, payload := range []string{`{"power":"ON","temp":24}`, `{"power":"OFF","temp":27}`} {
		live.handler.handle(t.Context(), &fakeMessage{
			topic:    "smarthome/ac/node_1/state",
			payload:  payload,
			retained: true,
		})
	}

	if n := len(live.api.types()); n != 0 {
		t.Fatalf("a state before the nodes were read gave %d triggers", n)
	}

	live.handler.nodes.Set([]iotbridge.NodeDevice{live.air, live.sensors})
	live.handler.handle(
		t.Context(),
		message("smarthome/ac/node_1/sensors", `{"temp":26.2,"humidity":51.4}`),
	)

	types := live.api.types()
	if len(types) == 0 || types[0] != eventState {
		t.Fatalf("triggers %v, want the held state first", types)
	}

	live.api.mu.Lock()
	state := live.api.got[0].State
	live.api.mu.Unlock()

	if state == nil || state.Power == nil || *state.Power || *state.Celsius != 27 {
		t.Errorf("held state %+v, want the later one", state)
	}

	live.api.reset()
	live.handler.handle(
		t.Context(),
		message("smarthome/ac/node_1/sensors", `{"temp":26.2,"humidity":51.4}`),
	)

	if slices.Contains(live.api.types(), eventState) {
		t.Error("the held state was reported twice")
	}
}

// TestLiveHandlerReadings: a retained reading is old and skipped; a reading of now is recorded once a minute; a reading
// the node kept while offline is recorded at its own minute, beside the minute of now; a reading counts as heard.
func TestLiveHandlerReadings(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	now := time.Now()
	stamp := func(at time.Time) string {
		return `{"node_id":"node_2","timestamp":"` + at.In(time.FixedZone("WIB", 7*3600)).
			Format(time.RFC3339) +
			`","temp":25.6,"humidity":43.8,"pm25":55}`
	}

	live.handler.handle(t.Context(), &fakeMessage{
		topic:    "smarthome/ac/node_2/sensors",
		payload:  stamp(now),
		retained: true,
	})

	if n := len(live.api.types()); n != 0 {
		t.Fatalf("a retained reading gave %d triggers", n)
	}

	live.handler.handle(
		t.Context(),
		message("smarthome/ac/node_2/sensors", stamp(now.Add(-9*time.Minute))),
	)
	live.api.reset() // a later kept minute first: the earlier one still passes
	live.handler.handle(
		t.Context(),
		message("smarthome/ac/node_2/sensors", stamp(now.Add(-10*time.Minute))),
	)

	if !slices.Equal(live.api.types(), []string{eventTelemetry, eventTelemetry, eventTelemetry}) {
		t.Fatalf("a replayed reading gave %v, want the readings only", live.api.types())
	}

	if m := live.api.last().Measurement; !m.ObservedAt.Equal(
		now.Add(-10 * time.Minute).UTC().Truncate(time.Minute),
	) {
		t.Errorf("the replayed reading is observed at %s", m.ObservedAt)
	}

	live.api.reset()
	live.handler.handle(t.Context(), message("smarthome/ac/node_2/sensors", stamp(now)))
	live.handler.handle(t.Context(), message("smarthome/ac/node_2/sensors", stamp(now)))

	if n := len(live.api.types()); n != 5 {
		t.Errorf(
			"two readings of now gave %d triggers, want one minute's 3 readings, evaluation and heartbeat",
			n,
		)
	}

	if gap := live.handler.heard.GapOf([]string{node2Name}, time.Now()); gap > time.Minute {
		t.Errorf("node_2 not heard: gap %s", gap)
	}
}

// TestLiveHandlerLastWill: the broker's last will of a node makes its gap at least LiveOfflineAfter at once, so the
// next report takes its module offline; its next message brings the gap back.
func TestLiveHandlerLastWill(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	live.handler.handle(
		t.Context(),
		message(node1StatusTopic, `{"online":true,"fw":"0.4.0"}`),
	)
	live.handler.handle(t.Context(), &fakeMessage{
		topic:    node1StatusTopic,
		payload:  `{"online":false}`,
		retained: true,
	})

	if gap := live.handler.heard.GapOf(
		[]string{node1Name},
		time.Now(),
	); gap < iotbridge.LiveOfflineAfter {
		t.Errorf("gap after the last will %s", gap)
	}

	if !live.handler.firmware.Answers(node1Name) {
		t.Error("the last will forgot the node's firmware")
	}

	live.handler.handle(
		t.Context(),
		message(node1StatusTopic, `{"online":true,"fw":"0.4.0"}`),
	)

	if gap := live.handler.heard.GapOf([]string{node1Name}, time.Now()); gap > time.Second {
		t.Errorf("gap after the node came back %s", gap)
	}

	live.handler.handle(
		t.Context(),
		message("smarthome/ac/node_1/command", `{"commandId":"x","power":"ON"}`),
	)

	if n := len(live.api.types()); n != 0 {
		t.Errorf("status and the bridge's own command gave %d triggers", n)
	}
}

// TestLiveHandlerRetainedOnlineIsNotAHeartbeat: a retained "online" status is what the broker replays on a reconnect,
// so it is not a sign of life; the node's own next message is.
func TestLiveHandlerRetainedOnlineIsNotAHeartbeat(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	live.handler.heard.Gone(node1Name, time.Now())
	live.handler.handle(t.Context(), &fakeMessage{
		topic:    node1StatusTopic,
		payload:  `{"online":true,"fw":"0.4.0"}`,
		retained: true,
	})

	if gap := live.handler.heard.GapOf(
		[]string{node1Name},
		time.Now(),
	); gap < iotbridge.LiveOfflineAfter {
		t.Errorf("a retained online status counted as a sign of life: gap %s", gap)
	}

	live.handler.handle(
		t.Context(),
		message(node1StatusTopic, `{"online":true,"fw":"0.4.0"}`),
	)

	if gap := live.handler.heard.GapOf([]string{node1Name}, time.Now()); gap > time.Second {
		t.Errorf("a live online status did not count: gap %s", gap)
	}
}

// The live site's node IDs and the event types the tests compare.
const (
	node1Name        = "node_1"
	node1StatusTopic = "smarthome/ac/node_1/status"
	node2Name        = "node_2"
	eventCommandSent = "command_sent"
	eventTelemetry   = "telemetry"
	eventState       = "state"
	testToken        = "tok"
	keyDeviceID      = "deviceId"
)

// TestLiveChecks: the running checks are read with the internal token and the tenant, and each is answered on the demo
// trigger: succeeded for a node heard just now, failed for one unheard for longer than LiveOfflineAfter; a refused read
// is an error.
func TestLiveChecks(t *testing.T) {
	t.Parallel()

	live := newLiveFixture(t)
	heard, silent := uuid.New(), uuid.New()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/system/queries/devices.mqttChecks" ||
			r.Header.Get(
				ops.InternalTokenHeader,
			) != testToken || r.Header.Get("X-Tenant-Id") != "t1" {
			w.WriteHeader(http.StatusUnauthorized)

			return
		}

		_ = json.NewEncoder(w).
			Encode(map[string]any{"data": map[string]any{"checks": []map[string]any{
				{"operationId": heard, keyDeviceID: live.air.DeviceID, "node": node1Name},
				{"operationId": silent, keyDeviceID: live.sensors.DeviceID, "node": node2Name},
			}}})
	}))
	t.Cleanup(api.Close)

	checks, err := fetchChecks(
		t.Context(),
		live.handler.httpc,
		liveAPI{equipment: api.URL, token: testToken, tenant: "t1"},
	)
	if err != nil || len(checks) != 2 || checks[0].OperationID != heard ||
		checks[1].Node != node2Name {
		t.Fatalf("checks %+v, %v", checks, err)
	}

	_, err = fetchChecks(
		t.Context(),
		live.handler.httpc,
		liveAPI{equipment: api.URL, token: "wrong", tenant: "t1"},
	)
	if err == nil {
		t.Error("a refused read is not an error")
	}

	now := time.Now()
	live.handler.heard.Heard(node1Name, now)
	live.handler.heard.Gone(node2Name, now)

	for _, check := range checks {
		answerCheck(
			t.Context(),
			live.handler.httpc,
			live.srv.URL,
			check,
			live.handler.heard.GapOf([]string{check.Node}, now),
			now,
		)
	}

	live.api.mu.Lock()
	defer live.api.mu.Unlock()

	if len(live.api.got) != 2 || live.api.got[0].Result != "succeeded" ||
		*live.api.got[0].OperationID != heard ||
		live.api.got[1].Result != "failed" ||
		*live.api.got[1].OperationID != silent {
		t.Errorf("answers %+v", live.api.got)
	}
}
