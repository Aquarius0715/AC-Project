package billing

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Releases starts restriction release requests when invoices are paid (Restrictions, IR35).
type Releases interface {
	OnInvoicePaid(ctx context.Context, c *ops.Call, invoice uuid.UUID) ([]uuid.UUID, error)
}

const paymentCols = `id, tenant_id, version, created_at, updated_at, amount_minor, currency, invoice_id, method, ` +
	`status, payment_reference, confirmed_at, confirmation_reason, event_ids`

func scanPayment(r pgx.Row) (Payment, error) {
	var payment Payment

	err := r.Scan(
		&payment.ID,
		&payment.TenantID,
		&payment.Version,
		&payment.CreatedAt,
		&payment.UpdatedAt,
		&payment.AmountMinor,
		&payment.Currency,
		&payment.InvoiceID,
		&payment.Method,
		&payment.Status,
		&payment.PaymentReference,
		&payment.ConfirmedAt,
		&payment.ConfirmationReason,
		&payment.EventIDs,
	)

	return payment, apperr.Wrap(err, "scan (scanPayment)")
}

// forCaller hides HQ's confirmation reason from non-HQ callers (DD-A08 shows it to HQ; IR199).
func forCaller(c *ops.Call, p Payment) Payment {
	if c.Principal.Role != "admin" {
		p.ConfirmationReason = nil
	}

	return p
}

func (m Billing) loadPayment(
	ctx context.Context,
	call *ops.Call,
	paymentID uuid.UUID,
) (Payment, Invoice, error) {
	payment, err := scanPayment(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+paymentCols+" FROM billing.payments WHERE id = $1 FOR UPDATE",
			paymentID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return payment, Invoice{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return payment, Invoice{}, err
	}

	inv, err := m.loadInvoice(ctx, call, payment.InvoiceID, true)

	return payment, inv, err
}

var paymentMethods = []string{"demo_credit_card", "demo_debit_card"}

func refOK(ref *string) bool {
	if ref == nil {
		return false
	}

	*ref = strings.TrimSpace(*ref)
	n := utf8.RuneCountInString(*ref)

	return n >= 1 && n <= 128
}

// referenceFree checks tenant-wide uniqueness of a payment reference (excluding one payment).
func referenceFree(ctx context.Context, c *ops.Call, ref string, except uuid.UUID) (bool, error) {
	var used bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.payments WHERE payment_reference = $1 AND id <>
		$2)`, ref, except).
		Scan(&used)

	return !used, apperr.Wrap(err, "select billing.payments")
}

// markPaid sets the invoice paid and triggers the IR35 release route.
func (m Billing) markPaid(ctx context.Context, call *ops.Call, invoice uuid.UUID) error {
	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.invoices SET status = 'paid', paid_at = $2, version = version + 1, updated_at = $2 WHERE id = $1`,
		invoice,
		call.Now,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update billing.invoices")
	}

	call.Emit(
		ops.Event{
			AggregateType: "invoice",
			AggregateID:   invoice,
			Type:          "InvoicePaid",
			Payload:       nil,
		},
	)

	if m.Releases == nil {
		return nil
	}

	_, err := m.Releases.OnInvoicePaid(ctx, call, invoice)

	return apperr.Wrap(err, "billing.Releases.OnInvoicePaid")
}

