package energy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Factor is EmissionFactor of service-contracts.ts.
type Factor struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenantId"`
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Region       string    `json:"region"`
	Year         int       `json:"year"`
	KgCO2ePerKWh float64   `json:"kgCO2ePerKWh"`
	Source       string    `json:"source"`
	IsDemo       bool      `json:"isDemo"`
}

const factorCols = `f.id, f.tenant_id, f.version, (SELECT min(created_at) FROM energy.emission_factors o WHERE o.id ` +
	`= f.id), f.created_at, f.region, f.year,
	f.kg_co2e_per_kwh::float8, f.source`

func scanFactor(r pgx.Row) (Factor, error) {
	var factor Factor

	err := r.Scan(
		&factor.ID,
		&factor.TenantID,
		&factor.Version,
		&factor.CreatedAt,
		&factor.UpdatedAt,
		&factor.Region,
		&factor.Year,
		&factor.KgCO2ePerKWh,
		&factor.Source,
	)
	factor.IsDemo = true

	return factor, apperr.Wrap(err, "scan (scanFactor)")
}

// LoadFactor returns a factor version (current when version is nil).
func LoadFactor(ctx context.Context, c *ops.Call, id uuid.UUID, version *int) (Factor, error) {
	q, args := "SELECT "+factorCols+" FROM energy.emission_factors f WHERE f.id = $1 AND f.is_current", []any{
		id,
	}
	if version != nil {
		q, args = "SELECT "+factorCols+" FROM energy.emission_factors f WHERE f.id = $1 AND f.version = $2", []any{
			id,
			*version,
		}
	}

	x, err := scanFactor(c.Tx.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return x, err
}

// listFactors serves factors.list.
//
//	@Summary		factors.list (read)
//	@ID				factors.list
//	@Description	Authorization: admin:mrv.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A14 · Query: filters region,year · sort id,year,region,createdAt,updatedAt (default year \
//	@Description	desc;region asc;id asc)
//	@Tags			factors
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,year,region,createdAt,updatedAt; default year desc;region asc;id asc"
//	@Param			region	query		string	false	"filter → region"
//	@Param			year	query		number	false	"filter → year"
//	@Success		200		{object}	ops.Envelope{data=FactorPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/factors [get]
func listFactors(
	ctx context.Context,
	call *ops.Call,
	in *paging.Query,
) (paging.Page[Factor], error) {
	var f struct {
		Region *string `json:"region,omitempty"`
		Year   *int    `json:"year,omitempty"`
	}

	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil {
			return paging.Page[Factor]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		in.Sort,
		map[string]string{
			"id":        "f.id",
			"year":      "f.year",
			"region":    "f.region",
			"createdAt": "f.created_at",
			"updatedAt": "f.created_at",
		},
		"f.year DESC, f.region ASC, f.id ASC",
	)
	if err != nil {
		return paging.Page[Factor]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*in, f, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Factor]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{}
	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"f.is_current"}
	if f.Region != nil {
		conds = append(conds, "f.region = "+add(*f.Region))
	}

	if f.Year != nil {
		conds = append(conds, "f.year = "+add(*f.Year))
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM energy.emission_factors f WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Factor]{}, apperr.Wrap(scanErr, "select energy.emission_factors")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM energy.emission_factors f WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			factorCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Factor]{}, apperr.Wrap(err, "select energy.emission_factors")
	}
	defer rows.Close()

	items := []Factor{}

	for rows.Next() {
		x, err := scanFactor(rows)
		if err != nil {
			return paging.Page[Factor]{}, err
		}

		items = append(items, x)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Factor]{}, apperr.Wrap(rowsErr, "select energy.emission_factors")
	}

	return paging.Page[Factor]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// FactorInput is factors.save input (Save<EmissionFactor>).
type FactorInput struct {
	ID           *uuid.UUID `json:"id,omitempty"`
	Region       string     `json:"region"`
	Year         int        `json:"year"`
	KgCO2ePerKWh float64    `json:"kgCO2ePerKWh"`
	Source       string     `json:"source"`
	IsDemo       bool       `json:"isDemo"`
}

// Validate implements ops.Validator (DD-A14, IR147 item 1).
func (in *FactorInput) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.Region, in.Source = strings.TrimSpace(in.Region), strings.TrimSpace(in.Source)
	if n := utf8.RuneCountInString(in.Region); n < 1 || n > 120 {
		fieldErrs["region"] = apperr.KeyLength
	}

	if in.Year < 2000 || in.Year > 2100 {
		fieldErrs["year"] = apperr.KeyRange
	}

	if math.IsNaN(in.KgCO2ePerKWh) || in.KgCO2ePerKWh <= 0 || in.KgCO2ePerKWh > 10 {
		fieldErrs["kgCO2ePerKWh"] = apperr.KeyRange
	}

	if n := utf8.RuneCountInString(in.Source); n < 1 || n > 500 {
		fieldErrs["source"] = apperr.KeyLength
	}

	if !in.IsDemo {
		fieldErrs["isDemo"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// saveFactor creates a factor or a new version of it (existing MRV reports keep the version they reference). One
// current factor per region and year (VALIDATION).
//
//	@Summary		factors.save (write)
//	@ID				factors.save
//	@Description	Authorization: admin:mrv.factors
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A14
//	@Tags			factors
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		FactorInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Factor}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/factors [post]
func saveFactor(ctx context.Context, call *ops.Call, in *FactorInput) (Factor, error) {
	id, version := uuid.New(), 1

	if in.ID != nil {
		cur, err := LoadFactor(ctx, call, *in.ID, nil)
		if err != nil {
			return cur, err
		}

		if cur.Version != *call.ExpectedVersion {
			return cur, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}

		id, version = cur.ID, cur.Version+1
	}

	var taken bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM energy.emission_factors WHERE is_current AND region = $1 AND
		year = $2 AND id <> $3)`, in.Region, in.Year, id).
		Scan(&taken)
	if err != nil {
		return Factor{}, apperr.Wrap(err, "select energy.emission_factors")
	}

	if taken {
		return Factor{}, apperr.Fields(map[string]string{"region": "errors.factor_exists"})
	}

	if version > 1 {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE energy.emission_factors SET is_current = false WHERE id = $1 AND is_current`,
			id,
		)
		if err != nil {
			return Factor{}, apperr.Wrap(err, "update energy.emission_factors")
		}
	}

	_, err = call.Tx.Exec(
		ctx,
		`INSERT INTO energy.emission_factors (id, version, tenant_id, region, year, kg_co2e_per_kwh, source, created_by,
			created_at)
		VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5, $6, $7, $8)`,
		id,
		version,
		in.Region,
		in.Year,
		in.KgCO2ePerKWh,
		in.Source,
		call.Principal.MembershipID,
		call.Now,
	)
	if err != nil {
		return Factor{}, apperr.Wrap(err, "insert energy.emission_factors")
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "factors.save",
			TargetKind:      "emission_factor",
			TargetID:        id.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return LoadFactor(ctx, call, id, nil)
}
