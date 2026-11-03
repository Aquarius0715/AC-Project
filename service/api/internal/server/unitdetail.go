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
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// unitDetails assembles the UnitDetail parts owned by Devices, Control, Restrictions and Maintenance (IR165); the
// restriction parts come from billing's queries.
type unitDetails struct{}

func (unitDetails) Detail(
	ctx context.Context,
	call *ops.Call,
	unit assets.Unit,
) (assets.DetailExtras, error) {
	var detailExtras assets.DetailExtras

	cp, ok, err := devices.LoadCapability(ctx, call, unit.ModelID, unit.CapabilityVersion)
	if err != nil {
		return detailExtras, apperr.Wrap(err, "devices.LoadCapability")
	} else if ok {
		detailExtras.Capabilities = cp
	}

	err = restrictionParts(ctx, call, unit.ID, &detailExtras)
	if err != nil {
		return detailExtras, err
	}

	detailExtras.Components = maintenance.Components(unit.ServiceScope)

	cmds, err := control.PendingCommands(ctx, call, unit.ID)
	if err != nil {
		return detailExtras, apperr.Wrap(err, "control.PendingCommands")
	}

	detailExtras.PendingCommands, detailExtras.PendingCommandIDs = cmds, []uuid.UUID{}
	for _, x := range cmds {
		detailExtras.PendingCommandIDs = append(detailExtras.PendingCommandIDs, x.ID)
	}
	// location: property > ancestor spaces > space, with the property address and access instructions
	detailExtras.Location.PathLabels = []string{}

	scanErr := call.Tx.QueryRow(ctx, `WITH RECURSIVE up(id, name, parent, depth) AS (
			SELECT s.id, s.name, s.parent_space_id, 0 FROM assets.spaces s WHERE s.id = $2
			UNION ALL SELECT a.id, a.name, a.parent_space_id, up.depth + 1 FROM up JOIN assets.spaces a ON a.id = up.parent)
		SELECT array_prepend(p.name, COALESCE((SELECT array_agg(name ORDER BY depth DESC) FROM up), '{}')), p.address,
		p.access_instructions
		FROM assets.properties p WHERE p.id =
		$1`, unit.PropertyID, unit.SpaceID).Scan(&detailExtras.Location.PathLabels, &detailExtras.Location.Address,
		&detailExtras.Location.AccessInstructions)
	if scanErr != nil {
		return detailExtras, apperr.Wrap(scanErr, "select assets.properties")
	}

	return detailExtras, nil
}

// restrictionParts sets the unit's effectiveControlPolicy and controlAvailability; restrictions belong to billing and
// are asked through its queries (IR194).
func restrictionParts(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	extras *assets.DetailExtras,
) error {
	// effectiveControlPolicy: the newest restriction that currently restricts the unit (IR46)
	unitRestriction, err := restrictions.Busy{}.UnitRestriction(ctx, call, unit)
	switch {
	case err != nil:
		return apperr.Wrap(err, "restrictions.Busy.UnitRestriction")
	case !unitRestriction.Found:
		extras.EffectiveControlPolicy = map[string]any{keyState: "unrestricted"}
	default:
		extras.EffectiveControlPolicy = map[string]any{
			keyState:    "restricted",
			"phase":     unitRestriction.Phase,
			"policy":    unitRestriction.Policy,
			"reasonKey": "control.restriction_active",
		}
	}
	// controlAvailability: blocked while any terminal-restriction recovery case of the unit is unresolved (SR26)
	blocked, err := restrictions.Busy{}.UnitRecovering(ctx, call, unit)
	if err != nil {
		return apperr.Wrap(err, "restrictions.Busy.UnitRecovering")
	}

	extras.ControlAvailability = map[string]any{keyState: "available"}
	if blocked {
		extras.ControlAvailability = map[string]any{
			keyState:    "blocked",
			"reasonKey": "control.reconciliation_required",
		}
	}

	return nil
}
