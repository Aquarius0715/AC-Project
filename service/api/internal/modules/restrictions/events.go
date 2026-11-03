package restrictions

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the restriction consumers of equipment's command events (IR185): acknowledgements and ends of
// restriction commands advance the restriction state in billing.
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.CommandAcknowledged: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var commands events.Commands

			err := e.Decode(&commands)
			if err != nil {
				return apperr.Wrap(err, "events.Event.Decode")
			}

			_, err = txn.Exec(
				ctx,
				`UPDATE restrictions.restriction_commands SET status = 'acknowledged', updated_at = $2 WHERE command_id = ANY($1)
					AND status = 'requested'`,
				commands.CommandIDs,
				commands.At,
			)
			if err != nil {
				return apperr.Wrap(err, "update restrictions.restriction_commands")
			}

			for _, id := range commands.CommandIDs {
				err := CommandAcknowledged(ctx, txn, id, commands.At)
				if err != nil {
					return err
				}
			}

			return nil
		},
		events.UnitRestrictionObserved: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var payload events.Observation

			decodeErr := e.Decode(&payload)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			var observed *Observation
			if len(payload.Observed) > 0 && string(payload.Observed) != "null" {
				observed = &Observation{
					RestrictionID: uuid.UUID{},
					RulesVersion:  "",
					Policy:        nil,
					ObservedAt:    time.Time{},
				}

				err := json.Unmarshal(payload.Observed, observed)
				if err != nil {
					return apperr.Wrap(err, "json.Unmarshal")
				}
			}

			_, err := Observe(ctx, txn, payload.At, payload.UnitID, observed, payload.EventID)

			return err
		},
		events.CommandsEnded: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var commands events.Commands

			err := e.Decode(&commands)
			if err != nil {
				return apperr.Wrap(err, "events.Event.Decode")
			}

			for _, commandID := range commands.CommandIDs { // billing's record takes equipment's final status (IR194)
				status := commands.Statuses[commandID]
				if status != "failed" && status != "cancelled" {
					status = "expired"
				}

				_, err := txn.Exec(
					ctx,
					`UPDATE restrictions.restriction_commands SET status = $2, updated_at = $3 WHERE command_id = $1 AND status =
						'requested'`,
					commandID,
					status,
					commands.At,
				)
				if err != nil {
					return apperr.Wrap(err, "update restrictions.restriction_commands")
				}
			}

			return CommandsEnded(ctx, txn, commands.CommandIDs, commands.At)
		},
	}
}
