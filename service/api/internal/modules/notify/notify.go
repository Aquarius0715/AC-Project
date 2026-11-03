// Package notify implements the Notifications module.
package notify

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// New is a notification to store for one recipient (inApp simulated, email/whatsapp preview).
type New struct {
	RecipientMembershipID uuid.UUID
	Channel               string
	Type, TemplateKey     string
	TargetKind            string
	TargetID              uuid.UUID
	Params                map[string]any
	Severity              string
	SourceAlertID         *uuid.UUID
}

// Store creates notifications for other modules (IR191): the producer assigns the notification ID and publishes
// NotificationRequested in its own transaction; identity-api stores the notification from the event with the
// recipient's scope version at that time. A recipient without a membership row is skipped.
type Store struct{}

// Create requests one notification and returns its ID.
func (Store) Create(ctx context.Context, call *ops.Call, n New) (uuid.UUID, error) {
	if n.Params == nil {
		n.Params = map[string]any{}
	}

	if n.Severity == "" {
		n.Severity = severityNormal
	}

	if n.Channel == "" {
		n.Channel = channelInApp
	}

	id := uuid.Must(uuid.NewV7())

	var tenant uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')::uuid`).
		Scan(&tenant)
	if err != nil {
		return id, apperr.Wrap(err, "select current_setting")
	}

	return id, apperr.Wrap(events.Publish(
		ctx,
		call.Tx,
		tenant,
		"notification",
		id,
		events.NotificationRequested,
		events.Notification{
			NotificationID:        id,
			RecipientMembershipID: n.RecipientMembershipID,
			Channel:               n.Channel,
			Type:                  n.Type,
			TemplateKey:           n.TemplateKey,
			Target: events.Target{
				Kind: n.TargetKind,
				ID:   n.TargetID,
			},
			Params:        n.Params,
			Severity:      n.Severity,
			SourceAlertID: n.SourceAlertID,
			OccurredAt:    call.Now,
		},
	), "events.Publish")
}

// EventHandlers store requested notifications (consumer identity, IR191).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.NotificationRequested: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var notification events.Notification

			decodeErr := e.Decode(&notification)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			target, _ := json.Marshal(notification.Target)
			params, _ := json.Marshal(notification.Params)
			_, err := txn.Exec(
				ctx,
				`INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, channel,
					template_key, target, params,
				source_alert_id, severity, occurred_at, created_at)
				SELECT $1, $2, m.id, m.scope_version, $4, $5, $6, $7, $8, $9, $10, $11, $11 FROM identity.memberships m WHERE m.id =
				$3
				ON CONFLICT (id) DO NOTHING`,
				notification.NotificationID,
				e.TenantID,
				notification.RecipientMembershipID,
				notification.Type,
				notification.Channel,
				notification.TemplateKey,
				target,
				params,
				notification.SourceAlertID,
				notification.Severity,
				notification.OccurredAt,
			)

			return apperr.Wrap(err, "insert notify.notifications")
		},
	}
}

// Replicas are notify's reference copies of other domains' rows (IR188), kept by the identity-api consumer from
// RowChanged events: targets (units, jobs, invoices, restrictions, devices, inquiries, certificates) and job parties
// (offers, assignments, the job histories frozen for them) for the IR142 read scope and recipients.
func Replicas() []events.Replica {
	return []events.Replica{
		{
			Source: "assets.units",
			Table:  "notify.ref_units",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, colCustomerOrgID, "property_id", "display_name"},
		},
		{
			Source: "assets.customers",
			Table:  "notify.ref_customers",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, "organization_id"},
		},
		{
			Source: "maintenance.jobs",
			Table:  "notify.ref_jobs",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, colUnitID, colCustomerOrgID},
		},
		{
			Source: "maintenance.offers",
			Table:  "notify.ref_offers",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, colJobID, "contractor_org_id"},
		},
		{
			Source: "maintenance.assignments",
			Table:  "notify.ref_assignments",
			Keys:   []string{"id"},
			Cols: []string{
				colTenantID,
				colJobID,
				"technician_membership_id",
				keyStatus,
				"scheduled",
			},
		},
		{
			Source: "billing.invoices",
			Table:  "notify.ref_invoices",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, "number", colCustomerID, keyStatus, "due_at"},
		},
		{
			Source: "billing.inquiries",
			Table:  "notify.ref_inquiries",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, colCustomerID, "subject_type"},
		},
		{
			Source: "restrictions.restrictions",
			Table:  "notify.ref_restrictions",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, colCustomerID},
		},
		{
			Source: "restrictions.restriction_units",
			Table:  "notify.ref_restriction_units",
			Keys:   []string{"restriction_id", colUnitID},
			Cols:   []string{colTenantID},
		},
		{
			Source: "devices.devices",
			Table:  "notify.ref_devices",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, "serial", colUnitID},
		},
		{
			Source: "assets.properties",
			Table:  "notify.ref_properties",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, colCustomerOrgID},
		}, // identity scopes (IR194)
		{
			Source: "maintenance.certificates",
			Table:  "notify.ref_certificates",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, "organization_id", "membership_id", "name"},
		}, // IR336
		{
			Source: "maintenance.job_history_snapshots",
			Table:  "notify.ref_job_histories",
			Keys:   []string{"id"},
			Cols:   []string{colTenantID, colJobID, "owner_kind", "owner_id"},
		}, // IR601
	}
}
