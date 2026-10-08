// Package events delivers outbox events to the services that consume them (backend architecture §8, IR183).
//
// Producers already write their events into platform.outbox in the same transaction as the state change
// (ops.Call.Emit). A Consumer polls the event types it subscribes to in outbox order (seq) and runs each handler in
// its own transaction in the event's tenant; platform.processed_events(consumer, event_id) makes delivery
// idempotent (at-least-once), so a crashed or repeated poll never applies an event twice. Polling for events not yet
// processed (instead of a moving checkpoint) cannot skip an event whose transaction committed late. In production the
// outbox relay publishes the same rows to SNS/SQS; the handler contract stays the same.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// Event is one outbox row.
type Event struct {
	ID            uuid.UUID
	Seq           int64
	TenantID      uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	Type          string
	Payload       json.RawMessage
	CorrelationID string
	OccurredAt    time.Time
}

// Decode unmarshals the payload into v.
func (e Event) Decode(v any) error { return json.Unmarshal(e.Payload, v) }

// Handler applies one event inside tx (tenant context set). Returning an error rolls the event back; it is retried
// on the next poll.
type Handler func(ctx context.Context, tx pgx.Tx, e Event) error

// Consumer is one subscriber (a service, or one projection of a service).
type Consumer struct {
	Name     string             // unique and stable: the de-duplication key
	DB       ops.Runner         // the service's transaction manager
	Handlers map[string]Handler // by event type
	Batch    int                // events per poll (default 100)
	Logf     func(string, ...any)
}

// Drain applies every pending event once, in order, and returns how many were applied. It stops at the first
// failing event so events of the same aggregate are never applied out of order; the failure is returned.
func (c *Consumer) Drain(ctx context.Context) (int, error) {
	if len(c.Handlers) == 0 {
		return 0, nil
	}
	types := make([]string, 0, len(c.Handlers))
	for t := range c.Handlers {
		types = append(types, t)
	}
	batch := c.Batch
	if batch <= 0 {
		batch = 100
	}
	applied := 0
	for {
		pending, err := c.pending(ctx, types, batch)
		if err != nil {
			return applied, err
		}
		if len(pending) == 0 {
			return applied, nil
		}
		for _, e := range pending {
			done, err := c.apply(ctx, e)
			if err != nil {
				return applied, fmt.Errorf("events: %s %s %s: %w", c.Name, e.Type, e.ID, err)
			}
			if done {
				applied++
			}
		}
		if len(pending) < batch {
			return applied, nil
		}
	}
}

func (c *Consumer) pending(ctx context.Context, types []string, limit int) ([]Event, error) {
	var out []Event
	err := c.DB.Run(ctx, true, nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT o.id, o.seq, o.tenant_id, o.aggregate_type, o.aggregate_id, o.event_type, o.payload, o.correlation_id, o.occurred_at
			FROM platform.outbox o
			WHERE o.event_type = ANY($1)
			  AND NOT EXISTS (SELECT 1 FROM platform.processed_events p WHERE p.consumer = $2 AND p.event_id = o.id)
			ORDER BY o.seq LIMIT $3`, types, c.Name, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Event
			if err := rows.Scan(&e.ID, &e.Seq, &e.TenantID, &e.AggregateType, &e.AggregateID, &e.Type, &e.Payload, &e.CorrelationID, &e.OccurredAt); err != nil {
				return err
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}

// apply runs the handler and records the event as processed in one transaction; false when another poll got there
// first.
func (c *Consumer) apply(ctx context.Context, e Event) (bool, error) {
	done := false
	err := c.DB.Run(ctx, false, &ops.Principal{TenantID: e.TenantID}, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO platform.processed_events (consumer, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, c.Name, e.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already applied
		}
		// the business time of the event (IR157): handlers' SQL defaults use the producer's clock
		if _, err := tx.Exec(ctx, `SELECT set_config('app.now', $1, true)`, e.OccurredAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if err := c.Handlers[e.Type](ctx, tx, e); err != nil {
			return err
		}
		done = true
		return nil
	})
	return done, err
}

// Run polls until ctx ends.
func (c *Consumer) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if n, err := c.Drain(ctx); err != nil && c.Logf != nil {
			c.Logf("consumer %s: %v (applied %d)", c.Name, err, n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
