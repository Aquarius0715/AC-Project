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

// Coverage sources owned by other modules.
type (
	ContractReader interface {
		ActiveContracts(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	}
	ClaimReader interface {
		ClaimableJobs(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]map[uuid.UUID]time.Time, error)
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
		return "no_coverage"
	case now.Before(warrantyEnds.Add(-90 * 24 * time.Hour)):
		return "under_warranty"
	default:
		return "expiring"
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

var coverageStatuses = map[string]bool{"under_warranty": true, "contract": true, "expiring": true, "no_coverage": true}

// @Summary		units.coverage (read)
// @ID				units.coverage
// @Description	Authorization: admin:asset.read | admin:contract.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A19 · Query: filters customerId,coverage,expiringWithinDays,search · sort id,name,dueAt (default dueAt asc;id asc)
// @Tags			units
// @Accept			json
// @Produce		json
// @Param			cursor				query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit				query		integer	false	"page size 1–100, default 25"
// @Param			sort				query		string	false	"field:direction — fields id,name,dueAt; default dueAt asc;id asc"
// @Param			customerId			query		string	false	"filter → customerId"
// @Param			coverage			query		string	false	"filter → status"
// @Param			expiringWithinDays	query		number	false	"filter → warrantyEndsAt within N days of now"
// @Param			search				query		string	false	"filter → unit name or ID contains"
// @Success		200					{object}	ops.Envelope{data=UnitCoveragePage}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/units/coverage [get]
func (m *Module) unitsCoverage(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[UnitCoverage], error) {
	var f struct {
		CustomerID         *uuid.UUID `json:"customerId,omitempty"`
		Coverage           *string    `json:"coverage,omitempty"`
		ExpiringWithinDays *int       `json:"expiringWithinDays,omitempty"`
		Search             *string    `json:"search,omitempty"`
	}
	if err := decodeFilters(in.Filters, &f); err != nil {
		return paging.Page[UnitCoverage]{}, err
	}
	if f.Coverage != nil && !coverageStatuses[*f.Coverage] {
		return paging.Page[UnitCoverage]{}, apperr.Fields(map[string]string{"filters.coverage": "error.invalid"})
	}
	if f.ExpiringWithinDays != nil && (*f.ExpiringWithinDays < 1 || *f.ExpiringWithinDays > 3650) {
		return paging.Page[UnitCoverage]{}, apperr.Fields(map[string]string{"filters.expiringWithinDays": "error.range"})
	}
	if in.Sort != nil && in.Sort.Field != "id" && in.Sort.Field != "name" && in.Sort.Field != "dueAt" {
		return paging.Page[UnitCoverage]{}, apperr.Fields(map[string]string{"sort.field": "error.invalid"})
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"NOT u.archived"}
	if f.CustomerID != nil {
		conds = append(conds, "cu.id = "+add(*f.CustomerID))
	}
	if f.Search != nil && strings.TrimSpace(*f.Search) != "" {
		q := "%" + strings.ToLower(strings.TrimSpace(*f.Search)) + "%"
		conds = append(conds, "(lower(u.display_name) LIKE "+add(q)+" OR u.id::text LIKE "+add(q)+")")
	}
	rows, err := c.Tx.Query(ctx, `SELECT u.id, cu.id, u.model_id, u.warranty_ends_at, u.display_name FROM assets.units u
		JOIN assets.customers cu ON cu.organization_id = u.customer_org_id WHERE `+strings.Join(conds, " AND "), args...)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}
	var all []UnitCoverage
	for rows.Next() {
		var x UnitCoverage
		if err := rows.Scan(&x.UnitID, &x.CustomerID, &x.ModelID, &x.WarrantyEndsAt, &x.name); err != nil {
			rows.Close()
			return paging.Page[UnitCoverage]{}, err
		}
		all = append(all, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[UnitCoverage]{}, err
	}
	ids := make([]uuid.UUID, len(all))
	for i := range all {
		ids[i] = all[i].UnitID
	}
	contracts, err := m.Contracts.ActiveContracts(ctx, c, ids)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}
	claims, err := m.Claims.ClaimableJobs(ctx, c, ids)
	if err != nil {
		return paging.Page[UnitCoverage]{}, err
	}
	var keep []UnitCoverage
	for _, x := range all {
		x.ContractIDs = append([]uuid.UUID{}, contracts[x.UnitID]...)
		x.ClaimableJobIDs = claimable(claims[x.UnitID], x.WarrantyEndsAt)
		x.Status = CoverageStatus(c.Now, x.WarrantyEndsAt, len(x.ContractIDs))
		if f.Coverage != nil && x.Status != *f.Coverage {
			continue
		}
		if f.ExpiringWithinDays != nil && (x.WarrantyEndsAt == nil || x.WarrantyEndsAt.Before(c.Now) ||
			x.WarrantyEndsAt.After(c.Now.Add(time.Duration(*f.ExpiringWithinDays)*24*time.Hour))) {
			continue
		}
		keep = append(keep, x)
	}
	desc := in.Sort != nil && in.Sort.Direction == "desc"
	field := "dueAt"
	if in.Sort != nil {
		field = in.Sort.Field
	}
	sort.SliceStable(keep, func(i, j int) bool {
		a, b := keep[i], keep[j]
		less, eq := false, false
		switch field {
		case "name":
			less, eq = a.name < b.name, a.name == b.name
		case "dueAt": // warrantyEndsAt, null last
			switch {
			case a.WarrantyEndsAt == nil || b.WarrantyEndsAt == nil:
				less, eq = b.WarrantyEndsAt == nil && a.WarrantyEndsAt != nil, a.WarrantyEndsAt == nil && b.WarrantyEndsAt == nil
				if !eq {
					return less // nulls stay last in both directions
				}
			default:
				less, eq = a.WarrantyEndsAt.Before(*b.WarrantyEndsAt), a.WarrantyEndsAt.Equal(*b.WarrantyEndsAt)
			}
		}
		if field == "id" || eq {
			return a.UnitID.String() < b.UnitID.String()
		}
		return less != desc
	})
	end := min(w.Offset+w.Limit, len(keep))
	items := []UnitCoverage{}
	if w.Offset < len(keep) {
		items = keep[w.Offset:end]
	}
	return paging.Page[UnitCoverage]{Items: items, NextCursor: w.Next(len(keep)), Total: len(keep), SnapshotVersion: w.Snapshot}, nil
}
