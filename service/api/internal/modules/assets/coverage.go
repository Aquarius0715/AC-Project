package assets

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// The coverage sources other modules own.
type (
	// ContractReader is billing's view of the units' active contracts.
	ContractReader interface {
		ActiveContracts(
			ctx context.Context,
			c *ops.Call,
			units []uuid.UUID,
		) (map[uuid.UUID][]uuid.UUID, error)
	}
	// ClaimReader is maintenance's view of the units' jobs a warranty claim can name.
	ClaimReader interface {
		ClaimableJobs(
			ctx context.Context,
			c *ops.Call,
			units []uuid.UUID,
		) (map[uuid.UUID]map[uuid.UUID]time.Time, error)
	}
)

// UnitCoverage is UnitCoverage of service-contracts.ts.
type UnitCoverage struct {
	UnitID          uuid.UUID   `json:"unitId"`
	CustomerID      uuid.UUID   `json:"customerId"`
	ModelID         uuid.UUID   `json:"modelId"`
	WarrantyEndsAt  *time.Time  `json:"warrantyEndsAt"`
	ContractIDs     []uuid.UUID `json:"contractIds"`
	Status          string      `json:"status"`
	ClaimableJobIDs []uuid.UUID `json:"claimableJobIds"`
	name            string
}

// CoverageStatus applies IR111: an active contract covers the unit; otherwise under_warranty when
// now < warrantyEndsAt − 90 days, expiring within 90 days, else no_coverage.
func CoverageStatus(now time.Time, warrantyEnds *time.Time, contracts int) string {
	switch {
	case contracts > 0:
		return "contract"
	case warrantyEnds == nil || !now.Before(*warrantyEnds):
		return coverageNone
	case now.Before(warrantyEnds.Add(-90 * 24 * time.Hour)):
		return "under_warranty"
	default:
		return coverageExpiring
	}
}

// claimable keeps the jobs completed while the warranty ran (warrantyEndsAt not before completedAt, the
// jobs.recordWarrantyClaim rule), the earliest completion first.
func claimable(jobs map[uuid.UUID]time.Time, ends *time.Time) []uuid.UUID {
	out := []uuid.UUID{}
	if ends == nil {
		return out
	}

	for id, at := range jobs {
		if !ends.Before(at) {
			out = append(out, id)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		a, b := jobs[out[i]], jobs[out[j]]

		return a.Before(b) || (a.Equal(b) && out[i].String() < out[j].String())
	})

	return out
}

// validCoverage reports whether status is a coverage status (IR111).
func validCoverage(status string) bool {
	switch status {
	case "under_warranty", "contract", coverageExpiring, coverageNone:
		return true
	}

	return false
}

// unitsCoverage serves units.coverage.
//
//	@Summary		units.coverage (read)
//	@ID				units.coverage
//	@Description	Authorization: admin:asset.read | admin:contract.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A19 · Query: filters customerId,coverage,expiringWithinDays,search · sort id,name,dueAt \
//	@Description	(default dueAt asc;id asc)
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			cursor				query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit				query		integer	false	"page size 1–100, default 25"
//	@Param			sort				query		string	false	"field:direction — fields id,name,dueAt; default dueAt asc;id asc"
//	@Param			customerId			query		string	false	"filter → customerId"
//	@Param			coverage			query		string	false	"filter → status"
//	@Param			expiringWithinDays	query		number	false	"filter → warrantyEndsAt within N days of now"
//	@Param			search				query		string	false	"filter → unit name or ID contains"
//	@Success		200					{object}	ops.Envelope{data=UnitCoveragePage}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units/coverage [get]
func (m *Module) unitsCoverage(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[UnitCoverage], error) {
	var filters coverageFilters

	err := decodeFilters(query.Filters, &filters)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}

	err = filters.check(query.Sort)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[UnitCoverage]{}, apperr.Wrap(err, "paging.Resolve")
	}

	all, err := coverageRows(ctx, call, filters)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}

	keep, err := m.withCoverage(ctx, call, all, filters)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}

	sortCoverage(keep, query.Sort)

	end := min(window.Offset+window.Limit, len(keep))

	items := []UnitCoverage{}
	if window.Offset < len(keep) {
		items = keep[window.Offset:end]
	}

	return paging.Page[UnitCoverage]{
		Items:           items,
		NextCursor:      window.Next(len(keep)),
		Total:           len(keep),
		SnapshotVersion: window.Snapshot,
	}, nil
}

// coverageFilters are the filters of units.coverage.
type coverageFilters struct {
	CustomerID         *uuid.UUID `json:"customerId,omitempty"`
	Coverage           *string    `json:"coverage,omitempty"`
	ExpiringWithinDays *int       `json:"expiringWithinDays,omitempty"`
	Search             *string    `json:"search,omitempty"`
}

// check refuses a coverage status that is not one, an expiry window outside 1–3650 days, and a sort by another field
// than id, name or dueAt.
func (filters coverageFilters) check(sortBy *paging.SortSpec) error {
	if filters.Coverage != nil && !validCoverage(*filters.Coverage) {
		return apperr.Fields(
			map[string]string{"filters.coverage": apperr.KeyInvalid},
		)
	}

	if filters.ExpiringWithinDays != nil &&
		(*filters.ExpiringWithinDays < 1 || *filters.ExpiringWithinDays > 3650) {
		return apperr.Fields(
			map[string]string{"filters.expiringWithinDays": apperr.KeyRange},
		)
	}

	if sortBy != nil && sortBy.Field != "id" && sortBy.Field != keyName &&
		sortBy.Field != keyDueAt {
		return apperr.Fields(
			map[string]string{"sort.field": apperr.KeyInvalid},
		)
	}

	return nil
}

