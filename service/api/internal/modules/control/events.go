package control

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the control consumers of billing's restriction command events (IR185).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.RestrictionCommandRequested: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.RestrictionCommand
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO control.commands (id, tenant_id, unit_id, device_id, actor_membership_id, source, action, restriction_id, status, delivery,
				requested_at, sent_at, expires_at, correlation_id, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, 'restriction', $6, $7, $8, $9, $10, $11, $12, $13, $10, $10) ON CONFLICT (id) DO NOTHING`,
				p.CommandID, e.TenantID, p.UnitID, p.DeviceID, p.ActorID, []byte(p.Action), p.RestrictionID, p.Status, p.Delivery, p.RequestedAt, p.SentAt, p.ExpiresAt, p.CorrelationID)
			return err
		},
		events.RestrictionCommandsCancelled: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.CommandsCancelled
			if err := e.Decode(&p); err != nil {
				return err
			}
			if len(p.CommandIDs) > 0 {
				_, err := tx.Exec(ctx, `UPDATE control.commands SET status = 'cancelled', version = version + 1, updated_at = platform.app_now()
					WHERE id = ANY($1) AND status IN ('requested','sent')`, p.CommandIDs)
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE control.commands SET status = 'cancelled', version = version + 1, updated_at = platform.app_now()
				WHERE restriction_id = $1 AND unit_id = $2 AND status = 'requested' AND delivery = 'not_sent'`, p.RestrictionID, p.UnitID)
			return err
		},
	}
}

// publishRestrictionCommands tells billing about restriction commands among ids (other commands are equipment-only).
func publishRestrictionCommands(ctx context.Context, tx pgx.Tx, eventType string, ids []uuid.UUID, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT tenant_id, id FROM control.commands WHERE id = ANY($1) AND source = 'restriction' ORDER BY tenant_id, id`, ids)
	if err != nil {
		return err
	}
	byTenant := map[uuid.UUID][]uuid.UUID{}
	var order []uuid.UUID
	for rows.Next() {
		var t, id uuid.UUID
		if err := rows.Scan(&t, &id); err != nil {
			rows.Close()
			return err
		}
		if _, ok := byTenant[t]; !ok {
			order = append(order, t)
		}
		byTenant[t] = append(byTenant[t], id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range order {
		if err := events.Publish(ctx, tx, t, "command", byTenant[t][0], eventType, events.Commands{CommandIDs: byTenant[t], At: at}); err != nil {
			return err
		}
	}
	return nil
}

// EndRestrictionCommands publishes CommandsEnded for failed restriction commands outside the expiry tick (device
// rejection, demo.trigger command_fail).
func EndRestrictionCommands(ctx context.Context, tx pgx.Tx, ids []uuid.UUID, at time.Time) error {
	return publishRestrictionCommands(ctx, tx, events.CommandsEnded, ids, at)
}
