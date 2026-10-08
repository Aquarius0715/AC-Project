package restrictions

import (
	"context"
	"encoding/json"
	"time"

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
	Policy        json.RawMessage `json:"policy"`
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
	if err == nil { // SR29: a recovery-case change also increments the target unit's version
		_, err = tx.Exec(ctx, `UPDATE assets.units SET version = version + 1, updated_at = $2 WHERE id IN (SELECT unit_id FROM restrictions.restriction_units WHERE restriction_id = $1)`, id, now)
	}
	return err
}

// Observe applies a device observation of a unit (demo.trigger restriction_observation, SR26). The unit's
// observedRestriction is updated. When the observed restriction is released or cancelled, the terminal state is kept
// and a pending recovery case is opened (or the open case for the same unit / restriction / rules version is updated)
// together with one open reconciliation_required Alert. It returns whether a recovery case changed.
func Observe(ctx context.Context, tx pgx.Tx, now time.Time, unit uuid.UUID, observed *Observation, eventID uuid.UUID) (bool, error) {
	var obsRaw any
	if observed != nil {
		obsRaw, _ = json.Marshal(observed)
	}
	if _, err := tx.Exec(ctx, `UPDATE assets.units SET observed_restriction = $2, last_seen_at = $3 WHERE id = $1`, unit, obsRaw, now); err != nil {
		return false, err
	}
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
	_, err = tx.Exec(ctx, `INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, type, severity, status, cause_code, evidence_kind, evidence_text, observed_at, detected_at, created_at, updated_at)
		SELECT u.tenant_id, u.id, u.customer_org_id, 'reconciliation_required', 'warning', 'open', 'unknown', 'demo_observation',
			'Device still reports an ended restriction; reconcile it from HQ', $2, $2, $2, $2 FROM assets.units u
		WHERE u.id = $1 AND NOT EXISTS (SELECT 1 FROM monitoring.alerts a WHERE a.unit_id = u.id AND a.type = 'reconciliation_required' AND a.status <> 'resolved')`, unit, now)
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
		var raw []byte
		var at *time.Time
		if err := c.Tx.QueryRow(ctx, `SELECT observed_restriction, last_seen_at FROM assets.units WHERE id = $1`, u).Scan(&raw, &at); err != nil {
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
func resolveRecoveryAlerts(ctx context.Context, tx pgx.Tx, units []uuid.UUID, now time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE monitoring.alerts a SET status = 'resolved', resolved_at = $2, resolution_reason = 'recovery resolved', version = version + 1, updated_at = $2
		WHERE a.unit_id = ANY($1) AND a.type = 'reconciliation_required' AND a.status <> 'resolved'
		  AND NOT EXISTS (SELECT 1 FROM restrictions.restrictions r, jsonb_array_elements(r.recovery_cases) k
			WHERE k->>'unitId' = a.unit_id::text AND k->>'state' <> 'resolved')`, units, now)
	return err
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
	if _, err := tx.Exec(ctx, `UPDATE assets.units SET observed_restriction = NULL WHERE id = $1 AND observed_restriction->>'restrictionId' = $2::text`, unit, restriction); err != nil {
		return true, err
	}
	if err := saveCases(ctx, tx, restriction, cases, at); err != nil {
		return true, err
	}
	return true, resolveRecoveryAlerts(ctx, tx, []uuid.UUID{unit}, at)
}

// recoveryEnded returns failed or expired recovery removes to pending (SR26).
func recoveryEnded(ctx context.Context, tx pgx.Tx, commands []uuid.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT restriction_id FROM control.commands WHERE id = ANY($1) AND source = 'restriction' AND restriction_id IS NOT NULL`, commands)
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
