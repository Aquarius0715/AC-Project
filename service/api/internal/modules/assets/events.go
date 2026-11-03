package assets

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// EventHandlers are the assets consumers of other domains' events (IR184): the unit's observed restriction and its
// version stay owned by equipment; billing (restrictions) only publishes what changed. The unit scope's copy of
// maintenance access (accepted Offers, active Assignments) is fed by change-capture events (IR186).
func EventHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.UnitRestrictionApplied: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var unitRestriction events.UnitRestriction

			decodeErr := e.Decode(&unitRestriction)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			_, err := txn.Exec(
				ctx,
				`UPDATE assets.units SET observed_restriction = $2 WHERE id = $1`,
				unitRestriction.UnitID,
				[]byte(unitRestriction.Observed),
			)

			return apperr.Wrap(err, "update assets.units")
		},
		events.UnitRestrictionCleared: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var unitRestriction events.UnitRestriction

			decodeErr := e.Decode(&unitRestriction)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			_, err := txn.Exec(
				ctx,
				`UPDATE assets.units SET observed_restriction = NULL WHERE id = $1 AND observed_restriction->>'restrictionId' =
					$2::text`,
				unitRestriction.UnitID,
				unitRestriction.RestrictionID,
			)

			return apperr.Wrap(err, "update assets.units")
		},
		events.RecoveryCasesChanged: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var units events.Units

			decodeErr := e.Decode(&units)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			_, err := txn.Exec(
				ctx,
				`UPDATE assets.units SET version = version + 1, updated_at = platform.app_now() WHERE id = ANY($1)`,
				units.UnitIDs,
			)

			return apperr.Wrap(err, "update assets.units")
		},
		events.OfferAccessChanged: func(ctx context.Context, tx pgx.Tx, event events.Event) error {
			var offerAccess events.OfferAccess

			decodeErr := event.Decode(&offerAccess)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			if !offerAccess.Accepted || offerAccess.UnitID == nil {
				_, err := tx.Exec(
					ctx,
					`DELETE FROM assets.unit_offer_access WHERE tenant_id = $1 AND offer_id = $2`,
					event.TenantID,
					offerAccess.OfferID,
				)

				return apperr.Wrap(err, "delete assets.unit_offer_access")
			}

			_, err := tx.Exec(
				ctx,
				`INSERT INTO assets.unit_offer_access (tenant_id, offer_id, job_id, unit_id, contractor_org_id, access_valid_from,
					access_valid_until)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (tenant_id, offer_id) DO UPDATE SET job_id = EXCLUDED.job_id, unit_id = EXCLUDED.unit_id,
				contractor_org_id = EXCLUDED.contractor_org_id,
				  access_valid_from = EXCLUDED.access_valid_from, access_valid_until = EXCLUDED.access_valid_until`,
				event.TenantID,
				offerAccess.OfferID,
				offerAccess.JobID,
				*offerAccess.UnitID,
				offerAccess.ContractorOrgID,
				offerAccess.AccessValidFrom,
				offerAccess.AccessValidUntil,
			)

			return apperr.Wrap(err, "insert assets.unit_offer_access")
		},
		events.AssignmentAccessChanged: func(ctx context.Context, txn pgx.Tx, e events.Event) error {
			var assignmentAccess events.AssignmentAccess

			decodeErr := e.Decode(&assignmentAccess)
			if decodeErr != nil {
				return apperr.Wrap(decodeErr, "events.Event.Decode")
			}

			if assignmentAccess.Deleted || assignmentAccess.UnitID == nil {
				_, err := txn.Exec(
					ctx,
					`DELETE FROM assets.unit_assignment_access WHERE tenant_id = $1 AND assignment_id = $2`,
					e.TenantID,
					assignmentAccess.AssignmentID,
				)

				return apperr.Wrap(err, "delete assets.unit_assignment_access")
			}

			_, err := txn.Exec(
				ctx,
				`INSERT INTO assets.unit_assignment_access (tenant_id, assignment_id, job_id, unit_id, technician_membership_id,
					created_at, scheduled, active)
				VALUES ($1, $2, $3, $4, $5, $6, tstzrange($7, $8), $9)
				ON CONFLICT (tenant_id, assignment_id) DO UPDATE SET job_id = EXCLUDED.job_id, unit_id = EXCLUDED.unit_id,
				  technician_membership_id = EXCLUDED.technician_membership_id, created_at = EXCLUDED.created_at, scheduled =
				  EXCLUDED.scheduled, active = EXCLUDED.active`,
				e.TenantID,
				assignmentAccess.AssignmentID,
				assignmentAccess.JobID,
				*assignmentAccess.UnitID,
				assignmentAccess.TechnicianMembershipID,
				assignmentAccess.CreatedAt,
				assignmentAccess.ScheduledFrom,
				assignmentAccess.ScheduledUntil,
				assignmentAccess.Active,
			)

			return apperr.Wrap(err, "insert assets.unit_assignment_access")
		},
	}
}
