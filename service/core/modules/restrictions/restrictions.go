package restrictions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/modules/notify"
	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
	"github.com/pradita/ac-project/service/core/platform/paging"
)

// SystemActor is the internal 'system-restriction' membership that owns restriction Commands (IR35); it matches
// the fixture ID system-restriction under the seed namespace.
var SystemActor = uuid.NewSHA1(uuid.MustParse("6f1c3a52-6d0b-5c1e-9a57-0d1b7a4c2e10"), []byte("system-restriction"))

// IntentTTL is the lifetime of a restriction Command intent (D03).
const IntentTTL = 30 * time.Second

// UnitCaps is what Restrictions needs to know about a target unit (Assets + Devices capability).
type UnitCaps struct {
	OrgID                  uuid.UUID
	Archived               bool
	Control                bool
	HasTemperature         bool
	TempMin, TempMax, Step float64
}

// Units resolves restriction targets.
type Units interface {
	RestrictionTarget(ctx context.Context, c *ops.Call, unit uuid.UUID) (UnitCaps, bool, error)
}

// Devices reports the device bound to a unit (Devices).
type Devices interface {
	BoundDevice(ctx context.Context, c *ops.Call, unit uuid.UUID) (id uuid.UUID, connection, powerSignal string, found bool, err error)
	OperationBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
}

// Restrictions is the restriction operation set (IR140).
type Restrictions struct {
	Units   Units
	Devices Devices
	Notify  notify.Store
}

// Unit is RestrictionUnit.
type Unit struct {
	UnitID              uuid.UUID       `json:"unitId"`
	ApplyState          string          `json:"applyState"`
	ReleaseState        string          `json:"releaseState"`
	ApplyCommandIDs     []uuid.UUID     `json:"applyCommandIds"`
	ReleaseCommandIDs   []uuid.UUID     `json:"releaseCommandIds"`
	ObservedRestriction json.RawMessage `json:"observedRestriction"`
	ObservedAt          *time.Time      `json:"observedAt"`
	EvidenceID          *uuid.UUID      `json:"evidenceId"`
	PendingReason       *string         `json:"pendingReason"`
}

// Exception is Restriction.exception.
type Exception struct {
	Until  time.Time `json:"until"`
	Reason *string   `json:"reason"`
}

