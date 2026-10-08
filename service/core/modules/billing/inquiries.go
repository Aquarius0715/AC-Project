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

	"github.com/pradita/ac-project/service/core/modules/notify"
	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
	"github.com/pradita/ac-project/service/core/platform/paging"
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

const inquiryCols = `q.id, q.tenant_id, q.version, q.created_at, q.updated_at, q.customer_id, q.invoice_id, q.restriction_id, q.subject_type, q.message, q.state, q.reply, cu.organization_id`

func scanInquiry(r pgx.Row) (Inquiry, error) {
	var x Inquiry
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.CustomerID, &x.InvoiceID, &x.RestrictionID, &x.SubjectType, &x.Message, &x.State, &x.Reply, &x.customerOrg)
	return x, err
}

func textLen(s *string, key string, max int, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > max {
		fe[key] = "error.length"
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
	fe := map[string]string{}
	switch in.SubjectType {
	case "payment":
		if in.InvoiceID == nil {
			fe["invoiceId"] = "error.required"
		}
		if in.RestrictionID != nil {
			fe["restrictionId"] = "error.notAllowed"
		}
	case "restriction":
		if in.RestrictionID == nil {
			fe["restrictionId"] = "error.required"
		}
	default:
		fe["subjectType"] = "error.invalid"
	}
	textLen(&in.Message, "message", 2000, fe)
	return fe
}

// notifyMembers stores one inquiry notification for each membership (IR95 inquiry.received / answered).
func (m Billing) notifyMembers(ctx context.Context, c *ops.Call, sql string, arg any, inquiry uuid.UUID, status string) error {
	rows, err := c.Tx.Query(ctx, sql, arg, c.Now, c.Principal.MembershipID)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := m.Notify.Create(ctx, c, notify.New{RecipientMembershipID: id, Type: "inquiry", TemplateKey: "inquiry", TargetKind: "inquiry", TargetID: inquiry,
			Params: map[string]any{"status": status}}); err != nil {
			return err
		}
	}
	return nil
}

const activeSQL = ` AND m.valid_from <= $2 AND (m.valid_until IS NULL OR m.valid_until > $2) AND m.id <> $3`

func (m Billing) createInquiry(ctx context.Context, c *ops.Call, in *InquiryInput) (Inquiry, error) {
	customer, ok, err := m.Customers.CustomerOfOrg(ctx, c, c.Principal.OrgID)
	if err != nil {
		return Inquiry{}, err
	}
	nf := apperr.E(apperr.NotFound, "error.notFound")
	if !ok {
		return Inquiry{}, nf
	}
	if in.InvoiceID != nil {
		var owned bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.invoices WHERE id = $1 AND customer_id = $2)`, *in.InvoiceID, customer).Scan(&owned); err != nil {
			return Inquiry{}, err
		}
		if !owned {
			return Inquiry{}, nf
		}
	}
	if in.RestrictionID != nil {
		var owned, cites bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restrictions WHERE id = $1 AND customer_id = $2),
			$3::uuid IS NULL OR EXISTS (SELECT 1 FROM restrictions.restriction_invoices WHERE restriction_id = $1 AND invoice_id = $3)`, *in.RestrictionID, customer, in.InvoiceID).Scan(&owned, &cites); err != nil {
			return Inquiry{}, err
		}
		if !owned {
			return Inquiry{}, nf
		}
		if !cites {
			return Inquiry{}, apperr.Fields(map[string]string{"invoiceId": "errors.not_a_cause_invoice"})
		}
	}
	id := uuid.New()
	if _, err := c.Tx.Exec(ctx, `INSERT INTO billing.inquiries (id, tenant_id, customer_id, invoice_id, restriction_id, subject_type, message, state, created_by, created_at, updated_at)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'received', $7, $8, $8)`, id, customer, in.InvoiceID, in.RestrictionID, in.SubjectType, in.Message,
		c.Principal.MembershipID, c.Now); err != nil {
		return Inquiry{}, err
	}
	if err := m.notifyMembers(ctx, c, `SELECT m.id FROM identity.memberships m WHERE m.role = 'admin' AND EXISTS (SELECT 1 FROM identity.membership_permissions p
		WHERE p.membership_id = m.id AND p.permission = $1)`+activeSQL+` ORDER BY m.id`, "billing.write", id, "received"); err != nil {
		return Inquiry{}, err
	}
	c.Emit(ops.Event{AggregateType: "inquiry", AggregateID: id, Type: "InquiryReceived", Payload: map[string]any{"subjectType": in.SubjectType}})
	v := 1
	c.Audit(ops.AuditEntry{Action: "inquiries.create", TargetKind: "inquiry", TargetID: id.String(), NextVersion: &v})
	return m.loadInquiry(ctx, c, id, false)
}

