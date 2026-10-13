package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// The retention of the platform tables (database design §6, IR428).
const (
	OutboxRetention    = 7 * 24 * time.Hour  // an event every subscriber has applied
	ProcessedRetention = 14 * 24 * time.Hour // a consumer's record of an event that is no longer in the outbox
)

// Swept counts the rows one Housekeep removed.
type Swept struct{ Outbox, Processed, Failures, IdempotencyKeys int64 }

// Housekeep deletes what the platform tables keep no longer (IR428), at now (business time, the clock the rows were
// written with):
//   - outbox events older than OutboxRetention that every consumer subscribing to their type has applied (or
//     dead-lettered): an event a consumer still needs stays, however old;
//   - processed-event records and failure records of events that are no longer in the outbox, the former once they
//     are older than ProcessedRetention, so an event is never delivered twice;
//   - idempotency keys past their expiry, tenant by tenant (the table has row-level security).
func Housekeep(ctx context.Context, m ops.Runner, now time.Time, consumers []*Consumer) (Swept, error) {
	var s Swept
	var names, types []string // one row per (consumer, subscribed type)
	for _, c := range consumers {
		for t := range c.Handlers {
			names, types = append(names, c.Name), append(types, t)
		}
	}
	err := m.Run(ctx, false, nil, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM platform.outbox o WHERE o.occurred_at < $1
			AND NOT EXISTS (SELECT 1 FROM unnest($2::text[], $3::text[]) AS sub(consumer, event_type)
				WHERE sub.event_type = o.event_type
				  AND NOT EXISTS (SELECT 1 FROM platform.processed_events p WHERE p.consumer = sub.consumer AND p.event_id = o.id))`,
			now.Add(-OutboxRetention), names, types)
		if err != nil {
			return err
		}
		s.Outbox = tag.RowsAffected()
		if tag, err = tx.Exec(ctx, `DELETE FROM platform.processed_events p WHERE p.processed_at < $1
			AND NOT EXISTS (SELECT 1 FROM platform.outbox o WHERE o.id = p.event_id)`, now.Add(-ProcessedRetention)); err != nil {
			return err
		}
		s.Processed = tag.RowsAffected()
		if tag, err = tx.Exec(ctx, `DELETE FROM platform.event_failures f WHERE NOT EXISTS (SELECT 1 FROM platform.outbox o WHERE o.id = f.event_id)`); err != nil {
			return err
		}
		s.Failures = tag.RowsAffected()
		return nil
	})
	if err != nil {
		return s, err
	}
	var tenants []uuid.UUID
	if err := m.Run(ctx, true, nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
		if err != nil {
			return err
		}
		tenants, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		return err
	}); err != nil {
		return s, err
	}
	for _, t := range tenants {
		if err := m.Run(ctx, false, &ops.Principal{TenantID: t}, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `DELETE FROM platform.idempotency_keys WHERE expires_at < $1`, now)
			s.IdempotencyKeys += tag.RowsAffected()
			return err
		}); err != nil {
			return s, err
		}
	}
	return s, nil
}
