package iotbridge

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// The nodes' messages as they arrive (smarthome/ac/<node>/sensors): each fills what it does not measure with zeros.
const (
	node1Message = `{"device_id":"node_1","timestamp":"2026-10-30 13:52:21","temp":27.2,"humidity":50.9,"pressure":1010.4,"gas":65110.0,"approx_altitude":23.4,"voltage":0.0,"current":0.000,"power":0.0,"energy":0.000}`
	node2Message = `{"device_id":"node_2","timestamp":"2026-10-30 13:52:22","temp":0.0,"humidity":0.0,"pressure":0.0,"gas":0.0,"approx_altitude":0.0,"voltage":229.8,"current":3.410,"power":742.0,"energy":12.345}`
)

var liveAt = time.Date(2026, 9, 20, 3, 15, 42, 0, time.UTC) // on the scenario clock

// TestLiveNode: the node is the level before sensors; other topics under the subscription carry no node.
func TestLiveNode(t *testing.T) {
	for topic, want := range map[string]string{"smarthome/ac/node_1/sensors": "node_1", "smarthome/ac/node_2/sensors": "node_2",
		"smarthome/ac/node_1/status": "", "sensors": "", "smarthome/ac/node_1/sensors/extra": ""} {
		if got := LiveNode(topic); got != want {
			t.Errorf("LiveNode(%q) = %q, want %q", topic, got, want)
		}
	}
}

