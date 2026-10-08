package assets

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/ops"
)

// UnitCounts is the equipment part of Summary.counts / AdminSummary (IR51, SR27).
type UnitCounts struct {
	Total, Online, Offline, Unknown int
	PowerOn, PowerOff, PowerUnknown int
	AlertCount                      int
}

// CountUnits counts the not archived units matching the conditions: in the caller's scope (scoped) or exactly the
// given IDs (already authorized by the caller, e.g. technician job projections). connection online/offline count
// as such, every other connection as unknown; power uses the effective power state; alertCount sums unresolved
// critical/warning alerts.
func (m *Module) CountUnits(ctx context.Context, c *ops.Call, scoped bool, ids *[]uuid.UUID, customer, property, unit *uuid.UUID) (UnitCounts, error) {
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"NOT u.archived"}
	if scoped {
		conds = append(conds, scopeSQL(c, &args))
	}
	if ids != nil {
		conds = append(conds, "u.id = ANY("+add(*ids)+")")
	}
	if customer != nil {
		conds = append(conds, "u.customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = "+add(*customer)+")")
	}
	if property != nil {
		conds = append(conds, "u.property_id = "+add(*property))
	}
	if unit != nil {
		conds = append(conds, "u.id = "+add(*unit))
	}
	units, _, err := m.query(ctx, c, strings.Join(conds, " AND "), args, "u.id ASC", 0, 0)
	if err != nil {
		return UnitCounts{}, err
	}
	if err := m.enrich(ctx, c, units); err != nil {
		return UnitCounts{}, err
	}
	var n UnitCounts
	for _, u := range units {
		n.Total++
		switch u.Connection {
		case "online":
			n.Online++
		case "offline":
			n.Offline++
		default:
			n.Unknown++
		}
		switch u.EffectivePowerState {
		case "on":
			n.PowerOn++
		case "off":
			n.PowerOff++
		default:
			n.PowerUnknown++
		}
		n.AlertCount += u.ActiveAlertCount
	}
	return n, nil
}
