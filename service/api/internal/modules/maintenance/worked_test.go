package maintenance

import (
	"testing"
	"time"
)

// TestWorkedFor (IR449): time on site leaves out pauses and the periods off the job, overlaps once.
func TestWorkedFor(t *testing.T) {
	at := func(m int) time.Time {
		return time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC).Add(time.Duration(m) * time.Minute)
	}
	iv := func(a, b int) [2]time.Time { return [2]time.Time{at(a), at(b)} }
	for name, c := range map[string]struct {
		off  [][2]time.Time
		want int
	}{
		"nothing off":                   {nil, 120},
		"one pause":                     {[][2]time.Time{iv(10, 20)}, 110},
		"a hold of a day":               {[][2]time.Time{iv(30, 30+24*60)}, 30}, // the hold runs past the end: 30 minutes worked
		"a pause inside a hold":         {[][2]time.Time{iv(30, 90), iv(40, 50)}, 60},
		"overlapping, out of order":     {[][2]time.Time{iv(50, 70), iv(40, 60)}, 90},
		"before the arrival is clipped": {[][2]time.Time{iv(-30, 10)}, 110},
	} {
		if got := int(workedFor(at(0), at(120), c.off) / time.Minute); got != c.want {
			t.Errorf("%s: %d minutes, want %d", name, got, c.want)
		}
	}
}
