package restrictions

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/events"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// RecoveryCase is RestrictionRecoveryCase (SR26).
type RecoveryCase struct {
	ID                     uuid.UUID   `json:"id"`
	UnitID                 uuid.UUID   `json:"unitId"`
	ObservedRestrictionID  uuid.UUID   `json:"observedRestrictionId"`
	ObservedRulesVersion   string      `json:"observedRulesVersion"`
	SuccessorRestrictionID *uuid.UUID  `json:"successorRestrictionId"`
	State                  string      `json:"state"`
	ObservationEventID     uuid.UUID   `json:"observationEventId"`
	CommandIDs             []uuid.UUID `json:"commandIds"`
	CreatedAt              time.Time   `json:"createdAt"`
	ResolvedAt             *time.Time  `json:"resolvedAt"`
}

// Observation is ObservedRestriction (null when no restriction is observed).
type Observation struct {
	RestrictionID uuid.UUID       `json:"restrictionId"`
	RulesVersion  string          `json:"rulesVersion"`
	Policy        json.RawMessage `json:"policy" swaggertype:"object"`
	ObservedAt    time.Time       `json:"observedAt"`
}

func loadCases(ctx context.Context, tx pgx.Tx, id uuid.UUID, lock bool) ([]RecoveryCase, string, int, error) {
	q := `SELECT recovery_cases, state, version FROM restrictions.restrictions WHERE id = $1`
	if lock {
		q += " FOR UPDATE"
	}
	var raw []byte
	var state string
	var version int
	if err := tx.QueryRow(ctx, q, id).Scan(&raw, &state, &version); err != nil {
		return nil, "", 0, err
	}
	var cases []RecoveryCase
	_ = json.Unmarshal(raw, &cases)
	return cases, state, version, nil
}

func saveCases(ctx context.Context, tx pgx.Tx, id uuid.UUID, cases []RecoveryCase, now time.Time) error {
	raw, _ := json.Marshal(cases)
	_, err := tx.Exec(ctx, `UPDATE restrictions.restrictions SET recovery_cases = $2, version = version + 1, updated_at = $3 WHERE id = $1`, id, raw, now)
	if err != nil {
		return err
	}
	// SR29: a recovery-case change also increments the target units' versions — owned by equipment (IR184)
	units, err := restrictionUnits(ctx, tx, id)
	if err != nil || len(units) == 0 {
		return err
	}
	return publish(ctx, tx, id, "restriction", id, events.RecoveryCasesChanged, events.Units{RestrictionID: id, UnitIDs: units})
}

func restrictionUnits(ctx context.Context, tx pgx.Tx, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT unit_id FROM restrictions.restriction_units WHERE restriction_id = $1 ORDER BY unit_id`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// Observe applies a device observation of a unit (demo.trigger restriction_observation, SR26). The unit's
// observedRestriction is updated. When the observed restriction is released or cancelled, the terminal state is kept
// and a pending recovery case is opened (or the open case for the same unit / restriction / rules version is updated)
// together with one open reconciliation_required Alert. It returns whether a recovery case changed.
// The unit's own observedRestriction is recorded by the device side before this is called (IR184).
func Observe(ctx context.Context, tx pgx.Tx, now time.Time, unit uuid.UUID, observed *Observation, eventID uuid.UUID) (bool, error) {
	if observed == nil {
		return false, nil
	}
	cases, state, _, err := loadCases(ctx, tx, observed.RestrictionID, true)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil || (state != "released" && state != "cancelled") {
		return false, err
	}
	var successor *uuid.UUID
	var s uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT r.id FROM restrictions.restrictions r JOIN restrictions.restriction_units ru ON ru.restriction_id = r.id
		WHERE ru.unit_id = $1 AND r.id <> $2 AND r.state NOT IN ('released','cancelled') ORDER BY r.created_at DESC LIMIT 1`, unit, observed.RestrictionID).Scan(&s); err == nil {
		successor = &s
	} else if err != pgx.ErrNoRows {
		return false, err
	}
	found := false
	for i := range cases {
		c := &cases[i]
		if c.State != "resolved" && c.UnitID == unit && c.ObservedRestrictionID == observed.RestrictionID && c.ObservedRulesVersion == observed.RulesVersion {
			c.ObservationEventID, c.SuccessorRestrictionID, found = eventID, successor, true
		}
	}
	if !found {
		cases = append(cases, RecoveryCase{ID: uuid.New(), UnitID: unit, ObservedRestrictionID: observed.RestrictionID, ObservedRulesVersion: observed.RulesVersion,
			SuccessorRestrictionID: successor, State: "pending", ObservationEventID: eventID, CommandIDs: []uuid.UUID{}, CreatedAt: now})
	}
	if err := saveCases(ctx, tx, observed.RestrictionID, cases, now); err != nil {
		return false, err
	}
	// the reconciliation_required alert is owned by equipment (IR184)
	err = publish(ctx, tx, observed.RestrictionID, "restriction", observed.RestrictionID, events.ReconciliationRequired, events.Units{RestrictionID: observed.RestrictionID, UnitIDs: []uuid.UUID{unit}})
	return true, err
}

