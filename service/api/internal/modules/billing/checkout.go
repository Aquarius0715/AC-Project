package billing

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/stripe"
)

// The payment channel of a Stripe Checkout payment (billing.payments.channel), and the Checkout metadata key that names
// the tenant on a session and on its payment intent.
const (
	channelStripeCheckout = "stripe_checkout"
	metaTenantID          = "tenant_id"
)

// Checkout creates the hosted payment pages of Stripe (DEC-68). A Billing without one (nil) has no Stripe configured,
// and payments.startCheckout answers UNAVAILABLE.
type Checkout interface {
	CreateSession(ctx context.Context, req stripe.SessionRequest) (stripe.Session, error)
	// ReleaseSession ends a session the payer left; true when it can no longer take a payment.
	ReleaseSession(ctx context.Context, id string) (bool, error)
}

// StartCheckoutInput is the payments.startCheckout input.
type StartCheckoutInput struct {
	InvoiceID uuid.UUID `json:"invoiceId"`
}

// Validate implements ops.Validator.
func (in *StartCheckoutInput) Validate() map[string]string {
	fieldErrs := map[string]string{}

	if in.InvoiceID == uuid.Nil {
		fieldErrs["invoiceId"] = apperr.KeyRequired
	}

	return fieldErrs
}

// CheckoutStart is the payments.startCheckout result: the payment in flight and the hosted page the payer goes to.
type CheckoutStart struct {
	PaymentID   uuid.UUID `json:"paymentId"`
	CheckoutURL string    `json:"checkoutUrl"`
}

