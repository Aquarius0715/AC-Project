package maintenance

import (
	"testing"
	"time"
)

// TestWorkedFor (IR449): time on site leaves out pauses and the periods off the job, overlaps once.
func TestWorkedFor(t *testing.T) {
	t.Parallel()

	atMinute := func(m int) time.Time {
		return time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC).Add(time.Duration(m) * time.Minute)
	}

	interval := func(a, b int) [2]time.Time { return [2]time.Time{atMinute(a), atMinute(b)} }
	for name, testCase := range map[string]struct {
		off  [][2]time.Time
		want int
	}{
		"nothing off": {nil, 120},
		"one pause":   {[][2]time.Time{interval(10, 20)}, 110},
		// the hold runs past the end: 30 minutes worked
		"a hold of a day":               {[][2]time.Time{interval(30, 30+24*60)}, 30},
		"a pause inside a hold":         {[][2]time.Time{interval(30, 90), interval(40, 50)}, 60},
		"overlapping, out of order":     {[][2]time.Time{interval(50, 70), interval(40, 60)}, 90},
		"before the arrival is clipped": {[][2]time.Time{interval(-30, 10)}, 110},
	} {
		if got := int(
			workedFor(atMinute(0), atMinute(120), testCase.off) / time.Minute,
		); got != testCase.want {
			t.Errorf("%s: %d minutes, want %d", name, got, testCase.want)
		}
	}
}
