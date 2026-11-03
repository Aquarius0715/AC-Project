package energy

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// TariffVersion is the only fictional tariff version (0.5 MYR/kWh, D07).
const TariffVersion = "tariff-demo-1"

// BoundaryText is the fixed explanation of the actual-data boundary (IR11).
const BoundaryText = "AC equipment input electricity (excludes shared equipment, solar and batteries)"

// Ref is {id, version}.
type Ref struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// Totals is EnergySummary.totals.
type Totals struct {
	KWh              *float64 `json:"kWh"`
	AmountMinor      *int64   `json:"amountMinor"`
	SavedKWh         *float64 `json:"savedKWh"`
	DeltaKWh         *float64 `json:"deltaKWh"`
	SavingPercentage *float64 `json:"savingPercentage"`
	SavedAmountMinor *int64   `json:"savedAmountMinor"`
	EmissionsKg      *float64 `json:"emissionsKg"`
	SavedEmissionsKg *float64 `json:"savedEmissionsKg"`
}

// Summary is EnergySummary of service-contracts.ts.
type Summary struct {
	Period           Range       `json:"period"`
	UnitIDs          []uuid.UUID `json:"unitIds"`
	Totals           Totals      `json:"totals"`
	Currency         string      `json:"currency"`
	BaselineRef      *Ref        `json:"baselineRef"`
	FactorRef        *Ref        `json:"factorRef"`
	BaselineSnapshot *Baseline   `json:"baselineSnapshot"`
	FactorSnapshot   *Factor     `json:"factorSnapshot"`
	TariffVersion    string      `json:"tariffVersion"`
	BoundaryID       string      `json:"boundaryId"`
	Boundary         string      `json:"boundary"`
	Coverage         *float64    `json:"coverage"`
	QualityWarnings  []string    `json:"qualityWarnings"`
}

// roundMinor rounds a MYR amount to minor units, half away from zero (IR44).
func roundMinor(myr *big.Rat) *int64 {
	if myr == nil {
		return nil
	}

	x := new(big.Rat).Mul(myr, big.NewRat(100, 1))
	neg := x.Sign() < 0
	x.Abs(x)
	q := new(big.Int).Quo(x.Num(), x.Denom())

	rem := new(big.Rat).Sub(x, new(big.Rat).SetInt(q))
	if rem.Cmp(big.NewRat(1, 2)) >= 0 {
		q.Add(q, big.NewInt(1))
	}

	v := q.Int64()
	if neg {
		v = -v
	}

	return &v
}

func mul(a, b *big.Rat) *big.Rat {
	if a == nil || b == nil {
		return nil
	}

	return new(big.Rat).Mul(a, b)
}

// DefaultFactorID is fixture.defaultEmissionFactorId (factor-demo-2026 under the seed namespace); energy.summary uses
// its current version at read start (SR09).
var DefaultFactorID = uuid.NewSHA1(
	uuid.MustParse("6f1c3a52-6d0b-5c1e-9a57-0d1b7a4c2e10"),
	[]byte("factor-demo-2026"),
)

// DefaultFactor returns the current default factor, or nil when it is missing.
func DefaultFactor(ctx context.Context, c *ops.Call) (*Factor, error) {
	factor, err := LoadFactor(ctx, c, DefaultFactorID, nil)
	if err != nil {
		var de *apperr.DomainError
		if errors.As(err, &de) && de.Code == apperr.NotFound {
			return nil, nil
		}

		return nil, err
	}

	return &factor, nil
}

func sameSet(a, b []uuid.UUID) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	cmp := func(p, q uuid.UUID) int { return strings.Compare(p.String(), q.String()) }
	slices.SortFunc(x, cmp)
	slices.SortFunc(y, cmp)

	return slices.Equal(x, y)
}

