package iotbridge

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

var (
	at       = time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	received = time.Date(2026, 10, 13, 2, 0, 1, 0, time.UTC)
)

func batch(device string, seqs ...int64) devicesim.Batch {
	b := devicesim.Batch{DeviceID: DeviceID(device).String()}
	for i, s := range seqs {
		name := []string{"temperature", "power"}[i%2]
		v := 24.5
		b.Measurements = append(b.Measurements, devicesim.Measurement{SensorID: name, Metric: name, Value: v, Unit: "°C", ObservedAt: at, Sequence: s})
	}
	return b
}

// TestTriggersUseTheFixtureIDs: a reading of a demo device becomes a telemetry trigger on its fixture unit and sensor,
// measured and valid, with the scenario label and the receiving time.
func TestTriggersUseTheFixtureIDs(t *testing.T) {
	r := NewResolver(Links)
	out, err := r.Triggers(batch("device-online-rto", 1, 2), received)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("%d triggers, want 2", len(out))
	}
	tr := out[0]
	if tr.ScenarioID != Scenario || tr.EventType != "telemetry" || tr.Measurement.Origin != "measured" {
		t.Errorf("trigger %+v", tr)
	}
	if tr.Measurement.UnitID != seed.ID("unit-online-rto") || tr.Measurement.SensorID != seed.ID("sensor-online-temperature") {
		t.Errorf("unit or sensor ID %v %v", tr.Measurement.UnitID, tr.Measurement.SensorID)
	}
	if tr.Measurement.Value == nil || *tr.Measurement.Value != 24.5 || !tr.Measurement.ReceivedAt.Equal(received) {
		t.Errorf("value or receipt time %+v", tr.Measurement)
	}
	// the API keys a repeat on the measurement's own event ID and the scenario sequence: both must be the reading's
	if tr.Measurement.Quality != "valid" || tr.Measurement.EventID != tr.EventID || tr.Measurement.Sequence == nil || *tr.Measurement.Sequence != 1 {
		t.Errorf("quality, event ID or sequence of the measurement: %+v", tr.Measurement)
	}
}

// TestEventIDIsOnePerReading: the same reading twice has the same event ID (a redelivery is one event), and the next
// reading of the device has another.
func TestEventIDIsOnePerReading(t *testing.T) {
	r := NewResolver(Links)
	a, _ := r.Triggers(batch("device-online-rto", 7), received)
	b, _ := r.Triggers(batch("device-online-rto", 7), received.Add(time.Minute))
	c, _ := r.Triggers(batch("device-online-rto", 8), received)
	if a[0].EventID != b[0].EventID {
		t.Error("a redelivered reading has a new event ID")
	}
	if a[0].EventID == c[0].EventID {
		t.Error("two readings share an event ID")
	}
}

// TestUnknownDeviceIsRefused: a device that is not a demo link converts to nothing and says why.
func TestUnknownDeviceIsRefused(t *testing.T) {
	out, err := NewResolver(Links).Triggers(batch("device-no-sensor", 1), received)
	if err == nil || len(out) != 0 {
		t.Fatalf("unknown device: %d triggers, err %v", len(out), err)
	}
}

// TestUnknownSensorSkipsOnlyThatReading: a sensor the device does not have is an error, and the other readings of the
// batch still convert.
func TestUnknownSensorSkipsOnlyThatReading(t *testing.T) {
	b := batch("device-offline-rto", 1, 2)
	b.Measurements = append(b.Measurements, devicesim.Measurement{SensorID: "humidity", Metric: "humidity", Value: 50, Unit: "%", ObservedAt: at, Sequence: 3})
	out, err := NewResolver(Links).Triggers(b, received)
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
	cmd := "9f67089f-44c9-5231-ba20-03d3b9a1be53"
	ack := func(result string) devicesim.Ack {
		return devicesim.Ack{DeviceID: DeviceID("device-online-rto").String(), CommandID: cmd, Result: result}
	}
	tr, ok, err := CommandTrigger(ack("applied"), received)
	if err != nil || !ok || tr.EventType != "command_ack" || tr.CommandID == nil || tr.CommandID.String() != cmd || tr.Measurement != nil {
		t.Fatalf("applied: %+v ok=%v err=%v", tr, ok, err)
	}
	tr, ok, _ = CommandTrigger(ack("unsupported"), received)
	if !ok || tr.EventType != "command_fail" {
		t.Errorf("unsupported: %+v ok=%v", tr, ok)
	}
	if _, ok, _ := CommandTrigger(ack("expired"), received); ok {
		t.Error("an expired acknowledgement is forwarded")
	}
	if _, _, err := CommandTrigger(devicesim.Ack{CommandID: "not-a-uuid", Result: "applied"}, received); err == nil {
		t.Error("a command ID that is not a UUID is accepted")
	}
}
