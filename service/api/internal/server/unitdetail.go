package server

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/assets"
	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// unitDetails assembles the UnitDetail parts owned by Devices, Control, Restrictions and Maintenance (IR165); the
// restriction parts come from billing's queries.
type unitDetails struct{}

func (unitDetails) Detail(ctx context.Context, c *ops.Call, u assets.Unit) (assets.DetailExtras, error) {
	var d assets.DetailExtras
	if cp, ok, err := devices.LoadCapability(ctx, c, u.ModelID, u.CapabilityVersion); err != nil {
		return d, err
	} else if ok {
		d.Capabilities = cp
	}
	// effectiveControlPolicy: the newest restriction that currently restricts the unit (IR46)
	// (restrictions belong to billing: asked through its queries, IR194)
	r, err := restrictions.Busy{}.UnitRestriction(ctx, c, u.ID)
	switch {
	case err != nil:
		return d, err
	case !r.Found:
		d.EffectiveControlPolicy = map[string]any{"state": "unrestricted"}
	default:
		d.EffectiveControlPolicy = map[string]any{"state": "restricted", "phase": r.Phase, "policy": r.Policy, "reasonKey": "control.restriction_active"}
	}
	// controlAvailability: blocked while any terminal-restriction recovery case of the unit is unresolved (SR26)
	blocked, err := restrictions.Busy{}.UnitRecovering(ctx, c, u.ID)
	if err != nil {
		return d, err
	}
	d.ControlAvailability = map[string]any{"state": "available"}
	if blocked {
		d.ControlAvailability = map[string]any{"state": "blocked", "reasonKey": "control.reconciliation_required"}
	}
	d.Components = maintenance.Components(u.ServiceScope)
	cmds, err := control.PendingCommands(ctx, c, u.ID)
	if err != nil {
		return d, err
	}
	d.PendingCommands, d.PendingCommandIDs = cmds, []uuid.UUID{}
	for _, x := range cmds {
		d.PendingCommandIDs = append(d.PendingCommandIDs, x.ID)
	}
	// location: property > ancestor spaces > space, with the property address and access instructions
	d.Location.PathLabels = []string{}
	if err := c.Tx.QueryRow(ctx, `WITH RECURSIVE up(id, name, parent, depth) AS (
			SELECT s.id, s.name, s.parent_space_id, 0 FROM assets.spaces s WHERE s.id = $2
			UNION ALL SELECT a.id, a.name, a.parent_space_id, up.depth + 1 FROM up JOIN assets.spaces a ON a.id = up.parent)
		SELECT array_prepend(p.name, COALESCE((SELECT array_agg(name ORDER BY depth DESC) FROM up), '{}')), p.address, p.access_instructions
		FROM assets.properties p WHERE p.id = $1`, u.PropertyID, u.SpaceID).Scan(&d.Location.PathLabels, &d.Location.Address, &d.Location.AccessInstructions); err != nil {
		return d, err
	}
	return d, nil
}
