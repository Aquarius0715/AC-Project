package iotbridge_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// The nodes' messages as they arrive (smarthome/ac/<node>/sensors): each fills what it does not measure with zeros.
const (
	node1Message = `{"device_id":"node_1","timestamp":"2026-10-30 13:52:21","temp":27.2,"humidity":50.9,` +
		`"pressure":1010.4,"gas":65110.0,"approx_altitude":23.4,"voltage":0.0,"current":0.000,"power":0.0,"energy":0.000}`
	node2Message = `{"device_id":"node_2","timestamp":"2026-10-30 13:52:22","temp":0.0,"humidity":0.0,"pressure":0.0,` +
		`"gas":0.0,"approx_altitude":0.0,"voltage":229.8,"current":3.410,"power":742.0,"energy":12.345}`
)

// liveAt is when the test messages arrive, on the scenario clock.
func liveAt() time.Time { return time.Date(2026, 9, 20, 3, 15, 42, 0, time.UTC) }

// TestLiveNode: the node is the level before sensors; other topics under the subscription carry no node.
func TestLiveNode(t *testing.T) {
	t.Parallel()

	for topic, want := range map[string]string{
		"smarthome/ac/node_1/sensors": "node_1", "smarthome/ac/node_2/sensors": "node_2",
		"smarthome/ac/node_1/status": "", "sensors": "", "smarthome/ac/node_1/sensors/extra": "",
	} {
		if got := iotbridge.LiveNode(topic); got != want {
			t.Errorf("LiveNode(%q) = %q, want %q", topic, got, want)
		}
	}
}

// TestLiveTriggersRoomNode: node_1 gives the room temperature and humidity on the live unit's sensors, stamped at the
// start of the scenario minute, measured and valid, and the module's heartbeat with no settings.
func TestLiveTriggersRoomNode(t *testing.T) {
	t.Parallel()

	out, err := iotbridge.LiveTriggers("node_1", []byte(node1Message), liveAt())
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != 4 {
		t.Fatalf(
			"%d triggers, want temperature, humidity, their alert evaluation and the heartbeat",
			len(out),
		)
	}

	slot := liveAt().Truncate(time.Minute)
	want := map[string]struct {
		sensor, unit string
		value        float64
	}{sensorTemperature: {"sensor-live-temperature", "°C", 27.2}, sensorHumidity: {"sensor-live-humidity", "%", 50.9}}

	for _, trigger := range out[:2] {
		measurement := trigger.Measurement

		expected, ok := want[measurement.Metric]
		if !ok || trigger.EventType != "telemetry" || trigger.ScenarioID != iotbridge.Scenario {
			t.Fatalf("trigger %+v", trigger)
		}

		if measurement.UnitID != seed.ID("unit-live-ac") ||
			measurement.SensorID != seed.ID(expected.sensor) ||
			measurement.Unit != expected.unit ||
			measurement.Value == nil ||
			*measurement.Value != expected.value {
			t.Errorf("%s reading %+v", measurement.Metric, measurement)
		}

		if !measurement.ObservedAt.Equal(slot) || !trigger.OccurredAt.Equal(slot) ||
			!measurement.ReceivedAt.Equal(liveAt()) {
			t.Errorf(
				"%s times: observed %s, occurred %s, received %s",
				measurement.Metric,
				measurement.ObservedAt,
				trigger.OccurredAt,
				measurement.ReceivedAt,
			)
		}

		if measurement.Origin != "measured" || measurement.Quality != qualityValid ||
			measurement.EventID != trigger.EventID ||
			measurement.Sequence != nil {
			t.Errorf("%s origin, quality, event or sequence: %+v", measurement.Metric, measurement)
		}
	}

	evaluation := out[2] // the readings as facts of the unit's alert policies (IR529)
	if evaluation.EventType != eventAutomation || evaluation.Event == nil ||
		evaluation.Event.EventID != evaluation.EventID ||
		evaluation.Event.Phase != "condition" ||
		!evaluation.Event.OccurredAt.Equal(slot) ||
		len(evaluation.Event.UnitIDs) != 1 ||
		evaluation.Event.UnitIDs[0] != seed.ID("unit-live-ac") ||
		len(evaluation.Event.Facts) != 2 {
		t.Fatalf("alert evaluation %+v", evaluation)
	}

	for idx, want := range []iotbridge.Fact{
		{
			UnitID: seed.ID("unit-live-ac"), Metric: sensorTemperature, Value: 27.2, Unit: "°C",
			ObservedAt: slot, Quality: qualityValid,
		},
		{
			UnitID: seed.ID("unit-live-ac"), Metric: sensorHumidity, Value: 50.9, Unit: "%",
			ObservedAt: slot, Quality: qualityValid,
		},
	} {
		if f := evaluation.Event.Facts[idx]; f.UnitID != want.UnitID || f.Metric != want.Metric ||
			f.Value != want.Value ||
			f.Unit != want.Unit ||
			!f.ObservedAt.Equal(slot) ||
			f.Quality != qualityValid {
			t.Errorf("fact %d %+v, want %+v", idx, f, want)
		}
	}

	heartbeat := out[3]
	if heartbeat.EventType != eventState || heartbeat.DeviceID == nil ||
		*heartbeat.DeviceID != seed.ID("device-live-ac") ||
		!heartbeat.OccurredAt.Equal(slot) {
		t.Errorf("heartbeat %+v", heartbeat)
	}

	if heartbeat.State == nil ||
		*heartbeat.State != (devicesim.ReportedState{Power: nil, Celsius: nil, Mode: nil, FanLevel: nil}) {
		t.Errorf("heartbeat state %+v, want no settings", heartbeat.State)
	}

	raw, marshalErr := json.Marshal(heartbeat)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	var body map[string]any

	unmarshalErr := json.Unmarshal(raw, &body)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	if _, ok := body[eventState].(map[string]any); !ok { // the API refuses a state report without its state object
		t.Errorf("heartbeat body %s", raw)
	}
}