// Compute builds an EnergySummary for units over [from, to) with the given factor (nil: factor_missing, emissions
// null) and optional baseline: the comparison is made only when comparable (IR148 item 1).
func Compute(
	ctx context.Context,
	call *ops.Call,
	units []uuid.UUID,
	from, until time.Time,
	f *Factor,
	baseline *Baseline,
) (Summary, Integration, error) {
	integration, err := Integrate(ctx, call, units, from, until)
	if err != nil {
		return Summary{}, integration, err
	}

	s := Summary{
		Period: Range{
			from,
			until,
		},
		UnitIDs:       units,
		Currency:      "MYR",
		TariffVersion: TariffVersion,
		BoundaryID:    Boundary,
		Boundary:      BoundaryText,
		QualityWarnings: append(
			[]string{},
			integration.Warnings...),
		Totals: Totals{
			KWh:              nil,
			AmountMinor:      nil,
			SavedKWh:         nil,
			DeltaKWh:         nil,
			SavingPercentage: nil,
			SavedAmountMinor: nil,
			EmissionsKg:      nil,
			SavedEmissionsKg: nil,
		},
		BaselineRef:      nil,
		FactorRef:        nil,
		BaselineSnapshot: nil,
		FactorSnapshot:   nil,
		Coverage:         nil,
	}
	if s.UnitIDs == nil {
		s.UnitIDs = []uuid.UUID{}
	}

	if integration.ExpectedSlots > 0 {
		cov := float64(integration.ValidSlots) / float64(integration.ExpectedSlots)
		s.Coverage = &cov
	}

	var factor *big.Rat

	if f != nil {
		s.FactorRef, s.FactorSnapshot = &Ref{f.ID, f.Version}, f
		factor = new(big.Rat).SetFloat64(f.KgCO2ePerKWh)
	} else {
		s.QualityWarnings = append(s.QualityWarnings, "factor_missing")
	}

	s.Totals.KWh = Float(integration.KWh)
	s.Totals.AmountMinor = roundMinor(mul(integration.KWh, TariffMYRPerKWh))

	s.Totals.EmissionsKg = Float(mul(integration.KWh, factor))
	if baseline != nil {
		s.BaselineRef, s.BaselineSnapshot = &Ref{baseline.ID, baseline.Version}, baseline

		var q struct {
			Kind     string   `json:"kind"`
			Coverage *float64 `json:"coverage"`
		}

		_ = jsonUnmarshal(baseline.Quality, &q)
		if q.Kind == "modeled" {
			s.QualityWarnings = append(s.QualityWarnings, "modeled_baseline")
		}

		if !Comparable(baseline, units, from, until, integration) {
			s.QualityWarnings = append(s.QualityWarnings, "baseline_not_comparable")
		} else {
			base := new(big.Rat).SetFloat64(*baseline.BaselineKWh)
			saved := new(big.Rat).Sub(base, integration.KWh)

			s.Totals.SavedKWh, s.Totals.DeltaKWh = Float(saved), Float(new(big.Rat).Neg(saved))
			if base.Sign() != 0 {
				s.Totals.SavingPercentage = Float(
					new(big.Rat).Mul(new(big.Rat).Quo(saved, base), big.NewRat(100, 1)),
				)
			}

			s.Totals.SavedAmountMinor = roundMinor(mul(saved, TariffMYRPerKWh))
			s.Totals.SavedEmissionsKg = Float(mul(saved, factor))
		}
	}

	s.QualityWarnings = sortedUnique(s.QualityWarnings)

	return s, integration, nil
}

// Comparable applies D07 / SR29: a value, the same unit set, boundary and minute count, actual coverage 1 and a
// modeled value or measured coverage 1.
func Comparable(b *Baseline, units []uuid.UUID, from, to time.Time, integration Integration) bool {
	var q struct {
		Kind     string   `json:"kind"`
		Coverage *float64 `json:"coverage"`
	}

	_ = jsonUnmarshal(b.Quality, &q)

	return b.BaselineKWh != nil && sameSet(b.UnitIDs, units) && b.BoundaryID == Boundary &&
		b.Period.To.Sub(b.Period.From) == to.Sub(from) &&
		integration.ExpectedSlots > 0 &&
		integration.ValidSlots == integration.ExpectedSlots &&
		(q.Kind == "modeled" || (q.Coverage != nil && *q.Coverage == 1))
}

