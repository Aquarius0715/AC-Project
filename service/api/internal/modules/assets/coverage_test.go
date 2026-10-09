package assets

import (
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestClaimable(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	jobs := map[uuid.UUID]time.Time{a: now.Add(-time.Hour), b: now.Add(-48 * time.Hour), c: now.Add(time.Hour)}
	ends := now
	got := claimable(jobs, &ends)
	if len(got) != 2 || got[0] != b || got[1] != a {
		t.Fatalf("completed while the warranty ran, earliest first: %v", got)
	}
	if got := claimable(jobs, nil); len(got) != 0 {
		t.Fatalf("no warranty → nothing claimable: %v", got)
	}
	exact := now.Add(-time.Hour)
	if got := claimable(map[uuid.UUID]time.Time{a: exact}, &exact); len(got) != 1 {
		t.Fatalf("completion on the last warranty instant counts: %v", got)
	}
}
