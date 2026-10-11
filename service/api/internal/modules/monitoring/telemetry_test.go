package monitoring

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// TestSeriesInputFilters: telemetry.series takes no query filters — an empty or null object passes, any key is
// refused for query.filters (IR121 item 2). The REST binding refuses an unknown query parameter first (error.notAllowed),
// so this rule guards the other callers of the operation.
func TestSeriesInputFilters(t *testing.T) {
	from := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	in := func(filters string) *SeriesInput {
		x := &SeriesInput{From: from, To: from.Add(time.Hour), UnitIDs: []uuid.UUID{uuid.New()}, Metric: "temperature"}
		if filters != "" {
			x.Query = paging.Query{Filters: json.RawMessage(filters)}
		}
		return x
	}
	for _, ok := range []string{"", "{}", "null"} {
		if fe := in(ok).Validate(); len(fe) != 0 {
			t.Errorf("filters %q: %v", ok, fe)
		}
	}
	if fe := in(`{"unitId":"x"}`).Validate(); len(fe) != 1 || fe["query.filters"] != "error.invalid" {
		t.Errorf("a filter: %v", fe)
	}
}
