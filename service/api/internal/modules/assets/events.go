package assets

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the assets consumers of other domains' events (IR184): the unit's observed restriction and its
// version stay owned by equipment; billing (restrictions) only publishes what changed.
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.UnitRestrictionApplied: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.UnitRestriction
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE assets.units SET observed_restriction = $2 WHERE id = $1`, p.UnitID, []byte(p.Observed))
			return err
		},
		events.UnitRestrictionCleared: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.UnitRestriction
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE assets.units SET observed_restriction = NULL WHERE id = $1 AND observed_restriction->>'restrictionId' = $2::text`, p.UnitID, p.RestrictionID)
			return err
		},
		events.RecoveryCasesChanged: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Units
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE assets.units SET version = version + 1, updated_at = platform.app_now() WHERE id = ANY($1)`, p.UnitIDs)
			return err
		},
	}
}
