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
	Quality     json.RawMessage `json:"quality"     swaggertype:"object"`
	BoundaryID  string          `json:"boundaryId"`
	Boundary    string          `json:"boundary"`
	Assumptions string          `json:"assumptions"`
	Source      string          `json:"source"`
}

const baselineCols = `b.id, b.tenant_id, b.version, (SELECT min(created_at) FROM energy.baselines o WHERE o.id = ` +
	`b.id), b.created_at, b.unit_ids, lower(b.period), upper(b.period),
	b.method, b.baseline_kwh, b.quality, b.boundary_id, b.boundary, b.assumptions, b.source`

func scanBaseline(r pgx.Row) (Baseline, error) {
	var baseline Baseline

	err := r.Scan(
		&baseline.ID,
		&baseline.TenantID,
		&baseline.Version,
		&baseline.CreatedAt,
		&baseline.UpdatedAt,
		&baseline.UnitIDs,
		&baseline.Period.From,
		&baseline.Period.To,
		&baseline.Method,
		&baseline.BaselineKWh,
		&baseline.Quality,
		&baseline.BoundaryID,
		&baseline.Boundary,
		&baseline.Assumptions,
		&baseline.Source,
	)
	slices.SortFunc(
		baseline.UnitIDs,
		func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) },
	)

	return baseline, apperr.Wrap(err, "scan (scanBaseline)")
}

// clientUnits restricts clients to baselines whose units all belong to their organization (IR90).
func clientUnits(c *ops.Call, args *[]any) string {
	if c.Principal.Role != "client" {
		return "TRUE"
	}

	*args = append(*args, c.Principal.OrgID)

	return fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM unnest(b.unit_ids) uid LEFT JOIN energy.ref_units u ON u.id = uid WHERE "+
			"u.customer_org_id IS DISTINCT FROM $%d)",
		len(*args),
	)
}

