package iotbridge_test

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

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
	// node_2 with its own room sensor (DEC-91).
	node2RoomMessage = `{"device_id":"node_2","timestamp":"2026-11-04 11:44:02","temp":28.3,"humidity":58.4,` +
		`"pressure":1009.6,"gas":243703.0,"approx_altitude":30.1,"voltage":229.8,"current":3.410,"power":742.0,` +
		`"energy":12.345}`
)

// liveAt is when the test messages arrive, on the scenario clock.
func liveAt() time.Time { return time.Date(2026, 9, 20, 3, 15, 42, 0, time.UTC) }

// liveDevice is what devices.mqttNodes answers for a module of the seed's live site: its device, unit and sensors under
// the module's node_id (IR635).
func liveDevice(module seed.LiveModule) iotbridge.NodeDevice {
	sensors := make([]iotbridge.NodeSensor, 0, len(module.Sensors))
	for _, sensor := range module.Sensors {
		sensors = append(
			sensors,
			iotbridge.NodeSensor{
				ID:     seed.ID(sensor.Name),
				Metric: sensor.Metric,
				Unit:   sensor.Unit,
			},
		)
	}

	return iotbridge.NodeDevice{
		Node:     module.Node,
		DeviceID: seed.ID(module.Device),
		UnitID:   seed.ID(module.Unit),
		Control:  module.Control,
		Sensors:  sensors,
	}
}

// airConditioner is the live site's air conditioner (node_1), roomSensors its node_2 sensor unit.
func airConditioner() iotbridge.NodeDevice { return liveDevice(seed.LiveSite().Modules()[0]) }
func roomSensors() iotbridge.NodeDevice    { return liveDevice(seed.LiveSite().Modules()[1]) }

// TestLiveTopic: a node's topics name the node before their kind (command/result has two levels); the commands the
// bridge publishes, and other topics under the subscription, are none of the node's.
func TestLiveTopic(t *testing.T) {
	t.Parallel()

	for topic, want := range map[string][2]string{
		"smarthome/ac/node_1/sensors":        {node1, iotbridge.LiveSensors},
		"smarthome/ac/node_2/sensors":        {node2, iotbridge.LiveSensors},
		"smarthome/ac/node_1/state":          {node1, iotbridge.LiveState},
		"smarthome/ac/node_1/status":         {node1, iotbridge.LiveStatus},
		"smarthome/ac/node_1/command/result": {node1, iotbridge.LiveResult},
		"smarthome/ac/node_1/command":        {"", ""},
		"sensors":                            {"", ""},
		"/sensors":                           {"", ""},
		"command/result":                     {"", ""},
		"smarthome/ac/node_1/sensors/extra":  {"", ""},
	} {
		node, kind, ok := iotbridge.LiveTopic(topic)
		if node != want[0] || kind != want[1] || ok != (want[0] != "") {
			t.Errorf(
				"LiveTopic(%q) = %q, %q, %t; want %q, %q",
				topic,
				node,
				kind,
				ok,
				want[0],
				want[1],
			)
		}
	}
}

