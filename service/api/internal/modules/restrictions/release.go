package restrictions

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// RequestRelease moves a requested/applied restriction to release_requested with a release intent and runs the
// D03 per-unit release evaluation once in the same transition (IR35, IR96). The caller checks the state.
func (m Restrictions) RequestRelease(
	ctx context.Context,
	call *ops.Call,
	restrictionID uuid.UUID,
	source string,
) error {
	intent, marshalErr := json.Marshal(
		map[string]any{
			keySource:           source,
			"at":                call.Now,
			"actorMembershipId": call.Principal.MembershipID,
		},
	)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, err := call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions SET state = 'release_requested', release_intent = $2, version = version + 1,
			updated_at = $3 WHERE id = $1`,
		restrictionID,
		intent,
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "update restrictions.restrictions")
	}

	call.Emit(
		ops.Event{
			AggregateType: kindRestriction,
			AggregateID:   restrictionID,
			Type:          "RestrictionReleaseRequested",
			Payload:       map[string]any{keySource: source},
		},
	)

	return m.evaluateRelease(ctx, call, restrictionID)
}

// evaluateRelease applies the D03 release table to every unit still at releaseState=none.
func (m Restrictions) evaluateRelease(ctx context.Context, call *ops.Call, id uuid.UUID) error {
	var rules string

	err := call.Tx.QueryRow(ctx, `SELECT rules_version FROM restrictions.restrictions WHERE id = $1`, id).
		Scan(&rules)
	if err != nil {
		return apperr.Wrap(err, "select restrictions.restrictions")
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT unit_id, apply_state FROM restrictions.restriction_units WHERE restriction_id = $1 AND release_state = 'none'
			ORDER BY unit_id`,
		id,
	)
	if err != nil {
		return apperr.Wrap(err, "select restrictions.restriction_units")
	}

	type pending struct {
		unit  uuid.UUID
		apply string
	}

	var units []pending

	for rows.Next() {
		var pendingUnit pending

		err := rows.Scan(&pendingUnit.unit, &pendingUnit.apply)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select restrictions.restriction_units")
		}

		units = append(units, pendingUnit)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select restrictions.restriction_units")
	}

	remove, marshalErr := json.Marshal(
		map[string]any{
			keyKind:          actionRemoveRestriction,
			keyRestrictionID: id,
			keyRulesVersion:  rules,
		},
	)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	for _, pendingUnit := range units {
		err := m.releaseUnit(ctx, call, id, pendingUnit.unit, pendingUnit.apply, remove)
		if err != nil {
			return err
		}
	}

	return finishRelease(ctx, call.Tx, id, call.Now)
}

// releaseUnit starts a unit's release by how far its apply went. An undelivered apply is finalised as cancelled (the
// zero-delivery evidence); a unit that never applied needs no release; one whose apply result is unknown waits for
// reconciling; an applied unit gets the remove command, or, while offline, waits for a release retry.
func (m Restrictions) releaseUnit(
	ctx context.Context,
	call *ops.Call,
	restriction, unit uuid.UUID,
	apply string,
	remove []byte,
) error {
	set := func(sql string, args ...any) error {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE restrictions.restriction_units SET `+sql+` WHERE restriction_id = $1 AND unit_id = $2`,
			append([]any{restriction, unit}, args...)...)

		return apperr.Wrap(err, "update restrictions.restriction_units")
	}

	switch apply {
	case applyNotSent:
		err := cancelUndelivered(ctx, call, restriction, unit)
		if err != nil {
			return err
		}

		return set(
			`apply_state = 'not_applied', release_state = 'not_required', pending_reason = NULL`,
		)
	case "not_applied":
		return set(`release_state = 'not_required', pending_reason = NULL`)
	case applySentUnknown:
		return set(`release_state = 'waiting_reconcile'`)
	case stateApplied:
		cmd, delivered, reason, err := m.send(ctx, call, restriction, unit, remove)
		if err != nil {
			return err
		}

		if delivered {
			return set(
				`release_state = 'requested', release_command_ids = release_command_ids || $3::uuid, pending_reason = NULL`,
				cmd,
			)
		}

		err = cancelCommand(ctx, call, restriction, cmd)
		if err != nil {
			return err
		}

		return set(`pending_reason = $3`, reason)
	}

	return nil
}

// cancelUndelivered cancels a unit's requested commands of the restriction that were never delivered, and tells
// equipment, which cancels them by the same rule.
func cancelUndelivered(ctx context.Context, call *ops.Call, restriction, unit uuid.UUID) error {
	err := events.Publish(
		ctx,
		call.Tx,
		call.Principal.TenantID,
		kindRestriction,
		restriction,
		events.RestrictionCommandsCancelled,
		events.CommandsCancelled{RestrictionID: restriction, UnitID: &unit, CommandIDs: nil},
	)
	if err != nil {
		return apperr.Wrap(err, "events.Publish")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restriction_commands SET status = 'cancelled', updated_at = $3
		WHERE restriction_id = $1 AND unit_id = $2 AND status = 'requested' AND NOT delivered`,
		restriction,
		unit,
		call.Now,
	)

	return apperr.Wrap(err, "update restrictions.restriction_commands")
}

