package control

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// The bridge records one sample per minute. Require the complete minute immediately before the request's minute and
// the first complete minute after the send's minute, rejecting gaps, duplicates and receipts outside their slots.
// An ACK that supplies sent_at is a conservative time bound because the command reached the device by then.
const confirmedManualStartsSQL = `WITH manual AS (
	SELECT c.id, c.tenant_id, c.unit_id, c.device_id, c.requested_at, c.sent_at, b.id AS binding_id, b.bound_at,
		date_trunc('minute', c.requested_at) - interval '1 minute' AS before_slot,
		date_trunc('minute', c.sent_at) + interval '1 minute' AS after_slot
	FROM devices.device_bindings b
	JOIN devices.devices d ON d.tenant_id = b.tenant_id AND d.id = b.device_id
		AND d.unit_id = b.unit_id AND d.binding_id = b.id
	JOIN control.commands c ON c.tenant_id = b.tenant_id AND c.unit_id = b.unit_id AND c.device_id = b.device_id
	WHERE b.unit_id = $1 AND b.unbound_at IS NULL AND b.customer_org_id = $5
		AND c.actor_membership_id = $2 AND c.requested_at >= $3 AND c.requested_at < $4
		AND c.source = 'ui' AND c.status = 'acknowledged' AND c.acknowledged_at <= $4
		AND c.action @> '{"kind":"set_power","power":true}'
		AND c.sent_at >= c.requested_at AND c.sent_at <= c.acknowledged_at
)
SELECT c.requested_at FROM manual c
LEFT JOIN control.start_power_bands bands ON bands.tenant_id = c.tenant_id AND bands.binding_id = c.binding_id
	AND bands.verified_from <= c.before_slot
LEFT JOIN LATERAL (
	SELECT true AS confirmed FROM monitoring.measurements m
	JOIN devices.sensors s ON s.tenant_id = c.tenant_id AND s.id = bands.sensor_id AND s.device_id = c.device_id
		AND s.metric = 'power' AND s.unit = 'kW' AND s.boundary_id = 'ac_input_electricity'
	WHERE m.tenant_id = c.tenant_id AND m.unit_id = c.unit_id AND m.sensor_id = s.id AND m.metric = 'power'
		AND m.observed_at IN (c.before_slot, c.after_slot) AND c.before_slot >= c.bound_at
		AND c.after_slot + interval '1 minute' <= $4 AND c.after_slot <= c.requested_at + interval '5 minutes'
	HAVING count(*) = 2 AND count(DISTINCT m.observed_at) = 2 AND bool_and(
		m.origin = 'measured' AND m.quality = 'valid' AND m.unit = 'kW'
		AND m.boundary_id IS NOT DISTINCT FROM s.boundary_id
		AND m.received_at >= m.observed_at AND m.received_at < m.observed_at + interval '1 minute'
		AND m.value IS NOT NULL AND m.value >= 0 AND m.value < 'Infinity'::double precision
		AND CASE WHEN m.observed_at = c.before_slot THEN m.value <= bands.standby_max_kw
			ELSE m.value >= bands.running_min_kw END
	)
) measured ON bands.binding_id IS NOT NULL
LEFT JOIN LATERAL (
	SELECT p.observed_at, p.power FROM control.power_observations p
	WHERE p.tenant_id = c.tenant_id AND p.binding_id = c.binding_id AND p.received_at <= $4
		AND p.observed_at >= c.bound_at
		AND p.observed_at >= c.requested_at - interval '5 minutes' AND p.observed_at < c.requested_at
	ORDER BY p.observed_at DESC LIMIT 1
) pre ON bands.binding_id IS NULL
LEFT JOIN LATERAL (
	SELECT p.observed_at, p.power FROM control.power_observations p
	WHERE p.tenant_id = c.tenant_id AND p.binding_id = c.binding_id AND p.received_at <= $4
		AND p.observed_at > c.requested_at AND p.observed_at <= c.requested_at + interval '5 minutes'
	ORDER BY p.observed_at LIMIT 1
) post ON bands.binding_id IS NULL
WHERE (measured.confirmed IS TRUE OR (pre.power IS FALSE AND post.power IS TRUE AND c.sent_at < post.observed_at))
	AND NOT EXISTS (
		SELECT 1 FROM control.commands other
		WHERE other.tenant_id = c.tenant_id AND other.unit_id = c.unit_id AND other.id <> c.id
			AND (other.action->>'kind' = 'set_power' OR other.source = 'restriction')
			AND (
				other.requested_at BETWEEN COALESCE(pre.observed_at, c.before_slot)
					AND COALESCE(post.observed_at, c.after_slot + interval '1 minute')
				OR other.sent_at BETWEEN COALESCE(pre.observed_at, c.before_slot)
					AND COALESCE(post.observed_at, c.after_slot + interval '1 minute')
			)
	)
ORDER BY c.requested_at`

// confirmedManualStarts uses verified ranges on the current binding to interpret metered power. Existing independent
// state observations still supply evidence for commands outside a meter profile's period. The two sources are never
// combined into one transition.
func confirmedManualStarts(
	ctx context.Context,
	call *ops.Call,
	rule Automation,
	since time.Time,
) ([]time.Time, error) {
	rows, err := call.Tx.Query(
		ctx,
		confirmedManualStartsSQL,
		rule.UnitIDs[0],
		rule.OwnerMembershipID,
		since,
		call.Now,
		rule.customerOrg,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select confirmed manual starts")
	}

	starts, err := pgx.CollectRows(rows, pgx.RowTo[time.Time])

	return starts, apperr.Wrap(err, "read confirmed manual starts")
}
