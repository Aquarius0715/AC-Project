package restrictions

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the restriction consumers of equipment's command events (IR185): acknowledgements and ends of
// restriction commands advance the restriction state in billing.
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.CommandAcknowledged: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Commands
			if err := e.Decode(&p); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE restrictions.restriction_commands SET status = 'acknowledged', updated_at = $2 WHERE command_id = ANY($1) AND status = 'requested'`,
				p.CommandIDs, p.At); err != nil {
				return err
			}
			for _, id := range p.CommandIDs {
				if err := CommandAcknowledged(ctx, tx, id, p.At); err != nil {
					return err
				}
			}
			return nil
		},
		events.UnitRestrictionObserved: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Observation
			if err := e.Decode(&p); err != nil {
				return err
			}
			var observed *Observation
			if len(p.Observed) > 0 && string(p.Observed) != "null" {
				observed = &Observation{}
				if err := json.Unmarshal(p.Observed, observed); err != nil {
					return err
				}
			}
			_, err := Observe(ctx, tx, p.At, p.UnitID, observed, p.EventID)
			return err
		},
		events.CommandsEnded: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Commands
			if err := e.Decode(&p); err != nil {
				return err
			}
			for _, id := range p.CommandIDs { // billing's record takes equipment's final status (IR194)
				status := p.Statuses[id]
				if status != "failed" && status != "cancelled" {
					status = "expired"
				}
				if _, err := tx.Exec(ctx, `UPDATE restrictions.restriction_commands SET status = $2, updated_at = $3 WHERE command_id = $1 AND status = 'requested'`,
					id, status, p.At); err != nil {
					return err
				}
			}
			return CommandsEnded(ctx, tx, p.CommandIDs, p.At)
		},
	}
}