// Event is AuditView.
type Event struct {
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

// Restriction is Restriction of service-contracts.ts.
type Restriction struct {
	ID                    uuid.UUID       `json:"id"`
	TenantID              uuid.UUID       `json:"tenantId"`
	Version               int             `json:"version"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
	RecoveryCases         json.RawMessage `json:"recoveryCases"`
	NoticeNotificationIDs []uuid.UUID     `json:"noticeNotificationIds"`
	ContractID            uuid.UUID       `json:"contractId"`
	ContractVersion       int             `json:"contractVersion"`
	CauseInvoiceIDs       []uuid.UUID     `json:"causeInvoiceIds"`
	UnitIDs               []uuid.UUID     `json:"unitIds"`
	RulesVersion          string          `json:"rulesVersion"`
	NoticeAt              time.Time       `json:"noticeAt"`
	ExecuteAfter          time.Time       `json:"executeAfter"`
	Reason                string          `json:"reason"`
	Policy                json.RawMessage `json:"policy"`
	State                 string          `json:"state"`
	Exception             *Exception      `json:"exception"`
	GraceUntil            *time.Time      `json:"graceUntil"`
	PerUnit               []Unit          `json:"perUnit"`
	Events                []Event         `json:"events"`
	ReleaseIntent         json.RawMessage `json:"releaseIntent"`
	customerID            uuid.UUID
	customerOrg           uuid.UUID
}

// ReleaseView is RestrictionReleaseView (IR03).
type ReleaseView struct {
	ID            uuid.UUID       `json:"id"`
	Version       int             `json:"version"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	UnitIDs       []uuid.UUID     `json:"unitIds"`
	RulesVersion  string          `json:"rulesVersion"`
	Policy        json.RawMessage `json:"policy"`
	State         string          `json:"state"`
	PerUnit       []Unit          `json:"perUnit"`
	RecoveryCases json.RawMessage `json:"recoveryCases"`
	NoticeAt      time.Time       `json:"noticeAt"`
	ExecuteAfter  time.Time       `json:"executeAfter"`
	Projection    string          `json:"projection"`
	ReleaseIntent json.RawMessage `json:"releaseIntent"`
}

const cols = `r.id, r.tenant_id, r.version, r.created_at, r.updated_at, r.recovery_cases, r.notice_notification_ids, r.contract_id, r.contract_version,
	r.rules_version, r.notice_at, r.execute_after, r.reason, r.policy, r.state, r.exception_until, r.exception_reason, r.grace_until, r.release_intent,
	r.customer_id, (SELECT organization_id FROM assets.customers cu WHERE cu.id = r.customer_id),
	COALESCE((SELECT array_agg(invoice_id ORDER BY invoice_id) FROM restrictions.restriction_invoices ri WHERE ri.restriction_id = r.id), '{}')`

func scan(row pgx.Row) (Restriction, error) {
	var x Restriction
	var until *time.Time
	var exReason *string
	var intent []byte
	err := row.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.RecoveryCases, &x.NoticeNotificationIDs, &x.ContractID, &x.ContractVersion,
		&x.RulesVersion, &x.NoticeAt, &x.ExecuteAfter, &x.Reason, &x.Policy, &x.State, &until, &exReason, &x.GraceUntil, &intent, &x.customerID, &x.customerOrg, &x.CauseInvoiceIDs)
	if until != nil {
		x.Exception = &Exception{Until: *until, Reason: exReason}
	}
	x.ReleaseIntent = json.RawMessage("null")
	if len(intent) > 0 {
		x.ReleaseIntent = intent
	}
	return x, err
}

// decorate loads per-unit state and the audit events.
func decorate(ctx context.Context, c *ops.Call, x *Restriction) error {
	rows, err := c.Tx.Query(ctx, `SELECT unit_id, apply_state, release_state, apply_command_ids, release_command_ids, observed_restriction, observed_at, evidence_id, pending_reason
		FROM restrictions.restriction_units WHERE restriction_id = $1 ORDER BY unit_id`, x.ID)
	if err != nil {
		return err
	}
	x.PerUnit, x.UnitIDs = []Unit{}, []uuid.UUID{}
	for rows.Next() {
		var u Unit
		var obs []byte
		if err := rows.Scan(&u.UnitID, &u.ApplyState, &u.ReleaseState, &u.ApplyCommandIDs, &u.ReleaseCommandIDs, &obs, &u.ObservedAt, &u.EvidenceID, &u.PendingReason); err != nil {
			rows.Close()
			return err
		}
		u.ObservedRestriction = json.RawMessage("null")
		if len(obs) > 0 {
			u.ObservedRestriction = obs
		}
		x.PerUnit = append(x.PerUnit, u)
		x.UnitIDs = append(x.UnitIDs, u.UnitID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = c.Tx.Query(ctx, `SELECT id, tenant_id, actor_id, actor_role_at_time, action, previous_version, next_version, occurred_at, correlation_id, result,
		masked_before, masked_after, reason FROM audit.audit_log WHERE target_kind = 'restriction' AND target_id = $1 ORDER BY occurred_at, correlation_id, id`, x.ID.String())
	if err != nil {
		return err
	}
	defer rows.Close()
	x.Events = []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ActorID, &e.ActorRoleAtTime, &e.Action, &e.PreviousVersion, &e.NextVersion, &e.OccurredAt, &e.CorrelationID, &e.Result,
			&e.MaskedBefore, &e.MaskedAfter, &e.Reason); err != nil {
			return err
		}
		e.Version, e.CreatedAt, e.UpdatedAt = 1, e.OccurredAt, e.OccurredAt
		e.TargetRef = map[string]string{"kind": "restriction", "id": x.ID.String()}
		x.Events = append(x.Events, e)
	}
	return rows.Err()
}

