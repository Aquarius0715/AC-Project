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

	"github.com/pradita/ac-project/service/api/internal/modules/audit"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// SystemActor is the internal 'system-restriction' membership that owns restriction Commands (IR35); it matches
// the fixture ID system-restriction under the seed namespace.
var SystemActor = uuid.NewSHA1(
	uuid.MustParse("6f1c3a52-6d0b-5c1e-9a57-0d1b7a4c2e10"),
	[]byte("system-restriction"),
)

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

// Units resolves restriction targets and the device observations equipment keeps on units.
type Units interface {
	RestrictionTarget(ctx context.Context, c *ops.Call, unit uuid.UUID) (UnitCaps, bool, error)
	// Observation returns the unit's observedRestriction (raw JSON, nil when none) and lastSeenAt (IR194).
	Observation(ctx context.Context, c *ops.Call, unit uuid.UUID) ([]byte, *time.Time, error)
}

// Devices reports the device bound to a unit (Devices).
type Devices interface {
	BoundDevice(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
	) (id uuid.UUID, connection, powerSignal string, found bool, err error)
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
	ObservedRestriction json.RawMessage `json:"observedRestriction" swaggertype:"object"`
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
	ActorName       *string            `json:"actorName"`
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
	RecoveryCases         json.RawMessage `json:"recoveryCases"         swaggertype:"object"`
	NoticeNotificationIDs []uuid.UUID     `json:"noticeNotificationIds"`
	ContractID            uuid.UUID       `json:"contractId"`
	ContractVersion       int             `json:"contractVersion"`
	CauseInvoiceIDs       []uuid.UUID     `json:"causeInvoiceIds"`
	UnitIDs               []uuid.UUID     `json:"unitIds"`
	RulesVersion          string          `json:"rulesVersion"`
	NoticeAt              time.Time       `json:"noticeAt"`
	ExecuteAfter          time.Time       `json:"executeAfter"`
	Reason                string          `json:"reason"`
	Policy                json.RawMessage `json:"policy"                swaggertype:"object"`
	State                 string          `json:"state"`
	Exception             *Exception      `json:"exception"`
	GraceUntil            *time.Time      `json:"graceUntil"`
	PerUnit               []Unit          `json:"perUnit"`
	Events                []Event         `json:"events"`
	ReleaseIntent         json.RawMessage `json:"releaseIntent"         swaggertype:"object"`
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
	Policy        json.RawMessage `json:"policy"        swaggertype:"object"`
	State         string          `json:"state"`
	PerUnit       []Unit          `json:"perUnit"`
	RecoveryCases json.RawMessage `json:"recoveryCases" swaggertype:"object"`
	NoticeAt      time.Time       `json:"noticeAt"`
	ExecuteAfter  time.Time       `json:"executeAfter"`
	Projection    string          `json:"projection"`
	ReleaseIntent json.RawMessage `json:"releaseIntent" swaggertype:"object"`
}

const cols = `r.id, r.tenant_id, r.version, r.created_at, r.updated_at, r.recovery_cases, r.notice_notification_ids, ` +
	`r.contract_id, r.contract_version,
	r.rules_version, r.notice_at, r.execute_after, r.reason, r.policy, r.state, r.exception_until, r.exception_reason, ` +
	`r.grace_until, r.release_intent,
	r.customer_id, (SELECT organization_id FROM billing.ref_customers cu WHERE cu.id = r.customer_id),
	COALESCE((SELECT array_agg(invoice_id ORDER BY invoice_id) FROM restrictions.restriction_invoices ri WHERE ` +
	`ri.restriction_id = r.id), '{}')`

func scan(row pgx.Row) (Restriction, error) {
	var (
		restriction Restriction
		until       *time.Time
		exReason    *string
		intent      []byte
	)

	err := row.Scan(
		&restriction.ID,
		&restriction.TenantID,
		&restriction.Version,
		&restriction.CreatedAt,
		&restriction.UpdatedAt,
		&restriction.RecoveryCases,
		&restriction.NoticeNotificationIDs,
		&restriction.ContractID,
		&restriction.ContractVersion,
		&restriction.RulesVersion,
		&restriction.NoticeAt,
		&restriction.ExecuteAfter,
		&restriction.Reason,
		&restriction.Policy,
		&restriction.State,
		&until,
		&exReason,
		&restriction.GraceUntil,
		&intent,
		&restriction.customerID,
		&restriction.customerOrg,
		&restriction.CauseInvoiceIDs,
	)
	if until != nil {
		restriction.Exception = &Exception{Until: *until, Reason: exReason}
	}

	restriction.ReleaseIntent = json.RawMessage("null")
	if len(intent) > 0 {
		restriction.ReleaseIntent = intent
	}

	return restriction, apperr.Wrap(err, "scan (scan)")
}

