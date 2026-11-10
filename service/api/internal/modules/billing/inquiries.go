package billing

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Inquiry is Inquiry of service-contracts.ts.
type Inquiry struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenantId"`
	Version       int        `json:"version"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	CustomerID    uuid.UUID  `json:"customerId"`
	InvoiceID     *uuid.UUID `json:"invoiceId"`
	RestrictionID *uuid.UUID `json:"restrictionId"`
	SubjectType   string     `json:"subjectType"`
	Message       string     `json:"message"`
	State         string     `json:"state"`
	Reply         *string    `json:"reply"`
	customerOrg   uuid.UUID
}

const inquiryCols = `q.id, q.tenant_id, q.version, q.created_at, q.updated_at, q.customer_id, q.invoice_id, ` +
	`q.restriction_id, q.subject_type, q.message, q.state, q.reply, cu.organization_id`

func scanInquiry(r pgx.Row) (Inquiry, error) {
	var inquiry Inquiry

	err := r.Scan(
		&inquiry.ID,
		&inquiry.TenantID,
		&inquiry.Version,
		&inquiry.CreatedAt,
		&inquiry.UpdatedAt,
		&inquiry.CustomerID,
		&inquiry.InvoiceID,
		&inquiry.RestrictionID,
		&inquiry.SubjectType,
		&inquiry.Message,
		&inquiry.State,
		&inquiry.Reply,
		&inquiry.customerOrg,
	)

	return inquiry, apperr.Wrap(err, "scan (scanInquiry)")
}

// The inquiry states (billing.inquiries.state) and subject types.
const (
	inquiryReceived    = "received"
	inquiryAnswered    = "answered"
	subjectPayment     = "payment"
	subjectRestriction = "restriction"
)

func textLen(s *string, key string, maxRunes int, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > maxRunes {
		fe[key] = apperr.KeyLength
	}
}

// InquiryInput is inquiries.create input.
type InquiryInput struct {
	SubjectType   string     `json:"subjectType"`
	InvoiceID     *uuid.UUID `json:"invoiceId,omitempty"`
	RestrictionID *uuid.UUID `json:"restrictionId,omitempty"`
	Message       string     `json:"message"`
}

// Validate implements ops.Validator (IR143 item 1).
func (in *InquiryInput) Validate() map[string]string {
	fieldErrs := map[string]string{}

	switch in.SubjectType {
	case subjectPayment:
		if in.InvoiceID == nil {
			fieldErrs["invoiceId"] = apperr.KeyRequired
		}

		if in.RestrictionID != nil {
			fieldErrs["restrictionId"] = apperr.KeyNotAllowed
		}
	case subjectRestriction:
		if in.RestrictionID == nil {
			fieldErrs["restrictionId"] = apperr.KeyRequired
		}
	default:
		fieldErrs["subjectType"] = apperr.KeyInvalid
	}

	textLen(&in.Message, "message", maxMessageLength, fieldErrs)

	return fieldErrs
}

// notifyMembers stores one inquiry notification for each active membership matching f except the caller (IR95
// inquiry.received / answered); identity resolves the memberships (IR191).
func (m Billing) notifyMembers(
	ctx context.Context,
	call *ops.Call,
	f identity.MembersInput,
	inquiry uuid.UUID,
	status string,
) error {
	ids, err := ops.Query[[]uuid.UUID](ctx, call, identity.QueryMembers, f)
	if err != nil {
		return err
	}

	for _, membershipID := range ids {
		if membershipID == call.Principal.MembershipID {
			continue
		}

		_, err := m.Notify.Create(ctx, call, notify.New{
			RecipientMembershipID: membershipID,
			Type:                  kindInquiry,
			TemplateKey:           kindInquiry,
			TargetKind:            kindInquiry,
			TargetID:              inquiry,
			Params: map[string]any{
				fieldStatus: status,
			},
			Channel:       "",
			Severity:      "",
			SourceAlertID: nil,
		})
		if err != nil {
			return apperr.Wrap(err, "notify.Store.Create")
		}
	}

	return nil
}

// createInquiry serves inquiries.create.
//
//	@Summary		inquiries.create (write)
//	@ID				inquiries.create
//	@Description	Authorization: client:self
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C12
//	@Tags			inquiries
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		InquiryInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Inquiry}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/inquiries [post]
func (m Billing) createInquiry(
	ctx context.Context,
	call *ops.Call,
	input *InquiryInput,
) (Inquiry, error) {
	customer, found, err := m.Customers.CustomerOfOrg(ctx, call, call.Principal.OrgID)
	if err != nil {
		return Inquiry{}, apperr.Wrap(err, "billing.Customers.CustomerOfOrg")
	}

	if !found {
		return Inquiry{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	subjectErr := checkSubject(ctx, call, input, customer)
	if subjectErr != nil {
		return Inquiry{}, subjectErr
	}

	inquiryID := uuid.New()

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO billing.inquiries (id, tenant_id, customer_id, invoice_id, restriction_id, subject_type, message, state,
			created_by, created_at, updated_at)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'received', $7, $8, $8)`,
		inquiryID,
		customer,
		input.InvoiceID,
		input.RestrictionID,
		input.SubjectType,
		input.Message,
		call.Principal.MembershipID,
		call.Now,
	)
	if execErr != nil {
		return Inquiry{}, apperr.Wrap(execErr, "insert billing.inquiries")
	}

	notifyMembersErr := m.notifyMembers(
		ctx,
		call,
		identity.MembersInput{
			Role:           roleAdmin,
			Permission:     "billing.write",
			IDs:            nil,
			OrganizationID: nil,
			ClientRole:     "",
		},
		inquiryID,
		inquiryReceived,
	)
	if notifyMembersErr != nil {
		return Inquiry{}, notifyMembersErr
	}

	recordCreate(call, inquiryID, input.SubjectType)

	return m.loadInquiry(ctx, call, inquiryID, false)
}

