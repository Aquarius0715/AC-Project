package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/modules/assets"
	"github.com/pradita/ac-project/service/internal/modules/control"
	"github.com/pradita/ac-project/service/internal/modules/devices"
	"github.com/pradita/ac-project/service/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/internal/ops"
)

// unitDetails assembles the UnitDetail parts owned by Devices, Control, Restrictions and Maintenance (IR165).
type unitDetails struct{}

func (unitDetails) Detail(ctx context.Context, c *ops.Call, u assets.Unit) (assets.DetailExtras, error) {
	var d assets.DetailExtras
	if cp, ok, err := devices.LoadCapability(ctx, c, u.ModelID, u.CapabilityVersion); err != nil {
		return d, err
	} else if ok {
		d.Capabilities = cp
	}
	// effectiveControlPolicy: the newest restriction that currently restricts the unit (IR46)
	var phase string
	var policy []byte
	err := c.Tx.QueryRow(ctx, `SELECT r.state, r.policy FROM restrictions.restriction_units ru JOIN restrictions.restrictions r ON r.id = ru.restriction_id
		WHERE ru.unit_id = $1 AND r.state IN ('requested','applied','release_requested') ORDER BY r.created_at DESC LIMIT 1`, u.ID).Scan(&phase, &policy)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		d.EffectiveControlPolicy = map[string]any{"state": "unrestricted"}
	case err != nil:
		return d, err
	default:
		d.EffectiveControlPolicy = map[string]any{"state": "restricted", "phase": phase, "policy": json.RawMessage(policy), "reasonKey": "control.restriction_active"}
	}
	// controlAvailability: blocked while any terminal-restriction recovery case of the unit is unresolved (SR26)
	var blocked bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restrictions r, jsonb_array_elements(r.recovery_cases) k
		WHERE k->>'unitId' = $1::text AND k->>'state' <> 'resolved')`, u.ID).Scan(&blocked); err != nil {
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
