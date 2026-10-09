package monitoring

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TamperAlert implements devices.AlertOpener: the tamper Alert of a unit for a device's tamper event (SR23: created in
// the same transition). The IR66 incident key is the unit with type tamper / cause unknown and no policy: an open or
// acknowledged Alert is kept, otherwise a new critical Alert links the latest resolved one as previousAlertId. Tamper
// Alerts resolve only by hand (alert.resolve); a tamper recovery event does not resolve them.
func (Alerts) TamperAlert(ctx context.Context, tx pgx.Tx, unit, org uuid.UUID, text string, at time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id IS NULL AND type = 'tamper' AND cause_code = 'unknown' AND status <> 'resolved'`,
		unit).Scan(&id)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO monitoring.alerts (tenant_id, unit_id, customer_org_id, type, severity, status, cause_code, evidence_kind, evidence_text,
		observed_at, detected_at, previous_alert_id, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'tamper', 'critical', 'open', 'unknown', 'demo_observation', $3, $4, $4,
			(SELECT p.id FROM monitoring.alerts p WHERE p.unit_id = $1 AND p.policy_id IS NULL AND p.type = 'tamper' AND p.cause_code = 'unknown' ORDER BY p.detected_at DESC, p.id DESC LIMIT 1),
			$4, $4)
		RETURNING id`, unit, org, text, at).Scan(&id)
	return id, err
}