func (m Billing) loadInquiry(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Inquiry, error) {
	q := "SELECT " + inquiryCols + " FROM billing.inquiries q JOIN assets.customers cu ON cu.id = q.customer_id WHERE q.id = $1"
	if lock {
		q += " FOR UPDATE OF q"
	}
	x, err := scanInquiry(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, err
}

func (m Billing) listInquiries(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Inquiry], error) {
	var f struct {
		State         *string    `json:"state,omitempty"`
		SubjectType   *string    `json:"subjectType,omitempty"`
		InvoiceID     *uuid.UUID `json:"invoiceId,omitempty"`
		RestrictionID *uuid.UUID `json:"restrictionId,omitempty"`
		CustomerID    *uuid.UUID `json:"customerId,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.State != nil && !slices.Contains([]string{"received", "answered"}, *f.State)) ||
			(f.SubjectType != nil && !slices.Contains([]string{"payment", "restriction"}, *f.SubjectType)) {
			return paging.Page[Inquiry]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "q.id", "createdAt": "q.created_at", "updatedAt": "q.updated_at"}, "q.created_at DESC, q.id DESC")
	if err != nil {
		return paging.Page[Inquiry]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Inquiry]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{clientScope(c, "cu.organization_id", &args)}
	for col, v := range map[string]any{"q.state": f.State, "q.subject_type": f.SubjectType, "q.invoice_id": f.InvoiceID, "q.restriction_id": f.RestrictionID, "q.customer_id": f.CustomerID} {
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
	from := "billing.inquiries q JOIN assets.customers cu ON cu.id = q.customer_id"
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM "+from+" WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Inquiry]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d", inquiryCols, from, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Inquiry]{}, err
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
	if err := rows.Err(); err != nil {
		return paging.Page[Inquiry]{}, err
	}
	return paging.Page[Inquiry]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// AnswerInput is inquiries.answer input.
type AnswerInput struct {
	InquiryID uuid.UUID `json:"inquiryId"`
	Reply     string    `json:"reply"`
}

// Validate implements ops.Validator.
func (in *AnswerInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.InquiryID == uuid.Nil {
		fe["inquiryId"] = "error.required"
	}
	textLen(&in.Reply, "reply", 2000, fe)
	return fe
}

func (m Billing) answerInquiry(ctx context.Context, c *ops.Call, in *AnswerInput) (Inquiry, error) {
	x, err := m.loadInquiry(ctx, c, in.InquiryID, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.State != "received" {
		return x, apperr.E(apperr.Conflict, "errors.inquiry_answered")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.inquiries SET state = 'answered', reply = $2, answered_by = $3, answered_at = $4, version = version + 1, updated_at = $4 WHERE id = $1`,
		x.ID, in.Reply, c.Principal.MembershipID, c.Now); err != nil {
		return x, err
	}
	if err := m.notifyMembers(ctx, c, `SELECT m.id FROM identity.memberships m WHERE m.role = 'client' AND m.organization_id = $1`+activeSQL+` ORDER BY m.id`,
		x.customerOrg, x.ID, "answered"); err != nil {
		return x, err
	}
	next := x.Version + 1
	c.Emit(ops.Event{AggregateType: "inquiry", AggregateID: x.ID, Type: "InquiryAnswered"})
	c.Audit(ops.AuditEntry{Action: "inquiries.answer", TargetKind: "inquiry", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next})
	return m.loadInquiry(ctx, c, x.ID, false)
}

// RegisterInquiries binds inquiries.*.
func RegisterInquiries(r *ops.Registry, m Billing) {
	ops.Register(r, "inquiries.create", m.createInquiry)
	ops.Register(r, "inquiries.list", m.listInquiries)
	ops.Register(r, "inquiries.answer", m.answerInquiry)
}