// TestLiveTriggersPowerNode: node_2's power in W is recorded in kW on the same unit, with the module's heartbeat; a
// zero power is a reading while the meter sees a voltage, and with no voltage the meter measured nothing.
func TestLiveTriggersPowerNode(t *testing.T) {
	t.Parallel()

	out, err := iotbridge.LiveTriggers("node_2", []byte(node2Message), liveAt())
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != 3 || out[0].Measurement == nil || out[0].Measurement.Metric != sensorPower ||
		out[1].EventType != eventAutomation ||
		out[2].EventType != eventState {
		t.Fatalf("triggers %+v", out)
	}

	if f := out[1].Event.Facts; len(f) != 1 || f[0].Metric != sensorPower || f[0].Value != 0.742 ||
		f[0].Unit != "kW" {
		t.Errorf("power fact %+v", f)
	}

	if m := out[0].Measurement; m.UnitID != seed.ID("unit-live-ac") ||
		m.SensorID != seed.ID("sensor-live-power") ||
		m.Unit != "kW" ||
		*m.Value != 0.742 {
		t.Errorf("power reading %+v (value %v)", m, *m.Value)
	}

	if hb := out[2]; hb.DeviceID == nil || *hb.DeviceID != seed.ID("device-live-ac") {
		t.Errorf("node_2's heartbeat names module %v", hb.DeviceID)
	}

	off, err := iotbridge.LiveTriggers("node_2", []byte(`{"voltage":230.1,"power":0.0}`), liveAt())
	if err != nil || len(off) != 3 || *off[0].Measurement.Value != 0 {
		t.Errorf("an air conditioner drawing nothing: %+v, %v", off, err)
	}

	none, err := iotbridge.LiveTriggers(
		"node_2",
		[]byte(`{"voltage":0.0,"current":0.0,"power":0.0}`),
		liveAt(),
	)
	if err != nil || none != nil {
		t.Errorf("a meter without voltage gave %+v, %v", none, err)
	}
}