// cancelCommand cancels one undelivered command, and tells equipment.
func cancelCommand(ctx context.Context, call *ops.Call, restriction, command uuid.UUID) error {
	err := events.Publish(
		ctx,
		call.Tx,
		call.Principal.TenantID,
		kindRestriction,
		restriction,
		events.RestrictionCommandsCancelled,
		events.CommandsCancelled{
			RestrictionID: restriction,
			CommandIDs:    []uuid.UUID{command},
			UnitID:        nil,
		},
	)
	if err != nil {
		return apperr.Wrap(err, "events.Publish")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restriction_commands SET status = 'cancelled', updated_at = $2 WHERE command_id = $1 AND
			status = 'requested'`,
		command,
		call.Now,
	)

	return apperr.Wrap(err, "update restrictions.restriction_commands")
}

// finishRelease sets a release_requested restriction released once every unit is released / not_required (D03).
func finishRelease(ctx context.Context, tx pgx.Tx, restrictionID uuid.UUID, now time.Time) error {
	_, err := tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions r SET state = 'released', version = version + 1, updated_at = $2
		WHERE r.id = $1 AND r.state = 'release_requested'
		  AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru WHERE ru.restriction_id = r.id AND ru.release_state
		  NOT IN ('released','not_required'))`,
		restrictionID,
		now,
	)

	return apperr.Wrap(err, "update restrictions.restrictions")
}

// CommandAcknowledged applies a timely acknowledgement of a restriction Command (control.Acknowledge): apply →
// applyState=applied with the observed restriction (aggregate applied once every unit is applied, only from
// requested); remove → releaseState=released with no observed restriction (aggregate released per D03).
func CommandAcknowledged(ctx context.Context, tx pgx.Tx, command uuid.UUID, when time.Time) error {
	var (
		restriction, unit uuid.UUID
		kind              string
	)

	err := tx.QueryRow(ctx, `SELECT restriction_id, unit_id, kind FROM restrictions.restriction_commands WHERE command_id =
		$1`, command).
		Scan(&restriction, &unit, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return apperr.Wrap(err, "select restrictions.restriction_commands")
	}
	// the restriction is locked before its unit changes: two answers for units of one restriction (or an answer beside
	// HQ's reconcile) wait for each other, and the second judges "every unit applied" on the first one's result (IR477)
	_, execErr := tx.Exec(
		ctx,
		`SELECT 1 FROM restrictions.restrictions WHERE id = $1 FOR UPDATE`,
		restriction,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "select restrictions.restrictions")
	}

	if kind == actionRemoveRestriction { // SR26: removes of terminal restrictions only resolve recovery cases
		terminal, err := recoveryAcknowledged(
			ctx,
			tx,
			restriction,
			unit,
			command,
			when,
		)
		if err != nil ||
			terminal {
			return err
		}
	}

	if kind == actionApplyRestriction {
		return unitApplied(ctx, tx, restriction, unit, when)
	}

	return unitReleased(ctx, tx, restriction, unit, when)
}

