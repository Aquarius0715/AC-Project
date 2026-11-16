package billing

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/stripe"
)

// The Stripe events billing acts on (DEC-68). Only these change a payment; every other delivery is stored as evidence.
const (
	stripeSessionCompleted = "checkout.session.completed"
	stripeSessionExpired   = "checkout.session.expired"
	stripeAsyncFailed      = "checkout.session.async_payment_failed"
	stripeAsyncSucceeded   = "checkout.session.async_payment_succeeded"
	stripeIntentSucceeded  = "payment_intent.succeeded"
	stripeIntentFailed     = "payment_intent.payment_failed"
	stripePaid             = "paid"
	stripeUnpaid           = "unpaid"  // a completed session whose payment method settles later (a bank payment)
	statusExpired          = "expired" // billing.payments.status of a Checkout Session the payer let lapse
	metaPaymentID          = "payment_id"
	stripeAuditAction      = "payments.webhook"
)

// ApplyStripeEvent applies one verified Stripe delivery (DEC-68). The delivery is stored first, keyed by its event ID:
// a repeated event returns false and changes nothing. A delivery that names no payment, or whose session or amount
// does not match the payment, is stored and changes nothing either, so that Stripe stops resending it. A confirmed
// payment pays its invoice through the same route as a manual or demo confirmation (IR35).
func (m Billing) ApplyStripeEvent(
	ctx context.Context,
	call *ops.Call,
	delivery stripe.Delivery,
) (bool, error) {
	paymentID, known, err := storedPaymentID(ctx, call, delivery.Metadata[metaPaymentID])
	if err != nil {
		return false, err
	}

	fresh, err := storeDelivery(ctx, call, delivery, paymentID, known)
	if err != nil || !fresh {
		return fresh, err
	}

	if !known || !m.matchesPayment(ctx, call, paymentID, delivery) {
		return true, nil
	}

	err = m.applyPaymentEvent(ctx, call, paymentID, delivery)
	if err != nil {
		return true, err
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE billing.stripe_events SET processed_at = $2 WHERE id = $1`,
		delivery.ID,
		call.Now,
	)
	if err != nil {
		return true, apperr.Wrap(err, "update billing.stripe_events")
	}

	return true, nil
}

// storedPaymentID reads the payment a delivery's metadata names; known is false for a missing or unknown payment.
func storedPaymentID(ctx context.Context, call *ops.Call, raw string) (uuid.UUID, bool, error) {
	id, ok := parseUUID(raw)
	if !ok {
		return uuid.Nil, false, nil // a delivery without a payment is stored and changes nothing
	}

	var found uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT id FROM billing.payments WHERE id = $1`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}

	if err != nil {
		return uuid.Nil, false, apperr.Wrap(err, "select billing.payments")
	}

	return found, true, nil
}

// parseUUID reads an ID from metadata; ok is false for anything that is not one.
func parseUUID(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)

	return id, err == nil
}

// storeDelivery inserts the event once; it reports false when the event ID is already stored.
func storeDelivery(
	ctx context.Context,
	call *ops.Call,
	delivery stripe.Delivery,
	paymentID uuid.UUID,
	known bool,
) (bool, error) {
	var payment *uuid.UUID
	if known {
		payment = &paymentID
	}

	var id string

	err := call.Tx.QueryRow(ctx, `INSERT INTO billing.stripe_events (id, type, livemode, payment_id, received_at, payload)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb) ON CONFLICT (id) DO NOTHING RETURNING id`,
		delivery.ID, delivery.Type, delivery.Livemode, payment, call.Now, string(delivery.Raw)).
		Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	return err == nil, apperr.Wrap(err, "insert billing.stripe_events")
}

// matchesPayment checks that a session event names the Checkout Session the payment was started with.
func (Billing) matchesPayment(
	ctx context.Context,
	call *ops.Call,
	paymentID uuid.UUID,
	delivery stripe.Delivery,
) bool {
	if delivery.SessionID == "" {
		return true
	}

	var session *string

	err := call.Tx.QueryRow(ctx, `SELECT stripe_checkout_session_id FROM billing.payments WHERE id = $1`, paymentID).
		Scan(&session)

	return err == nil && session != nil && *session == delivery.SessionID
}

