package monitoring

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// TamperAlert implements devices.AlertOpener: the tamper Alert of a unit for a device's tamper event (SR23: created in
// the same transition). The IR66 incident key is the unit with type tamper / cause unknown and no policy: an open or
// acknowledged Alert is kept, otherwise a new critical Alert links the latest resolved one as previousAlertId. Tamper
// Alerts resolve only by hand (alert.resolve); a tamper recovery event does not resolve them.
func (Alerts) TamperAlert(
	ctx context.Context,
	txn pgx.Tx,
	unit, org uuid.UUID,
	text string,
	when time.Time,
) (uuid.UUID, error) {
	var alertID uuid.UUID

	err := txn.QueryRow(ctx, `SELECT id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id IS NULL AND type = 'tamper'
		AND cause_code = 'unknown' AND status <> 'resolved'`,
		unit).
		Scan(&alertID)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return alertID, apperr.Wrap(err, "select monitoring.alerts")
	}

	err = txn.QueryRow(ctx, `INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, type, severity, status,
		cause_code, evidence_kind, evidence_text,
		observed_at, detected_at, previous_alert_id, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'tamper', 'critical', 'open', 'unknown', 'demo_observation',
		$3, $4, $4,
			(SELECT p.id FROM monitoring.alerts p WHERE p.unit_id = $1 AND p.policy_id IS NULL AND p.type = 'tamper' AND
			p.cause_code = 'unknown' ORDER BY p.detected_at DESC, p.id DESC LIMIT 1),
			$4, $4)
		ON CONFLICT DO NOTHING RETURNING id`, unit, org, text, when).
		Scan(&alertID)
	if errors.Is(
		err,
		pgx.ErrNoRows,
	) { // a concurrent tamper event opened it first (alerts_one_open_per_rule)
		err = txn.QueryRow(ctx, `SELECT id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id IS NULL AND type = 'tamper'
			AND cause_code = 'unknown' AND status <> 'resolved'`, unit).
			Scan(&alertID)
	}

	return alertID, apperr.Wrap(err, "select monitoring.alerts")
}

// BypassAlert implements devices.AlertOpener: the restriction_bypass Alert of a unit for a device fault reported while
// the unit observes a restriction (DEC-110, IR844). A restriction is infrared sent by the node, so a node that went
// silent, lost power or was opened may no longer hold it. One unresolved critical Alert per unit (IR66 incident key:
// the unit with type restriction_bypass and no policy); a later fault while it is open is the same incident, otherwise
// a new one links the latest resolved one as previousAlertId. It resolves only by hand (alert.resolve): a recovery
// event does not prove the restriction held meanwhile.
func (Alerts) BypassAlert(
	ctx context.Context,
	txn pgx.Tx,
	unit, org uuid.UUID,
	text string,
	when time.Time,
) (uuid.UUID, error) {
	const current = `SELECT id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id IS NULL
		AND type = 'restriction_bypass' AND status <> 'resolved'`

	var alertID uuid.UUID

	err := txn.QueryRow(ctx, current, unit).Scan(&alertID)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return alertID, apperr.Wrap(err, "select monitoring.alerts")
	}

	err = txn.QueryRow(ctx, `INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, type, severity, status,
		cause_code, evidence_kind, evidence_text, observed_at, detected_at, previous_alert_id, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'restriction_bypass', 'critical', 'open', 'unknown',
		'inferred', $3, $4, $4,
			(SELECT p.id FROM monitoring.alerts p WHERE p.unit_id = $1 AND p.policy_id IS NULL
			AND p.type = 'restriction_bypass' ORDER BY p.detected_at DESC, p.id DESC LIMIT 1),
			$4, $4)
		ON CONFLICT DO NOTHING RETURNING id`, unit, org, text, when).
		Scan(&alertID)
	if errors.Is(
		err,
		pgx.ErrNoRows,
	) { // a concurrent fault opened it first (alerts_one_open_per_rule)
		err = txn.QueryRow(ctx, current, unit).Scan(&alertID)
	}

	return alertID, apperr.Wrap(err, "select monitoring.alerts")
}
