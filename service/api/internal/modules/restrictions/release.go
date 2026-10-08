package restrictions

import (
	"context"
	"encoding/json"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// RequestRelease moves a requested/applied restriction to release_requested with a release intent and runs the
// D03 per-unit release evaluation once in the same transition (IR35, IR96). The caller checks the state.
func (m Restrictions) RequestRelease(ctx context.Context, c *ops.Call, id uuid.UUID, source string) error {
	intent, _ := json.Marshal(map[string]any{"source": source, "at": c.Now, "actorMembershipId": c.Principal.MembershipID})
	if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions SET state = 'release_requested', release_intent = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
		id, intent, c.Now); err != nil {
		return err
	}
	c.Emit(ops.Event{AggregateType: "restriction", AggregateID: id, Type: "RestrictionReleaseRequested", Payload: map[string]any{"source": source}})
	return m.evaluateRelease(ctx, c, id)
}

// evaluateRelease applies the D03 release table to every unit still at releaseState=none.
func (m Restrictions) evaluateRelease(ctx context.Context, c *ops.Call, id uuid.UUID) error {
	var rules string
	if err := c.Tx.QueryRow(ctx, `SELECT rules_version FROM restrictions.restrictions WHERE id = $1`, id).Scan(&rules); err != nil {
		return err
	}
	rows, err := c.Tx.Query(ctx, `SELECT unit_id, apply_state FROM restrictions.restriction_units WHERE restriction_id = $1 AND release_state = 'none' ORDER BY unit_id`, id)
	if err != nil {
		return err
	}
	type pending struct {
		unit  uuid.UUID
		apply string
	}
	var units []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.unit, &p.apply); err != nil {
			rows.Close()
			return err
		}
		units = append(units, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	remove, _ := json.Marshal(map[string]any{"kind": "remove_restriction", "restrictionId": id, "rulesVersion": rules})
	for _, u := range units {
		set := func(sql string, args ...any) error {
			_, err := c.Tx.Exec(ctx, `UPDATE restrictions.restriction_units SET `+sql+` WHERE restriction_id = $1 AND unit_id = $2`, append([]any{id, u.unit}, args...)...)
			return err
		}
		switch u.apply {
		case "not_sent": // undelivered apply is finalized as cancelled: zero-delivery evidence
			unit := u.unit
			if err := events.Publish(ctx, c.Tx, c.Principal.TenantID, "restriction", id, events.RestrictionCommandsCancelled, events.CommandsCancelled{RestrictionID: id, UnitID: &unit}); err != nil {
				return err
			}
			err = set(`apply_state = 'not_applied', release_state = 'not_required', pending_reason = NULL`)
		case "not_applied":
			err = set(`release_state = 'not_required', pending_reason = NULL`)
		case "sent_unknown":
			err = set(`release_state = 'waiting_reconcile'`)
		case "applied":
			var cmd uuid.UUID
			var delivered bool
			var reason *string
			cmd, delivered, reason, err = m.send(ctx, c, id, u.unit, remove)
			if err != nil {
				return err
			}
			if delivered {
				err = set(`release_state = 'requested', release_command_ids = release_command_ids || $3::uuid, pending_reason = NULL`, cmd)
			} else { // offline applied units wait for retry(phase=release); the undelivered intent is cancelled
				if err := events.Publish(ctx, c.Tx, c.Principal.TenantID, "restriction", id, events.RestrictionCommandsCancelled, events.CommandsCancelled{RestrictionID: id, CommandIDs: []uuid.UUID{cmd}}); err != nil {
					return err
				}
				err = set(`pending_reason = $3`, reason)
			}
		}
		if err != nil {
			return err
		}
	}
	return finishRelease(ctx, c.Tx, id, c.Now)
}

