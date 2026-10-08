// Package energy implements energy baselines, emission factors and energy summaries (FR-A13/A14/C06, D07, SR29).
package energy

import (
	"context"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

// Boundary is the 1A actual-data boundary (IR11).
const Boundary = "ac_input_electricity"

// TariffMYRPerKWh is the fictional fixed tariff (D07).
var TariffMYRPerKWh = big.NewRat(1, 2)

// Integration is the D07 60-second slot integration of power samples over units × [from, to).
type Integration struct {
	ExpectedSlots int
	ValidSlots    int
	KWh           *big.Rat // nil when no slot is valid
	Warnings      []string
}

// CheckRange validates a D07 energy range: UTC-minute aligned, from < to, at most 366 days.
func CheckRange(from, to time.Time) error {
	if from.IsZero() || to.IsZero() || !from.Before(to) || to.Sub(from) > 366*24*time.Hour ||
		!from.Equal(from.Truncate(time.Minute)) || !to.Equal(to.Truncate(time.Minute)) {
		return apperr.Fields(map[string]string{"period": "errors.energy_range"})
	}
	return nil
}

// Integrate applies D07 / IR08 / IR11: per unit and minute, the sample exactly at the slot start (highest sequence,
// then lowest ID) counts only when measured, valid, in kW and on the ac_input_electricity boundary; its value / 60
// is the slot energy. Later samples within a slot are never substituted.
func Integrate(ctx context.Context, c *ops.Call, units []uuid.UUID, from, to time.Time) (Integration, error) {
	out := Integration{ExpectedSlots: len(units) * int(to.Sub(from)/time.Minute)}
	if len(units) == 0 {
		return out, nil
	}
	rows, err := c.Tx.Query(ctx, `SELECT DISTINCT ON (unit_id, observed_at) value, unit, origin, quality, boundary_id
		FROM monitoring.measurements WHERE unit_id = ANY($1) AND metric = 'power' AND observed_at >= $2 AND observed_at < $3
		  AND observed_at = date_trunc('minute', observed_at)
		ORDER BY unit_id, observed_at, sequence DESC, event_id::text ASC`, units, from, to)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	sum := new(big.Rat)
	warn := map[string]bool{}
	for rows.Next() {
		var v *float64
		var unit, origin, quality string
		var boundary *string
		if err := rows.Scan(&v, &unit, &origin, &quality, &boundary); err != nil {
			return out, err
		}
		switch {
		case origin != "measured":
			warn["non_measured_input"] = true
		case boundary == nil || *boundary != Boundary:
			warn["boundary_mismatch"] = true
		case quality != "valid" || v == nil || unit != "kW":
		default:
			out.ValidSlots++
			sum.Add(sum, new(big.Rat).Quo(new(big.Rat).SetFloat64(*v), big.NewRat(60, 1)))
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if out.ValidSlots > 0 {
		out.KWh = sum
	}
	if out.ValidSlots < out.ExpectedSlots {
		warn["partial_coverage"] = true
	}
	for _, k := range []string{"non_measured_input", "boundary_mismatch", "partial_coverage"} {
		if warn[k] {
			out.Warnings = append(out.Warnings, k)
		}
	}
	return out, nil
}

// Float returns a rational as float64 (nil stays nil).
func Float(r *big.Rat) *float64 {
	if r == nil {
		return nil
	}
	f, _ := r.Float64()
	return &f
}