// TestLiveTriggersSkipWhatIsNotMeasured: zeros the room node did not measure are no readings, and a message with
// nothing measured sends nothing, not even the heartbeat.
func TestLiveTriggersSkipWhatIsNotMeasured(t *testing.T) {
	t.Parallel()

	out, err := iotbridge.LiveTriggers("node_1", []byte(`{"temp":26.4,"humidity":0.0}`), liveAt())
	if err != nil || len(out) != 3 || out[0].Measurement.Metric != sensorTemperature ||
		len(out[1].Event.Facts) != 1 {
		t.Errorf("temperature only: %+v, %v", out, err)
	}

	out, err = iotbridge.LiveTriggers(
		"node_1",
		[]byte(`{"temp":0.0,"humidity":0.0,"pressure":0.0}`),
		liveAt(),
	)
	if err != nil || out != nil {
		t.Errorf("a failed read gave %+v, %v", out, err)
	}

	out, err = iotbridge.LiveTriggers("node_1", []byte(`{"temp":"27.2","humidity":null}`), liveAt())
	if err != nil || out != nil {
		t.Errorf("fields that are no numbers gave %+v, %v", out, err)
	}
}

// TestLiveTriggersRefuse: a node the site does not know and a message that is no JSON are errors.
func TestLiveTriggersRefuse(t *testing.T) {
	t.Parallel()

	_, err := iotbridge.LiveTriggers("node_3", []byte(node1Message), liveAt())
	if err == nil {
		t.Error("an unknown node was converted")
	}

	_, err = iotbridge.LiveTriggers("node_1", []byte(`temp=27.2`), liveAt())
	if err == nil {
		t.Error("a message that is no JSON was converted")
	}
}

// TestLiveEventIDIsOnePerMinute: messages of one minute share their event IDs (the API keeps one reading a minute), the
// next minute has others, and both nodes send the module's heartbeat of a minute with the same body, so the API takes
// it once.
func TestLiveEventIDIsOnePerMinute(t *testing.T) {
	t.Parallel()

	first, _ := iotbridge.LiveTriggers("node_1", []byte(node1Message), liveAt())
	sameMinute, _ := iotbridge.LiveTriggers(
		"node_1",
		[]byte(`{"temp":27.4,"humidity":51.0}`),
		liveAt().Add(15*time.Second),
	)
	nextMinute, _ := iotbridge.LiveTriggers(
		"node_1",
		[]byte(node1Message),
		liveAt().Add(time.Minute),
	)

	if first[0].EventID != sameMinute[0].EventID || first[1].EventID != sameMinute[1].EventID {
		t.Error("two messages of one minute have other event IDs")
	}

	if first[0].EventID == nextMinute[0].EventID || first[0].EventID == first[1].EventID {
		t.Error("the next minute or another field shares an event ID")
	}

	if first[2].EventID != sameMinute[2].EventID ||
		first[2].EventID == nextMinute[2].EventID { // one alert evaluation per node and minute
		t.Error("the alert evaluations of one minute differ, or two minutes share one")
	}

	power, _ := iotbridge.LiveTriggers("node_2", []byte(node2Message), liveAt().Add(10*time.Second))

	roomBeat, marshalErr := json.Marshal(first[len(first)-1])
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	powerBeat, marshalErr := json.Marshal(power[len(power)-1])
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	if string(roomBeat) != string(powerBeat) {
		t.Errorf("the nodes' heartbeats of one minute differ:\n%s\n%s", roomBeat, powerBeat)
	}
}