func (m Billing) paymentResult(
	ctx context.Context,
	call *ops.Call,
	paymentID uuid.UUID,
	action string,
	reason string,
) (Payment, error) {
	payment, err := scanPayment(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+paymentCols+" FROM billing.payments WHERE id = $1",
			paymentID,
		),
	)
	if err != nil {
		return payment, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "payment",
			AggregateID:   paymentID,
			Type:          "PaymentChanged",
			Payload:       map[string]any{"status": payment.Status, "invoiceId": payment.InvoiceID},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          action,
			TargetKind:      "payment",
			TargetID:        paymentID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &payment.Version,
			Reason:          reason, Result: "", Before: nil, After: nil,
		},
	)

	return forCaller(call, payment), nil
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
	fieldErrs := map[string]string{}

	switch in.Event {
	case "initiate", "instructions":
		if in.InvoiceID == nil || in.DemoConfirmed == nil || !*in.DemoConfirmed ||
			in.PaymentID != nil ||
			in.EventID != nil {
			fieldErrs["invoiceId"] = apperr.KeyRequired
		}

		if in.Event == "initiate" &&
			(in.Method == nil || !slices.Contains(paymentMethods, *in.Method)) {
			fieldErrs["method"] = apperr.KeyInvalid
		}

		if in.Event == "instructions" && in.Method != nil {
			fieldErrs["method"] = apperr.KeyNotAllowed
		}
	case "processing", "confirm", "fail":
		if in.PaymentID == nil || in.EventID == nil || in.InvoiceID != nil {
			fieldErrs["paymentId"] = apperr.KeyRequired
		}

		if !refOK(in.PaymentReference) {
			fieldErrs["paymentReference"] = apperr.KeyLength
		}
	default:
		fieldErrs["event"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// simulate serves payments.simulate.
//
//	@Summary		payments.simulate (write)
//	@ID				payments.simulate
//	@Description	Authorization: client:self:demo-event-only
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR59 single path for customer processing/confirm/fail; IR60 demo-only; initiate: one attempt in \
//	@Description	flight or the one confirmed payment per invoice (CONFLICT errors.invoice_has_payment; IR474)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C11
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"event=initiate|instructions: required (target invoice, read invoices.get); event=processing|confirm|fail: required (target payment, read invoices.get)"
//	@Param			request				body		SimulateInput	true	"input"
//	@Success		200					{object}	ops.Envelope
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payments/simulate [post]
func (m Billing) simulate(ctx context.Context, c *ops.Call, input *SimulateInput) (any, error) {
	if input.Event == "initiate" || input.Event == "instructions" {
		inv, err := m.loadInvoice(ctx, c, *input.InvoiceID, true)
		if err != nil {
			return nil, err
		}

		if inv.Version != *c.ExpectedVersion {
			return nil, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}

		if inv.Status != "unpaid" {
			return nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		if input.Event == "initiate" {
			// one attempt at a time, and a paid invoice is not paid again: full amount only (IR474)
			var open bool

			err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.payments WHERE invoice_id = $1 AND status IN
				('initiated','processing','confirmed'))`, inv.ID).
				Scan(&open)
			if err != nil {
				return nil, apperr.Wrap(err, "select billing.payments")
			}

			if open {
				return nil, apperr.E(apperr.Conflict, "errors.invoice_has_payment")
			}
		}

		if input.Event == "instructions" {
			return map[string]any{
				"id":                     uuid.Nil,
				"tenantId":               c.Principal.TenantID,
				"version":                1,
				"createdAt":              c.Now,
				"updatedAt":              c.Now,
				"sourceAlertId":          nil,
				"type":                   "payment",
				"recipientMembershipId":  c.Principal.MembershipID,
				"scopeVersionAtCreation": c.Principal.ScopeVersion,
				"target":                 map[string]any{"kind": "invoice", "id": inv.ID},
				"templateKey":            "payment",
				"params": map[string]any{
					"targetName":  inv.Number,
					"at":          inv.DueAt,
					"status":      inv.Status,
					"reason":      nil,
					"amountMinor": inv.AmountMinor,
					"currency":    inv.Currency,
					"method":      inv.PaymentMethod,
					"message":     nil,
				},
				"channel":       "inApp",
				"deliveryState": "preview",
				"severity":      "normal",
				"occurredAt":    c.Now,
				"readAt":        nil,
			}, nil
		}

		var paymentID uuid.UUID

		scanErr := c.Tx.QueryRow(ctx, `INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, method,
			channel, status, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'demo', 'initiated', $5, $5) RETURNING
			id`, inv.ID, inv.AmountMinor, inv.Currency, *input.Method, c.Now).
			Scan(&paymentID)
		if scanErr != nil {
			return nil, apperr.Wrap(scanErr, "insert billing.payments")
		}

		_, execErr := c.Tx.Exec(
			ctx,
			`UPDATE billing.invoices SET version = version + 1, updated_at = $2 WHERE id = $1`,
			inv.ID,
			c.Now,
		)
		if execErr != nil {
			return nil, apperr.Wrap(execErr, "update billing.invoices")
		}

		return m.paymentResult(ctx, c, paymentID, "payments.simulate", "initiate")
	}

	payment, inv, err := m.loadPayment(ctx, c, *input.PaymentID)
	if err != nil {
		return nil, err
	}

	if slices.Contains(payment.EventIDs, *input.EventID) {
		return forCaller(c, payment), nil // same event again
	}

	if payment.Version != *c.ExpectedVersion {
		return nil, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	next, invoiceStatus := "", ""

	switch input.Event {
	case "processing":
		if payment.Status != "initiated" {
			return nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		next, invoiceStatus = "processing", "processing"
	case "confirm":
		if payment.Status != "processing" {
			return nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		free, err := referenceFree(ctx, c, *input.PaymentReference, payment.ID)
		if err != nil {
			return nil, err
		}

		if !free {
			return nil, apperr.E(apperr.Conflict, "errors.reference_used")
		}

		next = "confirmed"
	case "fail":
		if payment.Status != "initiated" && payment.Status != "processing" {
			return nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		next, invoiceStatus = "failed", "unpaid"
	}

	_, execErr := c.Tx.Exec(
		ctx,
		`UPDATE billing.payments SET status = $2, payment_reference = COALESCE($3, payment_reference), confirmed_at = CASE
			WHEN $2 = 'confirmed' THEN $4::timestamptz END,
		event_ids = array_append(event_ids, $5), version = version + 1, updated_at = $4 WHERE id = $1`,
		payment.ID,
		next,
		refIf(next == "confirmed", input.PaymentReference),
		c.Now,
		*input.EventID,
	)
	if execErr != nil {
		return nil, apperr.Wrap(execErr, "update billing.payments")
	}

	if next == "confirmed" {
		err := m.markPaid(ctx, c, inv.ID)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := c.Tx.Exec(
			ctx,
			`UPDATE billing.invoices SET status = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
			inv.ID,
			invoiceStatus,
			c.Now,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "update billing.invoices")
		}
	}

	return m.paymentResult(ctx, c, payment.ID, "payments.simulate", input.Event)
}

