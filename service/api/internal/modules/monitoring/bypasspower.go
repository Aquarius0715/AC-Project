package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// The power check of a power_off restriction (DEC-110, IR846): a unit that keeps drawing more than bypassPowerWatts
// for bypassPowerFor while it observes a power_off restriction is running although the restriction says off. 50 W is
// above an indoor unit's standby and below any cooling, and five minutes are five of the live nodes' minute readings.
const (
	bypassPowerWatts = 50.0
	bypassPowerFor   = 5 * time.Minute
	// bypassPowerGap is how far the first and last readings may lie from the window's ends: one minute reading
	// may be missing at each end, but the readings must cover the window.
	bypassPowerGap = time.Minute
)

// BypassPower is the equipment scheduler job that compares the power readings with power_off restrictions (DEC-110,
// IR846): a unit that observes a power_off restriction since the start of the window, has no unresolved
// restriction_bypass Alert, and whose valid power readings of the last bypassPowerFor all exceed bypassPowerWatts and
// cover the window, gets a restriction_bypass Alert (the same incident key as IR844), told to HQ by BypassNotices. A
// temperature limit lets the AC run, so it is not checked; a unit without power readings (no power meter) is not
// either.
type BypassPower struct{}

// Run opens the Alerts and returns how many it opened.
func (BypassPower) Run(ctx context.Context, call *ops.Call) (int, error) {
	from := call.Now.Add(-bypassPowerFor)

	rows, err := call.Tx.Query(
		ctx,
		`SELECT u.id, u.customer_org_id, min(m.value) FROM assets.units u
		JOIN monitoring.measurements m ON m.unit_id = u.id AND m.metric = 'power' AND m.quality = 'valid'
			AND m.value IS NOT NULL AND m.observed_at > $1 AND m.observed_at <= $2
		WHERE u.observed_restriction->'policy'->>'kind' = 'power_off'
			AND (u.observed_restriction->>'observedAt')::timestamptz <= $1
			AND NOT EXISTS (SELECT 1 FROM monitoring.alerts a WHERE a.unit_id = u.id AND a.policy_id IS NULL
				AND a.type = 'restriction_bypass' AND a.status <> 'resolved')
		GROUP BY u.id, u.customer_org_id
		HAVING min(m.value) > $3 AND min(m.observed_at) <= $1 + make_interval(secs => $4)
			AND max(m.observed_at) >= $2 - make_interval(secs => $4)`,
		from,
		call.Now,
		bypassPowerWatts,
		bypassPowerGap.Seconds(),
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select monitoring.measurements")
	}

	type running struct {
		unit, org uuid.UUID
		least     float64
	}

	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (running, error) {
		var unit running

		return unit, row.Scan(&unit.unit, &unit.org, &unit.least)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select monitoring.measurements")
	}

	for _, unit := range list {
		_, err := openBypass(
			ctx,
			call.Tx,
			unit.unit,
			unit.org,
			fmt.Sprintf(
				"The unit drew more than %.0f W (at least %.0f W) for %.0f minutes while it is under a power_off "+
					"restriction: the air conditioner runs although the restriction says off",
				bypassPowerWatts,
				unit.least,
				bypassPowerFor.Minutes(),
			),
			call.Now,
		)
		if err != nil {
			return 0, err
		}
	}

	return len(list), nil
}
