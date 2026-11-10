package energy

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
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

// orgRecords reports any MRV report, offset quote or offset record of the organization: records that stay, so the
// customer is not deleted (DEC-103).
func orgRecords(ctx context.Context, c *ops.Call, input *ops.OrgRecordsInput) (bool, error) {
	var held bool

	err := c.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM energy.mrv_reports WHERE organization_id = $1)
		OR EXISTS (SELECT 1 FROM energy.offset_quotes WHERE customer_id = $2)
		OR EXISTS (SELECT 1 FROM energy.offset_records WHERE customer_id = $2)`,
		input.OrgID,
		input.CustomerID,
	).Scan(&held)

	return held, apperr.Wrap(err, "select energy.mrv_reports")
}

// RegisterQueries binds energy's internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainEnergy, ops.QueryOrgRecordsEnergy, orgRecords)
	ops.RegisterQuery(
		r,
		ops.DomainEnergy,
		QueryActuals,
		func(ctx context.Context, call *ops.Call, input *ActualsInput) (Actuals, error) {
			factor, err := DefaultFactor(ctx, call)
			if err != nil {
				return Actuals{}, err
			}

			energySummary, integ, err := Compute(
				ctx,
				call,
				input.UnitIDs,
				input.From,
				input.To,
				factor,
				nil,
			)
			if err != nil {
				return Actuals{}, err
			}

			fc, err := ForecastFor(ctx, call, input.UnitIDs, input.From, input.To, integ)

			return Actuals{Summary: energySummary, Forecast: fc}, err
		},
	)
}