// stateEvents maps audited actions to the state-change events clients may see (IR42).
var stateEvents = map[string]string{"restrictions.schedule": "scheduled", "restrictions.execute": "requested", "restriction.applied": "applied",
	"restrictions.cancel": "release_requested", "restrictions.release": "release_requested", "restrictions.override": "release_requested",
	"restriction.released": "released", "restrictions.defer": "defer", "restrictions.exempt": "exempt", "payments.release": "release_requested"}

// maskForClient applies the client RestrictionDetail projection (IR42).
func maskForClient(x *Restriction) {
	events := []Event{}
	for _, e := range x.Events {
		if _, ok := stateEvents[e.Action]; !ok || e.Result != "success" {
			continue
		}
		e.ActorID, e.ActorRoleAtTime, e.Reason = "masked", "admin", nil
		e.MaskedBefore, e.MaskedAfter = map[string]*string{}, map[string]*string{}
		events = append(events, e)
	}
	x.Events = events
	if x.Exception != nil {
		x.Exception.Reason = nil
	}
	if len(x.ReleaseIntent) > 0 && string(x.ReleaseIntent) != "null" {
		var ri map[string]any
		if json.Unmarshal(x.ReleaseIntent, &ri) == nil {
			ri["actorMembershipId"] = "masked"
			x.ReleaseIntent, _ = json.Marshal(ri)
		}
	}
}

func releaseView(x Restriction) ReleaseView {
	intent := json.RawMessage("null")
	var ri map[string]any
	if json.Unmarshal(x.ReleaseIntent, &ri) == nil && ri != nil {
		intent, _ = json.Marshal(map[string]any{"source": ri["source"], "at": ri["at"]})
	}
	return ReleaseView{ID: x.ID, Version: x.Version, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt, UnitIDs: x.UnitIDs, RulesVersion: x.RulesVersion, Policy: x.Policy,
		State: x.State, PerUnit: x.PerUnit, RecoveryCases: x.RecoveryCases, NoticeAt: x.NoticeAt, ExecuteAfter: x.ExecuteAfter, Projection: "release", ReleaseIntent: intent}
}

// fullReader reports whether the caller receives the canonical Restriction (IR03).
func fullReader(c *ops.Call) bool {
	return c.Principal.Role == "admin" && (c.Principal.Permissions["restriction.read"] || c.Principal.Permissions["restriction.write"])
}

// project returns the RestrictionRead the caller may receive (IR03 / IR42).
func project(c *ops.Call, x Restriction) any {
	switch {
	case c.Principal.Role == "client":
		maskForClient(&x)
		return x
	case fullReader(c):
		return x
	default:
		return releaseView(x)
	}
}

func load(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Restriction, error) {
	q := "SELECT " + cols + " FROM restrictions.restrictions r WHERE r.id = $1"
	if lock {
		q += " FOR UPDATE OF r"
	}
	x, err := scan(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	return x, decorate(ctx, c, &x)
}

// GetInput is restrictions.get input.
type GetInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *GetInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

func (m Restrictions) get(ctx context.Context, c *ops.Call, in *GetInput) (any, error) {
	x, err := load(ctx, c, in.ID, false)
	if err != nil {
		return nil, err
	}
	return project(c, x), nil
}

var states = []string{"scheduled", "requested", "applied", "release_requested", "released", "cancelled"}

type listFilter struct {
	ContractID *uuid.UUID `json:"contractId,omitempty"`
	InvoiceID  *uuid.UUID `json:"invoiceId,omitempty"`
	State      *string    `json:"state,omitempty"`
}

func (m Restrictions) query(ctx context.Context, c *ops.Call, in paging.Query, f listFilter, extra []string, args []any) ([]Restriction, *string, int, int, error) {
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "r.id", "createdAt": "r.created_at", "executeAfter": "r.execute_after", "noticeAt": "r.notice_at"}, "r.created_at DESC, r.id DESC")
	if err != nil {
		return nil, nil, 0, 0, err
	}
	w, err := paging.Resolve(in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := append([]string{"TRUE"}, extra...)
	if f.ContractID != nil {
		conds = append(conds, "r.contract_id = "+add(*f.ContractID))
	}
	if f.InvoiceID != nil {
		conds = append(conds, "EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri WHERE ri.restriction_id = r.id AND ri.invoice_id = "+add(*f.InvoiceID)+")")
	}
	if f.State != nil {
		conds = append(conds, "r.state = "+add(*f.State))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM restrictions.restrictions r WHERE "+where, args...).Scan(&total); err != nil {
		return nil, nil, 0, 0, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM restrictions.restrictions r WHERE %s ORDER BY %s LIMIT %d OFFSET %d", cols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	out := []Restriction{}
	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			rows.Close()
			return nil, nil, 0, 0, err
		}
		out = append(out, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, 0, 0, err
	}
	for i := range out {
		if err := decorate(ctx, c, &out[i]); err != nil {
			return nil, nil, 0, 0, err
		}
	}
	return out, w.Next(total), total, w.Snapshot, nil
}

