package audit

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// The audit log belongs to identity (IR196): every service's recorder publishes AuditRecorded in the writer's
// transaction, the identity consumer stores the entry, and other domains read the history of their records through
// QueryHistory.

// QueryHistory returns the audit entries of one target in log order.
const QueryHistory = "audit.history"

// HistoryInput is QueryHistory input.
type HistoryInput struct {
	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
}

// History asks identity for the audit entries of a target (any domain; local for the worker).
func History(ctx context.Context, c *ops.Call, kind, id string) ([]View, error) {
	return ops.Delegate(ctx, c, QueryHistory, HistoryInput{TargetKind: kind, TargetID: id}, history)
}

func history(ctx context.Context, c *ops.Call, in *HistoryInput) ([]View, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, tenant_id, actor_id, actor_role_at_time, action, previous_version, next_version, occurred_at, correlation_id, result,
		masked_before, masked_after, reason FROM audit.audit_log WHERE target_kind = $1 AND target_id = $2 ORDER BY occurred_at, correlation_id, id`, in.TargetKind, in.TargetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []View{}
	for rows.Next() {
		var x View
		if err := rows.Scan(&x.ID, &x.TenantID, &x.ActorID, &x.ActorRoleAtTime, &x.Action, &x.PreviousVersion, &x.NextVersion, &x.OccurredAt, &x.CorrelationID, &x.Result,
			&x.MaskedBefore, &x.MaskedAfter, &x.Reason); err != nil {
			return nil, err
		}
		x.Version, x.CreatedAt, x.UpdatedAt = 1, x.OccurredAt, x.OccurredAt
		x.TargetRef = map[string]string{"kind": in.TargetKind, "id": in.TargetID}
		out = append(out, x)
	}
	return out, rows.Err()
}

// RegisterQueries binds the audit queries other domains ask.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainIdentity, QueryHistory, history)
}

// EventHandlers store published audit entries (consumer identity); the entry ID makes a replay a no-op.
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.AuditRecorded: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var a events.Audit
			if err := e.Decode(&a); err != nil {
				return err
			}
			before, after := a.MaskedBefore, a.MaskedAfter
			if before == nil {
				before = map[string]*string{}
			}
			if after == nil {
				after = map[string]*string{}
			}
			_, err := tx.Exec(ctx, `INSERT INTO audit.audit_log (id, tenant_id, actor_id, actor_role_at_time, membership_id, action, target_kind, target_id,
				previous_version, next_version, occurred_at, correlation_id, result, reason, masked_before, masked_after)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NULLIF($14, ''), $15, $16) ON CONFLICT (id, occurred_at) DO NOTHING`,
				a.ID, e.TenantID, a.ActorID, a.ActorRole, a.MembershipID, a.Action, a.TargetKind, a.TargetID, a.PreviousVersion, a.NextVersion, a.OccurredAt,
				a.CorrelationID, a.Result, a.Reason, before, after)
			return err
		},
	}
}
