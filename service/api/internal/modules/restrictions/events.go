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
		events.CommandAcknowledged: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Commands

			err := e.Decode(&p)
			if err != nil {
				return apperr.Wrap(err, "events.Event.Decode")
			}

			_, err = tx.Exec(
				ctx,
				`UPDATE restrictions.restriction_commands SET status = 'acknowledged', updated_at = $2 WHERE command_id = ANY($1)
					AND status = 'requested'`,
				p.CommandIDs,
				p.At,
			)
			if err != nil {
				return apperr.Wrap(err, "update restrictions.restriction_commands")
			}

			for _, id := range p.CommandIDs {
				err := CommandAcknowledged(ctx, tx, id, p.At)
				if err != nil {
					return err
				}
			}

			return nil
		},
		events.UnitRestrictionObserved: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Observation

			decodeErr := e.Decode(&p)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			var observed *Observation
			if len(p.Observed) > 0 && string(p.Observed) != "null" {
				observed = &Observation{
					RestrictionID: uuid.UUID{},
					RulesVersion:  "",
					Policy:        nil,
					ObservedAt:    time.Time{},
				}

				err := json.Unmarshal(p.Observed, observed)
				if err != nil {
					return apperr.Wrap(err, "json.Unmarshal")
				}
			}

			_, err := Observe(ctx, tx, p.At, p.UnitID, observed, p.EventID)

			return err
		},
		events.CommandsEnded: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Commands

			err := e.Decode(&p)
			if err != nil {
				return apperr.Wrap(err, "events.Event.Decode")
			}

			for _, id := range p.CommandIDs { // billing's record takes equipment's final status (IR194)
				status := p.Statuses[id]
				if status != "failed" && status != "cancelled" {
					status = "expired"
				}

				_, err := tx.Exec(
					ctx,
					`UPDATE restrictions.restriction_commands SET status = $2, updated_at = $3 WHERE command_id = $1 AND status =
						'requested'`,
					id,
					status,
					p.At,
				)
				if err != nil {
					return apperr.Wrap(err, "update restrictions.restriction_commands")
				}
			}

			return CommandsEnded(ctx, tx, p.CommandIDs, p.At)
		},
	}
}
