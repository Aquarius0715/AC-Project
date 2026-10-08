package maintenance

import (
	"testing"
	"time"
)

func TestNextOccurrence(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }
	for _, c := range []struct {
		from     string
		months   int
		anchor   int
		expected string
	}{
		{"2026-09-08T01:00:00Z", 3, 8, "2026-12-08T01:00:00Z"},
		{"2026-01-31T00:00:00Z", 1, 31, "2026-02-28T00:00:00Z"},
		{"2026-02-28T00:00:00Z", 1, 31, "2026-03-31T00:00:00Z"},
		{"2026-11-15T09:30:00Z", 2, 15, "2027-01-15T09:30:00Z"},
		{"2028-01-30T00:00:00Z", 1, 30, "2028-02-29T00:00:00Z"},
	} {
		if got := NextOccurrence(d(c.from), c.months, c.anchor); !got.Equal(d(c.expected)) {
			t.Errorf("%s +%d (day %d): %s want %s", c.from, c.months, c.anchor, got.Format(time.RFC3339), c.expected)
		}
	}
}
