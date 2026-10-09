package energy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Range is Range.
type Range struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Baseline is EnergyBaseline of service-contracts.ts.
type Baseline struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    uuid.UUID       `json:"tenantId"`
	Version     int             `json:"version"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	UnitIDs     []uuid.UUID     `json:"unitIds"`
	Period      Range           `json:"period"`
	Method      string          `json:"method"`
	BaselineKWh *float64        `json:"baselineKWh"`
	Quality     json.RawMessage `json:"quality" swaggertype:"object"`
	BoundaryID  string          `json:"boundaryId"`
	Boundary    string          `json:"boundary"`
	Assumptions string          `json:"assumptions"`
	Source      string          `json:"source"`
}

const baselineCols = `b.id, b.tenant_id, b.version, (SELECT min(created_at) FROM energy.baselines o WHERE o.id = b.id), b.created_at, b.unit_ids, lower(b.period), upper(b.period),
	b.method, b.baseline_kwh, b.quality, b.boundary_id, b.boundary, b.assumptions, b.source`

func scanBaseline(r pgx.Row) (Baseline, error) {
	var x Baseline
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.UnitIDs, &x.Period.From, &x.Period.To, &x.Method, &x.BaselineKWh, &x.Quality,
		&x.BoundaryID, &x.Boundary, &x.Assumptions, &x.Source)
	slices.SortFunc(x.UnitIDs, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return x, err
}

// clientUnits restricts clients to baselines whose units all belong to their organization (IR90).
func clientUnits(c *ops.Call, args *[]any) string {
	if c.Principal.Role != "client" {
		return "TRUE"
	}
	*args = append(*args, c.Principal.OrgID)
	return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM unnest(b.unit_ids) uid LEFT JOIN energy.ref_units u ON u.id = uid WHERE u.customer_org_id IS DISTINCT FROM $%d)", len(*args))
}

// LoadBaseline returns a readable baseline version (current when version is nil).
func LoadBaseline(ctx context.Context, c *ops.Call, id uuid.UUID, version *int) (Baseline, error) {
	args := []any{id}
	q := "SELECT " + baselineCols + " FROM energy.baselines b WHERE b.id = $1 AND "
	if version != nil {
		args = append(args, *version)
		q += "b.version = $2"
	} else {
		q += "b.is_current"
	}
	q += " AND " + clientUnits(c, &args)
	x, err := scanBaseline(c.Tx.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, err
}

// @Summary		baselines.list (read)
// @ID				baselines.list
// @Description	Authorization: client:self | admin:energy.read | admin:mrv.read; IR90 client only when all unitIds in scope
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A13, DD-C06, DD-A14 · Query: filters unitId,unitIds,from,to,customerId,propertyId,method,boundaryId · sort id,createdAt,updatedAt,periodFrom (default createdAt desc;id desc)
// @Tags			baselines
// @Accept			json
// @Produce		json
// @Param			request	body		paging.Query	true	"input"
// @Success		200		{object}	ops.Envelope{data=BaselinePage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/baselines.list [post]
func listBaselines(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Baseline], error) {
	var f struct {
		UnitID     *uuid.UUID   `json:"unitId,omitempty"`
		UnitIDs    *[]uuid.UUID `json:"unitIds,omitempty"` // the unit sets intersect
		From       *time.Time   `json:"from,omitempty"`    // [from, to) on period.from
		To         *time.Time   `json:"to,omitempty"`
		CustomerID *uuid.UUID   `json:"customerId,omitempty"` // any unit belongs to the customer
		PropertyID *uuid.UUID   `json:"propertyId,omitempty"` // any unit is in the property
		Method     *string      `json:"method,omitempty"`
		BoundaryID *string      `json:"boundaryId,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Method != nil && !slices.Contains(methods, *f.Method)) || (f.BoundaryID != nil && !slices.Contains(boundaries, *f.BoundaryID)) {
			return paging.Page[Baseline]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
		if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
			return paging.Page[Baseline]{}, apperr.Fields(map[string]string{"filters.to": "error.range"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "b.id", "createdAt": "b.created_at", "updatedAt": "b.created_at", "periodFrom": "lower(b.period)"}, "b.created_at DESC, b.id DESC")
	if err != nil {
		return paging.Page[Baseline]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Baseline]{}, err
	}
	args := []any{}
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"b.is_current", clientUnits(c, &args)}
	if f.UnitID != nil {
		conds = append(conds, add(*f.UnitID)+" = ANY(b.unit_ids)")
	}
	if f.UnitIDs != nil {
		conds = append(conds, "b.unit_ids && "+add(*f.UnitIDs)+"::uuid[]")
	}
	if f.From != nil {
		conds = append(conds, "lower(b.period) >= "+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "lower(b.period) < "+add(*f.To))
	}
	if f.CustomerID != nil {
		conds = append(conds, "EXISTS (SELECT 1 FROM energy.ref_units u JOIN energy.ref_customers cu ON cu.organization_id = u.customer_org_id WHERE u.id = ANY(b.unit_ids) AND cu.id = "+add(*f.CustomerID)+")")
	}
	if f.PropertyID != nil {
		conds = append(conds, "EXISTS (SELECT 1 FROM energy.ref_units u WHERE u.id = ANY(b.unit_ids) AND u.property_id = "+add(*f.PropertyID)+")")
	}
	if f.Method != nil {
		conds = append(conds, "b.method = "+add(*f.Method))
	}
	if f.BoundaryID != nil {
		conds = append(conds, "b.boundary_id = "+add(*f.BoundaryID))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM energy.baselines b WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Baseline]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM energy.baselines b WHERE %s ORDER BY %s LIMIT %d OFFSET %d", baselineCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Baseline]{}, err
	}
	defer rows.Close()
	items := []Baseline{}
	for rows.Next() {
		x, err := scanBaseline(rows)
		if err != nil {
			return paging.Page[Baseline]{}, err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		return paging.Page[Baseline]{}, err
	}
	return paging.Page[Baseline]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

var (
	methods    = []string{"demo_fixed", "demo_period_comparison"}
	boundaries = []string{"ac_input_electricity", "whole_building_electricity"}
)

// BaselineInput is BaselineInput.
type BaselineInput struct {
	ID          *uuid.UUID  `json:"id,omitempty"`
	UnitIDs     []uuid.UUID `json:"unitIds"`
	Period      Range       `json:"period"`
	BoundaryID  string      `json:"boundaryId"`
	Boundary    string      `json:"boundary"`
	Assumptions string      `json:"assumptions"`
	Source      string      `json:"source"`
	Method      string      `json:"method"`
	BaselineKWh *float64    `json:"baselineKWh,omitempty"`
}

// Validate implements ops.Validator (DD-A13, SR29, IR11).
func (in *BaselineInput) Validate() map[string]string {
	fe := map[string]string{}
	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fe["unitIds"] = "error.invalid"
		}
		seen[u] = true
	}
	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 100 {
		fe["unitIds"] = "error.invalid"
	}
	if CheckRange(in.Period.From, in.Period.To) != nil {
		fe["period"] = "errors.energy_range"
	}
	if !slices.Contains(boundaries, in.BoundaryID) {
		fe["boundaryId"] = "error.invalid"
	}
	text := func(k string, s *string, max int) {
		*s = strings.TrimSpace(*s)
		if n := utf8.RuneCountInString(*s); n < 1 || n > max {
			fe[k] = "error.length"
		}
	}
	text("boundary", &in.Boundary, 500)
	text("assumptions", &in.Assumptions, 2000)
	text("source", &in.Source, 500)
	switch in.Method {
	case "demo_fixed":
		if in.BaselineKWh == nil || math.IsNaN(*in.BaselineKWh) || math.IsInf(*in.BaselineKWh, 0) || *in.BaselineKWh < 0 {
			fe["baselineKWh"] = "error.range"
		}
	case "demo_period_comparison":
		if in.BaselineKWh != nil {
			fe["baselineKWh"] = "error.notAllowed"
		}
		if in.BoundaryID != Boundary {
			fe["boundaryId"] = "errors.boundary_not_measurable"
		}
	default:
		fe["method"] = "error.invalid"
	}
	return fe
}