// TestLiveNodesMatchTheSite: every recorded field names a sensor of the live module, whose metric and unit are the
// platform's (the units the simulator reports, which the API checks); no sensor is left unfed; and the commands go to
// a node the bridge listens to.
func TestLiveNodesMatchTheSite(t *testing.T) {
	t.Parallel()

	platform := map[string]string{}
	for _, s := range devicesim.Sensors() {
		platform[s.Metric] = s.Unit
	}

	site := map[string]seed.LiveSensor{}
	for _, liveSensor := range seed.LiveSite().Sensors {
		site[liveSensor.Name] = liveSensor
		if platform[liveSensor.Metric] != liveSensor.Unit {
			t.Errorf(
				"site sensor %s is %s in %q, the platform uses %q",
				liveSensor.Name,
				liveSensor.Metric,
				liveSensor.Unit,
				platform[liveSensor.Metric],
			)
		}
	}

	fed := map[string]bool{}

	for node, fields := range iotbridge.LiveNodes() {
		for _, liveField := range fields {
			if _, ok := site[liveField.Sensor]; !ok || liveField.Scale == 0 ||
				liveField.Gate == "" {
				t.Errorf(
					"%s.%s: sensor %s, scale %v, gate %q",
					node,
					liveField.Field,
					liveField.Sensor,
					liveField.Scale,
					liveField.Gate,
				)
			}

			fed[liveField.Sensor] = true
		}
	}

	if len(fed) != len(site) {
		t.Errorf("%d of the site's %d sensors are fed", len(fed), len(site))
	}

	if _, ok := iotbridge.LiveNodes()[iotbridge.LiveCommandNode]; !ok ||
		iotbridge.LiveCommandTopic(iotbridge.LiveCommandNode) != "smarthome/ac/node_1/command" {
		t.Errorf(
			"commands go to %s (%s)",
			iotbridge.LiveCommandNode,
			iotbridge.LiveCommandTopic(iotbridge.LiveCommandNode),
		)
	}
}

// TestLiveCommand: power on is the firmware's COOL24 with the settings it runs, power off is OFF and keeps the other
// settings, and an action the firmware does not take is not sent.
func TestLiveCommand(t *testing.T) {
	t.Parallel()

	command, ok, err := iotbridge.LiveCommand(json.RawMessage(`{"kind":"set_power","power":true}`))

	state := command.State
	if err != nil || !ok || command.Payload != "COOL24" || state == nil || !*state.Power ||
		*state.Celsius != 24 ||
		*state.Mode != "cool" ||
		state.FanLevel != nil {
		t.Errorf("power on: %+v %t %v", command, ok, err)
	}

	command, ok, err = iotbridge.LiveCommand(json.RawMessage(`{"kind":"set_power","power":false}`))
	if err != nil || !ok || command.Payload != "OFF" || command.State != nil {
		t.Errorf("power off: %+v %t %v", command, ok, err)
	}

	for _, action := range []string{
		`{"kind":"set_temperature","celsius":24}`, `{"kind":"set_mode","mode":"cool"}`,
		`{"kind":"set_fan","fanLevel":"mid"}`, `{"kind":"set_power"}`,
	} {
		command, ok, err := iotbridge.LiveCommand(json.RawMessage(action))
		if err != nil || ok || command.Payload != "" {
			t.Errorf("%s: %+v %t %v", action, command, ok, err)
		}
	}

	_, _, err = iotbridge.LiveCommand(json.RawMessage(`power on`))
	if err == nil {
		t.Error("an action that is no JSON was taken")
	}
}

