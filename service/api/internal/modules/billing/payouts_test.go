package billing

import (
	"testing"
	"time"
)

// TestEndedPeriod: the monthly drafts are for the month that ended last in Kuala Lumpur — UTC 16:00 on the last day
// is already the 1st there — including across a year (IR341).
func TestEndedPeriod(t *testing.T) {
	for name, c := range map[string]struct {
		now  time.Time
		want string
	}{
		"mid month":                {time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC), "2026-08"},
		"the 1st in Kuala Lumpur":  {time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC), "2026-09"},
		"the last day there":       {time.Date(2026, 9, 30, 15, 59, 0, 0, time.UTC), "2026-08"},
		"January":                  {time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC), "2026-12"},
		"New Year in Kuala Lumpur": {time.Date(2026, 12, 31, 16, 30, 0, 0, time.UTC), "2026-12"},
	} {
		if got := endedPeriod(c.now); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}
