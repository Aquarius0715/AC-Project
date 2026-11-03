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
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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

// senPerRinggit is the minor units of one MYR, tariffSenPerKWh the fictional fixed tariff (D07) in them, percent the
// scale of a percentage, and maxSummaryUnits the most units one summary covers.
const (
	senPerRinggit   = 100
	tariffSenPerKWh = 50
	percent         = 100
	maxSummaryUnits = 100
)

// tariffMYRPerKWh is the fictional fixed tariff (D07) in MYR.
func tariffMYRPerKWh() *big.Rat { return big.NewRat(tariffSenPerKWh, senPerRinggit) }

// share is part as a percentage of whole, nil when whole is zero.
func share(part, whole *big.Rat) *float64 {
	if whole.Sign() == 0 {
		return nil
	}

	return Float(new(big.Rat).Mul(new(big.Rat).Quo(part, whole), big.NewRat(percent, 1)))
}

// roundMinor rounds a MYR amount to minor units, half away from zero (IR44).
func roundMinor(myr *big.Rat) *int64 {
	if myr == nil {
		return nil
	}

	x := new(big.Rat).Mul(myr, big.NewRat(senPerRinggit, 1))
	neg := x.Sign() < 0
	x.Abs(x)
	minor := new(big.Int).Quo(x.Num(), x.Denom())

	rem := new(big.Rat).Sub(x, new(big.Rat).SetInt(minor))
	if new(
		big.Rat,
	).Add(rem, rem).
		Cmp(big.NewRat(1, 1)) >=
		0 { // the remainder is half a sen or more
		minor.Add(minor, big.NewInt(1))
	}

	v := minor.Int64()
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
func DefaultFactorID() uuid.UUID {
	return uuid.NewSHA1(
		uuid.MustParse("6f1c3a52-6d0b-5c1e-9a57-0d1b7a4c2e10"),
		[]byte("factor-demo-2026"),
	)
}

// DefaultFactor is the current default factor and whether there is one; without it the summaries report
// factor_missing and leave the emissions null.
func DefaultFactor(ctx context.Context, c *ops.Call) (*Factor, bool, error) {
	factor, err := LoadFactor(ctx, c, DefaultFactorID(), nil)
	if err != nil {
		var de *apperr.DomainError
		if errors.As(err, &de) && de.Code == apperr.NotFound {
			return nil, false, nil
		}

		return nil, false, err
	}

	return &factor, true, nil
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
	emissionFactor *Factor,
	baseline *Baseline,
) (Summary, Integration, error) {
	integration, err := Integrate(ctx, call, units, from, until)
	if err != nil {
		return Summary{}, integration, err
	}

	out := Summary{
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
	if out.UnitIDs == nil {
		out.UnitIDs = []uuid.UUID{}
	}

	if integration.ExpectedSlots > 0 {
		cov := float64(integration.ValidSlots) / float64(integration.ExpectedSlots)
		out.Coverage = &cov
	}

	var factor *big.Rat

	if emissionFactor != nil {
		out.FactorRef, out.FactorSnapshot = &Ref{
			emissionFactor.ID,
			emissionFactor.Version,
		}, emissionFactor
		factor = new(big.Rat).SetFloat64(emissionFactor.KgCO2ePerKWh)
	} else {
		out.QualityWarnings = append(out.QualityWarnings, "factor_missing")
	}

	out.Totals.KWh = Float(integration.KWh)
	out.Totals.AmountMinor = roundMinor(mul(integration.KWh, tariffMYRPerKWh()))

	out.Totals.EmissionsKg = Float(mul(integration.KWh, factor))
	if baseline != nil {
		err := compareBaseline(&out, baseline, units, from, until, integration, factor)
		if err != nil {
			return Summary{}, integration, err
		}
	}

	out.QualityWarnings = sortedUnique(out.QualityWarnings)

	return out, integration, nil
}

// compareBaseline adds the baseline to a summary: its reference and snapshot, a warning for a modeled one, and the
// savings when the baseline is comparable (otherwise a warning, and the savings stay null).
func compareBaseline(
	out *Summary,
	baseline *Baseline,
	units []uuid.UUID,
	from, until time.Time,
	integration Integration,
	factor *big.Rat,
) error {
	out.BaselineRef, out.BaselineSnapshot = &Ref{baseline.ID, baseline.Version}, baseline

	var quality struct {
		Kind     string   `json:"kind"`
		Coverage *float64 `json:"coverage"`
	}

	err := jsonUnmarshal(baseline.Quality, &quality)
	if err != nil {
		return err
	}

	if quality.Kind == "modeled" {
		out.QualityWarnings = append(out.QualityWarnings, "modeled_baseline")
	}

	if !Comparable(baseline, units, from, until, integration) {
		out.QualityWarnings = append(out.QualityWarnings, "baseline_not_comparable")

		return nil
	}

	base := new(big.Rat).SetFloat64(*baseline.BaselineKWh)
	saved := new(big.Rat).Sub(base, integration.KWh)

	out.Totals.SavedKWh, out.Totals.DeltaKWh = Float(saved), Float(new(big.Rat).Neg(saved))
	out.Totals.SavingPercentage = share(saved, base)
	out.Totals.SavedAmountMinor = roundMinor(mul(saved, tariffMYRPerKWh()))
	out.Totals.SavedEmissionsKg = Float(mul(saved, factor))

	return nil
}

// Comparable applies D07 / SR29: a value, the same unit set, boundary and minute count, actual coverage 1 and a
// modeled value or measured coverage 1.
func Comparable(
	baseline *Baseline,
	units []uuid.UUID,
	from, until time.Time,
	integration Integration,
) bool {
	var quality struct {
		Kind     string   `json:"kind"`
		Coverage *float64 `json:"coverage"`
	}

	if jsonUnmarshal(baseline.Quality, &quality) != nil {
		return false // a quality that does not decode compares with nothing
	}

	return baseline.BaselineKWh != nil && sameSet(baseline.UnitIDs, units) &&
		baseline.BoundaryID == Boundary &&
		baseline.Period.To.Sub(baseline.Period.From) == until.Sub(from) &&
		integration.ExpectedSlots > 0 &&
		integration.ValidSlots == integration.ExpectedSlots &&
		(quality.Kind == "modeled" || (quality.Coverage != nil && *quality.Coverage == 1))
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
		fieldErrs["from"] = keyEnergyRange
	}

	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fieldErrs[keyUnitIDs] = apperr.KeyInvalid
		}

		seen[u] = true
	}

	if in.UnitIDs == nil || len(in.UnitIDs) > maxSummaryUnits {
		fieldErrs[keyUnitIDs] = apperr.KeyInvalid
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
func summary(ctx context.Context, call *ops.Call, input *SummaryInput) (Summary, error) {
	if call.Principal.Role == authz.RoleClient {
		var mine int

		err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND customer_org_id =
			$2`, input.UnitIDs, call.Principal.OrgID).
			Scan(&mine)
		if err != nil {
			return Summary{}, apperr.Wrap(err, "select energy.ref_units")
		}

		if mine != len(input.UnitIDs) {
			return Summary{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	}

	var known int

	err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM energy.ref_units WHERE id = ANY($1) AND NOT
		archived`, input.UnitIDs).
		Scan(&known)
	if err != nil {
		return Summary{}, apperr.Wrap(err, "select energy.ref_units")
	}

	if known != len(input.UnitIDs) {
		return Summary{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	factor, _, err := DefaultFactor(ctx, call)
	if err != nil {
		return Summary{}, err
	}

	var base *Baseline

	if input.BaselineID != nil {
		b, err := LoadBaseline(ctx, call, *input.BaselineID, nil)
		if err != nil {
			return Summary{}, err
		}

		base = &b
	}

	s, _, err := Compute(ctx, call, input.UnitIDs, input.From, input.To, factor, base)

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

// modeledBaselineFor is the newest current demo_fixed baseline on the 1A boundary for exactly these units (a baseline's
// first version decides how new it is), and whether there is one.
func modeledBaselineFor(
	ctx context.Context,
	call *ops.Call,
	units []uuid.UUID,
) (Baseline, bool, error) {
	var none Baseline

	rows, err := call.Tx.Query(
		ctx,
		`SELECT id FROM energy.baselines WHERE is_current AND method = 'demo_fixed' AND boundary_id = $1 AND baseline_kwh IS
			NOT NULL
		ORDER BY (SELECT min(created_at) FROM energy.baselines o WHERE o.id = baselines.id) DESC, id ASC`,
		Boundary,
	)
	if err != nil {
		return none, false, apperr.Wrap(err, "select energy.baselines")
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return none, false, apperr.Wrap(err, "select energy.baselines")
	}

	for _, id := range ids {
		baseline, err := LoadBaseline(ctx, call, id, nil)
		if err != nil {
			return none, false, err
		}

		if sameSet(baseline.UnitIDs, units) {
			return baseline, true, nil
		}
	}

	return none, false, nil
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

	chosen, found, err := modeledBaselineFor(ctx, call, units)
	if err != nil {
		return forecast, err
	}

	if !found {
		forecast.QualityWarnings = append(forecast.QualityWarnings, "baseline_unavailable")
	} else {
		forecast.BaselineRef, forecast.BaselineSnapshot = &Ref{chosen.ID, chosen.Version}, &chosen
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
		forecast.ForecastSavingPercentage = share(saved, predBase)
	}

	forecast.QualityWarnings = sortedUnique(forecast.QualityWarnings)

	return forecast, nil
}