// TestLiveSentTriggers: a published command is sent and acknowledged on the module, COOL24 also reports the settings
// it runs with, OFF only its own action; a refused command fails.
func TestLiveSentTriggers(t *testing.T) {
	t.Parallel()

	waiting := iotbridge.Waiting{
		ID:       seed.ID("cmd-live"),
		DeviceID: seed.ID("device-live-ac"),
		UnitID:   seed.ID("unit-live-ac"), Action: nil, ExpiresAt: time.Time{},
	}

	command, _, err := iotbridge.LiveCommand(json.RawMessage(`{"kind":"set_power","power":true}`))
	if err != nil {
		t.Fatal(err)
	}

	powerOn := iotbridge.LiveSentTriggers(
		waiting,
		liveAt(),
		liveAt().Add(-2*time.Second),
		command.State,
	)
	if len(powerOn) != 3 || powerOn[0].EventType != "command_sent" ||
		powerOn[1].EventType != "command_ack" ||
		powerOn[2].EventType != eventState {
		t.Fatalf("power on triggers %+v", powerOn)
	}

	if *powerOn[1].CommandID != waiting.ID || powerOn[1].EventID == powerOn[0].EventID ||
		*powerOn[2].DeviceID != seed.ID("device-live-ac") ||
		*powerOn[2].State.Celsius != 24 ||
		!powerOn[2].OccurredAt.Equal(liveAt().Add(-2*time.Second)) {
		t.Errorf("ack %+v, state %+v", powerOn[1], powerOn[2])
	}

	if off := iotbridge.LiveSentTriggers(waiting, liveAt(), liveAt(), nil); len(off) != 2 {
		t.Errorf("power off reports %d triggers, want sent and acknowledged", len(off))
	}

	if f := iotbridge.LiveRefusedTrigger(
		waiting,
		liveAt(),
	); f.EventType != "command_fail" || *f.CommandID != waiting.ID ||
		!f.OccurredAt.Equal(liveAt()) {
		t.Errorf("refusal %+v", f)
	}
}

// TestLiveHeartbeatGap: the gap is the longest time a node has gone unheard, counted from the bridge's start for a node
// never heard; it is sent as heartbeat_gap in minutes (floored to a tenth) on the minute, one evaluation a minute.
func TestLiveHeartbeatGap(t *testing.T) {
	t.Parallel()

	start := liveAt().Add(-20 * time.Minute)

	liveHeard := iotbridge.NewLiveHeard(start)
	if g := liveHeard.Gap(liveAt()); g != 20*time.Minute {
		t.Errorf("no node heard since the start: gap %s, want 20m", g)
	}

	liveHeard.Heard("node_1", liveAt().Add(-5*time.Second))
	liveHeard.Heard("node_2", liveAt().Add(-3*time.Minute))
	liveHeard.Heard("node_2", liveAt().Add(-4*time.Minute)) // an older message changes nothing

	if g := liveHeard.Gap(liveAt()); g != 3*time.Minute {
		t.Errorf("gap %s, want node_2's 3m", g)
	}
	// on the wall clock: a reading of the monotonic clock (which stops while the machine sleeps) is not used
	w := iotbridge.NewLiveHeard(time.Now())
	w.Heard("node_1", time.Now())
	w.Heard("node_2", time.Now())

	if g := w.Gap(time.Now().Round(0).Add(2 * time.Hour)); g < 2*time.Hour-time.Second {
		t.Errorf("two hours on the wall clock gave a gap of %s", g)
	}

	slot := liveAt().Truncate(time.Minute)
	trigger := iotbridge.LiveGapTrigger(17*time.Minute+50*time.Second, slot)

	facts := trigger.Event.Facts
	if trigger.EventType != eventAutomation || len(facts) != 1 ||
		facts[0].Metric != "heartbeat_gap" ||
		facts[0].Unit != "min" ||
		facts[0].Value != 17.8 ||
		!facts[0].ObservedAt.Equal(slot) ||
		facts[0].UnitID != seed.ID("unit-live-ac") ||
		trigger.Event.Phase != "condition" {
		t.Errorf("gap trigger %+v %+v", trigger, facts)
	}

	if again := iotbridge.LiveGapTrigger(
		time.Minute,
		slot,
	); again.EventID != trigger.EventID ||
		iotbridge.LiveGapTrigger(time.Minute, slot.Add(time.Minute)).EventID == trigger.EventID {
		t.Error("the gap evaluation is not one a minute")
	}
}