func refIf(ok bool, s *string) *string {
	if ok {
		return s
	}

	return nil
}

// ---- payments.confirm / payments.recordManual ----

// PaymentEntry is what HQ enters for a payment it confirms or records: the bank reference, the full amount and why.
type PaymentEntry struct {
	PaymentReference     string `json:"paymentReference"`
	ConfirmedAmountMinor int64  `json:"confirmedAmountMinor"`
	Currency             string `json:"currency"`
	Reason               string `json:"reason"`
}

func (in *PaymentEntry) check(fieldErrs map[string]string) map[string]string {
	if !refOK(&in.PaymentReference) {
		fieldErrs["paymentReference"] = apperr.KeyLength
	}

	if in.Currency != "MYR" && in.Currency != "USD" {
		fieldErrs["currency"] = apperr.KeyInvalid
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs["reason"] = apperr.KeyLength
	}

	return fieldErrs
}

// ConfirmInput is the input of payments.confirm: a payment that is processing. It has no invoiceId: the contract's
// input has none, so one is an unknown field (IR390).
type ConfirmInput struct {
	PaymentID uuid.UUID `json:"paymentId"`
	PaymentEntry
}

// Validate implements ops.Validator.
func (in *ConfirmInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.PaymentID == uuid.Nil {
		fe["paymentId"] = apperr.KeyRequired
	}

	return in.check(fe)
}

// ManualInput is the input of payments.recordManual: a payment HQ received outside the payment page, for an invoice.
type ManualInput struct {
	InvoiceID uuid.UUID `json:"invoiceId"`
	PaymentEntry
}

// Validate implements ops.Validator.
func (in *ManualInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.InvoiceID == uuid.Nil {
		fe["invoiceId"] = apperr.KeyRequired
	}

	return in.check(fe)
}

func amountOK(in PaymentEntry, inv Invoice) error {
	if in.ConfirmedAmountMinor != inv.AmountMinor || in.Currency != inv.Currency {
		return apperr.Fields(map[string]string{"confirmedAmountMinor": "errors.full_amount_only"})
	}

	return nil
}

