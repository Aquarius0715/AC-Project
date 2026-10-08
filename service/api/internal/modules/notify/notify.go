// Package notify implements the Notifications module.
package notify

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
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
func (Store) Create(ctx context.Context, c *ops.Call, n New) (uuid.UUID, error) {
	if n.Params == nil {
		n.Params = map[string]any{}
	}
	if n.Severity == "" {
		n.Severity = "normal"
	}
	if n.Channel == "" {
		n.Channel = "inApp"
	}
	id := uuid.Must(uuid.NewV7())
	var tenant uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')::uuid`).Scan(&tenant); err != nil {
		return id, err
	}
	return id, events.Publish(ctx, c.Tx, tenant, "notification", id, events.NotificationRequested, events.Notification{
		NotificationID: id, RecipientMembershipID: n.RecipientMembershipID, Channel: n.Channel, Type: n.Type, TemplateKey: n.TemplateKey,
		Target: events.Target{Kind: n.TargetKind, ID: n.TargetID}, Params: n.Params, Severity: n.Severity, SourceAlertID: n.SourceAlertID, OccurredAt: c.Now})
}

// EventHandlers store requested notifications (consumer identity, IR191).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.NotificationRequested: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Notification
			if err := e.Decode(&p); err != nil {
				return err
			}
			target, _ := json.Marshal(p.Target)
			params, _ := json.Marshal(p.Params)
			_, err := tx.Exec(ctx, `INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, channel, template_key, target, params,
				source_alert_id, severity, occurred_at, created_at)
				SELECT $1, $2, m.id, m.scope_version, $4, $5, $6, $7, $8, $9, $10, $11, $11 FROM identity.memberships m WHERE m.id = $3
				ON CONFLICT (id) DO NOTHING`,
				p.NotificationID, e.TenantID, p.RecipientMembershipID, p.Type, p.Channel, p.TemplateKey, target, params, p.SourceAlertID, p.Severity, p.OccurredAt)
			return err
		},
	}
}

// Replicas are notify's reference copies of other domains' rows (IR188), kept by the identity-api consumer from
// RowChanged events: targets (units, jobs, invoices, restrictions, devices, inquiries) and job parties (offers,
// assignments) for the IR142 read scope and recipients.
var Replicas = []events.Replica{
	{Source: "assets.units", Table: "notify.ref_units", Keys: []string{"id"}, Cols: []string{"tenant_id", "customer_org_id", "property_id", "display_name"}},
	{Source: "assets.customers", Table: "notify.ref_customers", Keys: []string{"id"}, Cols: []string{"tenant_id", "organization_id"}},
	{Source: "maintenance.jobs", Table: "notify.ref_jobs", Keys: []string{"id"}, Cols: []string{"tenant_id", "unit_id", "customer_org_id"}},
	{Source: "maintenance.offers", Table: "notify.ref_offers", Keys: []string{"id"}, Cols: []string{"tenant_id", "job_id", "contractor_org_id"}},
	{Source: "maintenance.assignments", Table: "notify.ref_assignments", Keys: []string{"id"}, Cols: []string{"tenant_id", "job_id", "technician_membership_id", "status"}},
	{Source: "billing.invoices", Table: "notify.ref_invoices", Keys: []string{"id"}, Cols: []string{"tenant_id", "number", "customer_id", "status", "due_at"}},
	{Source: "billing.inquiries", Table: "notify.ref_inquiries", Keys: []string{"id"}, Cols: []string{"tenant_id", "customer_id", "subject_type"}},
	{Source: "restrictions.restrictions", Table: "notify.ref_restrictions", Keys: []string{"id"}, Cols: []string{"tenant_id", "customer_id"}},
	{Source: "restrictions.restriction_units", Table: "notify.ref_restriction_units", Keys: []string{"restriction_id", "unit_id"}, Cols: []string{"tenant_id"}},
	{Source: "devices.devices", Table: "notify.ref_devices", Keys: []string{"id"}, Cols: []string{"tenant_id", "serial", "unit_id"}},
}
