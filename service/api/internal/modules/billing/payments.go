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

// The payment statuses (billing.payments.status) and the events of payments.simulate.
const (
	paymentInitiated  = "initiated"
	paymentProcessing = "processing"
	paymentConfirmed  = "confirmed"
	paymentFailed     = "failed"

	simulateInitiate     = "initiate"
	simulateInstructions = "instructions"
	simulateProcessing   = "processing"
	simulateConfirm      = "confirm"
	simulateFail         = "fail"
)

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
	if c.Principal.Role != roleAdmin {
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

// paymentMethods are the demo payment methods of payments.simulate.
func paymentMethods() []string {
	return []string{"demo_credit_card", "demo_debit_card"}
}

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

// referenceAvailable is CONFLICT errors.reference_used when another payment of the tenant has the reference.
func referenceAvailable(ctx context.Context, c *ops.Call, ref string, except uuid.UUID) error {
	free, err := referenceFree(ctx, c, ref, except)
	if err != nil {
		return err
	}

	if !free {
		return apperr.E(apperr.Conflict, "errors.reference_used")
	}

	return nil
}

// paymentOpen reports a payment of the invoice in flight or confirmed.
func paymentOpen(ctx context.Context, c *ops.Call, invoice uuid.UUID) (bool, error) {
	var open bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.payments WHERE invoice_id = $1 AND status IN
		('initiated','processing','confirmed'))`, invoice).
		Scan(&open)

	return open, apperr.Wrap(err, "select billing.payments")
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
			AggregateType: kindInvoice,
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

func (Billing) paymentResult(
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
			AggregateType: kindPayment,
			AggregateID:   paymentID,
			Type:          "PaymentChanged",
			Payload: map[string]any{
				fieldStatus: payment.Status,
				"invoiceId": payment.InvoiceID,
			},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          action,
			TargetKind:      kindPayment,
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
	case simulateInitiate, simulateInstructions:
		in.checkInvoiceEvent(fieldErrs)
	case simulateProcessing, simulateConfirm, simulateFail:
		in.checkPaymentEvent(fieldErrs)
	default:
		fieldErrs["event"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// checkInvoiceEvent: initiate and instructions name the invoice; initiate names a demo method, instructions none.
func (in *SimulateInput) checkInvoiceEvent(fieldErrs map[string]string) {
	if in.invoiceEventIncomplete() {
		fieldErrs["invoiceId"] = apperr.KeyRequired
	}

	if in.Event == simulateInitiate &&
		(in.Method == nil || !slices.Contains(paymentMethods(), *in.Method)) {
		fieldErrs["method"] = apperr.KeyInvalid
	}

	if in.Event == simulateInstructions && in.Method != nil {
		fieldErrs["method"] = apperr.KeyNotAllowed
	}
}

// invoiceEventIncomplete: an invoice event names the invoice and the demo confirmation, and no payment or event.
func (in *SimulateInput) invoiceEventIncomplete() bool {
	return in.InvoiceID == nil || in.DemoConfirmed == nil || !*in.DemoConfirmed ||
		in.PaymentID != nil ||
		in.EventID != nil
}

// checkPaymentEvent: processing, confirm and fail name the payment and the event, no invoice, and a reference.
func (in *SimulateInput) checkPaymentEvent(fieldErrs map[string]string) {
	if in.PaymentID == nil || in.EventID == nil || in.InvoiceID != nil {
		fieldErrs["paymentId"] = apperr.KeyRequired
	}

	if !refOK(in.PaymentReference) {
		fieldErrs["paymentReference"] = apperr.KeyLength
	}
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
func (m Billing) simulate(ctx context.Context, call *ops.Call, input *SimulateInput) (any, error) {
	if input.Event == simulateInitiate || input.Event == simulateInstructions {
		return m.simulateInvoiceEvent(ctx, call, input)
	}

	return m.simulatePaymentEvent(ctx, call, input)
}

// simulateInvoiceEvent serves initiate, which starts a demo payment of the whole invoice, and instructions, which
// previews the payment notification.
func (m Billing) simulateInvoiceEvent(
	ctx context.Context,
	call *ops.Call,
	input *SimulateInput,
) (any, error) {
	inv, err := m.loadInvoice(ctx, call, *input.InvoiceID, true)
	if err != nil {
		return nil, err
	}

	if inv.Version != *call.ExpectedVersion {
		return nil, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if inv.Status != invoiceUnpaid {
		return nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	if input.Event == simulateInstructions {
		return instructionsPreview(call, inv), nil
	}

	// one attempt at a time, and a paid invoice is not paid again: full amount only (IR474)
	open, err := paymentOpen(ctx, call, inv.ID)
	if err != nil {
		return nil, err
	}

	if open {
		return nil, apperr.E(apperr.Conflict, "errors.invoice_has_payment")
	}

	var paymentID uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, method,
		channel, status, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'demo', 'initiated', $5, $5) RETURNING
		id`, inv.ID, inv.AmountMinor, inv.Currency, *input.Method, call.Now).
		Scan(&paymentID)
	if scanErr != nil {
		return nil, apperr.Wrap(scanErr, "insert billing.payments")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.invoices SET version = version + 1, updated_at = $2 WHERE id = $1`,
		inv.ID,
		call.Now,
	)
	if execErr != nil {
		return nil, apperr.Wrap(execErr, "update billing.invoices")
	}

	return m.paymentResult(ctx, call, paymentID, "payments.simulate", simulateInitiate)
}

// instructionsPreview is the payment notification of the invoice as instructions shows it: not stored, not sent.
func instructionsPreview(call *ops.Call, inv Invoice) map[string]any {
	return map[string]any{
		"id":                     uuid.Nil,
		"tenantId":               call.Principal.TenantID,
		"version":                1,
		fieldCreatedAt:           call.Now,
		fieldUpdatedAt:           call.Now,
		"sourceAlertId":          nil,
		"type":                   kindPayment,
		"recipientMembershipId":  call.Principal.MembershipID,
		"scopeVersionAtCreation": call.Principal.ScopeVersion,
		"target":                 map[string]any{"kind": kindInvoice, "id": inv.ID},
		"templateKey":            kindPayment,
		"params": map[string]any{
			"targetName":  inv.Number,
			"at":          inv.DueAt,
			fieldStatus:   inv.Status,
			"reason":      nil,
			"amountMinor": inv.AmountMinor,
			fieldCurrency: inv.Currency,
			"method":      inv.PaymentMethod,
			"message":     nil,
		},
		"channel":       "inApp",
		"deliveryState": "preview",
		"severity":      "normal",
		"occurredAt":    call.Now,
		"readAt":        nil,
	}
}

// simulatePaymentEvent serves processing, confirm and fail, which move a demo payment on (IR59); an event seen
// before returns the payment as it is.
func (m Billing) simulatePaymentEvent(
	ctx context.Context,
	call *ops.Call,
	input *SimulateInput,
) (any, error) {
	payment, inv, err := m.loadPayment(ctx, call, *input.PaymentID)
	if err != nil {
		return nil, err
	}

	if slices.Contains(payment.EventIDs, *input.EventID) {
		return forCaller(call, payment), nil // same event again
	}

	if payment.Version != *call.ExpectedVersion {
		return nil, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	next, invoiceStatus, err := nextPaymentStatus(ctx, call, input, payment)
	if err != nil {
		return nil, err
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.payments SET status = $2, payment_reference = COALESCE($3, payment_reference), confirmed_at = CASE
			WHEN $2 = 'confirmed' THEN $4::timestamptz END,
		event_ids = array_append(event_ids, $5), version = version + 1, updated_at = $4 WHERE id = $1`,
		payment.ID,
		next,
		refIf(next == paymentConfirmed, input.PaymentReference),
		call.Now,
		*input.EventID,
	)
	if execErr != nil {
		return nil, apperr.Wrap(execErr, "update billing.payments")
	}

	settleErr := m.settleInvoice(ctx, call, inv.ID, next, invoiceStatus)
	if settleErr != nil {
		return nil, settleErr
	}

	return m.paymentResult(ctx, call, payment.ID, "payments.simulate", input.Event)
}

