package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const startMeterMaximumStandby = 0.015

// proposalMeter creates a test-only verified profile. Its limits are synthetic.
func proposalMeter(t *testing.T, fixture startEvidenceFixture) uuid.UUID {
	t.Helper()

	sensor := uuid.New()
	owner(t, `INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id)
		SELECT $2, tenant_id, id, 'power', 'kW', 'ac_input_electricity' FROM devices.devices WHERE id = $1`,
		fixture.device, sensor)
	owner(t, `INSERT INTO control.start_power_bands
		(tenant_id, binding_id, sensor_id, verified_from, standby_max_kw, running_min_kw)
		SELECT tenant_id, id, $2, bound_at, $3, 0.08 FROM devices.device_bindings WHERE id = $1`,
		fixture.binding, sensor, startMeterMaximumStandby)

	t.Cleanup(func() {
		owner(t, `DELETE FROM monitoring.measurements WHERE sensor_id = $1`, sensor)
		owner(t, `DELETE FROM control.start_power_bands WHERE sensor_id = $1`, sensor)
		owner(t, `DELETE FROM devices.sensors WHERE id = $1`, sensor)
	})

	return sensor
}

func proposalReading(
	t *testing.T,
	unit string,
	sensor uuid.UUID,
	slot time.Time,
	kilowatts float64,
) {
	t.Helper()

	owner(t, `INSERT INTO monitoring.measurements
		(tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality,
		boundary_id, received_at, event_id)
		SELECT tenant_id, id, $2, 'power', $3, 1, $4, 'kW', 'measured', 'valid',
		'ac_input_electricity', $3::timestamptz + interval '2 seconds', gen_random_uuid()
		FROM assets.units WHERE id = $1`, unit, sensor, slot, kilowatts)
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartProposalsUseVerifiedMeter(t *testing.T) {
	srv := server(t)
	metered := proposalDevice(t, newUnit(t, srv, "Metered proposal AC"))
	other := proposalDevice(t, newUnit(t, srv, "Other metered AC"))
	sensor := proposalMeter(t, metered)
	otherSensor := proposalMeter(t, other)
	rule := startProposalRule(t, srv, metered.unit, "[3]")
	otherRule := startProposalRule(t, srv, other.unit, "[3]")

	// Send crosses into the next minute. Only the minute after the send is eligible on the post side.
	for i := range 3 {
		request := time.Date(2026, 8, 26+i*7, 12, 0, 55, 0, time.UTC)
		sent := request.Add(10 * time.Second)

		metered.delayedCommand(t, request, sent, "ui", actionPowerOn)
		proposalReading(
			t,
			other.unit,
			otherSensor,
			request.Truncate(time.Minute).Add(-time.Minute),
			0.01,
		)
		proposalReading(
			t,
			other.unit,
			otherSensor,
			sent.Truncate(time.Minute).Add(time.Minute),
			0.15,
		)
	}

	if proposal := proposalFor(t, srv, customerB(), rule); proposal != nil {
		t.Fatalf("only another AC had power readings: %v", proposal)
	}

	if proposal := proposalFor(t, srv, customerB(), otherRule); proposal != nil {
		t.Fatalf("only power readings without a command: %v", proposal)
	}

	for i := range 3 {
		before := time.Date(2026, 8, 26+i*7, 11, 59, 0, 0, time.UTC)
		proposalReading(t, metered.unit, sensor, before, 0.01)
	}

	if proposal := proposalFor(t, srv, customerB(), rule); proposal != nil {
		t.Fatalf("only one side of the transitions was measured: %v", proposal)
	}

	for i := range 3 {
		after := time.Date(2026, 8, 26+i*7, 12, 2, 0, 0, time.UTC)
		proposalReading(t, metered.unit, sensor, after, 0.15)
	}

	assertStartProposal(t, proposalFor(t, srv, customerB(), rule), metered.unit, 3, "20:00",
		[]any{"2026-08-26", "2026-09-02", "2026-09-09"})

	// All three dates are needed. Corrupt the post evidence on one date at a time and restore it for the next case.
	checkMeterRejections(t, srv, rule, metered, other, sensor, otherSensor)
}

func checkMeterRejections(
	t *testing.T,
	srv *apiserver.Server,
	rule map[string]any,
	metered, other startEvidenceFixture,
	sensor, otherSensor uuid.UUID,
) {
	t.Helper()

	postSlot := time.Date(2026, 8, 26, 12, 2, 0, 0, time.UTC)
	for _, test := range []struct {
		name, update    string
		value, original any
	}{
		{
			"suspect", `UPDATE monitoring.measurements SET quality = $3 WHERE sensor_id = $1 AND observed_at = $2`,
			"suspect", qualityValid,
		},
		{
			"estimated", `UPDATE monitoring.measurements SET origin = CASE WHEN $3 THEN 'estimated' ELSE 'measured' END
				WHERE sensor_id = $1 AND observed_at = $2`,
			true, false,
		},
		{
			"late", `UPDATE monitoring.measurements SET received_at = $3 WHERE sensor_id = $1 AND observed_at = $2`,
			postSlot.Add(time.Minute), postSlot.Add(2 * time.Second),
		},
		{
			"ambiguous watts", `UPDATE monitoring.measurements SET value = $3 WHERE sensor_id = $1 AND observed_at = $2`,
			0.04, 0.15,
		},
		{
			"wrong unit", `UPDATE monitoring.measurements SET unit_id = $3 WHERE sensor_id = $1 AND observed_at = $2`,
			other.unit, metered.unit,
		},
	} {
		owner(t, test.update, sensor, postSlot, test.value)

		if proposal := proposalFor(t, srv, customerB(), rule); proposal != nil {
			t.Errorf("%s supplied a third supporting date: %v", test.name, proposal)
		}

		owner(t, test.update, sensor, postSlot, test.original)
	}

	owner(t, `UPDATE monitoring.measurements SET sensor_id = $3, sequence = 2
		WHERE sensor_id = $1 AND observed_at = $2`, sensor, postSlot, otherSensor)

	if proposal := proposalFor(t, srv, customerB(), rule); proposal != nil {
		t.Fatalf("reading loaned by another AC's sensor was used: %v", proposal)
	}

	owner(t, `UPDATE monitoring.measurements SET sensor_id = $1, sequence = 1
		WHERE sensor_id = $2 AND unit_id = $3 AND observed_at = $4`, sensor, otherSensor, metered.unit, postSlot)

	owner(
		t,
		`UPDATE control.start_power_bands SET verified_from = $2 WHERE sensor_id = $1`,
		sensor,
		clock(),
	)

	if proposal := proposalFor(t, srv, customerB(), rule); proposal != nil {
		t.Fatalf("measurements before the verified profile period were used: %v", proposal)
	}

	owner(t, `UPDATE control.start_power_bands p SET verified_from = b.bound_at
		FROM devices.device_bindings b WHERE p.binding_id = b.id AND p.sensor_id = $1`, sensor)

	owner(
		t,
		`UPDATE monitoring.measurements SET observed_at = $3, received_at = $3::timestamptz + interval '20 seconds'
		WHERE sensor_id = $1 AND observed_at = $2`,
		sensor,
		postSlot,
		postSlot.Add(-time.Minute),
	)

	if proposal := proposalFor(t, srv, customerB(), rule); proposal != nil {
		t.Fatalf("same-minute post reading was used: %v", proposal)
	}

	owner(
		t,
		`UPDATE monitoring.measurements SET observed_at = $3, received_at = $3::timestamptz + interval '2 seconds'
		WHERE sensor_id = $1 AND observed_at = $2`,
		sensor,
		postSlot.Add(-time.Minute),
		postSlot,
	)

	// Request outside the observation window, but actual send inside it.
	metered.delayedCommand(
		t,
		postSlot.Add(-3*time.Minute-10*time.Second),
		postSlot.Add(-3*time.Minute+time.Second),
		"automation",
		actionPowerOff,
	)

	if proposal := proposalFor(t, srv, customerB(), rule); proposal != nil {
		t.Fatalf("a competing send inside the observation window was counted: %v", proposal)
	}
}