// unitApplied records that a unit applied the restriction: its observed restriction (owned by equipment, IR184, so
// published), and the restriction applied once every unit has.
func unitApplied(
	ctx context.Context,
	tx pgx.Tx,
	restriction, unit uuid.UUID,
	when time.Time,
) error {
	_, execErr := tx.Exec(
		ctx,
		`UPDATE restrictions.restriction_units ru SET apply_state = 'applied', observed_at = $3, pending_reason = NULL,
			observed_restriction = jsonb_build_object('restrictionId', r.id, 'rulesVersion', r.rules_version, 'policy',
			r.policy, 'observedAt', $3::timestamptz)
		FROM restrictions.restrictions r WHERE r.id = ru.restriction_id AND ru.restriction_id = $1 AND ru.unit_id = $2`,
		restriction,
		unit,
		when,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update restrictions.restriction_units")
	}

	var observed []byte

	scanErr := tx.QueryRow(ctx, `SELECT observed_restriction FROM restrictions.restriction_units WHERE restriction_id = $1
		AND unit_id = $2`, restriction, unit).
		Scan(&observed)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select restrictions.restriction_units")
	}
	// the unit's observedRestriction is owned by equipment (IR184)
	publishErr := publish(
		ctx,
		tx,
		restriction,
		"unit",
		unit,
		events.UnitRestrictionApplied,
		events.UnitRestriction{UnitID: unit, RestrictionID: restriction, Observed: observed},
	)
	if publishErr != nil {
		return publishErr
	}

	_, err := tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2,
		state = CASE WHEN r.state = 'requested' AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru WHERE
		ru.restriction_id = r.id AND ru.apply_state <> 'applied')
			THEN 'applied' ELSE r.state END
		WHERE r.id = $1`,
		restriction,
		when,
	)

	return apperr.Wrap(err, "update restrictions.restrictions")
}

// unitReleased records that a unit removed the restriction, published to equipment, and the restriction released once
// every unit has (or needed no release).
func unitReleased(
	ctx context.Context,
	tx pgx.Tx,
	restriction, unit uuid.UUID,
	when time.Time,
) error {
	_, execErr := tx.Exec(
		ctx,
		`UPDATE restrictions.restriction_units SET release_state = 'released', observed_restriction = NULL, observed_at = $3,
			pending_reason = NULL
		WHERE restriction_id = $1 AND unit_id = $2`,
		restriction,
		unit,
		when,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update restrictions.restriction_units")
	}

	publishErr := publish(
		ctx,
		tx,
		restriction,
		"unit",
		unit,
		events.UnitRestrictionCleared,
		events.UnitRestriction{UnitID: unit, RestrictionID: restriction, Observed: nil},
	)
	if publishErr != nil {
		return publishErr
	}

	_, err := tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2,
		state = CASE WHEN r.state = 'release_requested' AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru
			WHERE ru.restriction_id = r.id AND ru.release_state NOT IN ('released','not_required')) THEN 'released' ELSE r.state
			END
		WHERE r.id = $1`,
		restriction,
		when,
	)

	return apperr.Wrap(err, "update restrictions.restrictions")
}

// CommandsEnded applies D03 to restriction Commands that expired or failed: a remove that did not succeed sets
// releaseState=failed (aggregate stays release_requested); a rejected apply sets applyState=not_applied. Expired
// apply intents keep their state (not_sent: definitive non-delivery; sent_unknown: reconcile required).
func CommandsEnded(ctx context.Context, txn pgx.Tx, commands []uuid.UUID, now time.Time) error {
	if len(commands) == 0 {
		return nil
	}

	recoveryEndedErr := recoveryEnded(ctx, txn, commands, now)
	if recoveryEndedErr != nil {
		return recoveryEndedErr
	}

	_, execErr := txn.Exec(
		ctx,
		`UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2
		WHERE r.id IN (SELECT restriction_id FROM restrictions.restriction_commands WHERE command_id = ANY($1))`,
		commands,
		now,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update restrictions.restrictions")
	}

	_, execErr = txn.Exec(
		ctx,
		`UPDATE restrictions.restriction_units ru SET release_state = 'failed'
		FROM restrictions.restriction_commands k WHERE k.command_id = ANY($1) AND k.kind = 'remove_restriction'
		  AND ru.restriction_id = k.restriction_id AND ru.unit_id = k.unit_id AND ru.release_state = 'requested'`,
		commands,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update restrictions.restriction_units")
	}

	_, err := txn.Exec(
		ctx,
		`UPDATE restrictions.restriction_units ru SET apply_state = 'not_applied'
		FROM restrictions.restriction_commands k WHERE k.command_id = ANY($1) AND k.kind = 'apply_restriction' AND k.status =
		'failed'
		  AND ru.restriction_id = k.restriction_id AND ru.unit_id = k.unit_id AND ru.apply_state = 'sent_unknown'`,
		commands,
	)

	return apperr.Wrap(err, "update restrictions.restriction_units")
}