type measuredQuality struct {
	Kind           string  `json:"kind"`
	Coverage       float64 `json:"coverage"`
	ExpectedSlots  int     `json:"expectedSlots"`
	ValidSlots     int     `json:"validSlots"`
	SourceSnapshot struct {
		Generation  int       `json:"generation"`
		EventCursor int       `json:"eventCursor"`
		SnapshotAt  time.Time `json:"snapshotAt"`
	} `json:"sourceSnapshot"`
}

const modeledQuality = `{"kind":"modeled","coverage":null,"expectedSlots":null,"validSlots":null,"sourceSnapshot":null}`

// saveBaseline stores a new baseline or version (SR29): demo_fixed keeps the entered value as modeled; demo_period_
// comparison integrates the measured slots of the period now (D07) and saves kind=measured with coverage and the
// source snapshot. Saved versions never recalculate.
//
//	@Summary		baselines.save (write)
//	@ID				baselines.save
//	@Description	Authorization: admin:energy.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; SR29 Repository computes measured quality; fixed is modeled
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A13
//	@Tags			baselines
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			false	"id omitted: omit (target none, read none); id present: required (target baselines, read baselines.list)"
//	@Param			request				body		BaselineInput	true	"input"
//	@Success		200					{object}	ops.Envelope{data=Baseline}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/ops/baselines.save [post]
func saveBaseline(ctx context.Context, c *ops.Call, in *BaselineInput) (Baseline, error) {
	var known int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND NOT archived`, in.UnitIDs).Scan(&known); err != nil {
		return Baseline{}, err
	}
	if known != len(in.UnitIDs) {
		return Baseline{}, apperr.Fields(map[string]string{"unitIds": "errors.unit_unknown"})
	}
	id, version := uuid.New(), 1
	if in.ID != nil {
		cur, err := LoadBaseline(ctx, c, *in.ID, nil)
		if err != nil {
			return cur, err
		}
		if cur.Version != *c.ExpectedVersion {
			return cur, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		id, version = cur.ID, cur.Version+1
	}
	quality, value := []byte(modeledQuality), in.BaselineKWh
	if in.Method == "demo_period_comparison" {
		n, err := Integrate(ctx, c, in.UnitIDs, in.Period.From, in.Period.To)
		if err != nil {
			return Baseline{}, err
		}
		q := measuredQuality{Kind: "measured", ExpectedSlots: n.ExpectedSlots, ValidSlots: n.ValidSlots, Coverage: float64(n.ValidSlots) / float64(n.ExpectedSlots)}
		q.SourceSnapshot.Generation, q.SourceSnapshot.SnapshotAt = 1, c.Now
		if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM platform.outbox`).Scan(&q.SourceSnapshot.EventCursor); err != nil {
			return Baseline{}, err
		}
		quality, _ = json.Marshal(q)
		value = Float(n.KWh)
	}
	if version > 1 {
		if _, err := c.Tx.Exec(ctx, `UPDATE energy.baselines SET is_current = false WHERE id = $1 AND is_current`, id); err != nil {
			return Baseline{}, err
		}
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO energy.baselines (id, version, tenant_id, unit_ids, period, method, baseline_kwh, quality, boundary_id, boundary, assumptions, source, created_by, created_at)
		VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, tstzrange($4, $5), $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		id, version, in.UnitIDs, in.Period.From, in.Period.To, in.Method, value, quality, in.BoundaryID, in.Boundary, in.Assumptions, in.Source, c.Principal.MembershipID, c.Now); err != nil {
		return Baseline{}, err
	}
	c.Audit(ops.AuditEntry{Action: "baselines.save", TargetKind: "baseline", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &version})
	return LoadBaseline(ctx, c, id, nil)
}

// Register binds factors.* and baselines.*.
func Register(r *ops.Registry) {
	ops.Register(r, "factors.list", listFactors)
	ops.Register(r, "factors.save", saveFactor)
	ops.Register(r, "baselines.list", listBaselines)
	ops.Register(r, "baselines.save", saveBaseline)
	ops.Register(r, "energy.summary", summary)
	ops.Register(r, "energy.exportReport", exportReport)
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func decodeStrict(raw []byte, v any) error {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