// decorate loads per-unit state and the audit events.
func decorate(ctx context.Context, call *ops.Call, x *Restriction) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT unit_id, apply_state, release_state, apply_command_ids, release_command_ids, observed_restriction,
			observed_at, evidence_id, pending_reason
		FROM restrictions.restriction_units WHERE restriction_id = $1 ORDER BY unit_id`,
		x.ID,
	)
	if err != nil {
		return apperr.Wrap(err, "select restrictions.restriction_units")
	}

	x.PerUnit, x.UnitIDs = []Unit{}, []uuid.UUID{}

	for rows.Next() {
		var (
			unit Unit
			obs  []byte
		)

		err := rows.Scan(
			&unit.UnitID,
			&unit.ApplyState,
			&unit.ReleaseState,
			&unit.ApplyCommandIDs,
			&unit.ReleaseCommandIDs,
			&obs,
			&unit.ObservedAt,
			&unit.EvidenceID,
			&unit.PendingReason,
		)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select restrictions.restriction_units")
		}

		unit.ObservedRestriction = json.RawMessage("null")
		if len(obs) > 0 {
			unit.ObservedRestriction = obs
		}

		x.PerUnit = append(x.PerUnit, unit)
		x.UnitIDs = append(x.UnitIDs, unit.UnitID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select restrictions.restriction_units")
	}

	hist, err := audit.History(
		ctx,
		call,
		kindRestriction,
		x.ID.String(),
	) // identity's audit log (IR196)
	if err != nil {
		return apperr.Wrap(err, "audit.History")
	}

	x.Events = []Event{}
	for _, v := range hist {
		x.Events = append(x.Events, Event(v))
	}

	return nil
}

// stateEvents maps audited actions to the state-change events clients may see (IR42).
var stateEvents = map[string]string{
	"restrictions.schedule": stateScheduled,
	opExecute:               stateRequested,
	"restriction.applied":   stateApplied,
	"restrictions.cancel":   stateReleaseRequested,
	"restrictions.release":  stateReleaseRequested,
	"restrictions.override": stateReleaseRequested,
	"restriction.released":  stateReleased,
	"restrictions.defer":    "defer",
	"restrictions.exempt":   "exempt",
	"payments.release":      stateReleaseRequested,
}

// maskForClient applies the client RestrictionDetail projection (IR42).
func maskForClient(restriction *Restriction) {
	events := []Event{}

	for _, event := range restriction.Events {
		if _, ok := stateEvents[event.Action]; !ok || event.Result != "success" {
			continue
		}

		event.ActorID, event.ActorName, event.ActorRoleAtTime, event.Reason = "masked", nil, "admin", nil
		event.MaskedBefore, event.MaskedAfter = map[string]*string{}, map[string]*string{}
		events = append(events, event)
	}

	restriction.Events = events
	if restriction.Exception != nil {
		restriction.Exception.Reason = nil
	}

	if len(restriction.ReleaseIntent) > 0 && string(restriction.ReleaseIntent) != "null" {
		var ri map[string]any
		if json.Unmarshal(restriction.ReleaseIntent, &ri) == nil {
			ri["actorMembershipId"] = "masked"

			masked, err := json.Marshal(ri)
			if err != nil {
				masked = nil // never the unmasked intent
			}

			restriction.ReleaseIntent = masked
		}
	}
}

func releaseView(restriction Restriction) ReleaseView {
	intent := json.RawMessage("null")

	var ri map[string]any
	if json.Unmarshal(restriction.ReleaseIntent, &ri) == nil && ri != nil {
		view, err := json.Marshal(map[string]any{keySource: ri[keySource], "at": ri["at"]})
		if err == nil {
			intent = view
		}
	}

	return ReleaseView{
		ID:            restriction.ID,
		Version:       restriction.Version,
		CreatedAt:     restriction.CreatedAt,
		UpdatedAt:     restriction.UpdatedAt,
		UnitIDs:       restriction.UnitIDs,
		RulesVersion:  restriction.RulesVersion,
		Policy:        restriction.Policy,
		State:         restriction.State,
		PerUnit:       restriction.PerUnit,
		RecoveryCases: restriction.RecoveryCases,
		NoticeAt:      restriction.NoticeAt,
		ExecuteAfter:  restriction.ExecuteAfter,
		Projection:    phaseRelease,
		ReleaseIntent: intent,
	}
}

// fullReader reports whether the caller receives the canonical Restriction (IR03).
func fullReader(c *ops.Call) bool {
	return c.Principal.Role == "admin" &&
		(c.Principal.Permissions["restriction.read"] || c.Principal.Permissions["restriction.write"])
}

// project returns the RestrictionRead the caller may receive (IR03 / IR42).
func project(call *ops.Call, restriction Restriction) any {
	switch {
	case call.Principal.Role == authz.RoleClient:
		maskForClient(&restriction)

		return restriction
	case fullReader(call):
		return restriction
	default:
		return releaseView(restriction)
	}
}

func load(
	ctx context.Context,
	call *ops.Call,
	restrictionID uuid.UUID,
	lock bool,
) (Restriction, error) {
	q := "SELECT " + cols + " FROM restrictions.restrictions r WHERE r.id = $1"
	if lock {
		q += " FOR UPDATE OF r"
	}

	restriction, err := scan(call.Tx.QueryRow(ctx, q, restrictionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return restriction, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return restriction, err
	}

	return restriction, decorate(ctx, call, &restriction)
}

// GetInput is restrictions.get input.
type GetInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *GetInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// get serves restrictions.get.
//
//	@Summary		restrictions.get (read)
//	@ID				restrictions.get
//	@Description	Authorization: admin:restriction.read | admin:restriction.override:release-projection; \
//	@Description	IR19:all-target-units-required
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A09, DD-A10
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions/{id} [get]
func (m Restrictions) get(ctx context.Context, call *ops.Call, in *GetInput) (any, error) {
	x, err := load(ctx, call, in.ID, false)
	if err != nil {
		return nil, err
	}

	return project(call, x), nil
}

var states = []string{
	stateScheduled,
	stateRequested,
	stateApplied,
	stateReleaseRequested,
	stateReleased,
	stateCancelled,
}

type listFilter struct {
	ContractID *uuid.UUID `json:"contractId,omitempty"`
	InvoiceID  *uuid.UUID `json:"invoiceId,omitempty"`
	Status     *string    `json:"status,omitempty"` // => state
}

func (m Restrictions) query(
	ctx context.Context,
	call *ops.Call,
	query paging.Query,
	filter listFilter,
) ([]Restriction, *string, int, int, error) {
	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":            "r.id",
			"createdAt":     "r.created_at",
			"updatedAt":     "r.updated_at",
			keyExecuteAfter: "r.execute_after",
			"noticeAt":      "r.notice_at",
		},
		"r.created_at DESC, r.id DESC",
	)
	if err != nil {
		return nil, nil, 0, 0, apperr.Wrap(err, "paging.OrderBy")
	}
	// the cursor binds the filter (SR14)
	window, err := paging.Resolve(query, struct {
		F listFilter `json:"f"`
	}{filter}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return nil, nil, 0, 0, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"TRUE"}
	if filter.ContractID != nil {
		conds = append(conds, "r.contract_id = "+add(*filter.ContractID))
	}

	if filter.InvoiceID != nil {
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri WHERE ri.restriction_id = r.id AND ri.invoice_id = "+add(
				*filter.InvoiceID,
			)+")",
		)
	}

	if filter.Status != nil {
		conds = append(conds, "r.state = "+add(*filter.Status))
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM restrictions.restrictions r WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return nil, nil, 0, 0, apperr.Wrap(scanErr, "select restrictions.restrictions")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM restrictions.restrictions r WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			cols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return nil, nil, 0, 0, apperr.Wrap(err, "select restrictions.restrictions")
	}

	out := []Restriction{}

	for rows.Next() {
		restriction, err := scan(rows)
		if err != nil {
			rows.Close()

			return nil, nil, 0, 0, err
		}

		out = append(out, restriction)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, nil, 0, 0, apperr.Wrap(rowsErr, "select restrictions.restrictions")
	}

	for i := range out {
		err := decorate(ctx, call, &out[i])
		if err != nil {
			return nil, nil, 0, 0, err
		}
	}

	return out, window.Next(total), total, window.Snapshot, nil
}

func decodeFilter(raw json.RawMessage, filter *listFilter) error {
	if len(raw) == 0 {
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if dec.Decode(filter) != nil ||
		(filter.Status != nil && !slices.Contains(states, *filter.Status)) {
		return apperr.Fields(map[string]string{"filters": apperr.KeyInvalid})
	}

	return nil
}

// list serves restrictions.list.
//
//	@Summary		restrictions.list (read)
//	@ID				restrictions.list
//	@Description	Authorization: admin:restriction.read | admin:restriction.override:release-projection; \
//	@Description	IR19:all-target-units-required
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A09, DD-A10 · Query: filters invoiceId,contractId,status · sort \
//	@Description	id,createdAt,updatedAt,executeAfter,noticeAt (default createdAt desc;id desc)
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt,executeAfter,noticeAt; default createdAt desc;id desc"
//	@Param			invoiceId	query		string	false	"filter → causeInvoiceIds contains"
//	@Param			contractId	query		string	false	"filter → contractId"
//	@Param			status		query		string	false	"filter → state"
//	@Success		200			{object}	ops.Envelope{data=anyPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions [get]
func (m Restrictions) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[any], error) {
	var filter listFilter

	err := decodeFilter(query.Filters, &filter)
	if err != nil {
		return paging.Page[any]{}, err
	}

	if !fullReader(call) && (filter.ContractID != nil || filter.InvoiceID != nil) {
		return paging.Page[any]{}, apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	results, next, total, snap, err := m.query(ctx, call, *query, filter)
	if err != nil {
		return paging.Page[any]{}, err
	}

	items := make([]any, len(results))
	for i, x := range results {
		items[i] = project(call, x)
	}

	return paging.Page[any]{
		Items:           items,
		NextCursor:      next,
		Total:           total,
		SnapshotVersion: snap,
	}, nil
}

// ForInvoiceInput is restrictions.forInvoice input.
type ForInvoiceInput struct {
	InvoiceID uuid.UUID    `json:"invoiceId"`
	Query     paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *ForInvoiceInput) Validate() map[string]string {
	if in.InvoiceID == uuid.Nil {
		return map[string]string{"invoiceId": apperr.KeyRequired}
	}

	return nil
}

// forInvoice serves restrictions.forInvoice.
//
//	@Summary		restrictions.forInvoice (read)
//	@ID				restrictions.forInvoice
//	@Description	Authorization: client:self | admin:restriction.read; IR19:all-target-units-required
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C12 · Query: filters contractId,status · sort \
//	@Description	id,createdAt,updatedAt,executeAfter,noticeAt (default createdAt desc;id desc)
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			invoiceId	path		string	true	"input field invoiceId"
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt,executeAfter,noticeAt; default createdAt desc;id desc"
//	@Param			contractId	query		string	false	"filter → contractId"
//	@Param			status		query		string	false	"filter → state"
//	@Success		200			{object}	ops.Envelope{data=RestrictionPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/invoices/{invoiceId}/restrictions [get]
func (m Restrictions) forInvoice(
	ctx context.Context,
	call *ops.Call,
	input *ForInvoiceInput,
) (paging.Page[Restriction], error) {
	var org uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT k.customer_org_id FROM billing.invoices i JOIN billing.contracts k ON k.id =
		i.contract_id AND k.version = i.contract_version WHERE i.id = $1`, input.InvoiceID).
		Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == authz.RoleClient && org != call.Principal.OrgID) {
		return paging.Page[Restriction]{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return paging.Page[Restriction]{}, apperr.Wrap(err, "select billing.invoices")
	}

	var filter listFilter

	decodeFilterErr := decodeFilter(input.Query.Filters, &filter)
	if decodeFilterErr != nil {
		return paging.Page[Restriction]{}, decodeFilterErr
	}
	// query catalog: input.invoiceId (the REST path) => causeInvoiceIds contains; contractId => contractId; status =>
	// state. invoiceId is no filter of this read: the REST binding refuses it as error.notAllowed
	filter.InvoiceID = &input.InvoiceID

	results, next, total, snap, err := m.query(ctx, call, input.Query, filter)
	if err != nil {
		return paging.Page[Restriction]{}, err
	}

	if call.Principal.Role == authz.RoleClient {
		for i := range results {
			maskForClient(&results[i])
		}
	}

	return paging.Page[Restriction]{
		Items:           results,
		NextCursor:      next,
		Total:           total,
		SnapshotVersion: snap,
	}, nil
}

// Register binds the restriction operations.
func Register(reg *ops.Registry, module Restrictions) {
	ops.Register(reg, "restrictions.get", module.get)
	ops.Register(reg, "restrictions.list", module.list)
	ops.Register(reg, "restrictions.forInvoice", module.forInvoice)
	ops.Register(reg, "restrictions.schedule", module.schedule)
	ops.Register(reg, opExecute, module.execute)
	ops.Register(reg, "restrictions.cancel", module.cancel)
	RegisterActions(reg, module)
}