// LoadBaseline returns a readable baseline version (current when version is nil).
func LoadBaseline(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
	version *int,
) (Baseline, error) {
	args := []any{id}
	stmt := "SELECT " + baselineCols + " FROM energy.baselines b WHERE b.id = $1 AND "

	if version != nil {
		args = append(args, *version)
		stmt += "b.version = $2"
	} else {
		stmt += "b.is_current"
	}

	stmt += " AND " + clientUnits(call, &args)

	x, err := scanBaseline(call.Tx.QueryRow(ctx, stmt, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return x, err
}

// listBaselines serves baselines.list.
//
//	@Summary		baselines.list (read)
//	@ID				baselines.list
//	@Description	Authorization: client:self | admin:energy.read | admin:mrv.read; IR90 client only when all unitIds \
//	@Description	in scope
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A13, DD-C06, DD-A14, DD-C01 · Query: filters \
//	@Description	unitId,unitIds,from,to,customerId,propertyId,method,boundaryId · sort \
//	@Description	id,createdAt,updatedAt,periodFrom (default createdAt desc;id desc)
//	@Tags			baselines
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string		false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer		false	"page size 1–100, default 25"
//	@Param			sort		query		string		false	"field:direction — fields id,createdAt,updatedAt,periodFrom; default createdAt desc;id desc"
//	@Param			unitId		query		string		false	"filter → unitIds contains"
//	@Param			unitIds		query		[]string	false	"filter → unitIds intersection nonempty (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			from		query		string		false	"filter → [from,to) on period.from"
//	@Param			to			query		string		false	"filter → [from,to) on period.from"
//	@Param			customerId	query		string		false	"filter → any of unitIds belongs to the customer"
//	@Param			propertyId	query		string		false	"filter → any of unitIds is in the property"
//	@Param			method		query		string		false	"filter → method"
//	@Param			boundaryId	query		string		false	"filter → boundaryId"
//	@Success		200			{object}	ops.Envelope{data=BaselinePage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/baselines [get]
func listBaselines(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Baseline], error) {
	var filter struct {
		UnitID     *uuid.UUID   `json:"unitId,omitempty"`
		UnitIDs    *[]uuid.UUID `json:"unitIds,omitempty"` // the unit sets intersect
		From       *time.Time   `json:"from,omitempty"`    // [from, to) on period.from
		To         *time.Time   `json:"to,omitempty"`
		CustomerID *uuid.UUID   `json:"customerId,omitempty"` // any unit belongs to the customer
		PropertyID *uuid.UUID   `json:"propertyId,omitempty"` // any unit is in the property
		Method     *string      `json:"method,omitempty"`
		BoundaryID *string      `json:"boundaryId,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filter) != nil ||
			(filter.Method != nil && !slices.Contains(methods, *filter.Method)) ||
			(filter.BoundaryID != nil && !slices.Contains(boundaries, *filter.BoundaryID)) {
			return paging.Page[Baseline]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}

		if filter.From != nil && filter.To != nil && !filter.From.Before(*filter.To) {
			return paging.Page[Baseline]{}, apperr.Fields(
				map[string]string{"filters.to": apperr.KeyRange},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":         "b.id",
			"createdAt":  "b.created_at",
			"updatedAt":  "b.created_at",
			"periodFrom": "lower(b.period)",
		},
		"b.created_at DESC, b.id DESC",
	)
	if err != nil {
		return paging.Page[Baseline]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filter, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Baseline]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{}
	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"b.is_current", clientUnits(call, &args)}
	if filter.UnitID != nil {
		conds = append(conds, add(*filter.UnitID)+" = ANY(b.unit_ids)")
	}

	if filter.UnitIDs != nil {
		conds = append(conds, "b.unit_ids && "+add(*filter.UnitIDs)+"::uuid[]")
	}

	if filter.From != nil {
		conds = append(conds, "lower(b.period) >= "+add(*filter.From))
	}

	if filter.To != nil {
		conds = append(conds, "lower(b.period) < "+add(*filter.To))
	}

	if filter.CustomerID != nil {
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM energy.ref_units u JOIN energy.ref_customers cu ON cu.organization_id = u.customer_org_id "+
				"WHERE u.id = ANY(b.unit_ids) AND cu.id = "+add(
				*filter.CustomerID,
			)+")",
		)
	}

	if filter.PropertyID != nil {
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM energy.ref_units u WHERE u.id = ANY(b.unit_ids) AND u.property_id = "+add(
				*filter.PropertyID,
			)+")",
		)
	}

	if filter.Method != nil {
		conds = append(conds, "b.method = "+add(*filter.Method))
	}

	if filter.BoundaryID != nil {
		conds = append(conds, "b.boundary_id = "+add(*filter.BoundaryID))
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM energy.baselines b WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Baseline]{}, apperr.Wrap(scanErr, "select energy.baselines")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM energy.baselines b WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			baselineCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Baseline]{}, apperr.Wrap(err, "select energy.baselines")
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

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Baseline]{}, apperr.Wrap(rowsErr, "select energy.baselines")
	}

	return paging.Page[Baseline]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
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
	fieldErrs := map[string]string{}

	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fieldErrs["unitIds"] = apperr.KeyInvalid
		}

		seen[u] = true
	}

	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 100 {
		fieldErrs["unitIds"] = apperr.KeyInvalid
	}

	if CheckRange(in.Period.From, in.Period.To) != nil {
		fieldErrs["period"] = "errors.energy_range"
	}

	if !slices.Contains(boundaries, in.BoundaryID) {
		fieldErrs["boundaryId"] = apperr.KeyInvalid
	}

	text := func(k string, s *string, max int) {
		*s = strings.TrimSpace(*s)
		if n := utf8.RuneCountInString(*s); n < 1 || n > max {
			fieldErrs[k] = apperr.KeyLength
		}
	}
	text("boundary", &in.Boundary, 500)
	text("assumptions", &in.Assumptions, 2000)
	text("source", &in.Source, 500)

	switch in.Method {
	case "demo_fixed":
		if in.BaselineKWh == nil || math.IsNaN(*in.BaselineKWh) || math.IsInf(*in.BaselineKWh, 0) ||
			*in.BaselineKWh < 0 {
			fieldErrs["baselineKWh"] = apperr.KeyRange
		}
	case "demo_period_comparison":
		if in.BaselineKWh != nil {
			fieldErrs["baselineKWh"] = apperr.KeyNotAllowed
		}

		if in.BoundaryID != Boundary {
			fieldErrs["boundaryId"] = "errors.boundary_not_measurable"
		}
	default:
		fieldErrs["method"] = apperr.KeyInvalid
	}

	return fieldErrs
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
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	SR29 Repository computes measured quality; fixed is modeled
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A13
//	@Tags			baselines
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		BaselineInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Baseline}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/baselines [post]
func saveBaseline(ctx context.Context, call *ops.Call, input *BaselineInput) (Baseline, error) {
	var known int

	err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND NOT
		archived`, input.UnitIDs).
		Scan(&known)
	if err != nil {
		return Baseline{}, apperr.Wrap(err, "select energy.ref_units")
	}

	if known != len(input.UnitIDs) {
		return Baseline{}, apperr.Fields(map[string]string{"unitIds": "errors.unit_unknown"})
	}

	baselineID, version := uuid.New(), 1

	if input.ID != nil {
		cur, err := LoadBaseline(ctx, call, *input.ID, nil)
		if err != nil {
			return cur, err
		}

		if cur.Version != *call.ExpectedVersion {
			return cur, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}

		baselineID, version = cur.ID, cur.Version+1
	}

	quality, value := []byte(modeledQuality), input.BaselineKWh
	if input.Method == "demo_period_comparison" {
		integration, err := Integrate(ctx, call, input.UnitIDs, input.Period.From, input.Period.To)
		if err != nil {
			return Baseline{}, err
		}

		measured := measuredQuality{
			Kind:          "measured",
			ExpectedSlots: integration.ExpectedSlots,
			ValidSlots:    integration.ValidSlots,
			Coverage: float64(
				integration.ValidSlots,
			) / float64(
				integration.ExpectedSlots,
			),
			SourceSnapshot: *new(struct {
				Generation  int       `json:"generation"`
				EventCursor int       `json:"eventCursor"`
				SnapshotAt  time.Time `json:"snapshotAt"`
			}),
		}

		measured.SourceSnapshot.Generation, measured.SourceSnapshot.SnapshotAt = 1, call.Now

		scanErr := call.Tx.QueryRow(ctx, `SELECT count(*) FROM platform.outbox`).
			Scan(&measured.SourceSnapshot.EventCursor)
		if scanErr != nil {
			return Baseline{}, apperr.Wrap(scanErr, "select platform.outbox")
		}

		quality, _ = json.Marshal(measured)
		value = Float(integration.KWh)
	}

	if version > 1 {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE energy.baselines SET is_current = false WHERE id = $1 AND is_current`,
			baselineID,
		)
		if err != nil {
			return Baseline{}, apperr.Wrap(err, "update energy.baselines")
		}
	}

	_, err = call.Tx.Exec(
		ctx,
		`INSERT INTO energy.baselines (id, version, tenant_id, unit_ids, period, method, baseline_kwh, quality, boundary_id,
			boundary, assumptions, source, created_by, created_at)
		VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, tstzrange($4, $5), $6, $7, $8, $9, $10, $11, $12, $13,
		$14)`,
		baselineID,
		version,
		input.UnitIDs,
		input.Period.From,
		input.Period.To,
		input.Method,
		value,
		quality,
		input.BoundaryID,
		input.Boundary,
		input.Assumptions,
		input.Source,
		call.Principal.MembershipID,
		call.Now,
	)
	if err != nil {
		return Baseline{}, apperr.Wrap(err, "insert energy.baselines")
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "baselines.save",
			TargetKind:      "baseline",
			TargetID:        baselineID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return LoadBaseline(ctx, call, baselineID, nil)
}

// Register binds factors.* and baselines.*.
func Register(reg *ops.Registry) {
	ops.Register(reg, "factors.list", listFactors)
	ops.Register(reg, "factors.save", saveFactor)
	ops.Register(reg, "baselines.list", listBaselines)
	ops.Register(reg, "baselines.save", saveBaseline)
	ops.Register(reg, "energy.summary", summary)
	ops.Register(reg, "energy.exportReport", exportReport)
}

func jsonUnmarshal(
	b []byte,
	v any,
) error {
	return apperr.Wrap(json.Unmarshal(b, v), "json.Unmarshal")
}

func decodeStrict(raw []byte, dest any) error {
	if len(raw) == 0 {
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	return apperr.Wrap(dec.Decode(dest), "json.Decoder.Decode")
}
