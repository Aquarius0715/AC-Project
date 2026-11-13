package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// The states of an installation request (DEC-107).
const (
	requestSubmitted = "submitted"
	requestApproved  = "approved"
	requestDeclined  = "declined"
	requestCancelled = "cancelled"
)

// The limits of an installation request.
const (
	maxRequestUnits      = 10
	requestSlotCount     = 3
	maxRequestNoteLength = 2000
	minDeclineReason     = 5
	maxDeclineReason     = 500
)

const (
	fieldQuantity = "quantity"
	fieldNote     = "note"
	fieldPropID   = "propertyId"
	fieldModelID  = "modelId"

	fieldRequestID = "installationRequestId"
	fieldSortField = "sort.field"
	fieldSlots     = "preferredSlots"
	fieldCustOrgID = "customerOrgId"
	clientOwner    = "owner"
)

// InstallationRequest is InstallationRequest of service-contracts.ts: a customer owner's request for new aircons.
type InstallationRequest struct {
	ID             uuid.UUID   `json:"id"`
	TenantID       uuid.UUID   `json:"tenantId"`
	Version        int         `json:"version"`
	CreatedAt      time.Time   `json:"createdAt"`
	UpdatedAt      time.Time   `json:"updatedAt"`
	CustomerOrgID  uuid.UUID   `json:"customerOrgId"`
	PropertyID     uuid.UUID   `json:"propertyId"`
	SpaceID        *uuid.UUID  `json:"spaceId"`
	Quantity       int         `json:"quantity"`
	ModelID        *uuid.UUID  `json:"modelId"`
	WithIoT        bool        `json:"withIot"`
	PreferredSlots []Slot      `json:"preferredSlots"`
	ContactWindow  *string     `json:"contactWindow"`
	Note           *string     `json:"note"`
	Status         string      `json:"status"`
	DeclineReason  *string     `json:"declineReason"`
	DecidedAt      *time.Time  `json:"decidedAt"`
	JobIDs         []uuid.UUID `json:"jobIds"`
}

// InstallSiteInput is what the equipment domain is asked about the site of a request.
type InstallSiteInput struct {
	PropertyID uuid.UUID  `json:"propertyId"`
	SpaceID    *uuid.UUID `json:"spaceId"`
	ModelID    *uuid.UUID `json:"modelId"`
}

// InstallSite is the equipment domain's answer: the customer organization of the property, whether it exists and is
// archived, whether the space belongs to the property (and is not archived) and whether the model exists.
type InstallSite struct {
	OrgID    uuid.UUID `json:"orgId"`
	Found    bool      `json:"found"`
	Archived bool      `json:"archived"`
	SpaceOK  bool      `json:"spaceOk"`
	ModelOK  bool      `json:"modelOk"`
}

// InstallationRequests is the installation request operation set (FR-C20).
type InstallationRequests struct {
	Units JobUnits
}

// RegisterInstallationRequests binds the installationRequests.* operations.
func RegisterInstallationRequests(reg *ops.Registry, m InstallationRequests) {
	ops.Register(reg, "installationRequests.create", m.create)
	ops.Register(reg, "installationRequests.list", m.list)
	ops.Register(reg, "installationRequests.cancel", m.cancel)
	ops.Register(reg, "installationRequests.decline", m.decline)
}

// requestCols are the columns of a request row in the order scanRequest reads them; the linked jobs come last.
const requestCols = `r.id, r.tenant_id, r.version, r.created_at, r.updated_at, r.customer_org_id, r.property_id,
	r.space_id, r.quantity, r.model_id, r.with_iot, r.preferred_slots, r.contact_window, r.note, r.status,
	r.decline_reason, r.decided_at,
	COALESCE((SELECT array_agg(j.id ORDER BY j.created_at, j.id) FROM maintenance.jobs j
		WHERE j.installation_request_id = r.id), '{}')`

