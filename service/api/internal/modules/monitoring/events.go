package monitoring

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the monitoring consumers of other domains' events (IR184): reconciliation_required alerts are
// owned by equipment and opened / resolved on billing's (restrictions) request; filter cleaning reminders' maintenance
// alerts on maintenance's (IR239).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.FilterCleaningDue: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.FilterReminder
			if err := e.Decode(&p); err != nil {
				return err
			}
			failures := "[]"
			if p.NoRecipient {
				b, _ := json.Marshal([]map[string]any{{"id": uuid.New(), "policyId": nil, "eventId": e.ID, "occurredAt": p.At, "reason": "no_recipient", "deliveryState": "failed"}})
				failures = string(b)
			}
			// a replay, or an open reminder Alert of the unit (alerts_one_open_per_rule), adds none
			_, err := tx.Exec(ctx, `INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, rule_key, type, severity, status, cause_code, evidence_kind, evidence_text,
					delivery_failures, observed_at, detected_at, created_at, updated_at)
				VALUES ($1, $2, $3, $4, 'filter_cleaning', 'maintenance', 'normal', 'open', 'unknown', 'inferred', $5, $6, $7, $7, $7, $7) ON CONFLICT DO NOTHING`,
				p.AlertID, e.TenantID, p.UnitID, p.CustomerOrgID, p.Evidence, failures, p.At)
			return err
		},
		events.FilterCleaningCleared: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.FilterReminder
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE monitoring.alerts SET status = 'resolved', resolved_at = $2, resolution_reason = 'filter cleaned', version = version + 1, updated_at = $2
				WHERE id = $1 AND status <> 'resolved'`, p.AlertID, p.At)
			return err
		},
		events.ReconciliationRequired: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Units
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, type, severity, status, cause_code, evidence_kind, evidence_text, observed_at, detected_at, created_at, updated_at)
				SELECT u.tenant_id, u.id, u.customer_org_id, 'reconciliation_required', 'warning', 'open', 'unknown', 'demo_observation',
					'Device still reports an ended restriction; reconcile it from HQ', platform.app_now(), platform.app_now(), platform.app_now(), platform.app_now()
				FROM assets.units u
				WHERE u.id = ANY($1) AND NOT EXISTS (SELECT 1 FROM monitoring.alerts a WHERE a.unit_id = u.id AND a.type = 'reconciliation_required' AND a.status <> 'resolved')`, p.UnitIDs)
			return err
		},
		events.ReconciliationResolved: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Units
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE monitoring.alerts SET status = 'resolved', resolved_at = platform.app_now(), resolution_reason = 'recovery resolved',
					version = version + 1, updated_at = platform.app_now()
				WHERE unit_id = ANY($1) AND type = 'reconciliation_required' AND status <> 'resolved'`, p.UnitIDs)
			return err
		},
	}
}