// reconcileTerminal handles restrictions.reconcile for a released/cancelled restriction with unresolved recovery cases
// (SR26): per unit the observation must be at most 30 s old (TIMEOUT) and the device online without an unfinished
// restriction Command (OFFLINE / CONFLICT). Observation of the old restriction and rules version → a remove Command
// and state removing; no observation or the successor → resolved without Commands; any other restriction → CONFLICT.
func (m Restrictions) reconcileTerminal(ctx context.Context, c *ops.Call, x Restriction, units []uuid.UUID) (any, error) {
	cases, _, _, err := loadCases(ctx, c.Tx, x.ID, false)
	if err != nil {
		return nil, err
	}
	changed := false
	for _, u := range units {
		idx := -1
		for i, k := range cases {
			if k.UnitID == u && k.State != "resolved" {
				idx = i
			}
		}
		if idx < 0 {
			return nil, apperr.Fields(map[string]string{"unitIds": "errors.reconcile_not_needed"})
		}
		k := &cases[idx]
		if k.State == "removing" {
			return nil, apperr.E(apperr.Conflict, "errors.command_pending")
		}
		_, conn, power, bound, err := m.Devices.BoundDevice(ctx, c, u)
		if err != nil {
			return nil, err
		}
		if !bound || conn != "online" || power == "off" {
			return nil, apperr.E(apperr.Offline, "errors.device_offline")
		}
		raw, at, err := m.Units.Observation(ctx, c, u) // equipment's observation (IR194)
		if err != nil {
			return nil, err
		}
		if at == nil || c.Now.Sub(*at) > IntentTTL {
			return nil, apperr.E(apperr.Timeout, "errors.observation_stale")
		}
		var o *Observation
		if len(raw) > 0 {
			o = &Observation{}
			_ = json.Unmarshal(raw, o)
		}
		switch {
		case o == nil || (k.SuccessorRestrictionID != nil && o.RestrictionID == *k.SuccessorRestrictionID):
			k.State, k.ResolvedAt = "resolved", &c.Now
		case o.RestrictionID == k.ObservedRestrictionID && o.RulesVersion == k.ObservedRulesVersion:
			remove, _ := json.Marshal(map[string]any{"kind": "remove_restriction", "restrictionId": x.ID, "rulesVersion": k.ObservedRulesVersion})
			cmd, delivered, _, err := m.send(ctx, c, x.ID, u, remove)
			if err != nil {
				return nil, err
			}
			if !delivered {
				return nil, apperr.E(apperr.Conflict, "errors.unit_busy")
			}
			k.State, k.CommandIDs = "removing", append(k.CommandIDs, cmd)
		default:
			return nil, apperr.E(apperr.Conflict, "errors.observation_mismatch")
		}
		changed = true
	}
	if changed {
		if err := saveCases(ctx, c.Tx, x.ID, cases, c.Now); err != nil {
			return nil, err
		}
		if err := resolveRecoveryAlerts(ctx, c.Tx, units, c.Now); err != nil {
			return nil, err
		}
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "restrictions.reconcile", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next})
	y, err := load(ctx, c, x.ID, false)
	return project(c, y), err
}