// applyPaymentEvent moves the payment by the delivery's type. Each move starts only from a payment still in flight,
// so a late or repeated event of another outcome changes nothing.
func (m Billing) applyPaymentEvent(
	ctx context.Context,
	call *ops.Call,
	paymentID uuid.UUID,
	delivery stripe.Delivery,
) error {
	payment, inv, err := m.loadPayment(ctx, call, paymentID)
	if err != nil {
		return err
	}

	if payment.Status != paymentInitiated && payment.Status != paymentProcessing {
		return nil
	}

	switch {
	case delivery.Type == stripeSessionCompleted && delivery.PaymentStatus == stripePaid,
		delivery.Type == stripeAsyncSucceeded && delivery.PaymentStatus == stripePaid,
		delivery.Type == stripeIntentSucceeded:
		if !sameAmount(delivery, inv) {
			return nil // an amount other than the invoice's is not a payment of it
		}

		return m.confirmStripePayment(ctx, call, payment, inv, delivery)
	case delivery.Type == stripeSessionCompleted && delivery.PaymentStatus == stripeUnpaid:
		// the payer finished on Stripe's page with a method that settles later: processing is not paid, and the
		// session's async_payment_succeeded or async_payment_failed settles it
		if payment.Status != paymentInitiated || !sameAmount(delivery, inv) {
			return nil
		}

		return m.settleStripe(
			ctx,
			call,
			payment,
			inv,
			paymentProcessing,
			invoiceProcessing,
			delivery,
		)
	case delivery.Type == stripeIntentFailed, delivery.Type == stripeAsyncFailed:
		return m.settleStripe(ctx, call, payment, inv, paymentFailed, invoiceUnpaid, delivery)
	case delivery.Type == stripeSessionExpired:
		return m.settleStripe(ctx, call, payment, inv, statusExpired, invoiceUnpaid, delivery)
	default:
		return nil
	}
}

// confirmStripePayment confirms the payment and pays its invoice. Its reference is the PaymentIntent ID, the one HQ
// finds the payment by in Stripe's dashboard.
func (m Billing) confirmStripePayment(
	ctx context.Context,
	call *ops.Call,
	payment Payment,
	inv Invoice,
	delivery stripe.Delivery,
) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE billing.payments SET status = $2, confirmed_at = $3::timestamptz,
			stripe_payment_intent_id = COALESCE(NULLIF($4, ''), stripe_payment_intent_id),
			payment_reference = COALESCE(payment_reference, NULLIF($4, '')),
			version = version + 1, updated_at = $3 WHERE id = $1`,
		payment.ID,
		paymentConfirmed,
		call.Now,
		delivery.PaymentIntentID,
	)
	if err != nil {
		return apperr.Wrap(err, "update billing.payments")
	}

	err = m.settleInvoice(ctx, call, inv.ID, paymentConfirmed, "")
	if err != nil {
		return err
	}

	_, err = m.paymentResult(ctx, call, payment.ID, stripeAuditAction, delivery.Type)

	return err
}

// settleStripe moves a payment to processing, or to a failed or expired outcome with its invoice back to unpaid.
func (m Billing) settleStripe(
	ctx context.Context,
	call *ops.Call,
	payment Payment,
	inv Invoice,
	next, invoiceStatus string,
	delivery stripe.Delivery,
) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE billing.payments SET status = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
		payment.ID,
		next,
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "update billing.payments")
	}

	err = m.settleInvoice(ctx, call, inv.ID, next, invoiceStatus)
	if err != nil {
		return err
	}

	_, err = m.paymentResult(ctx, call, payment.ID, stripeAuditAction, delivery.Type)

	return err
}

// sameAmount reports whether Stripe charged the invoice's amount in its currency.
func sameAmount(delivery stripe.Delivery, inv Invoice) bool {
	return delivery.AmountTotal == inv.AmountMinor &&
		strings.EqualFold(delivery.Currency, inv.Currency)
}
