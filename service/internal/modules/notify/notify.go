// Package notify implements the Notifications module.
package notify

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/ops"
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

// Store creates notifications for other modules.
type Store struct{}

// Create stores one notification with the recipient's current scope version and returns its ID.
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
	target, _ := json.Marshal(map[string]any{"kind": n.TargetKind, "id": n.TargetID})
	params, _ := json.Marshal(n.Params)
	var id uuid.UUID
	err := c.Tx.QueryRow(ctx, `INSERT INTO notify.notifications (tenant_id, recipient_membership_id, scope_version_at_creation, type, channel, template_key, target, params,
		source_alert_id, severity, occurred_at, created_at)
		SELECT current_setting('app.tenant_id')::uuid, m.id, m.scope_version, $2, $3, $4, $5, $6, $7, $8, $9, $9 FROM identity.memberships m WHERE m.id = $1 RETURNING id`,
		n.RecipientMembershipID, n.Type, n.Channel, n.TemplateKey, target, params, n.SourceAlertID, n.Severity, c.Now).Scan(&id)
	return id, err
}