// SummaryInput is energy.summary input.
type SummaryInput struct {
	From          time.Time   `json:"from"`
	To            time.Time   `json:"to"`
	UnitIDs       []uuid.UUID `json:"unitIds"`
	BaselineID    *uuid.UUID  `json:"baselineId,omitempty"`
	TariffVersion *string     `json:"tariffVersion,omitempty"`
}

// Validate implements ops.Validator.
func (in *SummaryInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if CheckRange(in.From, in.To) != nil {
		fieldErrs["from"] = "errors.energy_range"
	}

	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fieldErrs["unitIds"] = apperr.KeyInvalid
		}

		seen[u] = true
	}

	if in.UnitIDs == nil || len(in.UnitIDs) > 100 {
		fieldErrs["unitIds"] = apperr.KeyInvalid
	}

	if in.TariffVersion != nil && *in.TariffVersion != TariffVersion {
		fieldErrs["tariffVersion"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// summary is energy.summary (D07, SR29, IR68, IR148): actuals for the units; with baselineId the signed comparison
// when the baseline is comparable (same unit set, boundary and minute count, actual coverage 1, measured coverage 1
// or a modeled value), otherwise the savings stay null.
//
//	@Summary		energy.summary (read)
//	@ID				energy.summary
//	@Description	Authorization: client:self | admin:energy.read | admin:mrv.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A13, DD-C06, DD-C13, DD-C01
//	@Tags			energy
//	@Accept			json
//	@Produce		json
//	@Param			from			query		string		false	"input field from"
//	@Param			to				query		string		false	"input field to"
//	@Param			unitIds			query		[]string	false	"input field unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			baselineId		query		string		false	"input field baselineId"
//	@Param			tariffVersion	query		string		false	"input field tariffVersion"
//	@Success		200				{object}	ops.Envelope{data=Summary}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/energy/summary [get]
func summary(ctx context.Context, c *ops.Call, input *SummaryInput) (Summary, error) {
	if c.Principal.Role == "client" {
		var mine int

		err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND customer_org_id =
			$2`, input.UnitIDs, c.Principal.OrgID).
			Scan(&mine)
		if err != nil {
			return Summary{}, apperr.Wrap(err, "select energy.ref_units")
		}

		if mine != len(input.UnitIDs) {
			return Summary{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	}

	var known int

	err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND NOT archived`, input.UnitIDs).
		Scan(&known)
	if err != nil {
		return Summary{}, apperr.Wrap(err, "select energy.ref_units")
	}

	if known != len(input.UnitIDs) {
		return Summary{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	factor, err := DefaultFactor(ctx, c)
	if err != nil {
		return Summary{}, err
	}

	var base *Baseline

	if input.BaselineID != nil {
		b, err := LoadBaseline(ctx, c, *input.BaselineID, nil)
		if err != nil {
			return Summary{}, err
		}

		base = &b
	}

	s, _, err := Compute(ctx, c, input.UnitIDs, input.From, input.To, factor, base)

	return s, err
}

func sortedUnique(xs []string) []string {
	sort.Strings(xs)

	return slices.Compact(xs)
}

// Forecast is EnergyForecast (IR78).
type Forecast struct {
	Period                   Range       `json:"period"`
	UnitIDs                  []uuid.UUID `json:"unitIds"`
	Basis                    string      `json:"basis"`
	BaselineRef              *Ref        `json:"baselineRef"`
	BaselineSnapshot         *Baseline   `json:"baselineSnapshot"`
	ExpectedUnitMinutes      int         `json:"expectedUnitMinutes"`
	ValidUnitMinutes         int         `json:"validUnitMinutes"`
	ActualKWhOnValidSlots    *float64    `json:"actualKWhOnValidSlots"`
	PredictedBaselineKWh     *float64    `json:"predictedBaselineKWh"`
	PredictedActualKWh       *float64    `json:"predictedActualKWh"`
	ForecastSavedKWh         *float64    `json:"forecastSavedKWh"`
	ForecastSavingPercentage *float64    `json:"forecastSavingPercentage"`
	QualityWarnings          []string    `json:"qualityWarnings"`
}

// ForecastFor computes IR78 for the unit set U over [from, to) with the integration n of U.
func ForecastFor(
	ctx context.Context,
	call *ops.Call,
	units []uuid.UUID,
	from, to time.Time,
	integration Integration,
) (Forecast, error) {
	forecast := Forecast{
		Period:                   Range{from, to},
		UnitIDs:                  units,
		Basis:                    "prorated_modeled_baseline",
		QualityWarnings:          []string{},
		BaselineRef:              nil,
		BaselineSnapshot:         nil,
		ExpectedUnitMinutes:      0,
		ValidUnitMinutes:         0,
		ActualKWhOnValidSlots:    nil,
		PredictedBaselineKWh:     nil,
		PredictedActualKWh:       nil,
		ForecastSavedKWh:         nil,
		ForecastSavingPercentage: nil,
	}
	if forecast.UnitIDs == nil {
		forecast.UnitIDs = []uuid.UUID{}
	}

	if len(units) == 0 {
		forecast.QualityWarnings = []string{"no_units"}

		return forecast, nil
	}

	forecast.ExpectedUnitMinutes, forecast.ValidUnitMinutes = integration.ExpectedSlots, integration.ValidSlots
	forecast.ActualKWhOnValidSlots = Float(integration.KWh)

	var predActual, predBase *big.Rat
	if integration.ValidSlots >= 1 {
		predActual = new(
			big.Rat,
		).Mul(new(big.Rat).Quo(integration.KWh, big.NewRat(int64(integration.ValidSlots), 1)),
			big.NewRat(int64(integration.ExpectedSlots), 1))
	} else {
		forecast.QualityWarnings = append(forecast.QualityWarnings, "actual_unavailable")
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT id FROM energy.baselines WHERE is_current AND method = 'demo_fixed' AND boundary_id = $1 AND baseline_kwh IS
			NOT NULL
		ORDER BY (SELECT min(created_at) FROM energy.baselines o WHERE o.id = baselines.id) DESC, id ASC`,
		Boundary,
	)
	if err != nil {
		return forecast, apperr.Wrap(err, "select energy.baselines")
	}

	ids := []uuid.UUID{}

	for rows.Next() {
		var id uuid.UUID

		err := rows.Scan(&id)
		if err != nil {
			rows.Close()

			return forecast, apperr.Wrap(err, "select energy.baselines")
		}

		ids = append(ids, id)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return forecast, apperr.Wrap(rowsErr, "select energy.baselines")
	}

	var chosen *Baseline

	for _, id := range ids {
		baseline, err := LoadBaseline(ctx, call, id, nil)
		if err != nil {
			return forecast, err
		}

		if sameSet(baseline.UnitIDs, units) {
			chosen = &baseline

			break
		}
	}

	if chosen == nil {
		forecast.QualityWarnings = append(forecast.QualityWarnings, "baseline_unavailable")
	} else {
		forecast.BaselineRef, forecast.BaselineSnapshot = &Ref{chosen.ID, chosen.Version}, chosen
		forecast.QualityWarnings = append(
			forecast.QualityWarnings,
			"modeled_baseline",
			"prorated_forecast",
		)

		minutes := int64(chosen.Period.To.Sub(chosen.Period.From) / time.Minute)
		if minutes > 0 {
			predBase = new(
				big.Rat,
			).Mul(new(big.Rat).Quo(new(big.Rat).SetFloat64(*chosen.BaselineKWh), big.NewRat(int64(len(units))*minutes, 1)),
				big.NewRat(int64(integration.ExpectedSlots), 1))
		}
	}

	if integration.ValidSlots < integration.ExpectedSlots {
		forecast.QualityWarnings = append(forecast.QualityWarnings, "partial_coverage")
	}

	forecast.PredictedActualKWh, forecast.PredictedBaselineKWh = Float(predActual), Float(predBase)
	if predBase != nil && predActual != nil {
		saved := new(big.Rat).Sub(predBase, predActual)

		forecast.ForecastSavedKWh = Float(saved)
		if predBase.Sign() != 0 {
			forecast.ForecastSavingPercentage = Float(
				new(big.Rat).Mul(new(big.Rat).Quo(saved, predBase), big.NewRat(100, 1)),
			)
		}
	}

	forecast.QualityWarnings = sortedUnique(forecast.QualityWarnings)

	return forecast, nil
}
