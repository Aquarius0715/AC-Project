package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func textLen(s *string, key string, max int, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > max {
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
	case "payment":
		if in.InvoiceID == nil {
			fieldErrs["invoiceId"] = apperr.KeyRequired
		}

		if in.RestrictionID != nil {
			fieldErrs["restrictionId"] = apperr.KeyNotAllowed
		}
	case "restriction":
		if in.RestrictionID == nil {
			fieldErrs["restrictionId"] = apperr.KeyRequired
		}
	default:
		fieldErrs["subjectType"] = apperr.KeyInvalid
	}

	textLen(&in.Message, "message", 2000, fieldErrs)

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
			Type:                  "inquiry",
			TemplateKey:           "inquiry",
			TargetKind:            "inquiry",
			TargetID:              inquiry,
			Params: map[string]any{
				"status": status,
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

	notFound := apperr.E(apperr.NotFound, apperr.KeyNotFound)
	if !found {
		return Inquiry{}, notFound
	}

	if input.InvoiceID != nil {
		var owned bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.invoices WHERE id = $1 AND customer_id =
			$2)`, *input.InvoiceID, customer).
			Scan(&owned)
		if err != nil {
			return Inquiry{}, apperr.Wrap(err, "select billing.invoices")
		}

		if !owned {
			return Inquiry{}, notFound
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
			return Inquiry{}, apperr.Wrap(err, "select restrictions.restrictions")
		}

		if !owned {
			return Inquiry{}, notFound
		}

		if !cites {
			return Inquiry{}, apperr.Fields(
				map[string]string{"invoiceId": "errors.not_a_cause_invoice"},
			)
		}
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
			Role:           "admin",
			Permission:     "billing.write",
			IDs:            nil,
			OrganizationID: nil,
			ClientRole:     "",
		},
		inquiryID,
		"received",
	)
	if notifyMembersErr != nil {
		return Inquiry{}, notifyMembersErr
	}

	call.Emit(
		ops.Event{
			AggregateType: "inquiry",
			AggregateID:   inquiryID,
			Type:          "InquiryReceived",
			Payload:       map[string]any{"subjectType": input.SubjectType},
		},
	)

	version := 1
	call.Audit(
		ops.AuditEntry{
			Action:          "inquiries.create",
			TargetKind:      "inquiry",
			TargetID:        inquiryID.String(),
			NextVersion:     &version,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return m.loadInquiry(ctx, call, inquiryID, false)
}

func (m Billing) loadInquiry(
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

	x, err := scanInquiry(call.Tx.QueryRow(ctx, stmt, inquiryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return x, err
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
func (m Billing) listInquiries(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Inquiry], error) {
	var filters struct {
		Status        *string    `json:"status,omitempty"` // => state
		SubjectType   *string    `json:"subjectType,omitempty"`
		InvoiceID     *uuid.UUID `json:"invoiceId,omitempty"`
		RestrictionID *uuid.UUID `json:"restrictionId,omitempty"`
		CustomerID    *uuid.UUID `json:"customerId,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil ||
			(filters.Status != nil && !slices.Contains([]string{"received", "answered"}, *filters.Status)) ||
			(filters.SubjectType != nil && !slices.Contains([]string{"payment", "restriction"}, *filters.SubjectType)) {
			return paging.Page[Inquiry]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "q.id", "createdAt": "q.created_at", "updatedAt": "q.updated_at"},
		"q.created_at DESC, q.id DESC",
	)
	if err != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}
	conds := []string{clientScope(call, "cu.organization_id", &args)}

	for col, v := range map[string]any{
		"q.state": filters.Status, "q.subject_type": filters.SubjectType, "q.invoice_id": filters.InvoiceID,
		"q.restriction_id": filters.RestrictionID, "q.customer_id": filters.CustomerID,
	} {
		switch x := v.(type) {
		case *string:
			if x != nil {
				conds = append(conds, col+" = "+add(*x))
			}
		case *uuid.UUID:
			if x != nil {
				conds = append(conds, col+" = "+add(*x))
			}
		}
	}

	where := strings.Join(conds, " AND ")
	from := "billing.inquiries q JOIN billing.ref_customers cu ON cu.id = q.customer_id"

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM "+from+" WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(scanErr, "select count")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			inquiryCols,
			from,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(err, "select (listInquiries)")
	}
	defer rows.Close()

	items := []Inquiry{}

	for rows.Next() {
		x, err := scanInquiry(rows)
		if err != nil {
			return paging.Page[Inquiry]{}, err
		}

		items = append(items, x)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Inquiry]{}, apperr.Wrap(rowsErr, "select (listInquiries)")
	}

	return paging.Page[Inquiry]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
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

	textLen(&in.Reply, "reply", 2000, fieldErrs)

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

	if inquiry.State != "received" {
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
			Role:           "client",
			OrganizationID: &inquiry.customerOrg,
			IDs:            nil,
			Permission:     "",
			ClientRole:     "",
		},
		inquiry.ID,
		"answered",
	)
	if notifyMembersErr != nil {
		return inquiry, notifyMembersErr
	}

	next := inquiry.Version + 1
	call.Emit(
		ops.Event{
			AggregateType: "inquiry",
			AggregateID:   inquiry.ID,
			Type:          "InquiryAnswered",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "inquiries.answer",
			TargetKind:      "inquiry",
			TargetID:        inquiry.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return m.loadInquiry(ctx, call, inquiry.ID, false)
}

// RegisterInquiries binds inquiries.*.
func RegisterInquiries(r *ops.Registry, m Billing) {
	ops.Register(r, "inquiries.create", m.createInquiry)
	ops.Register(r, "inquiries.list", m.listInquiries)
	ops.Register(r, "inquiries.answer", m.answerInquiry)
}
