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
		ClaimableJobs(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
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

var coverageStatuses = map[string]bool{"under_warranty": true, "contract": true, "expiring": true, "no_coverage": true}

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
		x.ClaimableJobIDs = append([]uuid.UUID{}, claims[x.UnitID]...)
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
