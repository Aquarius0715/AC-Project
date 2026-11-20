package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/ops"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartProposalsFromLivePowerMessages(t *testing.T) {
	srv := serverCfg(t, func(c *apiserver.Config) {
		c.DemoOps = true
		c.InternalToken = internalToken
	})
	metered := proposalDevice(t, newUnit(t, srv, "Proposal AC with live power messages"))
	sensor := proposalMeter(t, metered)
	rule := startProposalRule(t, srv, metered.unit, "[1,3,5]")
	ruleID := asStr(t, rule["id"])
	before := scheduleByID(t, srv, metered.unit, ruleID)

	unitID, err := uuid.Parse(metered.unit)
	if err != nil {
		t.Fatal(err)
	}

	node := iotbridge.NodeDevice{
		Node: "node_" + unitID.String(), DeviceID: metered.device, UnitID: unitID, Control: true,
		Sensors: []iotbridge.NodeSensor{{ID: sensor, Metric: "power", Unit: "kW"}},
	}
	firmwareZone := time.FixedZone("WIB", 7*60*60)
	ingest := func(slot time.Time, second int, watts float64) []byte {
		t.Helper()

		const voltage = 227.5

		received := slot.Add(time.Duration(second) * time.Second)
		payload := fmt.Appendf(
			nil,
			`{"node_id":"%s","timestamp":"%s","voltage":%.1f,"current":%.3f,"power":%.1f,"energy":0.067}`,
			node.Node,
			received.In(firmwareZone).Format(time.RFC3339),
			voltage,
			watts/voltage,
			watts,
		)
		ingestStartPowerMessage(t, srv, node, payload, received)

		return payload
	}
	transition := func(slot time.Time, beforeWatts float64) {
		t.Helper()

		metered.command(t, slot.Add(20*time.Second), "ui", actionPowerOn)
		ingest(slot.Add(-time.Minute), 2, beforeWatts)
		ingest(slot.Add(time.Minute), 2, 150)
	}
	noProposal := func(reason string) {
		t.Helper()

		if got := proposalFor(t, srv, customerB(), rule); got != nil {
			t.Fatalf("%s: unexpected proposal %v", reason, got)
		}
	}

	// The fixed test clock is Monday 2026-09-14 in Kuala Lumpur.
	wednesday := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	early := wednesday.AddDate(0, 0, -14).Add(-15 * time.Minute)
	middle := wednesday.AddDate(0, 0, -7)

	transition(early, 10)                        // Aug 26, 19:45
	transition(middle, 10)                       // Sep 2, 20:00
	transition(wednesday.AddDate(0, 0, -28), 10) // Aug 12, fifth Wednesday
	noProposal("the fifth Wednesday cannot supply the third vote")

	transition(wednesday.AddDate(0, 0, -21), 150) // Aug 19, already running
	noProposal("another ON while already running cannot supply the third vote")

	// A valid transition outside the 30-minute spread cannot join both existing votes.
	transition(wednesday.Add(20*time.Minute), 10) // Sep 9, 20:20
	noProposal("three recent starts do not fit within ±15 minutes of a common candidate")

	slot := wednesday.Add(15 * time.Minute) // Sep 9, 20:15
	metered.command(t, slot.Add(20*time.Second), "ui", actionPowerOn)
	ingest(slot.Add(-time.Minute), 2, 10)
	ingest(slot, 25, 150)
	noProposal("a power message received in the send minute cannot confirm the start")

	postSlot := slot.Add(time.Minute)
	received := postSlot.Add(2 * time.Second)
	payload := ingest(postSlot, 2, 150)
	// Redelivery in the same minute changes received_at but keeps the event ID. The first reading is retained.
	ingestStartPowerMessage(t, srv, node, payload, received.Add(5*time.Second))

	var total, valid int
	ownerScan(t, `SELECT count(*), count(*) FILTER (WHERE unit_id = $3 AND received_at = $4
		AND value = 0.15 AND unit = 'kW' AND origin = 'measured' AND quality = 'valid'
		AND boundary_id = 'ac_input_electricity')
		FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at = $2`,
		[]any{sensor, postSlot, unitID, received}, &total, &valid)

	if total != 1 || valid != 1 {
		t.Fatalf("stored post-start samples: %d total, %d matching, want 1 of each", total, valid)
	}

	dates := []any{
		early.Format(time.DateOnly),
		middle.Format(time.DateOnly),
		slot.Format(time.DateOnly),
	}
	proposal := proposalFor(t, srv, customerB(), rule)
	assertStartProposal(t, proposal, metered.unit, 3, "20:00", dates)

	displayed := scheduleByID(t, srv, metered.unit, ruleID)
	if !reflect.DeepEqual(before, displayed) {
		t.Fatalf(
			"listing the proposal modified the schedule: before %v, after %v",
			before,
			displayed,
		)
	}

	assertIngestedStartApproval(t, srv, metered.unit, rule, proposal)
}

