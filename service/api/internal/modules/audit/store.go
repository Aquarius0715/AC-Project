package audit

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
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
	out, err := ops.Delegate(
		ctx,
		c,
		QueryHistory,
		HistoryInput{TargetKind: kind, TargetID: id},
		history,
	)

	return out, apperr.Wrap(err, "delegate audit.history")
}

func history(ctx context.Context, c *ops.Call, input *HistoryInput) ([]View, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT a.id, a.tenant_id, a.actor_id, u.display_name, a.actor_role_at_time, a.action, a.previous_version,
			a.next_version, a.occurred_at, a.correlation_id, a.result,
		a.masked_before, a.masked_after, a.reason FROM audit.audit_log a
		`+actorJoin+` WHERE a.target_kind = $1 AND
			a.target_id = $2 ORDER BY a.occurred_at, a.correlation_id, a.id`,
		input.TargetKind,
		input.TargetID,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select audit.audit_log")
	}
	defer rows.Close()

	out := []View{}

	for rows.Next() {
		var view View

		err := rows.Scan(
			&view.ID,
			&view.TenantID,
			&view.ActorID,
			&view.ActorName,
			&view.ActorRoleAtTime,
			&view.Action,
			&view.PreviousVersion,
			&view.NextVersion,
			&view.OccurredAt,
			&view.CorrelationID,
			&view.Result,
			&view.MaskedBefore,
			&view.MaskedAfter,
			&view.Reason,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select audit.audit_log")
		}

		view.Version, view.CreatedAt, view.UpdatedAt = 1, view.OccurredAt, view.OccurredAt
		view.TargetRef = map[string]string{"kind": input.TargetKind, "id": input.TargetID}
		out = append(out, view)
	}

	return out, apperr.Wrap(rows.Err(), "select audit.audit_log")
}

// RegisterQueries binds the audit queries other domains ask.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainIdentity, QueryHistory, history)
}

// EventHandlers store published audit entries (consumer identity); the entry ID makes a replay a no-op.
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.AuditRecorded: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var audit events.Audit

			decodeErr := e.Decode(&audit)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			before, after := audit.MaskedBefore, audit.MaskedAfter
			if before == nil {
				before = map[string]*string{}
			}

			if after == nil {
				after = map[string]*string{}
			}

			_, err := txn.Exec(
				ctx,
				`INSERT INTO audit.audit_log (id, tenant_id, actor_id, actor_role_at_time, membership_id, action, target_kind,
					target_id,
				previous_version, next_version, occurred_at, correlation_id, result, reason, masked_before, masked_after)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NULLIF($14, ''), $15, $16) ON CONFLICT (id,
				occurred_at) DO NOTHING`,
				audit.ID,
				e.TenantID,
				audit.ActorID,
				audit.ActorRole,
				audit.MembershipID,
				audit.Action,
				audit.TargetKind,
				audit.TargetID,
				audit.PreviousVersion,
				audit.NextVersion,
				audit.OccurredAt,
				audit.CorrelationID,
				audit.Result,
				audit.Reason,
				before,
				after,
			)

			return apperr.Wrap(err, "insert audit.audit_log")
		},
	}
}
