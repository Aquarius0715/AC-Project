package monitoring

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the monitoring consumers of other domains' events (IR184): reconciliation_required alerts are
// owned by equipment and opened / resolved on billing's (restrictions) request; filter cleaning reminders' maintenance
// alerts on maintenance's (IR239).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.FilterCleaningDue:      filterCleaningDue,
		events.FilterCleaningCleared:  filterCleaningCleared,
		events.ReconciliationRequired: reconciliationRequired,
		events.ReconciliationResolved: reconciliationResolved,
	}
}

// filterCleaningDue opens the unit's filter cleaning reminder Alert (IR239), with a DeliveryFailure when no one
// would be notified; a replay, or an open reminder Alert of the unit (alerts_one_open_per_rule), adds none.
func filterCleaningDue(ctx context.Context, txn pgx.Tx, event events.Event) error {
	var filterReminder events.FilterReminder

	decodeErr := event.Decode(&filterReminder)
	if decodeErr != nil {
		return apperr.Wrap(decodeErr, "events.Event.Decode")
	}

	failures := "[]"

	if filterReminder.NoRecipient {
		failureJSON, err := json.Marshal(
			[]map[string]any{
				{
					"id":            uuid.New(),
					keyPolicyID:     nil,
					"eventId":       event.ID,
					"occurredAt":    filterReminder.At,
					"reason":        reasonNoRecipient,
					"deliveryState": outcomeFailed,
				},
			},
		)
		if err != nil {
			return apperr.Wrap(err, "json.Marshal")
		}

		failures = string(failureJSON)
	}

	_, err := txn.Exec(
		ctx,
		`INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, rule_key, type, severity, status,
			cause_code, evidence_kind, evidence_text,
			delivery_failures, observed_at, detected_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'filter_cleaning', 'maintenance', 'normal', 'open', 'unknown', 'inferred', $5, $6, $7, $7,
		$7, $7) ON CONFLICT DO NOTHING`,
		filterReminder.AlertID,
		event.TenantID,
		filterReminder.UnitID,
		filterReminder.CustomerOrgID,
		filterReminder.Evidence,
		failures,
		filterReminder.At,
	)

	return apperr.Wrap(err, "insert monitoring.alerts")
}

// filterCleaningCleared resolves the reminder Alert once the filter is cleaned.
func filterCleaningCleared(ctx context.Context, txn pgx.Tx, e events.Event) error {
	var filterReminder events.FilterReminder

	decodeErr := e.Decode(&filterReminder)
	if decodeErr != nil {
		return apperr.Wrap(decodeErr, "events.Event.Decode")
	}

	_, err := txn.Exec(
		ctx,
		`UPDATE monitoring.alerts SET status = 'resolved', resolved_at = $2, resolution_reason = 'filter cleaned', version =
			version + 1, updated_at = $2
		WHERE id = $1 AND status <> 'resolved'`,
		filterReminder.AlertID,
		filterReminder.At,
	)

	return apperr.Wrap(err, "update monitoring.alerts")
}

// reconciliationRequired opens a unit's reconciliation_required Alert while its device still reports an ended
// restriction; a concurrent delivery may have opened it (alerts_one_open_per_rule).
func reconciliationRequired(ctx context.Context, txn pgx.Tx, e events.Event) error {
	var units events.Units

	decodeErr := e.Decode(&units)
	if decodeErr != nil {
		return apperr.Wrap(decodeErr, "events.Event.Decode")
	}

	//nolint:dupword // SQL: the four time columns all take the scenario clock's now
	_, err := txn.Exec(
		ctx,
		`INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, type, severity, status, cause_code,
			evidence_kind, evidence_text, observed_at, detected_at, created_at, updated_at)
		SELECT u.tenant_id, u.id, u.customer_org_id, 'reconciliation_required', 'warning', 'open', 'unknown',
		'demo_observation',
			'Device still reports an ended restriction; reconcile it from HQ', platform.app_now(), platform.app_now(),
			platform.app_now(), platform.app_now()
		FROM assets.units u
		WHERE u.id = ANY($1) AND NOT EXISTS (SELECT 1 FROM monitoring.alerts a WHERE a.unit_id = u.id AND a.type =
		'reconciliation_required' AND a.status <> 'resolved')
		ON CONFLICT DO NOTHING`,
		units.UnitIDs,
	)

	return apperr.Wrap(err, "insert monitoring.alerts")
}

// reconciliationResolved resolves the units' reconciliation_required Alerts once the recovery resolved.
func reconciliationResolved(ctx context.Context, txn pgx.Tx, e events.Event) error {
	var units events.Units

	decodeErr := e.Decode(&units)
	if decodeErr != nil {
		return apperr.Wrap(decodeErr, "events.Event.Decode")
	}

	_, err := txn.Exec(
		ctx,
		`UPDATE monitoring.alerts SET status = 'resolved', resolved_at = platform.app_now(), resolution_reason =
			'recovery resolved',
			version = version + 1, updated_at = platform.app_now()
		WHERE unit_id = ANY($1) AND type = 'reconciliation_required' AND status <> 'resolved'`,
		units.UnitIDs,
	)

	return apperr.Wrap(err, "update monitoring.alerts")
}
