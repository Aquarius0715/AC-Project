package assets

import (
	"testing"
	"time"
)

func TestCoverageStatus(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	d := func(days int) *time.Time { x := now.Add(time.Duration(days) * 24 * time.Hour); return &x }
	cases := []struct {
		ends      *time.Time
		contracts int
		want      string
	}{
		{d(10), 1, "contract"},
		{nil, 1, "contract"},
		{d(91), 0, "under_warranty"},
		{d(90), 0, "expiring"},
		{d(1), 0, "expiring"},
		{d(0), 0, "no_coverage"},
		{d(-5), 0, "no_coverage"},
		{nil, 0, "no_coverage"},
	}
	for i, c := range cases {
		if got := CoverageStatus(now, c.ends, c.contracts); got != c.want {
			t.Errorf("case %d: %s want %s", i, got, c.want)
		}
	}
}
