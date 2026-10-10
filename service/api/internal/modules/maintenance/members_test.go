package maintenance

import (
	"testing"
	"time"
)

// TestUnionMinutes covers the busy time of a technician's day (members.capacity): overlapping, contained and
// touching slots count once, gaps do not count, and the order of the slots does not matter.
func TestUnionMinutes(t *testing.T) {
	base := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	slot := func(fromMin, toMin int) Slot {
		return Slot{StartAt: base.Add(time.Duration(fromMin) * time.Minute), EndAt: base.Add(time.Duration(toMin) * time.Minute)}
	}
	for name, c := range map[string]struct {
		slots []Slot
		want  int
	}{
		"none":        {nil, 0},
		"one":         {[]Slot{slot(60, 120)}, 60},
		"overlapping": {[]Slot{slot(60, 120), slot(90, 150)}, 90},
		"contained":   {[]Slot{slot(60, 180), slot(90, 120)}, 120},
		"touching":    {[]Slot{slot(60, 120), slot(120, 180)}, 120},
		"apart":       {[]Slot{slot(60, 120), slot(180, 210)}, 90},
		"unsorted":    {[]Slot{slot(300, 330), slot(60, 120), slot(100, 130)}, 100},
	} {
		if got := unionMinutes(c.slots); got != c.want {
			t.Errorf("%s: %d want %d", name, got, c.want)
		}
	}
}