func ingestStartPowerMessage(
	t *testing.T,
	srv *apiserver.Server,
	node iotbridge.NodeDevice,
	payload []byte,
	received time.Time,
) {
	t.Helper()

	triggers, err := iotbridge.LiveTriggers(node, payload, received)
	if err != nil {
		t.Fatal(err)
	}

	readings := 0

	for _, trigger := range triggers {
		if trigger.Measurement == nil {
			continue // alert evaluation and heartbeat do not feed start learning
		}

		readings++

		body, err := json.Marshal(trigger)
		if err != nil {
			t.Fatal(err)
		}

		method, path, reader := ops.ClientRequest("demo.trigger", string(body))
		req := httptest.NewRequestWithContext(t.Context(), method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", trigger.EventID.String())
		req.Header.Set(ops.InternalTokenHeader, internalToken)

		rec := httptest.NewRecorder()
		srv.Echo.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("ingest %s: status %d: %s", received, rec.Code, rec.Body)
		}
	}

	if readings != 1 {
		t.Fatalf("bridge converted %d measurements from %s, want 1", readings, payload)
	}
}

func assertIngestedStartApproval(
	t *testing.T, srv *apiserver.Server, unit string, rule, proposal map[string]any,
) {
	t.Helper()

	commandCount := func() int {
		t.Helper()

		var count int
		ownerScan(
			t,
			`SELECT count(*) FROM control.commands WHERE unit_id = $1`,
			[]any{unit},
			&count,
		)

		return count
	}
	before := commandCount()

	version := int(asNum(t, rule["version"]))

	code, res := write(srv, customerB(), approveStartProposal, approvalBody(t, proposal), version)
	if code != http.StatusOK {
		t.Fatalf("approve Wednesday: %d %v", code, res)
	}

	if after := commandCount(); after != before {
		t.Fatalf("approval changed the command count from %d to %d", before, after)
	}

	ruleID := asStr(t, rule["id"])

	appliedID := asStr(t, data(res)["id"])
	if appliedID == ruleID {
		t.Fatal("partial approval did not split the schedule")
	}

	assertPreservedSchedule(
		t,
		rule,
		scheduleByID(t, srv, unit, appliedID),
		[]any{float64(3)},
		"20:00",
	)
	assertPreservedSchedule(
		t,
		rule,
		scheduleByID(t, srv, unit, ruleID),
		[]any{float64(1), float64(5)},
		"18:30",
	)
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartProposalsUseIndependentObservationsBeforeMeterProfile(t *testing.T) {
	srv := server(t)
	metered := proposalDevice(t, newUnit(t, srv, "Proposal AC with newly verified meter"))
	sensor := proposalMeter(t, metered)
	rule := startProposalRule(t, srv, metered.unit, "[4]")
	last := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	dates := []any{"2026-08-27", "2026-09-03", "2026-09-10"}

	for _, daysAgo := range []int{14, 7, 0} {
		metered.transition(t, last.AddDate(0, 0, -daysAgo), "ui")
	}

	// The verified period starts after all three transitions, so independent observations still support the proposal.
	owner(
		t,
		`UPDATE control.start_power_bands SET verified_from = $2 WHERE sensor_id = $1`,
		sensor,
		last.Add(time.Hour),
	)
	assertStartProposal(t, proposalFor(t, srv, customerB(), rule), metered.unit, 4, "20:00", dates)

	// The latest transition is now inside that period. The existing independent post observation cannot fill its gap.
	owner(
		t,
		`UPDATE control.start_power_bands SET verified_from = $2 WHERE sensor_id = $1`,
		sensor,
		last.Add(-time.Minute),
	)
	proposalReading(t, metered.unit, sensor, last.Add(-time.Minute), 0.01)

	if got := proposalFor(t, srv, customerB(), rule); got != nil {
		t.Fatalf("old observations filled a metered evidence gap: %v", got)
	}

	proposalReading(t, metered.unit, sensor, last.Add(time.Minute), 0.15)
	assertStartProposal(t, proposalFor(t, srv, customerB(), rule), metered.unit, 4, "20:00", dates)
}
