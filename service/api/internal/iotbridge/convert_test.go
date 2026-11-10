package iotbridge_test

import (
	"slices"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// observedAt is when the test readings were taken, and receivedAt when the bridge received them.
func observedAt() time.Time { return time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC) }

func receivedAt() time.Time { return time.Date(2026, 10, 13, 2, 0, 1, 0, time.UTC) }

func batch(device string, seqs ...int64) devicesim.Batch {
	out := devicesim.Batch{DeviceID: iotbridge.DeviceID(device).String(), Measurements: nil}

	for i, seq := range seqs {
		name := []string{sensorTemperature, sensorPower}[i%2]
		v := 24.5
		out.Measurements = append(
			out.Measurements,
			devicesim.Measurement{
				SensorID:   name,
				Metric:     name,
				Value:      &v,
				Unit:       "°C",
				ObservedAt: observedAt(),
				Sequence:   seq,
			},
		)
	}

	return out
}

// TestTriggersUseTheFixtureIDs: a reading of a demo device becomes a telemetry trigger on its fixture unit and sensor,
// measured and valid, with the scenario label and the receiving time.
func TestTriggersUseTheFixtureIDs(t *testing.T) {
	t.Parallel()

	r := iotbridge.NewResolver(iotbridge.Links())

	out, err := r.Triggers(batch("device-online-rto", 1, 2), receivedAt())
	if err != nil {
		t.Fatal(err)
	}

	if len(out) != 2 {
		t.Fatalf("%d triggers, want 2", len(out))
	}

	trigger := out[0]
	if trigger.ScenarioID != iotbridge.Scenario || trigger.EventType != "telemetry" ||
		trigger.Measurement.Origin != "measured" {
		t.Errorf("trigger %+v", trigger)
	}

	if trigger.Measurement.UnitID != seed.ID("unit-online-rto") ||
		trigger.Measurement.SensorID != seed.ID("sensor-online-temperature") {
		t.Errorf(
			"unit or sensor ID %v %v",
			trigger.Measurement.UnitID,
			trigger.Measurement.SensorID,
		)
	}

	if trigger.Measurement.Value == nil || *trigger.Measurement.Value != 24.5 ||
		!trigger.Measurement.ReceivedAt.Equal(receivedAt()) {
		t.Errorf("value or receipt time %+v", trigger.Measurement)
	}
	// the API keys a repeat on the measurement's own event ID and the scenario sequence: both must be the reading's
	if trigger.Measurement.Quality != qualityValid ||
		trigger.Measurement.EventID != trigger.EventID ||
		trigger.Measurement.Sequence != nil {
		t.Errorf("quality, event ID or sequence of the measurement: %+v", trigger.Measurement)
	}
}

// TestEventIDIsOnePerReading: the same reading twice has the same event ID (a redelivery is one event), and the next
// reading of the device has another.
func TestEventIDIsOnePerReading(t *testing.T) {
	t.Parallel()

	resolver := iotbridge.NewResolver(iotbridge.Links())
	first, _ := resolver.Triggers(batch("device-online-rto", 7), receivedAt())
	redelivered, _ := resolver.Triggers(
		batch("device-online-rto", 7),
		receivedAt().Add(time.Minute),
	)
	next := batch("device-online-rto", 8)
	next.Measurements[0].ObservedAt = observedAt().Add(time.Minute)
	// the next reading of the sensor is another reading
	following, _ := resolver.Triggers(next, receivedAt())

	if first[0].EventID != redelivered[0].EventID {
		t.Error("a redelivered reading has a new event ID")
	}

	if first[0].EventID == following[0].EventID {
		t.Error("two readings share an event ID")
	}
	// a restarted simulator starts its sequence again: the same sequence at a later time is another reading
	later := batch("device-online-rto", 7)
	later.Measurements[0].ObservedAt = observedAt().Add(time.Hour)

	restarted, _ := resolver.Triggers(later, receivedAt())
	if restarted[0].EventID == first[0].EventID {
		t.Error("a restarted sequence at a later time is the same event")
	}
}