// TestLiveTriggersRoomNode: node_1 gives the room temperature and humidity on the air conditioner's sensors, stamped at
// the start of the scenario minute, measured and valid, and the device's heartbeat with no settings.
func TestLiveTriggersRoomNode(t *testing.T) {
	t.Parallel()

	out, err := iotbridge.LiveTriggers(airConditioner(), []byte(node1Message), liveAt())
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

// TestLiveTriggersPowerNode: node_2's power in W is recorded in kW on its sensor unit, with its device's heartbeat; a
// zero power is a reading while the meter sees a voltage, and with no voltage the meter measured nothing.
func TestLiveTriggersPowerNode(t *testing.T) {
	t.Parallel()

	dev := roomSensors()

	out, err := iotbridge.LiveTriggers(dev, []byte(node2Message), liveAt())
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

	if m := out[0].Measurement; m.UnitID != seed.ID("unit-live-node-2-sensors") ||
		m.SensorID != seed.ID("sensor-live-node-2-power") ||
		m.Unit != "kW" ||
		*m.Value != 0.742 {
		t.Errorf("power reading %+v (value %v)", m, *m.Value)
	}

	if hb := out[2]; hb.DeviceID == nil || *hb.DeviceID != seed.ID("device-live-node-2-sensors") {
		t.Errorf("node_2's heartbeat names device %v", hb.DeviceID)
	}

	off, err := iotbridge.LiveTriggers(dev, []byte(`{"voltage":230.1,"power":0.0}`), liveAt())
	if err != nil || len(off) != 3 || *off[0].Measurement.Value != 0 {
		t.Errorf("an air conditioner drawing nothing: %+v, %v", off, err)
	}

	none, err := iotbridge.LiveTriggers(
		dev,
		[]byte(`{"voltage":0.0,"current":0.0,"power":0.0}`),
		liveAt(),
	)
	if err != nil || none != nil {
		t.Errorf("a meter without voltage gave %+v, %v", none, err)
	}
}

// TestLiveTriggersNode2Room: node_2's room temperature, humidity and power are all recorded on its one device's sensors
// (IR635); one evaluation takes the facts of the unit, and the device gets one heartbeat.
func TestLiveTriggersNode2Room(t *testing.T) {
	t.Parallel()

	dev := roomSensors()

	out, err := iotbridge.LiveTriggers(dev, []byte(node2RoomMessage), liveAt())
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != 5 {
		t.Fatalf(
			"%d triggers, want temperature, humidity, power, their alert evaluation and the heartbeat",
			len(out),
		)
	}

	sensorUnit := seed.ID("unit-live-node-2-sensors")

	for idx, want := range []struct {
		metric, sensor string
		value          float64
	}{
		{sensorTemperature, "sensor-live-node-2-temperature", 28.3},
		{sensorHumidity, "sensor-live-node-2-humidity", 58.4},
		{sensorPower, "sensor-live-node-2-power", 0.742},
	} {
		m := out[idx].Measurement
		if m == nil || m.Metric != want.metric || m.SensorID != seed.ID(want.sensor) ||
			m.UnitID != sensorUnit ||
			*m.Value != want.value {
			t.Errorf("reading %d %+v, want %s on %s", idx, m, want.metric, want.sensor)
		}
	}

	evaluation := out[3].Event
	if out[3].EventType != eventAutomation || evaluation == nil ||
		!slices.Equal(evaluation.UnitIDs, []uuid.UUID{sensorUnit}) ||
		len(evaluation.Facts) != 3 {
		t.Fatalf("alert evaluation %+v", out[3])
	}

	for idx, fact := range evaluation.Facts {
		if m := out[idx].Measurement; fact.UnitID != m.UnitID || fact.Metric != m.Metric ||
			fact.Value != *m.Value {
			t.Errorf("fact %d %+v of reading %+v", idx, fact, m)
		}
	}

	if hb := out[4]; hb.EventType != eventState || hb.DeviceID == nil ||
		*hb.DeviceID != seed.ID("device-live-node-2-sensors") {
		t.Errorf("heartbeat names %v", hb.DeviceID)
	}

	roomOnly, err := iotbridge.LiveTriggers(
		dev,
		[]byte(`{"temp":28.1,"humidity":58.0,"voltage":0.0,"power":0.0}`),
		liveAt(),
	)
	if err != nil || len(roomOnly) != 4 || roomOnly[0].Measurement.UnitID != sensorUnit ||
		len(roomOnly[2].Event.Facts) != 2 {
		t.Errorf("room readings without the meter's: %+v, %v", roomOnly, err)
	}
}

// TestLiveTriggersOnlyWhatTheDeviceHas: a device registered with fewer sensors records only those, whatever else the
// node measures; a metric the bridge has no field for is left out.
func TestLiveTriggersOnlyWhatTheDeviceHas(t *testing.T) {
	t.Parallel()

	dev := roomSensors()
	dev.Sensors = []iotbridge.NodeSensor{
		dev.Sensors[0],
		{ID: seed.ID("sensor-x-co2"), Metric: "co2", Unit: "ppm"},
	}

	out, err := iotbridge.LiveTriggers(dev, []byte(node2RoomMessage), liveAt())
	if err != nil || len(out) != 3 || out[0].Measurement.Metric != sensorTemperature ||
		len(out[1].Event.Facts) != 1 {
		t.Errorf("temperature only: %+v, %v", out, err)
	}
}

// TestLiveTriggersSkipWhatIsNotMeasured: zeros the room node did not measure are no readings, and a message with
// nothing measured sends nothing, not even the heartbeat.
func TestLiveTriggersSkipWhatIsNotMeasured(t *testing.T) {
	t.Parallel()

	dev := airConditioner()

	out, err := iotbridge.LiveTriggers(dev, []byte(`{"temp":26.4,"humidity":0.0}`), liveAt())
	if err != nil || len(out) != 3 || out[0].Measurement.Metric != sensorTemperature ||
		len(out[1].Event.Facts) != 1 {
		t.Errorf("temperature only: %+v, %v", out, err)
	}

	out, err = iotbridge.LiveTriggers(
		dev,
		[]byte(`{"temp":0.0,"humidity":0.0,"pressure":0.0}`),
		liveAt(),
	)
	if err != nil || out != nil {
		t.Errorf("a failed read gave %+v, %v", out, err)
	}

	out, err = iotbridge.LiveTriggers(dev, []byte(`{"temp":"27.2","humidity":null}`), liveAt())
	if err != nil || out != nil {
		t.Errorf("fields that are no numbers gave %+v, %v", out, err)
	}
}

// TestLiveTriggersRefuse: a message that is no JSON is an error.
func TestLiveTriggersRefuse(t *testing.T) {
	t.Parallel()

	_, err := iotbridge.LiveTriggers(airConditioner(), []byte(`temp=27.2`), liveAt())
	if err == nil {
		t.Error("a message that is no JSON was converted")
	}
}

// node2Firmware04 is a message of node_2 on firmware 0.4, as the broker carried it on 2026-11-10: a sensor it does not
// have is left out, and the timestamp has the node's zone.
const node2Firmware04 = `{"node_id":"node_2","timestamp":"2026-11-10T17:35:26+07:00","temp":25.6,"humidity":43.8,` +
	`"pressure":1008.0,"gas":423837,"approx_altitude":43.9,"pm25":55}`

// TestLiveTriggersPM25: firmware 0.4's PM2.5 is recorded on node_2's sensor whenever it is there, 0 µg/m³ (clean air)
// too; a message without the field and without a meter's voltage has no PM2.5 and no power.
func TestLiveTriggersPM25(t *testing.T) {
	t.Parallel()

	dev := roomSensors()

	out, err := iotbridge.LiveTriggers(dev, []byte(node2Firmware04), liveAt())
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != 5 {
		t.Fatalf(
			"%d triggers, want temperature, humidity, PM2.5, their evaluation and the heartbeat",
			len(out),
		)
	}

	if m := out[2].Measurement; m == nil || m.Metric != metricPM25 ||
		m.SensorID != seed.ID("sensor-live-node-2-pm25") ||
		m.Unit != "µg/m³" ||
		*m.Value != 55 {
		t.Errorf("PM2.5 reading %+v", m)
	}

	clean, err := iotbridge.LiveTriggers(dev, []byte(`{"pm25":0}`), liveAt())
	if err != nil || len(clean) != 3 || clean[0].Measurement.Metric != metricPM25 ||
		*clean[0].Measurement.Value != 0 {
		t.Errorf("clean air: %+v, %v", clean, err)
	}

	if facts := out[3].Event.Facts; len(facts) != 3 || facts[2].Metric != metricPM25 ||
		facts[2].Value != 55 {
		t.Errorf("facts %+v", facts)
	}
}

// TestLiveReplay: a firmware-0.4 reading more than a minute older than its receipt is one the node kept while offline,
// measured at its timestamp moved onto the scenario clock; a reading of now, the first firmware's timestamp without a
// zone, and none at all are readings of now. A replayed reading has the event IDs of its minute, as a live one of that
// minute has, and goes to no alert policy and is no heartbeat.
func TestLiveReplay(t *testing.T) {
	t.Parallel()

	stamped := time.Date(2026, 11, 10, 10, 35, 26, 0, time.UTC) // node2Firmware04's 17:35:26+07:00
	offset := -50 * 24 * time.Hour                              // the scenario runs behind real time

	replayedAt, replayed := iotbridge.LiveReplayedAt(
		[]byte(node2Firmware04),
		stamped.Add(3*time.Minute),
		offset,
	)
	if !replayed || !replayedAt.Equal(stamped.Add(offset)) {
		t.Errorf("replayed at %s (%t), want %s", replayedAt, replayed, stamped.Add(offset))
	}

	for name, check := range map[string]struct {
		payload  string
		received time.Time
	}{
		"now":            {node2Firmware04, stamped.Add(5 * time.Second)},
		"a minute old":   {node2Firmware04, stamped.Add(time.Minute)},
		"first firmware": {node1Message, time.Date(2026, 10, 30, 6, 59, 0, 0, time.UTC)},
		"no timestamp":   {`{"temp":25.6}`, stamped.Add(time.Hour)},
		"not available":  {`{"timestamp":"N/A","temp":25.6}`, stamped.Add(time.Hour)},
		"no JSON":        {`temp=25.6`, stamped.Add(time.Hour)},
	} {
		if at, replayed := iotbridge.LiveReplayedAt(
			[]byte(check.payload),
			check.received,
			offset,
		); replayed {
			t.Errorf("%s: replayed at %s", name, at)
		}
	}

	dev := roomSensors()
	when := liveAt()
	observed := when.Add(-7*time.Minute - 20*time.Second)

	out, err := iotbridge.LiveReplayTriggers(dev, []byte(node2Firmware04), observed, when)
	if err != nil || len(out) != 3 {
		t.Fatalf("replay: %+v, %v; want temperature, humidity and PM2.5 only", out, err)
	}

	slot := observed.Truncate(time.Minute)
	for _, tr := range out {
		if m := tr.Measurement; m == nil || !m.ObservedAt.Equal(slot) ||
			!tr.OccurredAt.Equal(slot) ||
			!m.ReceivedAt.Equal(when) {
			t.Errorf("replayed reading %+v", tr)
		}
	}

	live, _ := iotbridge.LiveTriggers(dev, []byte(node2Firmware04), observed)
	if live[0].EventID != out[0].EventID {
		t.Error("a replayed reading and a live one of the same minute are two events")
	}

	_, err = iotbridge.LiveReplayTriggers(dev, []byte(`temp=25.6`), observed, when)
	if err == nil {
		t.Error("a replay that is no JSON was converted")
	}
}

// TestLiveHeardGone: a node's last will makes its gap at least LiveOfflineAfter at once, its real gap once that is
// longer, and a message after the will ends it; a will older than the last message changes nothing.
func TestLiveHeardGone(t *testing.T) {
	t.Parallel()

	heard := iotbridge.NewLiveHeard(liveAt().Add(-time.Hour))
	heard.Heard(node1, liveAt().Add(-10*time.Second))
	heard.Gone(node1, liveAt().Add(-5*time.Second))

	if g := heard.GapOf([]string{node1}, liveAt()); g != iotbridge.LiveOfflineAfter {
		t.Errorf("gap after the last will %s, want %s", g, iotbridge.LiveOfflineAfter)
	}

	if g := heard.GapOf(
		[]string{node1},
		liveAt().Add(5*time.Minute),
	); g != 5*time.Minute+10*time.Second {
		t.Errorf("gap 5 minutes after the will %s, want the real one", g)
	}

	heard.Heard(node1, liveAt())

	if g := heard.GapOf([]string{node1}, liveAt().Add(time.Second)); g != time.Second {
		t.Errorf("gap after the node came back %s, want 1s", g)
	}

	heard.Gone(node2, liveAt().Add(-time.Minute))
	heard.Heard(node2, liveAt())

	if g := heard.GapOf([]string{node2}, liveAt()); g != 0 {
		t.Errorf("an old will gave a gap of %s", g)
	}

	never := iotbridge.NewLiveHeard(liveAt())
	never.Gone(node1, liveAt())

	if g := never.GapOf(
		[]string{node1},
		liveAt().Add(time.Second),
	); g != iotbridge.LiveOfflineAfter {
		t.Errorf("a node never heard whose will came at the start: gap %s", g)
	}
}

// TestNodeRegistry: nothing is known before the first read; a node_id is matched exactly; a read replaces the whole
// table; the devices come in the order of their node_id.
func TestNodeRegistry(t *testing.T) {
	t.Parallel()

	registry := iotbridge.NewNodeRegistry()
	if registry.Loaded() || len(registry.Devices()) != 0 {
		t.Fatal("a new registry knows nodes")
	}

	air, sensors := airConditioner(), roomSensors()
	registry.Set([]iotbridge.NodeDevice{sensors, air})

	if !registry.Loaded() {
		t.Error("a registry that was set is not loaded")
	}

	if dev, ok := registry.Node(node1); !ok || dev.DeviceID != air.DeviceID {
		t.Errorf("node_1 is %+v", dev)
	}

	for _, unknown := range []string{"NODE_1", "node_3", "node_1 ", ""} {
		if _, ok := registry.Node(unknown); ok {
			t.Errorf("%q matched a node", unknown)
		}
	}

	if dev, ok := registry.Device(sensors.DeviceID); !ok || dev.Node != node2 {
		t.Errorf("device %s is %+v", sensors.DeviceID, dev)
	}

	if got := registry.Devices(); len(got) != 2 || got[0].Node != node1 || got[1].Node != node2 {
		t.Errorf("devices %+v", got)
	}

	registry.Set(
		[]iotbridge.NodeDevice{sensors},
	) // the node_id of the air conditioner was cleared in Admin

	if _, ok := registry.Node(node1); ok {
		t.Error("a node that was cleared is still registered")
	}

	if _, ok := registry.Device(air.DeviceID); ok {
		t.Error("a device that was cleared is still registered")
	}
}

// TestLiveEventIDIsOnePerMinute: messages of one minute share their event IDs (the API keeps one reading a minute), the
// next minute has others, and the heartbeat of a device and minute is one event whichever message it follows.
func TestLiveEventIDIsOnePerMinute(t *testing.T) {
	t.Parallel()

	dev := airConditioner()
	first, _ := iotbridge.LiveTriggers(dev, []byte(node1Message), liveAt())
	sameMinute, _ := iotbridge.LiveTriggers(
		dev,
		[]byte(`{"temp":27.4,"humidity":51.0}`),
		liveAt().Add(15*time.Second),
	)
	nextMinute, _ := iotbridge.LiveTriggers(
		dev,
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

	last := len(first) - 1
	if first[last].EventID != sameMinute[last].EventID ||
		first[last].EventID == nextMinute[last].EventID {
		t.Error("the heartbeats of one minute differ, or two minutes share one")
	}

	power, _ := iotbridge.LiveTriggers(roomSensors(), []byte(node2Message), liveAt())
	if power[len(power)-1].EventID == first[last].EventID {
		t.Error("two devices share a heartbeat event")
	}
}

// TestNodeFieldsMatchTheSite: every sensor of a live module is one the bridge reads (a metric of NodeFields, with a
// scale and a gate), whose unit is the platform's (the units the simulator reports, which the API checks); a sensor
// name is the site's once and a metric a module's once (devices.sensors is unique per device and metric); each module
// has its own node_id and a serial that Admin's register accepts (DD-T11); and the commands go to the node of the one
// module whose model takes them.
func TestNodeFieldsMatchTheSite(t *testing.T) {
	t.Parallel()

	platform := map[string]string{}
	for _, s := range devicesim.Sensors() {
		platform[s.Metric] = s.Unit
	}

	serial := regexp.MustCompile(`^[A-Z0-9-]{3,64}$`)
	node := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	names, nodes, fields := map[string]bool{}, map[string]bool{}, iotbridge.NodeFields()

	var commanded []string

	for _, module := range seed.LiveSite().Modules() {
		if !serial.MatchString(module.Serial) || !node.MatchString(module.Node) ||
			nodes[module.Node] {
			t.Errorf("%s: serial %q or node %q", module.UnitName, module.Serial, module.Node)
		}

		nodes[module.Node] = true

		if module.Control {
			commanded = append(commanded, module.Node)
		}

		metrics := map[string]bool{}

		for _, liveSensor := range module.Sensors {
			if names[liveSensor.Name] || metrics[liveSensor.Metric] {
				t.Errorf(
					"%s: sensor %s or metric %s twice",
					module.UnitName,
					liveSensor.Name,
					liveSensor.Metric,
				)
			}

			names[liveSensor.Name], metrics[liveSensor.Metric] = true, true

			field, ok := fields[liveSensor.Metric]
			if !ok || field.Scale == 0 || field.Field == "" {
				t.Errorf(
					"%s: the bridge does not read %s (%+v)",
					module.UnitName,
					liveSensor.Metric,
					field,
				)
			}

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
	}

	if !slices.Equal(commanded, []string{node1}) ||
		iotbridge.LiveCommandTopic(node1) != "smarthome/ac/node_1/command" {
		t.Errorf("commands go to %v (%s)", commanded, iotbridge.LiveCommandTopic(node1))
	}
}

// TestLiveHeartbeatGap: a module's gap is the longest time one of its nodes has gone unheard, counted from the bridge's
// start for a node never heard; it is sent for the module's unit as heartbeat_gap in minutes (floored to a tenth) on
// the minute, one evaluation a minute and unit.
func TestLiveHeartbeatGap(t *testing.T) {
	t.Parallel()

	start, both := liveAt().Add(-20*time.Minute), []string{node1, node2}

	liveHeard := iotbridge.NewLiveHeard(start)
	if g := liveHeard.GapOf(both, liveAt()); g != 20*time.Minute {
		t.Errorf("no node heard since the start: gap %s, want 20m", g)
	}

	liveHeard.Heard(node1, liveAt().Add(-5*time.Second))
	liveHeard.Heard(node2, liveAt().Add(-3*time.Minute))
	liveHeard.Heard(node2, liveAt().Add(-4*time.Minute)) // an older message changes nothing

	if g := liveHeard.GapOf(both, liveAt()); g != 3*time.Minute {
		t.Errorf("gap %s, want node_2's 3m", g)
	}

	if g := liveHeard.GapOf([]string{node1}, liveAt()); g != 5*time.Second {
		t.Errorf("node_1's own gap %s, want 5s", g)
	}
	// on the wall clock: a reading of the monotonic clock (which stops while the machine sleeps) is not used
	w := iotbridge.NewLiveHeard(time.Now())
	w.Heard(node1, time.Now())
	w.Heard(node2, time.Now())

	if g := w.GapOf(both, time.Now().Round(0).Add(2*time.Hour)); g < 2*time.Hour-time.Second {
		t.Errorf("two hours on the wall clock gave a gap of %s", g)
	}

	acUnit, sensorUnit := seed.ID("unit-live-ac"), seed.ID("unit-live-node-2-sensors")
	slot := liveAt().Truncate(time.Minute)
	trigger := iotbridge.LiveGapTrigger(acUnit, 17*time.Minute+50*time.Second, slot)

	facts := trigger.Event.Facts
	if trigger.EventType != eventAutomation || len(facts) != 1 ||
		facts[0].Metric != "heartbeat_gap" ||
		facts[0].Unit != "min" ||
		facts[0].Value != 17.8 ||
		!facts[0].ObservedAt.Equal(slot) ||
		facts[0].UnitID != acUnit ||
		!slices.Equal(trigger.Event.UnitIDs, []uuid.UUID{acUnit}) ||
		trigger.Event.Phase != "condition" {
		t.Errorf("gap trigger %+v %+v", trigger, facts)
	}

	if again := iotbridge.LiveGapTrigger(
		acUnit,
		time.Minute,
		slot,
	); again.EventID != trigger.EventID ||
		iotbridge.LiveGapTrigger(
			acUnit,
			time.Minute,
			slot.Add(time.Minute),
		).EventID == trigger.EventID {
		t.Error("the gap evaluation is not one a minute")
	}

	other := iotbridge.LiveGapTrigger(sensorUnit, time.Minute, slot)
	if other.EventID == trigger.EventID || other.Event.Facts[0].UnitID != sensorUnit ||
		!slices.Equal(other.Event.UnitIDs, []uuid.UUID{sensorUnit}) {
		t.Errorf("the sensor unit's gap of the minute %+v", other.Event)
	}
}

// TestLiveGate: one message per node and minute passes, other nodes are counted apart, an earlier minute never
// passes again, and a released minute is open again.
func TestLiveGate(t *testing.T) {
	t.Parallel()

	var liveGate iotbridge.LiveGate

	minute := liveAt().Truncate(time.Minute)
	if !liveGate.Claim(node1, minute) || liveGate.Claim(node1, minute) {
		t.Fatal("a minute passed twice or not at all")
	}

	if !liveGate.Claim(node2, minute) {
		t.Error("another node's minute was taken")
	}

	if !liveGate.Claim(node1, minute.Add(time.Minute)) || liveGate.Claim(node1, minute) {
		t.Error("the next minute did not pass, or an earlier one passed again")
	}

	liveGate.Release(node1, minute) // not the claimed minute: nothing changes

	if liveGate.Claim(node1, minute.Add(time.Minute)) {
		t.Error("releasing another minute opened the claimed one")
	}

	liveGate.Release(node1, minute.Add(time.Minute))

	if !liveGate.Claim(node1, minute.Add(time.Minute)) {
		t.Error("a released minute stayed closed")
	}
}

// TestLiveConnectionTrigger: silent nodes take the module offline once (communication_lost after 3 minutes, with the
// API's next sequence on the binding), nodes heard again bring it back by naming the open fault (restored within a
// minute), and in between, or while the connection already matches, or while unbound, nothing is sent.
func TestLiveConnectionTrigger(t *testing.T) {
	t.Parallel()

	binding, fault, dev := seed.ID(
		"binding-live-ac",
	), seed.ID(
		"fault-live",
	), seed.ID(
		"device-live-ac",
	)

	when := liveAt()
	if _, ok := iotbridge.LiveConnectionTrigger(
		dev,
		iotbridge.ConnectionFault{BindingID: nil, FaultID: nil, Sequence: 0},
		time.Hour,
		when,
	); ok {
		t.Error("an unbound module was reported")
	}

	lost, ok := iotbridge.LiveConnectionTrigger(
		dev,
		iotbridge.ConnectionFault{BindingID: &binding, Sequence: 7, FaultID: nil},
		4*time.Minute,
		when,
	)
	if !ok || lost.EventType != "device" || lost.Kind != "communication_lost" ||
		*lost.Sequence != 8 ||
		*lost.BindingID != binding ||
		*lost.DeviceID != dev ||
		lost.Recovery != nil ||
		!lost.OccurredAt.Equal(when) {
		t.Fatalf("loss %+v", lost)
	}

	sensors, sensorsBinding := seed.ID(
		"device-live-node-2-sensors",
	), seed.ID(
		"binding-live-node-2-sensors",
	)

	other, ok := iotbridge.LiveConnectionTrigger(
		sensors,
		iotbridge.ConnectionFault{BindingID: &sensorsBinding, Sequence: 7, FaultID: nil},
		4*time.Minute,
		when,
	)
	if !ok || *other.DeviceID != sensors || *other.BindingID != sensorsBinding ||
		other.EventID == lost.EventID {
		t.Errorf("the sensor unit's loss %+v", other)
	}

	again, _ := iotbridge.LiveConnectionTrigger(
		dev,
		iotbridge.ConnectionFault{BindingID: &binding, Sequence: 7, FaultID: nil},
		5*time.Minute,
		when.Add(time.Minute),
	)
	if again.EventID != lost.EventID {
		t.Error("a repeat of the loss is another event")
	}

	for _, gap := range []time.Duration{0, iotbridge.LiveOfflineAfter - time.Second} {
		if trigger, ok := iotbridge.LiveConnectionTrigger(
			dev,
			iotbridge.ConnectionFault{BindingID: &binding, FaultID: nil, Sequence: 0},
			gap,
			when,
		); ok {
			t.Errorf("gap %s took an online module offline: %+v", gap, trigger)
		}
	}

	back, ok := iotbridge.LiveConnectionTrigger(
		dev,
		iotbridge.ConnectionFault{BindingID: &binding, FaultID: &fault, Sequence: 8},
		20*time.Second,
		when,
	)
	if !ok || back.Kind != "restored" || *back.Sequence != 9 || back.Recovery == nil ||
		*back.Recovery != (iotbridge.Recovery{Axis: "connection", SourceEventID: fault, Value: connectionOnline}) {
		t.Fatalf("recovery %+v", back)
	}

	for _, gap := range []time.Duration{iotbridge.LiveOnlineWithin, 2 * time.Minute, time.Hour} {
		if trigger, ok := iotbridge.LiveConnectionTrigger(
			dev,
			iotbridge.ConnectionFault{BindingID: &binding, FaultID: &fault, Sequence: 8},
			gap,
			when,
		); ok {
			t.Errorf("gap %s restored an offline module: %+v", gap, trigger)
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

// TestLiveMinutes: a node's minute passes once in any order (a later one first does not close an earlier one), other
// nodes are counted apart, a released minute is open again, and minutes two days before the claimed one are
// forgotten.
func TestLiveMinutes(t *testing.T) {
	t.Parallel()

	var minutes iotbridge.LiveMinutes

	minute := liveAt().Truncate(time.Minute)
	if !minutes.Claim(node1, minute.Add(time.Minute)) || !minutes.Claim(node1, minute) {
		t.Fatal("a later minute first closed the earlier one")
	}

	if minutes.Claim(node1, minute) || minutes.Claim(node1, minute.Add(time.Minute)) {
		t.Error("a minute passed twice")
	}

	if !minutes.Claim(node2, minute) {
		t.Error("another node's minute was taken")
	}

	minutes.Release(node1, minute)

	if !minutes.Claim(node1, minute) {
		t.Error("a released minute stayed closed")
	}

	minutes.Release(node9, minute) // a node never claimed: nothing to give back

	if !minutes.Claim(node1, minute.Add(49*time.Hour)) || !minutes.Claim(node1, minute) {
		t.Error("a minute two days before the claimed one is still remembered")
	}
}

// TestLiveCheckTrigger: a live node's check succeeds while the node counts as online (unheard for less than
// LiveOfflineAfter) and fails after; the answer names the operation, one event per operation, in the body the demo
// trigger takes (operationId, result).
func TestLiveCheckTrigger(t *testing.T) {
	t.Parallel()

	check := iotbridge.LiveCheck{
		OperationID: seed.ID("op-live-check"),
		DeviceID:    seed.ID("device-live-ac"),
		Node:        node1,
	}

	for gap, want := range map[time.Duration]string{
		0:                                        resultSucceeded,
		iotbridge.LiveOfflineAfter - time.Second: resultSucceeded,
		iotbridge.LiveOfflineAfter:               "failed",
		time.Hour:                                "failed",
	} {
		trigger := iotbridge.LiveCheckTrigger(check, gap, liveAt())
		if trigger.EventType != "operation" || trigger.OperationID == nil ||
			*trigger.OperationID != check.OperationID ||
			trigger.Result != want ||
			!trigger.OccurredAt.Equal(liveAt()) {
			t.Errorf("gap %s: %+v, want %s", gap, trigger, want)
		}
	}

	first := iotbridge.LiveCheckTrigger(check, 0, liveAt())
	if again := iotbridge.LiveCheckTrigger(
		check,
		time.Hour,
		liveAt().Add(time.Second),
	); again.EventID != first.EventID {
		t.Error("two answers to one check are two events")
	}

	other := check
	other.OperationID = seed.ID("op-live-check-2")

	if iotbridge.LiveCheckTrigger(other, 0, liveAt()).EventID == first.EventID {
		t.Error("two checks share an event")
	}

	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}

	var body map[string]any

	err = json.Unmarshal(raw, &body)
	if err != nil {
		t.Fatal(err)
	}

	if body["operationId"] != check.OperationID.String() || body["result"] != resultSucceeded ||
		body["eventType"] != "operation" {
		t.Errorf("body %s", raw)
	}

	reading, err := json.Marshal(iotbridge.SentTrigger(iotbridge.Waiting{
		ID: seed.ID(
			"cmd-live",
		),
		DeviceID:  uuid.UUID{},
		UnitID:    uuid.UUID{},
		Action:    nil,
		ExpiresAt: time.Time{},
	}, liveAt()))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(reading), "operationId") ||
		strings.Contains(string(reading), `"result"`) {
		t.Errorf("another trigger carries the operation fields: %s", reading)
	}
}
