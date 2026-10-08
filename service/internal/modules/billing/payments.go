package billing

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

// Releases starts restriction release requests when invoices are paid (Restrictions, IR35).
type Releases interface {
	OnInvoicePaid(ctx context.Context, c *ops.Call, invoice uuid.UUID) ([]uuid.UUID, error)
}

const paymentCols = `id, tenant_id, version, created_at, updated_at, amount_minor, currency, invoice_id, method, status, payment_reference, confirmed_at, event_ids`

func scanPayment(r pgx.Row) (Payment, error) {
	var p Payment
	err := r.Scan(&p.ID, &p.TenantID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.AmountMinor, &p.Currency, &p.InvoiceID, &p.Method, &p.Status, &p.PaymentReference, &p.ConfirmedAt, &p.EventIDs)
	return p, err
}

func (m Billing) loadPayment(ctx context.Context, c *ops.Call, id uuid.UUID) (Payment, Invoice, error) {
	p, err := scanPayment(c.Tx.QueryRow(ctx, "SELECT "+paymentCols+" FROM billing.payments WHERE id = $1 FOR UPDATE", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, Invoice{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return p, Invoice{}, err
	}
	inv, err := m.loadInvoice(ctx, c, p.InvoiceID, true)
	return p, inv, err
}

var paymentMethods = []string{"demo_credit_card", "demo_debit_card"}

func refOK(s *string) bool {
	if s == nil {
		return false
	}
	*s = strings.TrimSpace(*s)
	n := utf8.RuneCountInString(*s)
	return n >= 1 && n <= 128
}

// referenceFree checks tenant-wide uniqueness of a payment reference (excluding one payment).
func referenceFree(ctx context.Context, c *ops.Call, ref string, except uuid.UUID) (bool, error) {
	var used bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.payments WHERE payment_reference = $1 AND id <> $2)`, ref, except).Scan(&used)
	return !used, err
}

// markPaid sets the invoice paid and triggers the IR35 release route.
func (m Billing) markPaid(ctx context.Context, c *ops.Call, invoice uuid.UUID) error {
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.invoices SET status = 'paid', paid_at = $2, version = version + 1, updated_at = $2 WHERE id = $1`, invoice, c.Now); err != nil {
		return err
	}
	c.Emit(ops.Event{AggregateType: "invoice", AggregateID: invoice, Type: "InvoicePaid"})
	if m.Releases == nil {
		return nil
	}
	_, err := m.Releases.OnInvoicePaid(ctx, c, invoice)
	return err
}

func (m Billing) paymentResult(ctx context.Context, c *ops.Call, id uuid.UUID, action string, reason string) (Payment, error) {
	p, err := scanPayment(c.Tx.QueryRow(ctx, "SELECT "+paymentCols+" FROM billing.payments WHERE id = $1", id))
	if err != nil {
		return p, err
	}
	c.Emit(ops.Event{AggregateType: "payment", AggregateID: id, Type: "PaymentChanged", Payload: map[string]any{"status": p.Status, "invoiceId": p.InvoiceID}})
	c.Audit(ops.AuditEntry{Action: action, TargetKind: "payment", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &p.Version, Reason: reason})
	return p, nil
}

// ---- payments.simulate ----

// SimulateInput is the payments.simulate union.
type SimulateInput struct {
	Event            string     `json:"event"`
	InvoiceID        *uuid.UUID `json:"invoiceId,omitempty"`
	Method           *string    `json:"method,omitempty"`
	DemoConfirmed    *bool      `json:"demoConfirmed,omitempty"`
	PaymentID        *uuid.UUID `json:"paymentId,omitempty"`
	EventID          *uuid.UUID `json:"eventId,omitempty"`
	PaymentReference *string    `json:"paymentReference,omitempty"`
}

// Validate implements ops.Validator.
func (in *SimulateInput) Validate() map[string]string {
	fe := map[string]string{}
	switch in.Event {
	case "initiate", "instructions":
		if in.InvoiceID == nil || in.DemoConfirmed == nil || !*in.DemoConfirmed || in.PaymentID != nil || in.EventID != nil {
			fe["invoiceId"] = "error.required"
		}
		if in.Event == "initiate" && (in.Method == nil || !slices.Contains(paymentMethods, *in.Method)) {
			fe["method"] = "error.invalid"
		}
		if in.Event == "instructions" && in.Method != nil {
			fe["method"] = "error.notAllowed"
		}
	case "processing", "confirm", "fail":
		if in.PaymentID == nil || in.EventID == nil || in.InvoiceID != nil {
			fe["paymentId"] = "error.required"
		}
		if !refOK(in.PaymentReference) {
			fe["paymentReference"] = "error.length"
		}
	default:
		fe["event"] = "error.invalid"
	}
	return fe
}

