package assets

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the assets consumers of other domains' events (IR184): the unit's observed restriction and its
// version stay owned by equipment; billing (restrictions) only publishes what changed. The unit scope's copy of
// maintenance access (accepted Offers, active Assignments) is fed by change-capture events (IR186).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.UnitRestrictionApplied: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.UnitRestriction
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE assets.units SET observed_restriction = $2 WHERE id = $1`, p.UnitID, []byte(p.Observed))
			return err
		},
		events.UnitRestrictionCleared: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.UnitRestriction
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE assets.units SET observed_restriction = NULL WHERE id = $1 AND observed_restriction->>'restrictionId' = $2::text`, p.UnitID, p.RestrictionID)
			return err
		},
		events.RecoveryCasesChanged: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.Units
			if err := e.Decode(&p); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE assets.units SET version = version + 1, updated_at = platform.app_now() WHERE id = ANY($1)`, p.UnitIDs)
			return err
		},
		events.OfferAccessChanged: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.OfferAccess
			if err := e.Decode(&p); err != nil {
				return err
			}
			if !p.Accepted || p.UnitID == nil {
				_, err := tx.Exec(ctx, `DELETE FROM assets.unit_offer_access WHERE tenant_id = $1 AND offer_id = $2`, e.TenantID, p.OfferID)
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO assets.unit_offer_access (tenant_id, offer_id, job_id, unit_id, contractor_org_id, access_valid_from, access_valid_until)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (tenant_id, offer_id) DO UPDATE SET job_id = EXCLUDED.job_id, unit_id = EXCLUDED.unit_id, contractor_org_id = EXCLUDED.contractor_org_id,
				  access_valid_from = EXCLUDED.access_valid_from, access_valid_until = EXCLUDED.access_valid_until`,
				e.TenantID, p.OfferID, p.JobID, *p.UnitID, p.ContractorOrgID, p.AccessValidFrom, p.AccessValidUntil)
			return err
		},
		events.AssignmentAccessChanged: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var p events.AssignmentAccess
			if err := e.Decode(&p); err != nil {
				return err
			}
			if p.Deleted || p.UnitID == nil {
				_, err := tx.Exec(ctx, `DELETE FROM assets.unit_assignment_access WHERE tenant_id = $1 AND assignment_id = $2`, e.TenantID, p.AssignmentID)
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO assets.unit_assignment_access (tenant_id, assignment_id, job_id, unit_id, technician_membership_id, created_at, scheduled, active)
				VALUES ($1, $2, $3, $4, $5, $6, tstzrange($7, $8), $9)
				ON CONFLICT (tenant_id, assignment_id) DO UPDATE SET job_id = EXCLUDED.job_id, unit_id = EXCLUDED.unit_id,
				  technician_membership_id = EXCLUDED.technician_membership_id, created_at = EXCLUDED.created_at, scheduled = EXCLUDED.scheduled, active = EXCLUDED.active`,
				e.TenantID, p.AssignmentID, p.JobID, *p.UnitID, p.TechnicianMembershipID, p.CreatedAt, p.ScheduledFrom, p.ScheduledUntil, p.Active)
			return err
		},
	}
}
