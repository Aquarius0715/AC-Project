package assets

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// ---- warranty reminders (scheduler job) ----

// warrantyReminderDays are the BR-A19 reminders in days before a warranty ends, the latest first.
var warrantyReminderDays = []int{30, 90}

// warrantyStage is the latest reminder a warranty end has reached at now (0 when it ends later than 90 days).
func warrantyStage(ends, now time.Time) int {
	for _, d := range warrantyReminderDays {
		if !now.Before(ends.Add(-time.Duration(d) * 24 * time.Hour)) {
			return d
		}
	}
	return 0
}

// RemindWarranties is the equipment scheduler job of BR-A19 (IR340). A unit that is not archived, whose warranty
// ends within 90 days and that no active contract covers, gets the latest reminder it has reached: 90 or 30 days
// before warrantyEndsAt. Each reminder goes once per warranty end, and an earlier one never follows a later one. Each
// HQ member who reads the units (asset.read) gets one warranty_expiring notification. A changed warranty end gets its
// own reminders.
func (m *Module) RemindWarranties(ctx context.Context, c *ops.Call) (int, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, warranty_ends_at FROM assets.units WHERE NOT archived AND warranty_ends_at > $1 AND warranty_ends_at <= $2
		ORDER BY warranty_ends_at, id`, c.Now, c.Now.Add(time.Duration(warrantyReminderDays[len(warrantyReminderDays)-1])*24*time.Hour))
	if err != nil {
		return 0, err
	}
	type due struct {
		id   uuid.UUID
		ends time.Time
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (due, error) {
		var d due
		return d, r.Scan(&d.id, &d.ends)
	})
	if err != nil || len(list) == 0 {
		return 0, err
	}
	ids := make([]uuid.UUID, len(list))
	for i, d := range list {
		ids[i] = d.id
	}
	contracts, err := m.Contracts.ActiveContracts(ctx, c, ids)
	if err != nil {
		return 0, err
	}
	var recipients []uuid.UUID
	n := 0
	for _, d := range list {
		if len(contracts[d.id]) > 0 { // a maintenance contract covers the unit (IR111)
			continue
		}
		days := warrantyStage(d.ends, c.Now)
		tag, err := c.Tx.Exec(ctx, `INSERT INTO assets.warranty_reminders (unit_id, warranty_ends_at, days_before, tenant_id, sent_at)
			SELECT $1, $2, $3, current_setting('app.tenant_id')::uuid, $4
			WHERE NOT EXISTS (SELECT 1 FROM assets.warranty_reminders WHERE unit_id = $1 AND warranty_ends_at = $2 AND days_before <= $3) ON CONFLICT DO NOTHING`,
			d.id, d.ends, days, c.Now)
		if err != nil {
			return n, err
		}
		if tag.RowsAffected() == 0 { // this reminder (or a later one) went already
			continue
		}
		if recipients == nil {
			if recipients, err = identity.Members(ctx, c, identity.MembersInput{Role: "admin", Permission: "asset.read"}); err != nil {
				return n, err
			}
		}
		severity := "normal"
		if days == warrantyReminderDays[0] {
			severity = "warning"
		}
		for _, r := range recipients {
			if _, err := (notify.Store{}).Create(ctx, c, notify.New{RecipientMembershipID: r, Type: "warranty_expiring", TemplateKey: "warranty_expiring", TargetKind: "unit",
				TargetID: d.id, Severity: severity, Params: map[string]any{"at": d.ends, "status": "expiring"}}); err != nil {
				return n, err
			}
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE assets.warranty_reminders SET notifications = $4 WHERE unit_id = $1 AND warranty_ends_at = $2 AND days_before = $3`,
			d.id, d.ends, days, len(recipients)); err != nil {
			return n, err
		}
		c.Audit(ops.AuditEntry{Action: "units.remindWarranty", TargetKind: "unit", TargetID: d.id.String(), Reason: fmt.Sprintf("%d days before the warranty ends", days)})
		n++
	}
	return n, nil
}
