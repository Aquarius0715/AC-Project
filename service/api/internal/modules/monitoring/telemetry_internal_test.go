package monitoring

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// TestSeriesInputFilters: telemetry.series takes no query filters — an empty or null object passes, any key is refused
// for query.filters (IR121 item 2). The REST binding refuses an unknown query parameter first (error.notAllowed), so
// this rule guards the other callers of the operation.
func TestSeriesInputFilters(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

	input := func(filters string) *SeriesInput {
		seriesInput := &SeriesInput{
			From:          from,
			To:            from.Add(time.Hour),
			UnitIDs:       []uuid.UUID{uuid.New()},
			Metric:        metricTemperature,
			SpaceID:       nil,
			SensorID:      nil,
			BucketMinutes: nil,
			Query:         paging.Query{Cursor: nil, Limit: nil, Sort: nil, Filters: nil},
		}
		if filters != "" {
			seriesInput.Query = paging.Query{
				Filters: json.RawMessage(filters),
				Cursor:  nil,
				Limit:   nil,
				Sort:    nil,
			}
		}

		return seriesInput
	}
	for _, ok := range []string{"", "{}", "null"} {
		if fe := input(ok).Validate(); len(fe) != 0 {
			t.Errorf("filters %q: %v", ok, fe)
		}
	}

	if fe := input(
		`{"unitId":"x"}`,
	).Validate(); len(fe) != 1 ||
		fe["query.filters"] != apperr.KeyInvalid {
		t.Errorf("a filter: %v", fe)
	}
}