// finishRelease sets a release_requested restriction released once every unit is released / not_required (D03).
func finishRelease(ctx context.Context, tx pgx.Tx, id uuid.UUID, now time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE restrictions.restrictions r SET state = 'released', version = version + 1, updated_at = $2
		WHERE r.id = $1 AND r.state = 'release_requested'
		  AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru WHERE ru.restriction_id = r.id AND ru.release_state NOT IN ('released','not_required'))`, id, now)
	return err
}

// CommandAcknowledged applies a timely acknowledgement of a restriction Command (control.Acknowledge): apply →
// applyState=applied with the observed restriction (aggregate applied once every unit is applied, only from
// requested); remove → releaseState=released with no observed restriction (aggregate released per D03).
func CommandAcknowledged(ctx context.Context, tx pgx.Tx, command uuid.UUID, at time.Time) error {
	var restriction, unit uuid.UUID
	var kind string
	err := tx.QueryRow(ctx, `SELECT restriction_id, unit_id, action->>'kind' FROM control.commands WHERE id = $1 AND source = 'restriction'`, command).Scan(&restriction, &unit, &kind)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if kind == "remove_restriction" { // SR26: removes of terminal restrictions only resolve recovery cases
		if terminal, err := recoveryAcknowledged(ctx, tx, restriction, unit, command, at); err != nil || terminal {
			return err
		}
	}
	if kind == "apply_restriction" {
		if _, err := tx.Exec(ctx, `UPDATE restrictions.restriction_units ru SET apply_state = 'applied', observed_at = $3, pending_reason = NULL,
				observed_restriction = jsonb_build_object('restrictionId', r.id, 'rulesVersion', r.rules_version, 'policy', r.policy, 'observedAt', $3::timestamptz)
			FROM restrictions.restrictions r WHERE r.id = ru.restriction_id AND ru.restriction_id = $1 AND ru.unit_id = $2`, restriction, unit, at); err != nil {
			return err
		}
		var observed []byte
		if err := tx.QueryRow(ctx, `SELECT observed_restriction FROM restrictions.restriction_units WHERE restriction_id = $1 AND unit_id = $2`, restriction, unit).Scan(&observed); err != nil {
			return err
		}
		// the unit's observedRestriction is owned by equipment (IR184)
		if err := publish(ctx, tx, restriction, "unit", unit, events.UnitRestrictionApplied, events.UnitRestriction{UnitID: unit, RestrictionID: restriction, Observed: observed}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2,
			state = CASE WHEN r.state = 'requested' AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru WHERE ru.restriction_id = r.id AND ru.apply_state <> 'applied')
				THEN 'applied' ELSE r.state END
			WHERE r.id = $1`, restriction, at)
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE restrictions.restriction_units SET release_state = 'released', observed_restriction = NULL, observed_at = $3, pending_reason = NULL
		WHERE restriction_id = $1 AND unit_id = $2`, restriction, unit, at); err != nil {
		return err
	}
	if err := publish(ctx, tx, restriction, "unit", unit, events.UnitRestrictionCleared, events.UnitRestriction{UnitID: unit, RestrictionID: restriction}); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2,
		state = CASE WHEN r.state = 'release_requested' AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru
			WHERE ru.restriction_id = r.id AND ru.release_state NOT IN ('released','not_required')) THEN 'released' ELSE r.state END
		WHERE r.id = $1`, restriction, at)
	return err
}

// CommandsEnded applies D03 to restriction Commands that expired or failed: a remove that did not succeed sets
// releaseState=failed (aggregate stays release_requested); a rejected apply sets applyState=not_applied. Expired
// apply intents keep their state (not_sent: definitive non-delivery; sent_unknown: reconcile required).
func CommandsEnded(ctx context.Context, tx pgx.Tx, commands []uuid.UUID, now time.Time) error {
	if len(commands) == 0 {
		return nil
	}
	if err := recoveryEnded(ctx, tx, commands, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2
		WHERE r.id IN (SELECT restriction_id FROM control.commands WHERE id = ANY($1) AND source = 'restriction')`, commands, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE restrictions.restriction_units ru SET release_state = 'failed'
		FROM control.commands k WHERE k.id = ANY($1) AND k.source = 'restriction' AND k.action->>'kind' = 'remove_restriction'
		  AND ru.restriction_id = k.restriction_id AND ru.unit_id = k.unit_id AND ru.release_state = 'requested'`, commands); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE restrictions.restriction_units ru SET apply_state = 'not_applied'
		FROM control.commands k WHERE k.id = ANY($1) AND k.source = 'restriction' AND k.action->>'kind' = 'apply_restriction' AND k.status = 'failed'
		  AND ru.restriction_id = k.restriction_id AND ru.unit_id = k.unit_id AND ru.apply_state = 'sent_unknown'`, commands)
	return err
}
