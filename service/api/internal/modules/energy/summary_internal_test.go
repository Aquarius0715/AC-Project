package energy

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// comparableCase is a baseline that compares with one unit's five measured minutes: the cases change one thing each.
func comparableCase() (*Baseline, []uuid.UUID, time.Time, time.Time, Integration) {
	unit := uuid.New()
	from := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	until := from.Add(5 * time.Minute)

	baseline := new(Baseline)
	baseline.UnitIDs = []uuid.UUID{unit}
	baseline.Period = Range{
		From: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 8, 1, 0, 5, 0, 0, time.UTC),
	}
	baseline.BaselineKWh = new(float64(10))
	baseline.BoundaryID = Boundary
	baseline.Quality = json.RawMessage(`{"kind":"measured","coverage":1}`)

	integration := new(Integration)
	integration.ExpectedSlots, integration.ValidSlots, integration.KWh = 5, 5, big.NewRat(5, 1)

	return baseline, []uuid.UUID{unit}, from, until, *integration
}

// A baseline whose quality does not decode compares with nothing (D07, SR29); a full measured coverage or a modeled
// value compares, a partial measured coverage does not.
func TestComparableQuality(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		quality string
		want    bool
	}{
		"measured, full coverage":    {`{"kind":"measured","coverage":1}`, true},
		"modeled":                    {`{"kind":"modeled","coverage":null}`, true},
		"measured, partial coverage": {`{"kind":"measured","coverage":0.5}`, false},
		"measured, no coverage":      {`{"kind":"measured"}`, false},
		"not an object":              {`"modeled"`, false},
		"not JSON":                   {`{`, false},
	} {
		baseline, units, from, until, integration := comparableCase()
		baseline.Quality = json.RawMessage(testCase.quality)

		if got := Comparable(baseline, units, from, until, integration); got != testCase.want {
			t.Errorf("%s: %v, want %v", name, got, testCase.want)
		}
	}
}

// A saving against a zero baseline has no percentage (nil, never a division by zero or 0 %).
func TestShareOfZeroIsNil(t *testing.T) {
	t.Parallel()

	if got := share(big.NewRat(-5, 1), new(big.Rat)); got != nil {
		t.Errorf("zero whole: %v, want nil", *got)
	}

	if got := share(big.NewRat(5, 1), big.NewRat(10, 1)); got == nil || *got != 50 {
		t.Errorf("5 of 10: %v, want 50", got)
	}
}

// A report month starts at midnight in the user's timezone (IR189): Kuala Lumpur when unset, UTC for a zone the
// server does not know; a month that has not ended is refused on its field.
func TestReportMonthTimezone(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)

	for timezone, wantOffset := range map[string]int{
		"":                 8 * 3600,
		"America/New_York": -4 * 3600,
		"Not/AZone":        0,
	} {
		call := new(ops.Call)
		call.Principal = new(ops.Principal)
		call.Principal.Timezone = timezone
		call.Now = now

		start, end, err := reportMonth(call, "2026-09")
		if err != nil {
			t.Fatalf("%q: %v", timezone, err)
		}

		if _, offset := start.Zone(); offset != wantOffset || start.Day() != 1 ||
			start.Hour() != 0 {
			t.Errorf("%q: start %v, want offset %d", timezone, start, wantOffset)
		}

		if !end.Equal(start.AddDate(0, 1, 0)) {
			t.Errorf("%q: end %v", timezone, end)
		}
	}

	call := new(ops.Call)
	call.Principal = new(ops.Principal)
	call.Now = now

	_, _, err := reportMonth(call, "2026-11")
	if err == nil {
		t.Error("the current month must be refused")
	}
}