// TestUnknownDeviceIsRefused: a device that is not a demo link converts to nothing and says why.
func TestUnknownDeviceIsRefused(t *testing.T) {
	t.Parallel()

	out, err := iotbridge.NewResolver(iotbridge.Links()).
		Triggers(batch("device-unknown", 1), receivedAt())
	if err == nil || len(out) != 0 {
		t.Fatalf("unknown device: %d triggers, err %v", len(out), err)
	}
}

// TestUnknownSensorSkipsOnlyThatReading: a sensor the device does not have is an error, and the other readings of the
// batch still convert.
func TestUnknownSensorSkipsOnlyThatReading(t *testing.T) {
	t.Parallel()

	readings := batch("device-offline-rto", 1, 2)
	readings.Measurements = append(
		readings.Measurements,
		devicesim.Measurement{
			SensorID:   sensorHumidity,
			Metric:     sensorHumidity,
			Value:      new(float64(50)),
			Unit:       "%",
			ObservedAt: observedAt(),
			Sequence:   3,
		},
	)

	out, err := iotbridge.NewResolver(iotbridge.Links()).Triggers(readings, receivedAt())
	if err == nil {
		t.Error("an unknown sensor is not reported")
	}

	if len(out) != 2 {
		t.Errorf("%d triggers, want the two known readings", len(out))
	}
}

// TestCommandTriggerMapsTheAcknowledgement: applied is command_ack, unsupported is command_fail, and an expired
// acknowledgement is not forwarded at all.
func TestCommandTriggerMapsTheAcknowledgement(t *testing.T) {
	t.Parallel()

	cmd := "9f67089f-44c9-5231-ba20-03d3b9a1be53"
	ack := func(result string) devicesim.Ack {
		return devicesim.Ack{
			DeviceID:   iotbridge.DeviceID("device-online-rto").String(),
			CommandID:  cmd,
			Result:     result,
			ReceivedAt: time.Time{},
			Observed:   devicesim.Observed{SetpointCelsius: 0},
		}
	}

	trigger, ok, err := iotbridge.CommandTrigger(ack("applied"), receivedAt())
	if err != nil || !ok || trigger.EventType != "command_ack" || trigger.CommandID == nil ||
		trigger.CommandID.String() != cmd ||
		trigger.Measurement != nil {
		t.Fatalf("applied: %+v ok=%v err=%v", trigger, ok, err)
	}

	trigger, ok, _ = iotbridge.CommandTrigger(ack("unsupported"), receivedAt())
	if !ok || trigger.EventType != "command_fail" {
		t.Errorf("unsupported: %+v ok=%v", trigger, ok)
	}

	if _, ok, _ := iotbridge.CommandTrigger(ack("expired"), receivedAt()); ok {
		t.Error("an expired acknowledgement is forwarded")
	}

	_, _, commandTriggerErr := iotbridge.CommandTrigger(
		devicesim.Ack{
			CommandID:  "not-a-uuid",
			Result:     "applied",
			DeviceID:   "",
			ReceivedAt: time.Time{},
			Observed:   devicesim.Observed{SetpointCelsius: 0},
		},
		receivedAt(),
	)
	if commandTriggerErr == nil {
		t.Error("a command ID that is not a UUID is accepted")
	}
}

// TestBadDeviceIDIsRefused: a device ID in the message that is not a UUID converts to nothing.
func TestBadDeviceIDIsRefused(t *testing.T) {
	t.Parallel()

	out, err := iotbridge.NewResolver(
		iotbridge.Links(),
	).Triggers(devicesim.Batch{DeviceID: "not-a-uuid", Measurements: nil}, receivedAt())
	if err == nil || len(out) != 0 {
		t.Fatalf("bad device ID: %d triggers, err %v", len(out), err)
	}
}

