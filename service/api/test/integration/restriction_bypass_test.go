package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	alertRestrictionBypass = "restriction_bypass"
	severityCritical       = "critical"
	kindTamper             = "tamper"
)

// TestRestrictionBypassAlert: a device fault while its unit observes an applied restriction opens one critical
// restriction_bypass Alert for the unit (DEC-110, IR844) — the restriction is infrared from that node, so a node that
// went silent, lost power or was opened may no longer hold it. Every fault of the incident names the same Alert, a
// tamper also opens its own tamper Alert, a restored does not resolve it, a fault after it was resolved opens a new
// one that links it, and a unit without a restriction gets none.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionBypassAlert(t *testing.T) {
	srv := serverWith(t, true) // demo.trigger carries the device faults
	unit := boundUnit(t, srv, "online")
	plain := boundUnit(t, srv, "online") // under no restriction
	k, inv := overdueContract(t, srv, unit)

	_, res := executed(t, srv, k, inv, []string{unit})
	if !ackCommand(t, cmdOf(t, res, unit, "applyCommandIds"), clock().Add(time.Second)) {
		t.Fatal("the restriction command was not acknowledged")
	}

	var observed *string
	ownerScan(
		t,
		`SELECT observed_restriction->'policy'->>'kind' FROM assets.units WHERE id = $1`,
		[]any{unit},
		&observed,
	)

	if observed == nil {
		t.Fatal("the unit does not observe the applied restriction")
	}

	var serial string
	ownerScan(t, `SELECT serial FROM devices.devices WHERE unit_id = $1`, []any{unit}, &serial)

	sequences := map[string]int{}
	signal := func(target, kind string, sec int, recovery string) []uuid.UUID {
		t.Helper()

		var dev, binding string
		ownerScan(
			t,
			`SELECT id::text, binding_id::text FROM devices.devices WHERE unit_id = $1`,
			[]any{target},
			&dev,
			&binding,
		)

		sequences[target]++
		event := uuid.NewString()
		recoveryField := ""

		if recovery != "" {
			recoveryField = `,"recovery":` + recovery
		}

		code, answer := write(
			srv,
			customerB(),
			"demo.trigger",
			`{"scenarioId":"bypass","eventId":"`+event+
				`","occurredAt":"`+clock().Add(time.Duration(sec)*time.Second).Format(time.RFC3339)+
				`","eventType":"device","deviceId":"`+dev+`","bindingId":"`+binding+`","kind":"`+kind+
				`","sequence":`+itoa(sequences[target])+recoveryField+`}`,
			0,
		)
		if code != 200 {
			t.Fatalf("%s: %d %v", kind, code, answer)
		}

		var alerts []uuid.UUID
		ownerScan(
			t,
			`SELECT alert_ids FROM devices.device_events WHERE event_id = $1`,
			[]any{event},
			&alerts,
		)

		return alerts
	}
	alertOf := func(id uuid.UUID) (string, string, string, string, *uuid.UUID) {
		t.Helper()

		var (
			typ, severity, status, text string
			previous                    *uuid.UUID
		)
		ownerScan(
			t,
			`SELECT type, severity, status, evidence_text, previous_alert_id FROM monitoring.alerts WHERE id = $1`,
			[]any{id},
			&typ,
			&severity,
			&status,
			&text,
			&previous,
		)

		return typ, severity, status, text, previous
	}
	bypassAlerts := func(target string) int {
		t.Helper()

		var total int
		ownerScan(
			t,
			`SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND type = 'restriction_bypass'`,
			[]any{target},
			&total,
		)

		return total
	}

	// ① the node goes silent: one critical restriction_bypass Alert naming the fault and the restriction
	lost := signal(unit, "communication_lost", 10, "")
	if len(lost) != 1 {
		t.Fatalf("communication_lost under a restriction: alerts %v", lost)
	}

	bypass := lost[0]
	if typ, severity, status, text, previous := alertOf(bypass); typ != alertRestrictionBypass ||
		severity != severityCritical || status != statusOpen || previous != nil ||
		text != "Device "+serial+" stopped reporting while the unit is under a "+*observed+
			" restriction: the restriction may no longer hold" {
		t.Fatalf("bypass alert: %s %s %s %q %v", typ, severity, status, text, previous)
	}

	// HQ's restriction managers hear of it once, at the next tick (IR845), however many faults the incident has
	notices := func() (int, int) {
		t.Helper()
		drainEvents(t)

		var total, manager int
		ownerScan(
			t,
			`SELECT count(*), count(*) FILTER (WHERE recipient_membership_id = $2) FROM notify.notifications
			WHERE source_alert_id = $1 AND params->>'status' = 'restriction_bypass' AND severity = 'critical'`,
			[]any{bypass, seed.ID("hq-restriction-manager")},
			&total,
			&manager,
		)

		return total, manager
	}

	_, err := schedTick(context.Background(), srv, clock().Add(15*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	told, manager := notices()
	if told == 0 || manager != 1 {
		t.Fatalf(
			"HQ notices of the bypass alert: %d, restriction manager %d, want at least one and one",
			told,
			manager,
		)
	}

	// ② power lost in the same incident: the same Alert, no second one
	if power := signal(unit, "power_lost", 20, ""); len(power) != 1 || power[0] != bypass {
		t.Errorf("power_lost while the bypass alert is open: %v, want [%s]", power, bypass)
	}

	_, err = schedTick(context.Background(), srv, clock().Add(25*time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if again, _ := notices(); again != told {
		t.Errorf("HQ told again in the same incident: %d, then %d", told, again)
	}

	// ③ a tamper opens its own tamper Alert beside the same bypass Alert
	tamper := signal(unit, kindTamper, 30, "")
	if len(tamper) != 2 || tamper[1] != bypass {
		t.Fatalf("tamper under a restriction: %v, want [tamper, %s]", tamper, bypass)
	}

	if typ, _, _, _, _ := alertOf(tamper[0]); typ != kindTamper {
		t.Errorf("first alert of the tamper event: %s, want tamper", typ)
	}

	// ④ restored does not resolve it (it resolves by hand only)
	var lostID string
	ownerScan(
		t,
		`SELECT id::text FROM devices.device_events WHERE alert_ids = $1 AND event_type = 'communication_lost'`,
		[]any{lost},
		&lostID,
	)
	signal(
		unit,
		"restored",
		40,
		`{"axis":"connection","sourceEventId":"`+lostID+`","value":"online"}`,
	)

	if _, _, status, _, _ := alertOf(bypass); status != statusOpen {
		t.Errorf("bypass alert after restored: %s, want open", status)
	}

	// ⑤ resolved by hand, a later fault opens a new one that links it
	owner(
		t,
		`UPDATE monitoring.alerts SET status = 'resolved', resolved_at = $2 WHERE id = $1`,
		bypass,
		clock().Add(time.Minute),
	)

	again := signal(unit, "communication_lost", 120, "")
	if len(again) != 1 || again[0] == bypass {
		t.Fatalf("fault after the bypass alert was resolved: %v", again)
	}

	if _, _, status, _, previous := alertOf(
		again[0],
	); status != statusOpen || previous == nil ||
		*previous != bypass {
		t.Errorf("new bypass alert: %s previous %v, want open linking %s", status, previous, bypass)
	}

	if total := bypassAlerts(unit); total != 2 {
		t.Errorf("bypass alerts of the unit: %d, want 2", total)
	}

	// ⑥ a unit under no restriction gets no bypass Alert
	if alerts := signal(plain, "communication_lost", 10, ""); len(alerts) != 0 {
		t.Errorf("communication_lost without a restriction: alerts %v", alerts)
	}

	if total := bypassAlerts(plain); total != 0 {
		t.Errorf("bypass alerts of a unit without a restriction: %d", total)
	}
}
