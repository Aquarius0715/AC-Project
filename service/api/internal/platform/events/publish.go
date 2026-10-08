package events

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Publish writes one event of tenant into platform.outbox inside tx. Use it
// where a module only holds a transaction (consumers, internal state transitions); operation handlers can use
// ops.Call.Emit, which the recorder turns into the same rows.
func Publish(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, aggregateType string, aggregateID uuid.UUID, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id)
		VALUES ($1, $2, $3, $4, $5, COALESCE(NULLIF(current_setting('app.correlation_id', true), ''), 'event'))`,
		tenant, aggregateType, aggregateID, eventType, raw)
	return err
}