// keeps reports whether a unit's coverage passes the status filter and, with an expiry window, whether its warranty
// ends within the window from now.
func (filters coverageFilters) keeps(coverage UnitCoverage, now time.Time) bool {
	if filters.Coverage != nil && coverage.Status != *filters.Coverage {
		return false
	}

	return filters.ExpiringWithinDays == nil ||
		coverage.WarrantyEndsAt != nil && !coverage.WarrantyEndsAt.Before(now) &&
			!coverage.WarrantyEndsAt.After(
				now.Add(time.Duration(*filters.ExpiringWithinDays)*24*time.Hour),
			)
}

// coverageRows are the units the filters name, with their customer, model and warranty end.
func coverageRows(
	ctx context.Context,
	call *ops.Call,
	filters coverageFilters,
) ([]UnitCoverage, error) {
	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{unarchivedUnits}
	if filters.CustomerID != nil {
		conds = append(conds, "cu.id = "+add(*filters.CustomerID))
	}

	if filters.Search != nil && strings.TrimSpace(*filters.Search) != "" {
		q := paging.Contains(strings.ToLower(*filters.Search))
		conds = append(
			conds,
			"(lower(u.display_name) LIKE "+add(q)+" OR u.id::text LIKE "+add(q)+")",
		)
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT u.id, cu.id, u.model_id, u.warranty_ends_at, u.display_name FROM assets.units u
		JOIN assets.customers cu ON cu.organization_id = u.customer_org_id WHERE `+strings.Join(conds, " AND "),
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.units")
	}

	var all []UnitCoverage

	for rows.Next() {
		var unitCoverage UnitCoverage

		err := rows.Scan(
			&unitCoverage.UnitID,
			&unitCoverage.CustomerID,
			&unitCoverage.ModelID,
			&unitCoverage.WarrantyEndsAt,
			&unitCoverage.name,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select assets.units")
		}

		all = append(all, unitCoverage)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select assets.units")
	}

	return all, nil
}

// withCoverage gives each unit its active contracts, its jobs a warranty claim can name and its coverage status
// (IR111), and keeps the ones the filters pass.
func (m *Module) withCoverage(
	ctx context.Context,
	call *ops.Call,
	all []UnitCoverage,
	filters coverageFilters,
) ([]UnitCoverage, error) {
	ids := make([]uuid.UUID, len(all))
	for i := range all {
		ids[i] = all[i].UnitID
	}

	contracts, err := m.Contracts.ActiveContracts(ctx, call, ids)
	if err != nil {
		return nil, apperr.Wrap(
			err,
			"assets.ContractReader.ActiveContracts",
		)
	}

	claims, err := m.Claims.ClaimableJobs(ctx, call, ids)
	if err != nil {
		return nil, apperr.Wrap(err, "assets.ClaimReader.ClaimableJobs")
	}

	var keep []UnitCoverage

	for _, unitCoverage := range all {
		unitCoverage.ContractIDs = append([]uuid.UUID{}, contracts[unitCoverage.UnitID]...)
		unitCoverage.ClaimableJobIDs = claimable(
			claims[unitCoverage.UnitID],
			unitCoverage.WarrantyEndsAt,
		)

		unitCoverage.Status = CoverageStatus(
			call.Now,
			unitCoverage.WarrantyEndsAt,
			len(unitCoverage.ContractIDs),
		)
		if !filters.keeps(unitCoverage, call.Now) {
			continue
		}

		keep = append(keep, unitCoverage)
	}

	return keep, nil
}

// sortCoverage orders the units by the sort field: dueAt (the warranty end, none last in both directions) unless id
// or name is named; ties go by unit ID.
func sortCoverage(units []UnitCoverage, sortBy *paging.SortSpec) {
	desc := sortBy != nil && sortBy.Direction == "desc"

	field := keyDueAt
	if sortBy != nil {
		field = sortBy.Field
	}

	sort.SliceStable(units, func(i, j int) bool {
		unitCoverage, otherCoverage := units[i], units[j]
		less, equal := false, false

		switch field {
		case keyName:
			less, equal = unitCoverage.name < otherCoverage.name, unitCoverage.name == otherCoverage.name
		case keyDueAt: // warrantyEndsAt, null last
			switch {
			case unitCoverage.WarrantyEndsAt == nil || otherCoverage.WarrantyEndsAt == nil:
				less, equal = otherCoverage.WarrantyEndsAt == nil &&
					unitCoverage.WarrantyEndsAt != nil, unitCoverage.WarrantyEndsAt == nil &&
					otherCoverage.WarrantyEndsAt == nil
				if !equal {
					return less // nulls stay last in both directions
				}
			default:
				less, equal = unitCoverage.WarrantyEndsAt.Before(
					*otherCoverage.WarrantyEndsAt,
				), unitCoverage.WarrantyEndsAt.Equal(
					*otherCoverage.WarrantyEndsAt,
				)
			}
		}

		if field == "id" || equal {
			return unitCoverage.UnitID.String() < otherCoverage.UnitID.String()
		}

		return less != desc
	})
}