// resolveRecoveryAlerts resolves the reconciliation_required Alert of units without unresolved cases.
// The alerts belong to equipment: restrictions publishes the units that no longer have an unresolved case (IR184).
func resolveRecoveryAlerts(ctx context.Context, tx pgx.Tx, units []uuid.UUID, _ time.Time) error {
	rows, err := tx.Query(ctx, `SELECT u FROM unnest($1::uuid[]) AS u
		WHERE NOT EXISTS (SELECT 1 FROM restrictions.restrictions r, jsonb_array_elements(r.recovery_cases) k
			WHERE k->>'unitId' = u::text AND k->>'state' <> 'resolved') ORDER BY u`, units)
	if err != nil {
		return err
	}
	done, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(done) == 0 {
		return err
	}
	var tenant uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')::uuid`).Scan(&tenant); err != nil {
		return err
	}
	return events.Publish(ctx, tx, tenant, "unit", done[0], events.ReconciliationResolved, events.Units{UnitIDs: done})
}

// recoveryAcknowledged resolves the removing case of a terminal restriction when its remove Command is acknowledged.
func recoveryAcknowledged(ctx context.Context, tx pgx.Tx, restriction, unit, command uuid.UUID, at time.Time) (bool, error) {
	cases, state, _, err := loadCases(ctx, tx, restriction, true)
	if err != nil || (state != "released" && state != "cancelled") {
		return false, err
	}
	hit := false
	for i := range cases {
		for _, id := range cases[i].CommandIDs {
			if id == command && cases[i].State == "removing" {
				cases[i].State, cases[i].ResolvedAt, hit = "resolved", &at, true
			}
		}
	}
	if !hit {
		return true, nil // terminal restriction: never change its per-unit state
	}
	if err := publish(ctx, tx, restriction, "unit", unit, events.UnitRestrictionCleared, events.UnitRestriction{UnitID: unit, RestrictionID: restriction}); err != nil {
		return true, err
	}
	if err := saveCases(ctx, tx, restriction, cases, at); err != nil {
		return true, err
	}
	return true, resolveRecoveryAlerts(ctx, tx, []uuid.UUID{unit}, at)
}

// recoveryEnded returns failed or expired recovery removes to pending (SR26).
func recoveryEnded(ctx context.Context, tx pgx.Tx, commands []uuid.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT restriction_id FROM restrictions.restriction_commands WHERE command_id = ANY($1)`, commands)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, r := range ids {
		cases, state, _, err := loadCases(ctx, tx, r, true)
		if err != nil || (state != "released" && state != "cancelled") {
			if err != nil {
				return err
			}
			continue
		}
		hit := false
		for i := range cases {
			for _, id := range cases[i].CommandIDs {
				for _, done := range commands {
					if id == done && cases[i].State == "removing" {
						cases[i].State, hit = "pending", true
					}
				}
			}
		}
		if hit {
			if err := saveCases(ctx, tx, r, cases, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// publish writes an event in the tenant of restriction (the transaction may run without a tenant context: worker
// ticks, device acknowledgements).
func publish(ctx context.Context, tx pgx.Tx, restriction uuid.UUID, aggregateType string, aggregateID uuid.UUID, eventType string, payload any) error {
	var tenant uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT tenant_id FROM restrictions.restrictions WHERE id = $1`, restriction).Scan(&tenant); err != nil {
		return err
	}
	return events.Publish(ctx, tx, tenant, aggregateType, aggregateID, eventType, payload)
}