func scanRequest(row pgx.Row) (InstallationRequest, error) {
	var (
		request   InstallationRequest
		preferred []byte
	)

	err := row.Scan(
		&request.ID,
		&request.TenantID,
		&request.Version,
		&request.CreatedAt,
		&request.UpdatedAt,
		&request.CustomerOrgID,
		&request.PropertyID,
		&request.SpaceID,
		&request.Quantity,
		&request.ModelID,
		&request.WithIoT,
		&preferred,
		&request.ContactWindow,
		&request.Note,
		&request.Status,
		&request.DeclineReason,
		&request.DecidedAt,
		&request.JobIDs,
	)
	if err != nil {
		return request, apperr.Wrap(err, "scan (scanRequest)")
	}

	err = json.Unmarshal(preferred, &request.PreferredSlots)

	return request, apperr.Wrap(err, "json.Unmarshal")
}

// requireOwner is the client:self-customer:owner alternative: a client must be an owner (FORBIDDEN).
func requireOwner(c *ops.Call) error {
	if c.Principal.Role == roleClient && c.Principal.ClientRole != clientOwner {
		return apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	return nil
}

// ---- installationRequests.create ----

// CreateRequestInput is installationRequests.create input.
type CreateRequestInput struct {
	PropertyID     uuid.UUID  `json:"propertyId"`
	SpaceID        *uuid.UUID `json:"spaceId,omitempty"`
	Quantity       int        `json:"quantity"`
	ModelID        *uuid.UUID `json:"modelId,omitempty"`
	WithIoT        bool       `json:"withIot"`
	PreferredSlots []Slot     `json:"preferredSlots"`
	ContactWindow  *string    `json:"contactWindow,omitempty"`
	Note           *string    `json:"note,omitempty"`
}

// Validate implements ops.Validator (shape only; slot timing needs the clock).
func (in *CreateRequestInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.PropertyID == uuid.Nil {
		fieldErrs[fieldPropID] = apperr.KeyRequired
	}

	if in.Quantity < 1 || in.Quantity > maxRequestUnits {
		fieldErrs[fieldQuantity] = apperr.KeyRange
	}

	if len(in.PreferredSlots) != requestSlotCount {
		fieldErrs[fieldSlots] = apperr.KeyCount
	}

	if in.ContactWindow != nil {
		*in.ContactWindow = strings.TrimSpace(*in.ContactWindow)
		if utf8.RuneCountInString(*in.ContactWindow) > maxContactWindowLength {
			fieldErrs["contactWindow"] = apperr.KeyLength
		} else if !ContactWindowOK(*in.ContactWindow) {
			fieldErrs["contactWindow"] = "errors.contact_details_forbidden"
		}
	}

	if in.Note != nil {
		*in.Note = strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(*in.Note) > maxRequestNoteLength {
			fieldErrs[fieldNote] = apperr.KeyLength
		}
	}

	return fieldErrs
}

// create serves installationRequests.create.
//
//	@Summary		installationRequests.create (write)
//	@ID				installationRequests.create
//	@Description	Authorization: client:self-customer:owner
//	@Description	Validation: D01; input constraints in the corresponding DD; property and space of the caller's \
//	@Description	customer and not archived; quantity 1–10; 3 distinct future slots ≥1 day ahead (IR113); note ≤2000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C20
//	@Tags			installationRequests
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		CreateRequestInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=InstallationRequest}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/installation-requests [post]
func (m InstallationRequests) create(
	ctx context.Context,
	call *ops.Call,
	input *CreateRequestInput,
) (InstallationRequest, error) {
	err := requireOwner(call)
	if err != nil {
		return InstallationRequest{}, err
	}

	site, err := m.Units.InstallSite(
		ctx,
		call,
		InstallSiteInput{
			PropertyID: input.PropertyID,
			SpaceID:    input.SpaceID,
			ModelID:    input.ModelID,
		},
	)
	if err != nil {
		return InstallationRequest{}, apperr.Wrap(err, "maintenance.JobUnits.InstallSite")
	}

	if !site.Found || site.OrgID != call.Principal.OrgID ||
		(input.SpaceID != nil && !site.SpaceOK) {
		return InstallationRequest{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	switch {
	case site.Archived:
		return InstallationRequest{}, apperr.Fields(
			map[string]string{fieldPropID: apperr.KeyUnitArchived},
		)
	case input.ModelID != nil && !site.ModelOK:
		return InstallationRequest{}, apperr.Fields(
			map[string]string{fieldModelID: apperr.KeyNotFound},
		)
	case !PreferredSlotsOK(call.Now, input.PreferredSlots):
		return InstallationRequest{}, apperr.Fields(
			map[string]string{fieldSlots: apperr.KeySlotRules},
		)
	}

	slots, err := json.Marshal(input.PreferredSlots)
	if err != nil {
		return InstallationRequest{}, apperr.Wrap(err, "json.Marshal")
	}

	var contact *string
	if input.ContactWindow != nil && *input.ContactWindow != "" {
		contact = input.ContactWindow
	}

	var note *string
	if input.Note != nil && *input.Note != "" {
		note = input.Note
	}

	var id uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO maintenance.installation_requests (tenant_id, customer_org_id,
		property_id, space_id, quantity, model_id, with_iot, preferred_slots, contact_window, note, created_by)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		site.OrgID, input.PropertyID, input.SpaceID, input.Quantity, input.ModelID, input.WithIoT, slots, contact,
		note, call.Principal.UserID).
		Scan(&id)
	if err != nil {
		return InstallationRequest{}, apperr.Wrap(err, "insert maintenance.installation_requests")
	}

	request, err := m.load(ctx, call, id, false)
	if err != nil {
		return request, err
	}

	call.Emit(outboxEvent("installation_request", id, "InstallationRequested",
		map[string]any{fieldCustOrgID: site.OrgID, "quantity": input.Quantity}))
	call.Audit(
		auditEntry(
			"installationRequests.create",
			"installation_request",
			id,
			nil,
			&request.Version,
			"",
		),
	)

	return request, nil
}

// load reads a request with its linked jobs; forUpdate locks the row. A client sees only its own customer's requests
// (NOT_FOUND otherwise).
func (InstallationRequests) load(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
	forUpdate bool,
) (InstallationRequest, error) {
	query := `SELECT ` + requestCols + ` FROM maintenance.installation_requests r WHERE r.id = $1`
	if forUpdate {
		query += ` FOR UPDATE OF r`
	}

	request, err := scanRequest(call.Tx.QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && call.Principal.Role == roleClient &&
		request.CustomerOrgID != call.Principal.OrgID) {
		return InstallationRequest{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return request, err
}

// ---- installationRequests.list ----

// requestFilters are the filters of installationRequests.list.
type requestFilters struct {
	Status     *string    `json:"status,omitempty"`
	CustomerID *uuid.UUID `json:"customerId,omitempty"`
	PropertyID *uuid.UUID `json:"propertyId,omitempty"`
}

// requestSorts maps the sort fields to their columns.
func requestSorts() map[string]string {
	return map[string]string{
		"id":        "r.id",
		fieldStatus: "r.status",
		"createdAt": "r.created_at",
		"updatedAt": "r.updated_at",
	}
}

// list serves installationRequests.list.
//
//	@Summary		installationRequests.list (read)
//	@ID				installationRequests.list
//	@Description	Authorization: client:self-customer:owner | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C20, DD-A06 · Query: filters status,customerId,propertyId · sort \
//	@Description	id,status,createdAt,updatedAt (default createdAt desc;id asc)
//	@Tags			installationRequests
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,status,createdAt,updatedAt; default createdAt desc;id asc"
//	@Param			status		query		string	false	"filter → status"
//	@Param			customerId	query		string	false	"filter → customerOrgId"
//	@Param			propertyId	query		string	false	"filter → propertyId"
//	@Success		200			{object}	ops.Envelope{data=InstallationRequestPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/installation-requests [get]
func (m InstallationRequests) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[InstallationRequest], error) {
	err := requireOwner(call)
	if err != nil {
		return paging.Page[InstallationRequest]{}, err
	}

	var filters requestFilters

	if !decodeFilters(query.Filters, &filters) {
		return paging.Page[InstallationRequest]{}, invalidFilters()
	}

	if filters.Status != nil && !map[string]bool{
		requestSubmitted: true, requestApproved: true, requestDeclined: true, requestCancelled: true,
	}[*filters.Status] {
		return paging.Page[InstallationRequest]{}, invalidFilter(fieldStatus)
	}

	order, err := requestOrder(query.Sort)
	if err != nil {
		return paging.Page[InstallationRequest]{}, err
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[InstallationRequest]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args, err := m.requestWhere(ctx, call, &filters)
	if err != nil {
		return paging.Page[InstallationRequest]{}, err
	}

	var total int

	err = call.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.installation_requests r WHERE `+where, args...).
		Scan(&total)
	if err != nil {
		return paging.Page[InstallationRequest]{}, apperr.Wrap(
			err,
			"count maintenance.installation_requests",
		)
	}

	offsetArg := len(args) + 1

	rows, err := call.Tx.Query(
		ctx,
		`SELECT `+requestCols+` FROM maintenance.installation_requests r WHERE `+where+` ORDER BY `+order+
			` OFFSET $`+strconv.Itoa(
			offsetArg,
		)+
			` LIMIT $`+strconv.Itoa(
			offsetArg+1,
		),
		append(args, window.Offset, window.Limit)...,
	)
	if err != nil {
		return paging.Page[InstallationRequest]{}, apperr.Wrap(
			err,
			"select maintenance.installation_requests",
		)
	}
	defer rows.Close()

	items := []InstallationRequest{}

	for rows.Next() {
		request, err := scanRequest(rows)
		if err != nil {
			return paging.Page[InstallationRequest]{}, err
		}

		items = append(items, request)
	}

	err = rows.Err()
	if err != nil {
		return paging.Page[InstallationRequest]{}, apperr.Wrap(
			err,
			"select maintenance.installation_requests",
		)
	}

	return paging.Page[InstallationRequest]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// requestOrder is the ORDER BY of the list: newest first unless the query sorts by id, status, createdAt or
// updatedAt; the id keeps equal rows in a fixed order.
func requestOrder(sort *paging.SortSpec) (string, error) {
	if sort == nil {
		return "r.created_at DESC, r.id", nil
	}

	column, ok := requestSorts()[sort.Field]
	if !ok {
		return "", apperr.Fields(map[string]string{fieldSortField: apperr.KeyInvalid})
	}

	direction := "ASC"
	if sort.Direction == sortDesc {
		direction = "DESC"
	}

	return column + " " + direction + ", r.id", nil
}

// requestWhere is the WHERE of the list and its arguments: a client sees its own customer, HQ any (a customer filter
// is the customer's organization, as in jobs.list).
func (m InstallationRequests) requestWhere(
	ctx context.Context,
	call *ops.Call,
	filters *requestFilters,
) (string, []any, error) {
	where, args := "TRUE", []any{}

	add := func(condition string, value any) {
		args = append(args, value)
		where += " AND " + strings.ReplaceAll(condition, "?", "$"+strconv.Itoa(len(args)))
	}

	if call.Principal.Role == roleClient {
		add("r.customer_org_id = ?", call.Principal.OrgID)
	}

	if filters.CustomerID != nil {
		org, found, err := m.Units.OrgOfCustomer(ctx, call, *filters.CustomerID)
		if err != nil {
			return "", nil, apperr.Wrap(err, "maintenance.JobUnits.OrgOfCustomer")
		}

		if !found {
			return "FALSE", args, nil
		}

		add("r.customer_org_id = ?", org)
	}

	if filters.Status != nil {
		add("r.status = ?", *filters.Status)
	}

	if filters.PropertyID != nil {
		add("r.property_id = ?", *filters.PropertyID)
	}

	return where, args, nil
}

// ---- installationRequests.cancel and .decline ----

// RequestIDInput is installationRequests.cancel input.
type RequestIDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *RequestIDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return map[string]string{}
}

// DeclineRequestInput is installationRequests.decline input.
type DeclineRequestInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *DeclineRequestInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < minDeclineReason || n > maxDeclineReason {
		fieldErrs[fieldReason] = apperr.KeyLength
	}

	return fieldErrs
}

// cancel serves installationRequests.cancel.
//
//	@Summary		installationRequests.cancel (write)
//	@ID				installationRequests.cancel
//	@Description	Authorization: client:self-customer:owner
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	own customer's submitted request only (CONFLICT otherwise)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C20
//	@Tags			installationRequests
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target installation_request, read installationRequests.list)"
//	@Param			id					path		string			true	"input field id"
//	@Param			request				body		RequestIDInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=InstallationRequest}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/installation-requests/{id}/cancel [post]
func (m InstallationRequests) cancel(
	ctx context.Context,
	call *ops.Call,
	input *RequestIDInput,
) (InstallationRequest, error) {
	err := requireOwner(call)
	if err != nil {
		return InstallationRequest{}, err
	}

	return m.decide(ctx, call, input.ID, requestCancelled, "", "installationRequests.cancel")
}

// decline serves installationRequests.decline.
//
//	@Summary		installationRequests.decline (write)
//	@ID				installationRequests.decline
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	reason 5–500; submitted request only (CONFLICT otherwise)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			installationRequests
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer				true	"all: required (target installation_request, read installationRequests.list)"
//	@Param			id					path		string				true	"input field id"
//	@Param			request				body		DeclineRequestInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=InstallationRequest}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/installation-requests/{id}/decline [post]
func (m InstallationRequests) decline(
	ctx context.Context,
	call *ops.Call,
	input *DeclineRequestInput,
) (InstallationRequest, error) {
	return m.decide(
		ctx,
		call,
		input.ID,
		requestDeclined,
		input.Reason,
		"installationRequests.decline",
	)
}

// decide moves a submitted request to cancelled or declined (CONFLICT from any other state).
func (m InstallationRequests) decide(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
	status, reason, action string,
) (InstallationRequest, error) {
	request, err := m.load(ctx, call, id, true)
	if err != nil {
		return request, err
	}

	if request.Version != *call.ExpectedVersion {
		return InstallationRequest{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if request.Status != requestSubmitted {
		return InstallationRequest{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	var declineReason *string
	if reason != "" {
		declineReason = &reason
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.installation_requests SET status = $2, decline_reason = $3,
		decided_by = $4, decided_at = $5, version = version + 1, updated_at = $5 WHERE id = $1`,
		id,
		status,
		declineReason,
		call.Principal.UserID,
		call.Now,
	)
	if err != nil {
		return InstallationRequest{}, apperr.Wrap(err, "update maintenance.installation_requests")
	}

	next, err := m.load(ctx, call, id, false)
	if err != nil {
		return next, err
	}

	call.Emit(
		outboxEvent(
			"installation_request",
			id,
			"InstallationRequest"+strings.ToUpper(status[:1])+status[1:],
			map[string]any{fieldCustOrgID: next.CustomerOrgID},
		),
	)
	call.Audit(
		auditEntry(action, "installation_request", id, &request.Version, &next.Version, reason),
	)

	return next, nil
}

// ---- the link of jobs.create ----

// checkRequestLink applies DEC-107 to a new installation job made for a request: only HQ links a request, the request
// is submitted or approved and of the unit's customer, and fewer unfinished jobs than its quantity belong to it. The
// request row is locked so two jobs made at once cannot pass the count together.
func checkRequestLink(
	ctx context.Context,
	call *ops.Call,
	input *CreateInput,
	org uuid.UUID,
) error {
	if input.InstallationRequestID == nil {
		return nil
	}

	if call.Principal.Role == roleClient || input.Type != jobInstallation {
		return apperr.Fields(map[string]string{fieldRequestID: apperr.KeyNotAllowed})
	}

	var (
		status   string
		customer uuid.UUID
		quantity int
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, customer_org_id, quantity FROM maintenance.installation_requests
		WHERE id = $1 FOR UPDATE`, *input.InstallationRequestID).
		Scan(&status, &customer, &quantity)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && customer != org) {
		return apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return apperr.Wrap(err, "select maintenance.installation_requests")
	}

	if status != requestSubmitted && status != requestApproved {
		return apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	var open int

	err = call.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.jobs WHERE installation_request_id = $1
		AND status NOT IN ('completed','cancelled')`, *input.InstallationRequestID).
		Scan(&open)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.jobs")
	}

	if open >= quantity {
		return apperr.E(apperr.Conflict, apperr.KeyConflict)
	}

	return nil
}

// approveRequest sets the request approved once a job is made for it.
func approveRequest(ctx context.Context, call *ops.Call, request uuid.UUID) error {
	_, err := call.Tx.Exec(ctx, `UPDATE maintenance.installation_requests SET status = 'approved',
		decided_by = COALESCE(decided_by, $2), decided_at = COALESCE(decided_at, $3), version = version + 1,
		updated_at = $3 WHERE id = $1 AND status = 'submitted'`, request, call.Principal.UserID, call.Now)

	return apperr.Wrap(err, "update maintenance.installation_requests")
}