// nextPaymentStatus is where a payment event takes the payment and its invoice: processing from initiated, confirmed
// from processing with an unused reference, failed from either.
func nextPaymentStatus(
	ctx context.Context,
	call *ops.Call,
	input *SimulateInput,
	payment Payment,
) (string, string, error) {
	switch input.Event {
	case simulateProcessing:
		if payment.Status != paymentInitiated {
			return "", "", apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		return paymentProcessing, invoiceProcessing, nil
	case simulateConfirm:
		if payment.Status != paymentProcessing {
			return "", "", apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		referenceErr := referenceAvailable(ctx, call, *input.PaymentReference, payment.ID)
		if referenceErr != nil {
			return "", "", referenceErr
		}

		return paymentConfirmed, "", nil
	case simulateFail:
		if payment.Status != paymentInitiated && payment.Status != paymentProcessing {
			return "", "", apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		return paymentFailed, invoiceUnpaid, nil
	}

	return "", "", nil
}

// settleInvoice follows the payment on its invoice: a confirmed payment pays it, any other sets its status.
func (m Billing) settleInvoice(
	ctx context.Context,
	call *ops.Call,
	invoice uuid.UUID,
	next, invoiceStatus string,
) error {
	if next == paymentConfirmed {
		return m.markPaid(ctx, call, invoice)
	}

	_, err := call.Tx.Exec(
		ctx,
		`UPDATE billing.invoices SET status = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
		invoice,
		invoiceStatus,
		call.Now,
	)

	return apperr.Wrap(err, "update billing.invoices")
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

	if !knownCurrency(in.Currency) {
		fieldErrs[fieldCurrency] = apperr.KeyInvalid
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > maxReasonLength {
		fieldErrs["reason"] = apperr.KeyLength
	}

	return fieldErrs
}

// ConfirmInput is the input of payments.confirm: a payment that is processing. It has no invoiceId: the contract's
// input has none, so one is an unknown field (IR390).
type ConfirmInput struct {
	PaymentEntry

	PaymentID uuid.UUID `json:"paymentId"`
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
	PaymentEntry

	InvoiceID uuid.UUID `json:"invoiceId"`
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

	if payment.Status != paymentProcessing {
		return payment, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	amountOKErr := amountOK(input.PaymentEntry, inv)
	if amountOKErr != nil {
		return payment, amountOKErr
	}

	referenceErr := referenceAvailable(ctx, call, input.PaymentReference, payment.ID)
	if referenceErr != nil {
		return payment, referenceErr
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

	recordableErr := recordable(ctx, call, inv)
	if recordableErr != nil {
		return Payment{}, recordableErr
	}

	amountOKErr := amountOK(input.PaymentEntry, inv)
	if amountOKErr != nil {
		return Payment{}, amountOKErr
	}

	referenceErr := referenceAvailable(ctx, call, input.PaymentReference, uuid.Nil)
	if referenceErr != nil {
		return Payment{}, referenceErr
	}

	var paymentID uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, method,
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

// recordable checks a payments.recordManual of the invoice: the expected version, and no payment in flight or
// confirmed. A failed or expired attempt does not stop it: the customer may have paid another way (IR447).
func recordable(ctx context.Context, call *ops.Call, inv Invoice) error {
	if inv.Version != *call.ExpectedVersion {
		return apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	open, err := paymentOpen(ctx, call, inv.ID)
	if err != nil {
		return err
	}

	if open || inv.Status == invoicePaid {
		return apperr.E(apperr.Conflict, "errors.invoice_has_payment")
	}

	return nil
}

// RegisterPayments binds the payment operations.
func RegisterPayments(r *ops.Registry, m Billing) {
	ops.Register(r, "payments.simulate", m.simulate)
	ops.Register(r, "payments.confirm", m.confirm)
	ops.Register(r, "payments.recordManual", m.recordManual)
}