// TestLiveGate: one message per node and minute passes, other nodes are counted apart, an earlier minute never
// passes again, and a released minute is open again.
func TestLiveGate(t *testing.T) {
	t.Parallel()

	var liveGate iotbridge.LiveGate

	minute := liveAt().Truncate(time.Minute)
	if !liveGate.Claim("node_1", minute) || liveGate.Claim("node_1", minute) {
		t.Fatal("a minute passed twice or not at all")
	}

	if !liveGate.Claim("node_2", minute) {
		t.Error("another node's minute was taken")
	}

	if !liveGate.Claim("node_1", minute.Add(time.Minute)) || liveGate.Claim("node_1", minute) {
		t.Error("the next minute did not pass, or an earlier one passed again")
	}

	liveGate.Release("node_1", minute) // not the claimed minute: nothing changes

	if liveGate.Claim("node_1", minute.Add(time.Minute)) {
		t.Error("releasing another minute opened the claimed one")
	}

	liveGate.Release("node_1", minute.Add(time.Minute))

	if !liveGate.Claim("node_1", minute.Add(time.Minute)) {
		t.Error("a released minute stayed closed")
	}
}

// TestLiveConnectionTrigger: silent nodes take the module offline once (communication_lost after 3 minutes, with the
// API's next sequence on the binding), nodes heard again bring it back by naming the open fault (restored within a
// minute), and in between, or while the connection already matches, or while unbound, nothing is sent.
func TestLiveConnectionTrigger(t *testing.T) {
	t.Parallel()

	binding, fault := seed.ID("binding-live-ac"), seed.ID("fault-live")

	when := liveAt()
	if _, ok := iotbridge.LiveConnectionTrigger(
		iotbridge.ConnectionFault{BindingID: nil, FaultID: nil, Sequence: 0},
		time.Hour,
		when,
	); ok {
		t.Error("an unbound module was reported")
	}

	lost, ok := iotbridge.LiveConnectionTrigger(
		iotbridge.ConnectionFault{BindingID: &binding, Sequence: 7, FaultID: nil},
		4*time.Minute,
		when,
	)
	if !ok || lost.EventType != "device" || lost.Kind != "communication_lost" ||
		*lost.Sequence != 8 ||
		*lost.BindingID != binding ||
		*lost.DeviceID != seed.ID("device-live-ac") ||
		lost.Recovery != nil ||
		!lost.OccurredAt.Equal(when) {
		t.Fatalf("loss %+v", lost)
	}

	again, _ := iotbridge.LiveConnectionTrigger(
		iotbridge.ConnectionFault{BindingID: &binding, Sequence: 7, FaultID: nil},
		5*time.Minute,
		when.Add(time.Minute),
	)
	if again.EventID != lost.EventID {
		t.Error("a repeat of the loss is another event")
	}

	for _, gap := range []time.Duration{0, iotbridge.LiveOfflineAfter - time.Second} {
		if tr, ok := iotbridge.LiveConnectionTrigger(
			iotbridge.ConnectionFault{BindingID: &binding, FaultID: nil, Sequence: 0},
			gap,
			when,
		); ok {
			t.Errorf("gap %s took an online module offline: %+v", gap, tr)
		}
	}

	back, ok := iotbridge.LiveConnectionTrigger(
		iotbridge.ConnectionFault{BindingID: &binding, FaultID: &fault, Sequence: 8},
		20*time.Second,
		when,
	)
	if !ok || back.Kind != "restored" || *back.Sequence != 9 || back.Recovery == nil ||
		*back.Recovery != (iotbridge.Recovery{Axis: "connection", SourceEventID: fault, Value: connectionOnline}) {
		t.Fatalf("recovery %+v", back)
	}

	for _, gap := range []time.Duration{iotbridge.LiveOnlineWithin, 2 * time.Minute, time.Hour} {
		if tr, ok := iotbridge.LiveConnectionTrigger(
			iotbridge.ConnectionFault{BindingID: &binding, FaultID: &fault, Sequence: 8},
			gap,
			when,
		); ok {
			t.Errorf("gap %s restored an offline module: %+v", gap, tr)
		}
	}

	raw, marshalErr := json.Marshal(back)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	var body map[string]any

	unmarshalErr := json.Unmarshal(raw, &body)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	recovery, _ := body["recovery"].(map[string]any)
	if body["bindingId"] != binding.String() || body["sequence"] != 9.0 ||
		body["kind"] != "restored" ||
		recovery["sourceEventId"] != fault.String() {
		t.Errorf("recovery body %s", raw)
	}
}