// checkSubject: the invoice and the restriction an inquiry is about are the customer's (NOT_FOUND otherwise), and a
// restriction cites the invoice.
func checkSubject(
	ctx context.Context,
	call *ops.Call,
	input *InquiryInput,
	customer uuid.UUID,
) error {
	if input.InvoiceID != nil {
		var owned bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.invoices WHERE id = $1 AND customer_id =
			$2)`, *input.InvoiceID, customer).
			Scan(&owned)
		if err != nil {
			return apperr.Wrap(err, "select billing.invoices")
		}

		if !owned {
			return apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	}

	if input.RestrictionID != nil {
		var owned, cites bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restrictions WHERE id = $1 AND customer_id =
			$2),
			$3::uuid IS NULL OR EXISTS (SELECT 1 FROM restrictions.restriction_invoices WHERE restriction_id = $1 AND invoice_id
			= $3)`, *input.RestrictionID, customer, input.InvoiceID).
			Scan(&owned, &cites)
		if err != nil {
			return apperr.Wrap(err, "select restrictions.restrictions")
		}

		if !owned {
			return apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		if !cites {
			return apperr.Fields(
				map[string]string{"invoiceId": "errors.not_a_cause_invoice"},
			)
		}
	}

	return nil
}

// recordCreate emits InquiryReceived and audits inquiries.create.
func recordCreate(call *ops.Call, inquiryID uuid.UUID, subjectType string) {
	call.Emit(
		ops.Event{
			AggregateType: kindInquiry,
			AggregateID:   inquiryID,
			Type:          "InquiryReceived",
			Payload:       map[string]any{"subjectType": subjectType},
		},
	)

	version := 1
	call.Audit(
		ops.AuditEntry{
			Action:          "inquiries.create",
			TargetKind:      kindInquiry,
			TargetID:        inquiryID.String(),
			NextVersion:     &version,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)
}

func (Billing) loadInquiry(
	ctx context.Context,
	call *ops.Call,
	inquiryID uuid.UUID,
	lock bool,
) (Inquiry, error) {
	stmt := "SELECT " + inquiryCols + " FROM billing.inquiries q JOIN billing.ref_customers cu ON cu.id = q.customer_id " +
		"WHERE q.id = $1"
	if lock {
		stmt += " FOR UPDATE OF q"
	}

	inquiry, err := scanInquiry(call.Tx.QueryRow(ctx, stmt, inquiryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return inquiry, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return inquiry, err
}

// listInquiries serves inquiries.list.
//
//	@Summary		inquiries.list (read)
//	@ID				inquiries.list
//	@Description	Authorization: client:self | admin:billing.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A08, DD-C12 · Query: filters invoiceId,restrictionId,status,subjectType,customerId · sort \
//	@Description	id,createdAt,updatedAt (default createdAt desc;id desc)
//	@Tags			inquiries
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,createdAt,updatedAt; default createdAt desc;id desc"
//	@Param			invoiceId		query		string	false	"filter → invoiceId"
//	@Param			restrictionId	query		string	false	"filter → restrictionId"
//	@Param			status			query		string	false	"filter → state"
//	@Param			subjectType		query		string	false	"filter → subjectType"
//	@Param			customerId		query		string	false	"filter → customerId"
//	@Success		200				{object}	ops.Envelope{data=InquiryPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/inquiries [get]
func (Billing) listInquiries(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Inquiry], error) {
	var filters inquiryFilters

	filterErr := filters.decode(query.Filters)
	if filterErr != nil {
		return paging.Page[Inquiry]{}, filterErr
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":           "q.id",
			fieldCreatedAt: "q.created_at",
			fieldUpdatedAt: "q.updated_at",
		},
		"q.created_at DESC, q.id DESC",
	)
	if err != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args := filters.conditions(call)
	from := "billing.inquiries q JOIN billing.ref_customers cu ON cu.id = q.customer_id"

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM "+from+" WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(scanErr, "select count")
	}

	items, err := inquiryPage(ctx, call, pageStmt(inquiryCols, from, where, order, window), args)
	if err != nil {
		return paging.Page[Inquiry]{}, err
	}

	return paging.Page[Inquiry]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// inquiryPage reads one page of inquiries.
func inquiryPage(ctx context.Context, call *ops.Call, stmt string, args []any) ([]Inquiry, error) {
	rows, err := call.Tx.Query(ctx, stmt, args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select (listInquiries)")
	}
	defer rows.Close()

	items := []Inquiry{}

	for rows.Next() {
		inquiry, err := scanInquiry(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, inquiry)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select (listInquiries)")
	}

	return items, nil
}

// inquiryFilters are the filters of inquiries.list.
type inquiryFilters struct {
	Status        *string    `json:"status,omitempty"` // => state
	SubjectType   *string    `json:"subjectType,omitempty"`
	InvoiceID     *uuid.UUID `json:"invoiceId,omitempty"`
	RestrictionID *uuid.UUID `json:"restrictionId,omitempty"`
	CustomerID    *uuid.UUID `json:"customerId,omitempty"`
}

// decode reads the filters: a known state and subject type.
func (f *inquiryFilters) decode(raw json.RawMessage) error {
	if decodeFilters(raw, f) != nil ||
		(f.Status != nil && !slices.Contains([]string{inquiryReceived, inquiryAnswered}, *f.Status)) ||
		(f.SubjectType != nil && !slices.Contains([]string{subjectPayment, subjectRestriction}, *f.SubjectType)) {
		return invalidFilters()
	}

	return nil
}

// conditions is the WHERE clause of inquiries.list and its arguments: a client sees its own organization's.
func (f *inquiryFilters) conditions(call *ops.Call) (string, []any) {
	var params placeholders

	conds := []string{clientScope(call, "cu.organization_id", &params.args)}

	for col, value := range map[string]any{
		"q.state": f.Status, "q.subject_type": f.SubjectType, "q.invoice_id": f.InvoiceID,
		"q.restriction_id": f.RestrictionID, "q.customer_id": f.CustomerID,
	} {
		switch filterValue := value.(type) {
		case *string:
			if filterValue != nil {
				conds = append(conds, col+" = "+params.add(*filterValue))
			}
		case *uuid.UUID:
			if filterValue != nil {
				conds = append(conds, col+" = "+params.add(*filterValue))
			}
		}
	}

	return strings.Join(conds, " AND "), params.args
}

// AnswerInput is inquiries.answer input.
type AnswerInput struct {
	InquiryID uuid.UUID `json:"inquiryId"`
	Reply     string    `json:"reply"`
}

// Validate implements ops.Validator.
func (in *AnswerInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.InquiryID == uuid.Nil {
		fieldErrs["inquiryId"] = apperr.KeyRequired
	}

	textLen(&in.Reply, "reply", maxMessageLength, fieldErrs)

	return fieldErrs
}

// answerInquiry serves inquiries.answer.
//
//	@Summary		inquiries.answer (write)
//	@ID				inquiries.answer
//	@Description	Authorization: admin:billing.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A08
//	@Tags			inquiries
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target inquiries, read inquiries.list)"
//	@Param			inquiryId			path		string		true	"input field inquiryId"
//	@Param			request				body		AnswerInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Inquiry}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/inquiries/{inquiryId}/answer [post]
func (m Billing) answerInquiry(
	ctx context.Context,
	call *ops.Call,
	input *AnswerInput,
) (Inquiry, error) {
	inquiry, err := m.loadInquiry(ctx, call, input.InquiryID, true)
	if err != nil {
		return inquiry, err
	}

	if inquiry.Version != *call.ExpectedVersion {
		return inquiry, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if inquiry.State != inquiryReceived {
		return inquiry, apperr.E(apperr.Conflict, "errors.inquiry_answered")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.inquiries SET state = 'answered', reply = $2, answered_by = $3, answered_at = $4, version = version +
			1, updated_at = $4 WHERE id = $1`,
		inquiry.ID,
		input.Reply,
		call.Principal.MembershipID,
		call.Now,
	)
	if execErr != nil {
		return inquiry, apperr.Wrap(execErr, "update billing.inquiries")
	}

	notifyMembersErr := m.notifyMembers(
		ctx,
		call,
		identity.MembersInput{
			Role:           roleClient,
			OrganizationID: &inquiry.customerOrg,
			IDs:            nil,
			Permission:     "",
			ClientRole:     "",
		},
		inquiry.ID,
		inquiryAnswered,
	)
	if notifyMembersErr != nil {
		return inquiry, notifyMembersErr
	}

	recordAnswer(call, inquiry)

	return m.loadInquiry(ctx, call, inquiry.ID, false)
}

// recordAnswer emits InquiryAnswered and audits inquiries.answer.
func recordAnswer(call *ops.Call, inquiry Inquiry) {
	next := inquiry.Version + 1
	call.Emit(
		ops.Event{
			AggregateType: kindInquiry,
			AggregateID:   inquiry.ID,
			Type:          "InquiryAnswered",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "inquiries.answer",
			TargetKind:      kindInquiry,
			TargetID:        inquiry.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next, Reason: "", Result: "", Before: nil, After: nil,
		},
	)
}

// RegisterInquiries binds inquiries.*.
func RegisterInquiries(r *ops.Registry, m Billing) {
	ops.Register(r, "inquiries.create", m.createInquiry)
	ops.Register(r, "inquiries.list", m.listInquiries)
	ops.Register(r, "inquiries.answer", m.answerInquiry)
}
