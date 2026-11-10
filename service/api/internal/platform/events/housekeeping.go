package events

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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
//     dead-lettered), a split consumer counting what the one it was split from applied: an event a consumer still
//     needs stays, however old;
//   - processed-event records and failure records of events that are no longer in the outbox, the former once they
//     are older than ProcessedRetention, so an event is never delivered twice;
//   - idempotency keys past their expiry, tenant by tenant (the table has row-level security).
func Housekeep(
	ctx context.Context,
	runner ops.Runner,
	now time.Time,
	consumers []*Consumer,
) (Swept, error) {
	var swept Swept

	// one row per (consumer, subscribed type), with the consumer it was split from: what that one applied before the
	// split counts as applied (IR511, IR760)
	var names, types, splits []string

	for _, c := range consumers {
		for t := range c.Handlers {
			names = append(names, c.Name)
			types = append(types, t)
			splits = append(splits, c.SplitFrom)
		}
	}

	err := runner.Run(ctx, false, nil, func(txn pgx.Tx) error {
		tag, err := txn.Exec(ctx, `DELETE FROM platform.outbox o WHERE o.occurred_at < $1
			AND NOT EXISTS (SELECT 1 FROM unnest($2::text[], $3::text[], $4::text[]) AS sub(consumer, event_type, split)
				WHERE sub.event_type = o.event_type
				  AND NOT EXISTS (SELECT 1 FROM platform.processed_events p WHERE p.event_id = o.id
				    AND (p.consumer = sub.consumer OR (sub.split <> '' AND p.consumer = sub.split))))`,
			now.Add(-OutboxRetention), names, types, splits)
		if err != nil {
			return apperr.Wrap(err, "delete platform.outbox")
		}

		swept.Outbox = tag.RowsAffected()

		tag, err = txn.Exec(
			ctx,
			`DELETE FROM platform.processed_events p WHERE p.processed_at < $1
			AND NOT EXISTS (SELECT 1 FROM platform.outbox o WHERE o.id = p.event_id)`,
			now.Add(-ProcessedRetention),
		)
		if err != nil {
			return apperr.Wrap(err, "delete platform.processed_events")
		}

		swept.Processed = tag.RowsAffected()

		tag, err = txn.Exec(
			ctx,
			`DELETE FROM platform.event_failures f WHERE NOT EXISTS (SELECT 1 FROM platform.outbox o WHERE o.id = f.event_id)`,
		)
		if err != nil {
			return apperr.Wrap(err, "delete platform.event_failures")
		}

		swept.Failures = tag.RowsAffected()

		return nil
	})
	if err != nil {
		return swept, apperr.Wrap(err, "ops.Runner.Run")
	}

	swept.IdempotencyKeys, err = sweepIdempotencyKeys(ctx, runner, now)

	return swept, err
}

// sweepIdempotencyKeys deletes the idempotency keys past their expiry, tenant by tenant (the table has row-level
// security), and returns how many it deleted, also when a tenant fails.
func sweepIdempotencyKeys(ctx context.Context, runner ops.Runner, now time.Time) (int64, error) {
	var (
		tenants []uuid.UUID
		swept   int64
	)

	runErr := runner.Run(ctx, true, nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
		if err != nil {
			return apperr.Wrap(err, "select platform.tenants")
		}

		tenants, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])

		return apperr.Wrap(err, "select platform.tenants")
	})
	if runErr != nil {
		return swept, apperr.Wrap(runErr, "ops.Runner.Run")
	}

	for _, t := range tenants {
		err := runner.Run(
			ctx,
			false,
			&ops.Principal{
				TenantID:          t,
				Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
				MembershipID:      uuid.UUID{},
				UserID:            uuid.UUID{},
				OrgID:             uuid.UUID{},
				ScopeVersion:      0,
				Scopes:            nil,
				Employment:        "",
				Timezone:          "",
				SessionsRevokedAt: nil,
				IssuerHost:        "",
			},
			func(tx pgx.Tx) error {
				tag, err := tx.Exec(
					ctx,
					`DELETE FROM platform.idempotency_keys WHERE expires_at < $1`,
					now,
				)
				swept += tag.RowsAffected()

				return apperr.Wrap(err, "delete platform.idempotency_keys")
			},
		)
		if err != nil {
			return swept, apperr.Wrap(err, "ops.Runner.Run")
		}
	}

	return swept, nil
}
