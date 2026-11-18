package monitoring

import (
	"context"

	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// statusRestrictionBypass is the status word of a suspected restriction bypass notice (DEC-110, IR845).
const statusRestrictionBypass = "restriction_bypass"

// BypassNotices is the equipment scheduler job that tells HQ about a suspected restriction bypass (DEC-110, IR845): a
// restriction_bypass Alert (IR844) that HQ has not been told about gives each HQ member who holds restriction.write one
// critical alert notice about the unit, at the next tick after it opened. escalated_at records that HQ was told (no
// version bump, as for IR784), so it is told once per Alert however many faults the incident has; an Alert already
// acknowledged or resolved by then is not told.
type BypassNotices struct {
	Notify notify.Store
}

// Run tells HQ about the restriction_bypass Alerts not told yet and returns how many it told.
func (job BypassNotices) Run(ctx context.Context, call *ops.Call) (int, error) {
	return tellHQ(ctx, call, job.Notify, hqNotice{
		marking: `UPDATE monitoring.alerts SET escalated_at = $1 WHERE type = 'restriction_bypass' AND status = 'open'
		AND escalated_at IS NULL RETURNING id, unit_id, severity`,
		permission: "restriction.write",
		status:     statusRestrictionBypass,
		reason:     statusRestrictionBypass,
	})
}