// confirm serves payments.confirm.
//
//	@Summary		payments.confirm (write)
//	@ID				payments.confirm
//	@Description	Authorization: admin:billing.payment
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A08
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target payment, read invoices.get)"
//	@Param			request				body		ConfirmInput	true	"input"
//	@Success		200					{object}	ops.Envelope{data=Payment}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payments/confirm [post]
func (m Billing) confirm(
	ctx context.Context,
	call *ops.Call,
	input *ConfirmInput,
) (Payment, error) {
	payment, inv, err := m.loadPayment(ctx, call, input.PaymentID)
	if err != nil {
		return payment, err
	}

	if payment.Version != *call.ExpectedVersion {
		return payment, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if payment.Status != "processing" {
		return payment, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	amountOKErr := amountOK(input.PaymentEntry, inv)
	if amountOKErr != nil {
		return payment, amountOKErr
	}

	free, referenceFreeErr := referenceFree(ctx, call, input.PaymentReference, payment.ID)
	if referenceFreeErr != nil || !free {
		if referenceFreeErr != nil {
			return payment, referenceFreeErr
		}

		return payment, apperr.E(apperr.Conflict, "errors.reference_used")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.payments SET status = 'confirmed', payment_reference = $2, confirmed_at = $3, confirmation_reason =
			$4, recorded_by_membership_id = $5,
		version = version + 1, updated_at = $3 WHERE id = $1`,
		payment.ID,
		input.PaymentReference,
		call.Now,
		input.Reason,
		call.Principal.MembershipID,
	)
	if execErr != nil {
		return payment, apperr.Wrap(execErr, "update billing.payments")
	}

	markPaidErr := m.markPaid(ctx, call, inv.ID)
	if markPaidErr != nil {
		return payment, markPaidErr
	}

	return m.paymentResult(ctx, call, payment.ID, "payments.confirm", input.Reason)
}

// recordManual serves payments.recordManual.
//
//	@Summary		payments.recordManual (write)
//	@ID				payments.recordManual
//	@Description	Authorization: admin:billing.payment
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A08
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target invoice, read invoices.get)"
//	@Param			request				body		ManualInput	true	"input"
//	@Success		200					{object}	ops.Envelope{data=Payment}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payments/manual [post]
func (m Billing) recordManual(
	ctx context.Context,
	call *ops.Call,
	input *ManualInput,
) (Payment, error) {
	inv, err := m.loadInvoice(ctx, call, input.InvoiceID, true)
	if err != nil {
		return Payment{}, err
	}
	// the same invoice / reference / amount again returns the existing success
	existing, err := scanPayment(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+paymentCols+" FROM billing.payments WHERE invoice_id = $1 AND status = 'confirmed' AND "+
				"payment_reference = $2 AND amount_minor = $3",
			inv.ID,
			input.PaymentReference,
			input.ConfirmedAmountMinor,
		),
	)
	if err == nil {
		return existing, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, err
	}

	if inv.Version != *call.ExpectedVersion {
		return Payment{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	var any bool
	// a failed or expired attempt does not stop it: the customer may have paid another way (IR447); one still in flight
	// does
	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.payments WHERE invoice_id = $1 AND status IN
		('initiated','processing','confirmed'))`, inv.ID).
		Scan(&any)
	if scanErr != nil {
		return Payment{}, apperr.Wrap(scanErr, "select billing.payments")
	}

	if any || inv.Status == "paid" {
		return Payment{}, apperr.E(apperr.Conflict, "errors.invoice_has_payment")
	}

	amountOKErr := amountOK(input.PaymentEntry, inv)
	if amountOKErr != nil {
		return Payment{}, amountOKErr
	}

	free, referenceFreeErr := referenceFree(ctx, call, input.PaymentReference, uuid.Nil)
	if referenceFreeErr != nil || !free {
		if referenceFreeErr != nil {
			return Payment{}, referenceFreeErr
		}

		return Payment{}, apperr.E(apperr.Conflict, "errors.reference_used")
	}

	var paymentID uuid.UUID

	scanErr = call.Tx.QueryRow(ctx, `INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, method,
		channel, status, payment_reference, recorded_by_membership_id,
		confirmed_at, confirmation_reason, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3,
		NULL, 'manual', 'confirmed', $4, $5, $6, $7, $6, $6) RETURNING id`,
		inv.ID, inv.AmountMinor, inv.Currency, input.PaymentReference, call.Principal.MembershipID, call.Now, input.Reason).
		Scan(&paymentID)
	if scanErr != nil {
		return Payment{}, apperr.Wrap(scanErr, "insert billing.payments")
	}

	markPaidErr := m.markPaid(ctx, call, inv.ID)
	if markPaidErr != nil {
		return Payment{}, markPaidErr
	}

	return m.paymentResult(ctx, call, paymentID, "payments.recordManual", input.Reason)
}

// RegisterPayments binds the payment operations.
func RegisterPayments(r *ops.Registry, m Billing) {
	ops.Register(r, "payments.simulate", m.simulate)
	ops.Register(r, "payments.confirm", m.confirm)
	ops.Register(r, "payments.recordManual", m.recordManual)
}
