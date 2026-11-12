package integration_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestEnergySlotWarnings is D07: a slot counts only when its sample is measured, valid, in kW and on the
// ac_input_electricity boundary; an estimated sample and a sample of another boundary are excluded with their own
// warning, an empty slot adds partial_coverage, and nothing falls back to another sample.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestEnergySlotWarnings(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Slot warnings AC")
	start := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)

	powerSeries(t, unit, start, "measured", new(float64(60)))
	powerSeries(t, unit, start.Add(time.Minute), "estimated", new(float64(60)))
	owner(
		t,
		`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
			origin, quality, boundary_id, received_at, event_id)
		VALUES ($1,$2,$3,'power',$4,1,60,'kW','measured','valid','building',$4,$5)`,
		seed.ID("tenant-a"),
		unit,
		uuid.NewString(),
		start.Add(2*time.Minute),
		uuid.NewString(),
	)

	end := start.Add(4 * time.Minute) // minute 3 has no sample
	body := fmt.Sprintf(
		`{"from":%q,"to":%q,"unitIds":[%q]}`,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		unit,
	)
	code, resp := post(srv, hq(), "energy.summary", body)

	if code != 200 {
		t.Fatalf("summary: %d %v", code, resp)
	}

	if kwh := asNum(t, asObj(t, data(resp)["totals"])["kWh"]); kwh != 1 {
		t.Errorf("only the measured slot counts: %v kWh, want 1", kwh)
	}

	if cov := asNum(t, data(resp)["coverage"]); cov != 0.25 {
		t.Errorf("coverage %v, want 0.25", cov)
	}

	warnings := asArr(t, data(resp)["qualityWarnings"])
	for _, want := range []string{"non_measured_input", "boundary_mismatch", "partial_coverage"} {
		if !slices.Contains(warnings, any(want)) {
			t.Errorf("warnings %v lack %s", warnings, want)
		}
	}
}
