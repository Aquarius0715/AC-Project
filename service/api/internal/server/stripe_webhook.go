package server

import (
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/billing"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/platform/stripe"
)

// maxWebhookBody bounds one delivery; Stripe's events are a few kilobytes.
const maxWebhookBody = 1 << 20

// stripeWebhook receives Stripe's signed deliveries (DEC-68) and applies each to the payment it names, in one
// transaction with the events and audit it causes. It answers 2xx only after the delivery is stored, so that Stripe
// resends a delivery that was not recorded. There is no sign-in: the signature is the authentication.
func stripeWebhook(
	reg *ops.Registry,
	txm *db.TxManager,
	bill billing.Billing,
	secret string,
) echo.HandlerFunc {
	return func(ectx *echo.Context) error {
		// the handler runs on the request's own context, which the transaction below uses
		ctx := ectx.Request().Context()

		body, err := io.ReadAll(
			http.MaxBytesReader(ectx.Response(), ectx.Request().Body, maxWebhookBody),
		)
		if err != nil {
			return apperr.Wrap(err, "read webhook body")
		}

		event, err := stripe.VerifyEvent(
			body,
			ectx.Request().Header.Get("Stripe-Signature"),
			secret,
		)
		if err != nil {
			return apperr.Wrap(err, "stripe signature")
		}

		delivery, err := stripe.ParseDelivery(event, body)
		if err != nil {
			return apperr.E(apperr.Validation, apperr.KeyInvalid)
		}

		// the tenant comes from the signed metadata written at checkout. A delivery without one (an event of the Stripe
		// account that no checkout here started) is stored under no tenant and changes nothing: refusing it would make
		// Stripe resend it for days
		tenant, err := uuid.Parse(delivery.Metadata["tenant_id"])
		if err != nil {
			tenant = uuid.Nil
		}

		// the Stripe webhook acts as the system: its audit entries name the system actor, not a person
		actor := ops.SystemPrincipal(tenant, uuid.Nil)

		err = txm.Run(ctx, false, actor, func(txn pgx.Tx) error {
			call := &ops.Call{
				Tx:              txn,
				Principal:       actor,
				Now:             reg.Clock(),
				CorrelationID:   ectx.Response().Header().Get(echo.HeaderXRequestID),
				Bearer:          "",
				Queries:         reg,
				ExpectedVersion: nil,
				Candidates:      nil,
				Events:          nil,
				Audits:          nil,
			}

			_, applyErr := bill.ApplyStripeEvent(ctx, call, delivery)
			if applyErr != nil {
				return apperr.Wrap(applyErr, "apply stripe event")
			}

			return apperr.Wrap(
				db.Recorder{}.Record(ctx, txn, call, stripeAuditAction),
				"record webhook effects",
			)
		})
		if err != nil {
			return apperr.Wrap(err, "stripe webhook")
		}

		// one process serves every domain: the events of this delivery are applied now, as after any other write
		if reg.AfterCommit != nil {
			reg.AfterCommit(ctx)
		}

		return ectx.NoContent(http.StatusOK)
	}
}

// stripeAuditAction names the audit entries and events of a webhook delivery.
const stripeAuditAction = "payments.webhook"
