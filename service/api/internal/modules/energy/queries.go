package energy

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// QueryActuals is the internal query of actuals and the IR78 forecast for the read models (IR190).
const QueryActuals = "energy.actuals"

// ActualsInput is QueryActuals input.
type ActualsInput struct {
	UnitIDs []uuid.UUID `json:"unitIds"`
	From    time.Time   `json:"from"`
	To      time.Time   `json:"to"`
}

// Actuals is QueryActuals output: actuals only (no baseline) and the forecast.
type Actuals struct {
	Summary  Summary  `json:"summary"`
	Forecast Forecast `json:"forecast"`
}

// RegisterQueries binds energy's internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(
		r,
		ops.DomainEnergy,
		QueryActuals,
		func(ctx context.Context, call *ops.Call, in *ActualsInput) (Actuals, error) {
			f, err := DefaultFactor(ctx, call)
			if err != nil {
				return Actuals{}, err
			}

			s, integ, err := Compute(ctx, call, in.UnitIDs, in.From, in.To, f, nil)
			if err != nil {
				return Actuals{}, err
			}

			fc, err := ForecastFor(ctx, call, in.UnitIDs, in.From, in.To, integ)

			return Actuals{Summary: s, Forecast: fc}, err
		},
	)
}
