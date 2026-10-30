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

// TestLiveTriggersRoomNode: node_1 gives the room temperature and humidity on its own unit's sensors, stamped at the
// start of the scenario minute, measured and valid, and its module's heartbeat with no settings.
func TestLiveTriggersRoomNode(t *testing.T) {
	out, err := LiveTriggers("node_1", []byte(node1Message), liveAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("%d triggers, want temperature, humidity and the heartbeat", len(out))
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
	hb := out[2]
	if hb.EventType != "state" || hb.DeviceID == nil || *hb.DeviceID != seed.ID("device-live-ac") || !hb.OccurredAt.Equal(slot) { // node_1's module
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

// TestLiveTriggersPowerNode: node_2's power in W is recorded in kW on its own unit, with its own module's heartbeat; a
// zero power is a reading while the meter sees a voltage, and with no voltage the meter measured nothing.
func TestLiveTriggersPowerNode(t *testing.T) {
	out, err := LiveTriggers("node_2", []byte(node2Message), liveAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Measurement == nil || out[0].Measurement.Metric != "power" || out[1].EventType != "state" {
		t.Fatalf("triggers %+v", out)
	}
	if m := out[0].Measurement; m.UnitID != seed.ID("unit-live-node-2") || m.SensorID != seed.ID("sensor-live-power") || m.Unit != "kW" || *m.Value != 0.742 {
		t.Errorf("power reading %+v (value %v)", m, *m.Value)
	}
	if hb := out[1]; hb.DeviceID == nil || *hb.DeviceID != seed.ID("device-live-node-2") {
		t.Errorf("node_2's heartbeat names module %v", hb.DeviceID)
	}
	off, err := LiveTriggers("node_2", []byte(`{"voltage":230.1,"power":0.0}`), liveAt)
	if err != nil || len(off) != 2 || *off[0].Measurement.Value != 0 {
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
	if err != nil || len(out) != 2 || out[0].Measurement.Metric != "temperature" {
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
// next minute has others, and each node's heartbeat of a minute is its own module's.
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
	p, _ := LiveTriggers("node_2", []byte(node2Message), liveAt.Add(10*time.Second))
	if h1, h2 := a[len(a)-1], p[len(p)-1]; h1.EventID == h2.EventID || *h1.DeviceID == *h2.DeviceID {
		t.Error("the nodes share a heartbeat: each is its own module")
	}
	again, _ := LiveTriggers("node_1", []byte(node1Message), liveAt.Add(5*time.Second))
	h1, _ := json.Marshal(a[len(a)-1])
	h2, _ := json.Marshal(again[len(again)-1])
	if string(h1) != string(h2) { // a repeat of the minute's heartbeat is the same request
		t.Errorf("one node's heartbeats of one minute differ:\n%s\n%s", h1, h2)
	}
}

// TestLiveNodesMatchTheSite: the bridge knows the site's nodes and no others; every recorded field names a sensor of
// its own node's module, whose metric and unit are the platform's (the units the simulator reports, which the API
// checks); and no sensor of the site is left unfed.
func TestLiveNodesMatchTheSite(t *testing.T) {
	platform := map[string]string{}
	for _, s := range devicesim.Sensors {
		platform[s.Metric] = s.Unit
	}
	if len(LiveNodes) != len(seed.LiveSite.Nodes) {
		t.Errorf("the bridge knows %d nodes, the site has %d", len(LiveNodes), len(seed.LiveSite.Nodes))
	}
	units, devices := map[string]bool{}, map[string]bool{}
	for _, n := range seed.LiveSite.Nodes {
		units[n.Unit], devices[n.Device] = true, true
		own := map[string]bool{}
		for _, s := range n.Sensors {
			own[s.Name] = true
			if platform[s.Metric] != s.Unit {
				t.Errorf("%s sensor %s is %s in %q, the platform uses %q", n.Node, s.Name, s.Metric, s.Unit, platform[s.Metric])
			}
		}
		fed := map[string]bool{}
		for _, f := range LiveNodes[n.Node] {
			if !own[f.Sensor] || f.Scale == 0 || f.Gate == "" {
				t.Errorf("%s.%s: sensor %s (not its module's?), scale %v, gate %q", n.Node, f.Field, f.Sensor, f.Scale, f.Gate)
			}
			fed[f.Sensor] = true
		}
		if len(fed) != len(own) {
			t.Errorf("%d of %s's %d sensors are fed", len(fed), n.Node, len(own))
		}
	}
	if len(units) != len(seed.LiveSite.Nodes) || len(devices) != len(seed.LiveSite.Nodes) {
		t.Error("two nodes share a unit or a module: a unit has at most one active module")
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
