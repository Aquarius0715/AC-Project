package devices

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/ops"
)

// Usage reports whether a unit has IoT history (any device binding), which blocks physical deletion (DD-A02).
type Usage struct{}

// UnitInUse implements assets.UnitUsage.
func (Usage) UnitInUse(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_bindings WHERE unit_id = $1)`, unit).Scan(&b)
	return b, err
}

// UnitActive reports an active binding or an in-flight device operation on the unit (blocks units.archive, D05).
func (Usage) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_bindings WHERE unit_id = $1 AND unbound_at IS NULL)
		OR EXISTS (SELECT 1 FROM devices.device_operations WHERE unit_id = $1 AND status IN ('queued','running') AND expires_at > $2)`, unit, c.Now).Scan(&b)
	return b, err
}