func decodeFilter(raw json.RawMessage, f *listFilter) error {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if dec.Decode(f) != nil || (f.State != nil && !slices.Contains(states, *f.State)) {
		return apperr.Fields(map[string]string{"filters": "error.invalid"})
	}
	return nil
}

func (m Restrictions) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[any], error) {
	var f listFilter
	if err := decodeFilter(in.Filters, &f); err != nil {
		return paging.Page[any]{}, err
	}
	if !fullReader(c) && (f.ContractID != nil || f.InvoiceID != nil) {
		return paging.Page[any]{}, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	xs, next, total, snap, err := m.query(ctx, c, *in, f, nil, nil)
	if err != nil {
		return paging.Page[any]{}, err
	}
	items := make([]any, len(xs))
	for i, x := range xs {
		items[i] = project(c, x)
	}
	return paging.Page[any]{Items: items, NextCursor: next, Total: total, SnapshotVersion: snap}, nil
}

// ForInvoiceInput is restrictions.forInvoice input.
type ForInvoiceInput struct {
	InvoiceID uuid.UUID    `json:"invoiceId"`
	Query     paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *ForInvoiceInput) Validate() map[string]string {
	if in.InvoiceID == uuid.Nil {
		return map[string]string{"invoiceId": "error.required"}
	}
	return nil
}

func (m Restrictions) forInvoice(ctx context.Context, c *ops.Call, in *ForInvoiceInput) (paging.Page[Restriction], error) {
	var org uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT k.customer_org_id FROM billing.invoices i JOIN billing.contracts k ON k.id = i.contract_id AND k.version = i.contract_version WHERE i.id = $1`, in.InvoiceID).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "client" && org != c.Principal.OrgID) {
		return paging.Page[Restriction]{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return paging.Page[Restriction]{}, err
	}
	var f listFilter
	if err := decodeFilter(in.Query.Filters, &f); err != nil {
		return paging.Page[Restriction]{}, err
	}
	if f.InvoiceID != nil || f.ContractID != nil {
		return paging.Page[Restriction]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
	}
	f.InvoiceID = &in.InvoiceID
	xs, next, total, snap, err := m.query(ctx, c, in.Query, f, nil, nil)
	if err != nil {
		return paging.Page[Restriction]{}, err
	}
	if c.Principal.Role == "client" {
		for i := range xs {
			maskForClient(&xs[i])
		}
	}
	return paging.Page[Restriction]{Items: xs, NextCursor: next, Total: total, SnapshotVersion: snap}, nil
}

// Register binds the restriction operations.
func Register(r *ops.Registry, m Restrictions) {
	ops.Register(r, "restrictions.get", m.get)
	ops.Register(r, "restrictions.list", m.list)
	ops.Register(r, "restrictions.forInvoice", m.forInvoice)
	ops.Register(r, "restrictions.schedule", m.schedule)
	ops.Register(r, "restrictions.execute", m.execute)
	ops.Register(r, "restrictions.cancel", m.cancel)
	RegisterActions(r, m)
}
