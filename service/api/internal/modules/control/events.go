package control

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the control consumers of billing's restriction command events (IR185) and of identity's
// consent withdrawals (IR53).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.ConsentRevoked: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var consentRevocation events.ConsentRevocation

			decodeErr := e.Decode(&consentRevocation)
			if decodeErr != nil || consentRevocation.Purpose != "location_automation" {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}
			// IR53: every enabled location rule of the membership stops with consent_revoked (version + 1); renewed
			// consent never re-enables them (an explicit save with enabled=true does, IR27)
			_, err := txn.Exec(
				ctx,
				`UPDATE control.automations SET enabled = false, disabled_reason = 'consent_revoked', version = version + 1,
					updated_at = $2
				WHERE owner_membership_id = $1 AND kind = 'event' AND definition->'condition'->>'type' = 'location' AND enabled`,
				consentRevocation.MembershipID,
				consentRevocation.At,
			)

			return apperr.Wrap(err, "update control.automations")
		},
		events.RestrictionCommandRequested: func(ctx context.Context, tx pgx.Tx, event events.Event) error {
			var restrictionCommand events.RestrictionCommand

			decodeErr := event.Decode(&restrictionCommand)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			_, err := tx.Exec(
				ctx,
				`INSERT INTO control.commands (id, tenant_id, unit_id, device_id, actor_membership_id, source, action,
					restriction_id, status, delivery,
				requested_at, sent_at, expires_at, correlation_id, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, 'restriction', $6, $7, $8, $9, $10, $11, $12, $13, $10, $10) ON CONFLICT (id) DO
				NOTHING`,
				restrictionCommand.CommandID,
				event.TenantID,
				restrictionCommand.UnitID,
				restrictionCommand.DeviceID,
				restrictionCommand.ActorID,
				[]byte(restrictionCommand.Action),
				restrictionCommand.RestrictionID,
				restrictionCommand.Status,
				restrictionCommand.Delivery,
				restrictionCommand.RequestedAt,
				restrictionCommand.SentAt,
				restrictionCommand.ExpiresAt,
				restrictionCommand.CorrelationID,
			)

			return apperr.Wrap(err, "insert control.commands")
		},
		events.RestrictionCommandsCancelled: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var commandsCancelled events.CommandsCancelled

			decodeErr := e.Decode(&commandsCancelled)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			if len(commandsCancelled.CommandIDs) > 0 {
				_, err := txn.Exec(
					ctx,
					`UPDATE control.commands SET status = 'cancelled', version = version + 1, updated_at = platform.app_now()
					WHERE id = ANY($1) AND status IN ('requested','sent')`,
					commandsCancelled.CommandIDs,
				)

				return apperr.Wrap(err, "update control.commands")
			}

			_, err := txn.Exec(
				ctx,
				`UPDATE control.commands SET status = 'cancelled', version = version + 1, updated_at = platform.app_now()
				WHERE restriction_id = $1 AND unit_id = $2 AND status = 'requested' AND delivery = 'not_sent'`,
				commandsCancelled.RestrictionID,
				commandsCancelled.UnitID,
			)

			return apperr.Wrap(err, "update control.commands")
		},
	}
}

// publishRestrictionCommands tells billing about restriction commands among ids (other commands are equipment-only).
func publishRestrictionCommands(
	ctx context.Context,
	txn pgx.Tx,
	eventType string,
	ids []uuid.UUID,
	when time.Time,
) error {
	if len(ids) == 0 {
		return nil
	}

	rows, err := txn.Query(
		ctx,
		`SELECT tenant_id, id, status FROM control.commands WHERE id = ANY($1) AND source = 'restriction' ORDER BY tenant_id,
			id`,
		ids,
	)
	if err != nil {
		return apperr.Wrap(err, "select control.commands")
	}

	byTenant := map[uuid.UUID][]uuid.UUID{}
	statuses := map[uuid.UUID]string{}

	var order []uuid.UUID

	for rows.Next() {
		var (
			t, id  uuid.UUID
			status string
		)

		err := rows.Scan(&t, &id, &status)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select control.commands")
		}

		if _, ok := byTenant[t]; !ok {
			order = append(order, t)
		}

		byTenant[t] = append(byTenant[t], id)
		statuses[id] = status
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select control.commands")
	}

	for _, t := range order {
		st := map[uuid.UUID]string{}
		for _, id := range byTenant[t] {
			st[id] = statuses[id]
		}

		err := events.Publish(
			ctx,
			txn,
			t,
			"command",
			byTenant[t][0],
			eventType,
			events.Commands{CommandIDs: byTenant[t], At: when, Statuses: st},
		)
		if err != nil {
			return apperr.Wrap(err, "events.Publish")
		}
	}

	return nil
}

// EndRestrictionCommands publishes CommandsEnded for failed restriction commands outside the expiry tick (device
// rejection, demo.trigger command_fail).
func EndRestrictionCommands(ctx context.Context, tx pgx.Tx, ids []uuid.UUID, at time.Time) error {
	return publishRestrictionCommands(ctx, tx, events.CommandsEnded, ids, at)
}