// startCheckout serves payments.startCheckout: one Stripe Checkout payment of the whole unpaid invoice (IR474). The
// payment is stored with its Checkout Session before the transaction commits; the invoice is paid only when the
// webhook confirms the payment (IR35 release route), never on the return to the site.
//
//	@Summary		payments.startCheckout (write)
//	@ID				payments.startCheckout
//	@Description	Authorization: client:self
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR474 full amount and one attempt in flight (CONFLICT errors.invoice_has_payment); DEC-68 hosted \
//	@Description	Stripe Checkout in MYR; paid only by the signed webhook
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C11, DD-A08
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer				true	"all: required (target invoice, read invoices.get)"
//	@Param			request				body		StartCheckoutInput	true	"input"
//	@Success		200					{object}	ops.Envelope{data=CheckoutStart}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/payments/checkout [post]
func (m Billing) startCheckout(
	ctx context.Context,
	call *ops.Call,
	input *StartCheckoutInput,
) (CheckoutStart, error) {
	inv, err := m.loadInvoice(ctx, call, input.InvoiceID, true)
	if err != nil {
		return CheckoutStart{}, err
	}

	if inv.Version != *call.ExpectedVersion {
		return CheckoutStart{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if inv.Status != invoiceUnpaid {
		return CheckoutStart{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	// one attempt at a time, and a paid invoice is not paid again: full amount only (IR474). A Checkout Session the
	// payer left (cancelled, or the page closed) is ended at Stripe first, so the payer can start again at once
	open, err := paymentOpen(ctx, call, inv.ID)
	if err != nil {
		return CheckoutStart{}, err
	}

	if open {
		released, releaseErr := m.releaseLeftSession(ctx, call, inv.ID)
		if releaseErr != nil {
			return CheckoutStart{}, releaseErr
		}

		if !released {
			return CheckoutStart{}, apperr.E(apperr.Conflict, "errors.invoice_has_payment")
		}
	}

	// a refusal of the request comes before the provider's availability: stale versions are conflicts, always
	if m.Checkout == nil {
		return CheckoutStart{}, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	paymentID, err := m.insertCheckoutPayment(ctx, call, inv)
	if err != nil {
		return CheckoutStart{}, err
	}

	session, err := m.Checkout.CreateSession(ctx, m.sessionRequest(ctx, call, inv, paymentID))
	if err != nil {
		// the payment row rolls back with the transaction; the same key may be sent again (D04)
		return CheckoutStart{}, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.payments SET stripe_checkout_session_id = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
		paymentID,
		session.ID,
		call.Now,
	)
	if execErr != nil {
		return CheckoutStart{}, apperr.Wrap(execErr, "update billing.payments")
	}

	_, bumpErr := call.Tx.Exec(
		ctx,
		`UPDATE billing.invoices SET version = version + 1, updated_at = $2 WHERE id = $1`,
		inv.ID,
		call.Now,
	)
	if bumpErr != nil {
		return CheckoutStart{}, apperr.Wrap(bumpErr, "update billing.invoices")
	}

	_, auditErr := m.paymentResult(ctx, call, paymentID, "payments.startCheckout", "")
	if auditErr != nil {
		return CheckoutStart{}, auditErr
	}

	return CheckoutStart{PaymentID: paymentID, CheckoutURL: session.URL}, nil
}

// insertCheckoutPayment stores the attempt before Stripe is called, so that the session it creates has a payment row.
func (Billing) insertCheckoutPayment(
	ctx context.Context,
	call *ops.Call,
	inv Invoice,
) (uuid.UUID, error) {
	var paymentID uuid.UUID

	err := call.Tx.QueryRow(ctx, `INSERT INTO billing.payments (tenant_id, invoice_id, amount_minor, currency, channel,
		status, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'initiated', $5, $5) RETURNING id`,
		inv.ID, inv.AmountMinor, inv.Currency, channelStripeCheckout, call.Now).
		Scan(&paymentID)

	return paymentID, apperr.Wrap(err, "insert billing.payments")
}

// sessionRequest is the Checkout Session of one payment. The return pages are the customer's page of the invoice,
// which shows the state the webhook has reached.
func (m Billing) sessionRequest(
	ctx context.Context,
	call *ops.Call,
	inv Invoice,
	paymentID uuid.UUID,
) stripe.SessionRequest {
	page := m.CustomerBase + "/customer/payments/" + inv.ID.String()

	return stripe.SessionRequest{
		// the write's key with the payment: the SDK's own retries reuse it, and a write retried after a rollback (a new
		// payment row) is a new request to Stripe, not a reuse of the key with other parameters, which Stripe refuses
		IdempotencyKey: ops.IdempotencyKeyOf(ctx) + ":" + paymentID.String(),
		AmountMinor:    inv.AmountMinor,
		Currency:       inv.Currency,
		Description:    inv.Number,
		Metadata: map[string]string{
			metaTenantID: call.Principal.TenantID.String(),
			"invoice_id": inv.ID.String(),
			"payment_id": paymentID.String(),
		},
		SuccessURL: page + "?checkout=return",
		CancelURL:  page + "?checkout=cancelled",
	}
}

// releaseLeftSession ends the invoice's open Stripe Checkout attempt when the payer left it: a payment still
// initiated, with a session that Stripe reports open (then expired here) or expired. The payment moves to expired, as
// the session's expiry webhook would move it, and that webhook then changes nothing. It reports false for any other
// open payment (a demo attempt, one Stripe is processing, a confirmed one): that payment stays and the start is
// refused. A provider failure answers UNAVAILABLE and nothing changes.
func (m Billing) releaseLeftSession(
	ctx context.Context,
	call *ops.Call,
	invoice uuid.UUID,
) (bool, error) {
	if m.Checkout == nil {
		return false, nil
	}

	var (
		paymentID uuid.UUID
		session   *string
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, stripe_checkout_session_id FROM billing.payments
		WHERE invoice_id = $1 AND status = 'initiated' AND channel = $2 FOR UPDATE`,
		invoice, channelStripeCheckout).
		Scan(&paymentID, &session)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, apperr.Wrap(err, "select billing.payments")
	}

	if session == nil {
		return false, nil
	}

	released, err := m.Checkout.ReleaseSession(ctx, *session)
	if err != nil {
		return false, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	if !released {
		return false, nil
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE billing.payments SET status = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
		paymentID,
		statusExpired,
		call.Now,
	)
	if err != nil {
		return false, apperr.Wrap(err, "update billing.payments")
	}

	_, err = m.paymentResult(ctx, call, paymentID, "payments.startCheckout", "")

	return err == nil, err
}