// TestFleetLinksResolve: a mock fleet module's reading converts onto its own unit and sensor, and a unit without a
// module has no link.
func TestFleetLinksResolve(t *testing.T) {
	t.Parallel()

	f, err := devicesim.MockFleet()
	if err != nil {
		t.Fatal(err)
	}

	links := iotbridge.FleetLinks(f)
	if len(links) != 18 {
		t.Fatalf("%d fleet links, want one per pattern with a module (18)", len(links))
	}

	resolver := iotbridge.NewResolver(
		slices.Concat(iotbridge.Links(), links),
	)

	out, err := resolver.Triggers(batch("device-mock-tamper", 1, 2), receivedAt())
	if err != nil || len(out) != 2 {
		t.Fatalf("fleet reading: %d triggers, err %v", len(out), err)
	}

	if out[0].Measurement.UnitID != seed.ID("unit-mock-tamper") ||
		out[0].Measurement.SensorID != seed.ID("sensor-mock-tamper-temperature") {
		t.Errorf("fleet IDs %v %v", out[0].Measurement.UnitID, out[0].Measurement.SensorID)
	}

	_, triggersErr := resolver.Triggers(batch("device-mock-no-module", 1), receivedAt())
	if triggersErr == nil {
		t.Error("a unit without a module has a link")
	}
}

// TestToScenarioKeepsTheMinuteSlot: a reading received 19 seconds into a real minute lands at the start of the scenario
// minute it was received in, whatever seconds the clock offset has; a reading the module stamped an hour ahead stays an
// hour ahead; the batch given is not changed.
func TestToScenarioKeepsTheMinuteSlot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 13, 5, 0, 19, 0, time.UTC)
	offset := -(29*24*time.Hour + 4*time.Hour + 17*time.Second + 403*time.Millisecond)
	margin := 2 * time.Second
	readings := batch("device-online-rto", 1, 2)
	readings.Measurements[0].ObservedAt = now.Truncate(time.Minute)
	readings.Measurements[1].ObservedAt = now.Truncate(time.Minute).Add(time.Hour)
	got := iotbridge.ToScenario(readings, now, offset, margin)

	slot := now.Add(offset - margin).Truncate(time.Minute)
	if !got.Measurements[0].ObservedAt.Equal(slot) {
		t.Errorf("reading at %v, want the received minute %v", got.Measurements[0].ObservedAt, slot)
	}

	if !got.Measurements[1].ObservedAt.Equal(slot.Add(time.Hour)) {
		t.Errorf(
			"future reading at %v, want %v",
			got.Measurements[1].ObservedAt,
			slot.Add(time.Hour),
		)
	}

	if !readings.Measurements[0].ObservedAt.Equal(now.Truncate(time.Minute)) {
		t.Error("the given batch was changed")
	}

	if received := now.Add(offset - margin); got.Measurements[0].ObservedAt.After(received) {
		t.Error("a reading is observed after it was received")
	}
	// a batch the module stamped a minute earlier (sent late) still lands in the minute it was received in
	late := batch("device-online-rto", 3)

	late.Measurements[0].ObservedAt = now.Truncate(time.Minute).Add(-time.Minute)
	if got := iotbridge.ToScenario(
		late,
		now,
		offset,
		margin,
	); !got.Measurements[0].ObservedAt.Equal(
		slot,
	) {
		t.Errorf("late reading at %v, want %v", got.Measurements[0].ObservedAt, slot)
	}
}

// TestLinksFollowTheFixture: the links are the fixture contract's devices (demoSeed.devices), each with its unit and
// its connection there, so the simulator runs every fixture module and plays the connection the seed gives it (IR613).
func TestLinksFollowTheFixture(t *testing.T) {
	t.Parallel()

	fixture, err := seed.Load("../../../../docs/04-agentic-sdlc/fixture-contract.json")
	if err != nil {
		t.Fatal(err)
	}

	devices := map[string]struct{ unit, connection string }{}
	for _, device := range fixture.DemoSeed.Devices {
		unit := ""
		if device.UnitID != nil {
			unit = *device.UnitID
		}

		devices[device.ID] = struct{ unit, connection string }{unit, device.Connection}
	}

	links := iotbridge.Links()
	for _, link := range links {
		want, ok := devices[link.Device]
		if !ok || want.unit != link.Unit || want.connection != link.Connection {
			t.Errorf("%s: link %s/%s, fixture %+v", link.Device, link.Unit, link.Connection, want)
		}
	}

	if len(links) != len(devices) {
		t.Errorf("%d links for %d fixture devices", len(links), len(devices))
	}
}
