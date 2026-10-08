// Package audit implements the read-only audit log search (FR-A16).
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// View is AuditView of service-contracts.ts.
type View struct {
	ID              uuid.UUID          `json:"id"`
	TenantID        uuid.UUID          `json:"tenantId"`
	Version         int                `json:"version"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	ActorID         string             `json:"actorId"`
	ActorRoleAtTime string             `json:"actorRoleAtTime"`
	Action          string             `json:"action"`
	TargetRef       map[string]string  `json:"targetRef"`
	PreviousVersion *int               `json:"previousVersion"`
	NextVersion     *int               `json:"nextVersion"`
	OccurredAt      time.Time          `json:"occurredAt"`
	CorrelationID   string             `json:"correlationId"`
	Result          string             `json:"result"`
	MaskedBefore    map[string]*string `json:"maskedBefore"`
	MaskedAfter     map[string]*string `json:"maskedAfter"`
	Reason          *string            `json:"reason"`
}

// Filter is the audit.list filter set (DD-A16, IR143 item 4).
type Filter struct {
	From          *time.Time `json:"from"`
	To            *time.Time `json:"to"`
	ActorID       *string    `json:"actorId,omitempty"`
	TargetKind    *string    `json:"targetKind,omitempty"`
	TargetID      *string    `json:"targetId,omitempty"`
	CorrelationID *string    `json:"correlationId,omitempty"`
	Result        *string    `json:"result,omitempty"`
	Action        *string    `json:"action,omitempty"`
}

func list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[View], error) {
	var f Filter
	bad := apperr.Fields(map[string]string{"filters": "error.invalid"})
	if len(in.Filters) == 0 {
		return paging.Page[View]{}, apperr.Fields(map[string]string{"filters.from": "error.required"})
	}
	dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
	dec.DisallowUnknownFields()
	if dec.Decode(&f) != nil || (f.Result != nil && !slices.Contains([]string{"success", "denied", "failed", "pending"}, *f.Result)) {
		return paging.Page[View]{}, bad
	}
	if f.From == nil || f.To == nil {
		return paging.Page[View]{}, apperr.Fields(map[string]string{"filters.from": "error.required"})
	}
	if !f.From.Before(*f.To) || f.To.Sub(*f.From) > 366*24*time.Hour {
		return paging.Page[View]{}, apperr.Fields(map[string]string{"filters.to": "errors.period_range"})
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "a.id", "occurredAt": "a.occurred_at", "createdAt": "a.occurred_at", "updatedAt": "a.occurred_at"}, "a.occurred_at DESC, a.id DESC") // entries are immutable: created = updated = occurred
	if err != nil {
		return paging.Page[View]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[View]{}, err
	}
	args := []any{*f.From, *f.To}
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"a.occurred_at >= $1", "a.occurred_at < $2"}
	for col, v := range map[string]*string{"a.actor_id": f.ActorID, "a.target_kind": f.TargetKind, "a.target_id": f.TargetID, "a.correlation_id": f.CorrelationID,
		"a.result": f.Result, "a.action": f.Action} {
		if v != nil {
			conds = append(conds, col+" = "+add(*v))
		}
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM audit.audit_log a WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[View]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf(`SELECT a.id, a.tenant_id, a.actor_id, a.actor_role_at_time, a.action, a.target_kind, a.target_id, a.previous_version, a.next_version,
		a.occurred_at, a.correlation_id, a.result, a.masked_before, a.masked_after, a.reason FROM audit.audit_log a WHERE %s ORDER BY %s LIMIT %d OFFSET %d`, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[View]{}, err
	}
	defer rows.Close()
	items := []View{}
	for rows.Next() {
		var x View
		var kind, id string
		if err := rows.Scan(&x.ID, &x.TenantID, &x.ActorID, &x.ActorRoleAtTime, &x.Action, &kind, &id, &x.PreviousVersion, &x.NextVersion, &x.OccurredAt, &x.CorrelationID,
			&x.Result, &x.MaskedBefore, &x.MaskedAfter, &x.Reason); err != nil {
			return paging.Page[View]{}, err
		}
		x.Version, x.CreatedAt, x.UpdatedAt = 1, x.OccurredAt, x.OccurredAt
		x.TargetRef = map[string]string{"kind": kind, "id": id}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		return paging.Page[View]{}, err
	}
	return paging.Page[View]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// Register binds audit.list.
func Register(r *ops.Registry) { ops.Register(r, "audit.list", list) }