func (m Billing) simulate(ctx context.Context, c *ops.Call, in *SimulateInput) (any, error) {
	if in.Event == "initiate" || in.Event == "instructions" {
		inv, err := m.loadInvoice(ctx, c, *in.InvoiceID, true)
		if err != nil {
			return nil, err
		}
		if inv.Version != *c.ExpectedVersion {
			return nil, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		if inv.Status != "unpaid" {
			return nil, apperr.E(apperr.Conflict, "error.invalidState")
		}
		if in.Event == "instructions" {
			return map[string]any{"id": uuid.Nil, "tenantId": c.Principal.TenantID, "version": 1, "createdAt": c.Now, "updatedAt": c.Now, "sourceAlertId": nil,
				"type": "payment", "recipientMembershipId": c.Principal.MembershipID, "scopeVersionAtCreation": c.Principal.ScopeVersion,
				"target": map[string]any{"kind": "invoice", "id": inv.ID}, "templateKey": "payment",
				"params":  map[string]any{"targetName": inv.Number, "at": inv.DueAt, "status": inv.Status, "reason": nil, "amountMinor": inv.AmountMinor, "currency": inv.Currency, "method": inv.PaymentMethod, "message": nil},
				"channel": "inApp", "deliveryState": "preview", "severity": "normal", "occurredAt": c.Now, "readAt": nil}, nil
		}
		var id uuid.UUID
		if err := c.Tx.QueryRow(ctx, `INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, method, channel, status, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'demo', 'initiated', $5, $5) RETURNING id`, inv.ID, inv.AmountMinor, inv.Currency, *in.Method, c.Now).Scan(&id); err != nil {
			return nil, err
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE billing.invoices SET version = version + 1, updated_at = $2 WHERE id = $1`, inv.ID, c.Now); err != nil {
			return nil, err
		}
		return m.paymentResult(ctx, c, id, "payments.simulate", "initiate")
	}
	p, inv, err := m.loadPayment(ctx, c, *in.PaymentID)
	if err != nil {
		return nil, err
	}
	if slices.Contains(p.EventIDs, *in.EventID) {
		return p, nil // same event again
	}
	if p.Version != *c.ExpectedVersion {
		return nil, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	next, invoiceStatus := "", ""
	switch in.Event {
	case "processing":
		if p.Status != "initiated" {
			return nil, apperr.E(apperr.Conflict, "error.invalidState")
		}
		next, invoiceStatus = "processing", "processing"
	case "confirm":
		if p.Status != "processing" {
			return nil, apperr.E(apperr.Conflict, "error.invalidState")
		}
		free, err := referenceFree(ctx, c, *in.PaymentReference, p.ID)
		if err != nil {
			return nil, err
		}
		if !free {
			return nil, apperr.E(apperr.Conflict, "errors.reference_used")
		}
		next = "confirmed"
	case "fail":
		if p.Status != "initiated" && p.Status != "processing" {
			return nil, apperr.E(apperr.Conflict, "error.invalidState")
		}
		next, invoiceStatus = "failed", "unpaid"
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.payments SET status = $2, payment_reference = COALESCE($3, payment_reference), confirmed_at = CASE WHEN $2 = 'confirmed' THEN $4::timestamptz END,
		event_ids = array_append(event_ids, $5), version = version + 1, updated_at = $4 WHERE id = $1`, p.ID, next, refIf(next == "confirmed", in.PaymentReference), c.Now, *in.EventID); err != nil {
		return nil, err
	}
	if next == "confirmed" {
		if err := m.markPaid(ctx, c, inv.ID); err != nil {
			return nil, err
		}
	} else if _, err := c.Tx.Exec(ctx, `UPDATE billing.invoices SET status = $2, version = version + 1, updated_at = $3 WHERE id = $1`, inv.ID, invoiceStatus, c.Now); err != nil {
		return nil, err
	}
	return m.paymentResult(ctx, c, p.ID, "payments.simulate", in.Event)
}

func refIf(ok bool, s *string) *string {
	if ok {
		return s
	}
	return nil
}

// ---- payments.confirm / payments.recordManual ----

// ConfirmInput is the input of payments.confirm (paymentId) and payments.recordManual (invoiceId).
type ConfirmInput struct {
	PaymentID            *uuid.UUID `json:"paymentId,omitempty"`
	InvoiceID            *uuid.UUID `json:"invoiceId,omitempty"`
	PaymentReference     string     `json:"paymentReference"`
	ConfirmedAmountMinor int64      `json:"confirmedAmountMinor"`
	Currency             string     `json:"currency"`
	Reason               string     `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ConfirmInput) Validate() map[string]string {
	fe := map[string]string{}
	if (in.PaymentID == nil) == (in.InvoiceID == nil) {
		fe["paymentId"] = "error.required"
	}
	if !refOK(&in.PaymentReference) {
		fe["paymentReference"] = "error.length"
	}
	if in.Currency != "MYR" && in.Currency != "USD" {
		fe["currency"] = "error.invalid"
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fe["reason"] = "error.length"
	}
	return fe
}

func amountOK(in *ConfirmInput, inv Invoice) error {
	if in.ConfirmedAmountMinor != inv.AmountMinor || in.Currency != inv.Currency {
		return apperr.Fields(map[string]string{"confirmedAmountMinor": "errors.full_amount_only"})
	}
	return nil
}

func (m Billing) confirm(ctx context.Context, c *ops.Call, in *ConfirmInput) (Payment, error) {
	if in.PaymentID == nil {
		return Payment{}, apperr.Fields(map[string]string{"paymentId": "error.required"})
	}
	p, inv, err := m.loadPayment(ctx, c, *in.PaymentID)
	if err != nil {
		return p, err
	}
	if p.Version != *c.ExpectedVersion {
		return p, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if p.Status != "processing" {
		return p, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if err := amountOK(in, inv); err != nil {
		return p, err
	}
	if free, err := referenceFree(ctx, c, in.PaymentReference, p.ID); err != nil || !free {
		if err != nil {
			return p, err
		}
		return p, apperr.E(apperr.Conflict, "errors.reference_used")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE billing.payments SET status = 'confirmed', payment_reference = $2, confirmed_at = $3, confirmation_reason = $4, recorded_by_membership_id = $5,
		version = version + 1, updated_at = $3 WHERE id = $1`, p.ID, in.PaymentReference, c.Now, in.Reason, c.Principal.MembershipID); err != nil {
		return p, err
	}
	if err := m.markPaid(ctx, c, inv.ID); err != nil {
		return p, err
	}
	return m.paymentResult(ctx, c, p.ID, "payments.confirm", in.Reason)
}

func (m Billing) recordManual(ctx context.Context, c *ops.Call, in *ConfirmInput) (Payment, error) {
	if in.InvoiceID == nil {
		return Payment{}, apperr.Fields(map[string]string{"invoiceId": "error.required"})
	}
	inv, err := m.loadInvoice(ctx, c, *in.InvoiceID, true)
	if err != nil {
		return Payment{}, err
	}
	// the same invoice / reference / amount again returns the existing success
	existing, err := scanPayment(c.Tx.QueryRow(ctx, "SELECT "+paymentCols+" FROM billing.payments WHERE invoice_id = $1 AND status = 'confirmed' AND payment_reference = $2 AND amount_minor = $3",
		inv.ID, in.PaymentReference, in.ConfirmedAmountMinor))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, err
	}
	if inv.Version != *c.ExpectedVersion {
		return Payment{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	var any bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.payments WHERE invoice_id = $1)`, inv.ID).Scan(&any); err != nil {
		return Payment{}, err
	}
	if any || inv.Status == "paid" {
		return Payment{}, apperr.E(apperr.Conflict, "errors.invoice_has_payment")
	}
	if err := amountOK(in, inv); err != nil {
		return Payment{}, err
	}
	if free, err := referenceFree(ctx, c, in.PaymentReference, uuid.Nil); err != nil || !free {
		if err != nil {
			return Payment{}, err
		}
		return Payment{}, apperr.E(apperr.Conflict, "errors.reference_used")
	}
	var id uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, method, channel, status, payment_reference, recorded_by_membership_id,
		confirmed_at, confirmation_reason, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, NULL, 'manual', 'confirmed', $4, $5, $6, $7, $6, $6) RETURNING id`,
		inv.ID, inv.AmountMinor, inv.Currency, in.PaymentReference, c.Principal.MembershipID, c.Now, in.Reason).Scan(&id); err != nil {
		return Payment{}, err
	}
	if err := m.markPaid(ctx, c, inv.ID); err != nil {
		return Payment{}, err
	}
	return m.paymentResult(ctx, c, id, "payments.recordManual", in.Reason)
}

// RegisterPayments binds the payment operations.
func RegisterPayments(r *ops.Registry, m Billing) {
	ops.Register(r, "payments.simulate", m.simulate)
	ops.Register(r, "payments.confirm", m.confirm)
	ops.Register(r, "payments.recordManual", m.recordManual)
}