// TestLiveTriggersRoomNode: node_1 gives the room temperature and humidity on the live unit's sensors, stamped at the
// start of the scenario minute, measured and valid, and the module's heartbeat with no settings.
func TestLiveTriggersRoomNode(t *testing.T) {
	out, err := LiveTriggers("node_1", []byte(node1Message), liveAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 {
		t.Fatalf("%d triggers, want temperature, humidity, their alert evaluation and the heartbeat", len(out))
	}
	slot := liveAt.Truncate(time.Minute)
	want := map[string]struct {
		sensor, unit string
		value        float64
	}{"temperature": {"sensor-live-temperature", "°C", 27.2}, "humidity": {"sensor-live-humidity", "%", 50.9}}
	for _, tr := range out[:2] {
		m := tr.Measurement
		w, ok := want[m.Metric]
		if !ok || tr.EventType != "telemetry" || tr.ScenarioID != Scenario {
			t.Fatalf("trigger %+v", tr)
		}
		if m.UnitID != seed.ID("unit-live-ac") || m.SensorID != seed.ID(w.sensor) || m.Unit != w.unit || m.Value == nil || *m.Value != w.value {
			t.Errorf("%s reading %+v", m.Metric, m)
		}
		if !m.ObservedAt.Equal(slot) || !tr.OccurredAt.Equal(slot) || !m.ReceivedAt.Equal(liveAt) {
			t.Errorf("%s times: observed %s, occurred %s, received %s", m.Metric, m.ObservedAt, tr.OccurredAt, m.ReceivedAt)
		}
		if m.Origin != "measured" || m.Quality != "valid" || m.EventID != tr.EventID || m.Sequence != nil {
			t.Errorf("%s origin, quality, event or sequence: %+v", m.Metric, m)
		}
	}
	ev := out[2] // the readings as facts of the unit's alert policies (IR529)
	if ev.EventType != "automation" || ev.Event == nil || ev.Event.EventID != ev.EventID || ev.Event.Phase != "condition" || !ev.Event.OccurredAt.Equal(slot) ||
		len(ev.Event.UnitIDs) != 1 || ev.Event.UnitIDs[0] != seed.ID("unit-live-ac") || len(ev.Event.Facts) != 2 {
		t.Fatalf("alert evaluation %+v", ev)
	}
	for i, want := range []Fact{{UnitID: seed.ID("unit-live-ac"), Metric: "temperature", Value: 27.2, Unit: "°C", ObservedAt: slot, Quality: "valid"},
		{UnitID: seed.ID("unit-live-ac"), Metric: "humidity", Value: 50.9, Unit: "%", ObservedAt: slot, Quality: "valid"}} {
		if f := ev.Event.Facts[i]; f.UnitID != want.UnitID || f.Metric != want.Metric || f.Value != want.Value || f.Unit != want.Unit || !f.ObservedAt.Equal(slot) || f.Quality != "valid" {
			t.Errorf("fact %d %+v, want %+v", i, f, want)
		}
	}
	hb := out[3]
	if hb.EventType != "state" || hb.DeviceID == nil || *hb.DeviceID != seed.ID("device-live-ac") || !hb.OccurredAt.Equal(slot) {
		t.Errorf("heartbeat %+v", hb)
	}
	if hb.State == nil || *hb.State != (devicesim.ReportedState{}) {
		t.Errorf("heartbeat state %+v, want no settings", hb.State)
	}
	raw, _ := json.Marshal(hb)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if _, ok := body["state"].(map[string]any); !ok { // the API refuses a state report without its state object
		t.Errorf("heartbeat body %s", raw)
	}
}

// TestLiveTriggersPowerNode: node_2's power in W is recorded in kW on the same unit, with the module's heartbeat; a zero
// power is a reading while the meter sees a voltage, and with no voltage the meter measured nothing.
func TestLiveTriggersPowerNode(t *testing.T) {
	out, err := LiveTriggers("node_2", []byte(node2Message), liveAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 || out[0].Measurement == nil || out[0].Measurement.Metric != "power" || out[1].EventType != "automation" || out[2].EventType != "state" {
		t.Fatalf("triggers %+v", out)
	}
	if f := out[1].Event.Facts; len(f) != 1 || f[0].Metric != "power" || f[0].Value != 0.742 || f[0].Unit != "kW" {
		t.Errorf("power fact %+v", f)
	}
	if m := out[0].Measurement; m.UnitID != seed.ID("unit-live-ac") || m.SensorID != seed.ID("sensor-live-power") || m.Unit != "kW" || *m.Value != 0.742 {
		t.Errorf("power reading %+v (value %v)", m, *m.Value)
	}
	if hb := out[2]; hb.DeviceID == nil || *hb.DeviceID != seed.ID("device-live-ac") {
		t.Errorf("node_2's heartbeat names module %v", hb.DeviceID)
	}
	off, err := LiveTriggers("node_2", []byte(`{"voltage":230.1,"power":0.0}`), liveAt)
	if err != nil || len(off) != 3 || *off[0].Measurement.Value != 0 {
		t.Errorf("an air conditioner drawing nothing: %+v, %v", off, err)
	}
	none, err := LiveTriggers("node_2", []byte(`{"voltage":0.0,"current":0.0,"power":0.0}`), liveAt)
	if err != nil || none != nil {
		t.Errorf("a meter without voltage gave %+v, %v", none, err)
	}
}

// TestLiveTriggersSkipWhatIsNotMeasured: zeros the room node did not measure are no readings, and a message with
// nothing measured sends nothing, not even the heartbeat.
func TestLiveTriggersSkipWhatIsNotMeasured(t *testing.T) {
	out, err := LiveTriggers("node_1", []byte(`{"temp":26.4,"humidity":0.0}`), liveAt)
	if err != nil || len(out) != 3 || out[0].Measurement.Metric != "temperature" || len(out[1].Event.Facts) != 1 {
		t.Errorf("temperature only: %+v, %v", out, err)
	}
	out, err = LiveTriggers("node_1", []byte(`{"temp":0.0,"humidity":0.0,"pressure":0.0}`), liveAt)
	if err != nil || out != nil {
		t.Errorf("a failed read gave %+v, %v", out, err)
	}
	out, err = LiveTriggers("node_1", []byte(`{"temp":"27.2","humidity":null}`), liveAt)
	if err != nil || out != nil {
		t.Errorf("fields that are no numbers gave %+v, %v", out, err)
	}
}

// TestLiveTriggersRefuse: a node the site does not know and a message that is no JSON are errors.
func TestLiveTriggersRefuse(t *testing.T) {
	if _, err := LiveTriggers("node_3", []byte(node1Message), liveAt); err == nil {
		t.Error("an unknown node was converted")
	}
	if _, err := LiveTriggers("node_1", []byte(`temp=27.2`), liveAt); err == nil {
		t.Error("a message that is no JSON was converted")
	}
}

// TestLiveEventIDIsOnePerMinute: messages of one minute share their event IDs (the API keeps one reading a minute), the
// next minute has others, and both nodes send the module's heartbeat of a minute with the same body, so the API takes
// it once.
func TestLiveEventIDIsOnePerMinute(t *testing.T) {
	a, _ := LiveTriggers("node_1", []byte(node1Message), liveAt)
	b, _ := LiveTriggers("node_1", []byte(`{"temp":27.4,"humidity":51.0}`), liveAt.Add(15*time.Second))
	c, _ := LiveTriggers("node_1", []byte(node1Message), liveAt.Add(time.Minute))
	if a[0].EventID != b[0].EventID || a[1].EventID != b[1].EventID {
		t.Error("two messages of one minute have other event IDs")
	}
	if a[0].EventID == c[0].EventID || a[0].EventID == a[1].EventID {
		t.Error("the next minute or another field shares an event ID")
	}
	if a[2].EventID != b[2].EventID || a[2].EventID == c[2].EventID { // one alert evaluation per node and minute
		t.Error("the alert evaluations of one minute differ, or two minutes share one")
	}
	p, _ := LiveTriggers("node_2", []byte(node2Message), liveAt.Add(10*time.Second))
	h1, _ := json.Marshal(a[len(a)-1])
	h2, _ := json.Marshal(p[len(p)-1])
	if string(h1) != string(h2) {
		t.Errorf("the nodes' heartbeats of one minute differ:\n%s\n%s", h1, h2)
	}
}

// TestLiveNodesMatchTheSite: every recorded field names a sensor of the live module, whose metric and unit are the
// platform's (the units the simulator reports, which the API checks); no sensor is left unfed; and the commands go to
// a node the bridge listens to.
func TestLiveNodesMatchTheSite(t *testing.T) {
	platform := map[string]string{}
	for _, s := range devicesim.Sensors {
		platform[s.Metric] = s.Unit
	}
	site := map[string]seed.LiveSensor{}
	for _, s := range seed.LiveSite.Sensors {
		site[s.Name] = s
		if platform[s.Metric] != s.Unit {
			t.Errorf("site sensor %s is %s in %q, the platform uses %q", s.Name, s.Metric, s.Unit, platform[s.Metric])
		}
	}
	fed := map[string]bool{}
	for node, fields := range LiveNodes {
		for _, f := range fields {
			if _, ok := site[f.Sensor]; !ok || f.Scale == 0 || f.Gate == "" {
				t.Errorf("%s.%s: sensor %s, scale %v, gate %q", node, f.Field, f.Sensor, f.Scale, f.Gate)
			}
			fed[f.Sensor] = true
		}
	}
	if len(fed) != len(site) {
		t.Errorf("%d of the site's %d sensors are fed", len(fed), len(site))
	}
	if _, ok := LiveNodes[LiveCommandNode]; !ok || LiveCommandTopic(LiveCommandNode) != "smarthome/ac/node_1/command" {
		t.Errorf("commands go to %s (%s)", LiveCommandNode, LiveCommandTopic(LiveCommandNode))
	}
}

// TestLiveCommand: power on is the firmware's COOL24 with the settings it runs, power off is OFF and keeps the other
// settings, and an action the firmware does not take is not sent.
func TestLiveCommand(t *testing.T) {
	payload, state, ok, err := LiveCommand(json.RawMessage(`{"kind":"set_power","power":true}`))
	if err != nil || !ok || payload != "COOL24" || state == nil || !*state.Power || *state.Celsius != 24 || *state.Mode != "cool" || state.FanLevel != nil {
		t.Errorf("power on: %q %+v %t %v", payload, state, ok, err)
	}
	if payload, state, ok, err := LiveCommand(json.RawMessage(`{"kind":"set_power","power":false}`)); err != nil || !ok || payload != "OFF" || state != nil {
		t.Errorf("power off: %q %+v %t %v", payload, state, ok, err)
	}
	for _, a := range []string{`{"kind":"set_temperature","celsius":24}`, `{"kind":"set_mode","mode":"cool"}`, `{"kind":"set_fan","fanLevel":"mid"}`, `{"kind":"set_power"}`} {
		if payload, _, ok, err := LiveCommand(json.RawMessage(a)); err != nil || ok || payload != "" {
			t.Errorf("%s: %q %t %v", a, payload, ok, err)
		}
	}
	if _, _, _, err := LiveCommand(json.RawMessage(`power on`)); err == nil {
		t.Error("an action that is no JSON was taken")
	}
}

// TestLiveSentTriggers: a published command is sent and acknowledged on the module, COOL24 also reports the settings
// it runs with, OFF only its own action; a refused command fails.
func TestLiveSentTriggers(t *testing.T) {
	w := Waiting{ID: seed.ID("cmd-live"), DeviceID: seed.ID("device-live-ac"), UnitID: seed.ID("unit-live-ac")}
	_, state, _, _ := LiveCommand(json.RawMessage(`{"kind":"set_power","power":true}`))
	on := LiveSentTriggers(w, liveAt, liveAt.Add(-2*time.Second), state)
	if len(on) != 3 || on[0].EventType != "command_sent" || on[1].EventType != "command_ack" || on[2].EventType != "state" {
		t.Fatalf("power on triggers %+v", on)
	}
	if *on[1].CommandID != w.ID || on[1].EventID == on[0].EventID || *on[2].DeviceID != seed.ID("device-live-ac") || *on[2].State.Celsius != 24 || !on[2].OccurredAt.Equal(liveAt.Add(-2*time.Second)) {
		t.Errorf("ack %+v, state %+v", on[1], on[2])
	}
	if off := LiveSentTriggers(w, liveAt, liveAt, nil); len(off) != 2 {
		t.Errorf("power off reports %d triggers, want sent and acknowledged", len(off))
	}
	if f := LiveRefusedTrigger(w, liveAt); f.EventType != "command_fail" || *f.CommandID != w.ID || !f.OccurredAt.Equal(liveAt) {
		t.Errorf("refusal %+v", f)
	}
}

// TestLiveHeartbeatGap: the gap is the longest time a node has gone unheard, counted from the bridge's start for a node
// never heard; it is sent as heartbeat_gap in minutes (floored to a tenth) on the minute, one evaluation a minute.
func TestLiveHeartbeatGap(t *testing.T) {
	start := liveAt.Add(-20 * time.Minute)
	h := NewLiveHeard(start)
	if g := h.Gap(liveAt); g != 20*time.Minute {
		t.Errorf("no node heard since the start: gap %s, want 20m", g)
	}
	h.Heard("node_1", liveAt.Add(-5*time.Second))
	h.Heard("node_2", liveAt.Add(-3*time.Minute))
	h.Heard("node_2", liveAt.Add(-4*time.Minute)) // an older message changes nothing
	if g := h.Gap(liveAt); g != 3*time.Minute {
		t.Errorf("gap %s, want node_2's 3m", g)
	}
	// on the wall clock: a reading of the monotonic clock (which stops while the machine sleeps) is not used
	w := NewLiveHeard(time.Now())
	w.Heard("node_1", time.Now())
	w.Heard("node_2", time.Now())
	if g := w.Gap(time.Now().Round(0).Add(2 * time.Hour)); g < 2*time.Hour-time.Second {
		t.Errorf("two hours on the wall clock gave a gap of %s", g)
	}
	slot := liveAt.Truncate(time.Minute)
	tr := LiveGapTrigger(17*time.Minute+50*time.Second, slot)
	f := tr.Event.Facts
	if tr.EventType != "automation" || len(f) != 1 || f[0].Metric != "heartbeat_gap" || f[0].Unit != "min" || f[0].Value != 17.8 || !f[0].ObservedAt.Equal(slot) ||
		f[0].UnitID != seed.ID("unit-live-ac") || tr.Event.Phase != "condition" {
		t.Errorf("gap trigger %+v %+v", tr, f)
	}
	if again := LiveGapTrigger(time.Minute, slot); again.EventID != tr.EventID || LiveGapTrigger(time.Minute, slot.Add(time.Minute)).EventID == tr.EventID {
		t.Error("the gap evaluation is not one a minute")
	}
}

// TestLiveGate: one message per node and minute passes, other nodes are counted apart, an earlier minute never
// passes again, and a released minute is open again.
func TestLiveGate(t *testing.T) {
	var g LiveGate
	m := liveAt.Truncate(time.Minute)
	if !g.Claim("node_1", m) || g.Claim("node_1", m) {
		t.Fatal("a minute passed twice or not at all")
	}
	if !g.Claim("node_2", m) {
		t.Error("another node's minute was taken")
	}
	if !g.Claim("node_1", m.Add(time.Minute)) || g.Claim("node_1", m) {
		t.Error("the next minute did not pass, or an earlier one passed again")
	}
	g.Release("node_1", m) // not the claimed minute: nothing changes
	if g.Claim("node_1", m.Add(time.Minute)) {
		t.Error("releasing another minute opened the claimed one")
	}
	g.Release("node_1", m.Add(time.Minute))
	if !g.Claim("node_1", m.Add(time.Minute)) {
		t.Error("a released minute stayed closed")
	}
}
