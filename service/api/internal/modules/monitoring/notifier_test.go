package monitoring

import (
	"testing"
	"time"
)

func TestInWindow(t *testing.T) {
	kl := time.FixedZone("MYT", 8*3600)
	at := func(day, h, m int) time.Time { return time.Date(2026, 9, day, h, m, 0, 0, kl) } // 2026-09-14 is a Monday
	day := &ActiveWindow{Weekdays: []int{1, 2, 3, 4, 5}, StartLocal: "08:00", EndLocal: "19:00"}
	night := &ActiveWindow{Weekdays: []int{1}, StartLocal: "22:00", EndLocal: "06:00"}
	sunday := &ActiveWindow{Weekdays: []int{7}, StartLocal: "23:00", EndLocal: "01:00"}
	cases := []struct {
		name string
		w    *ActiveWindow
		at   time.Time
		want bool
	}{
		{"no window", nil, at(14, 3, 0), true},
		{"weekday inside", day, at(14, 8, 0), true},
		{"weekday end is exclusive", day, at(14, 19, 0), false},
		{"saturday", day, at(19, 10, 0), false},
		{"overnight start day evening", night, at(14, 22, 30), true},
		{"overnight next morning", night, at(15, 5, 59), true},
		{"overnight end is exclusive", night, at(15, 6, 0), false},
		{"overnight before the start", night, at(14, 21, 59), false},
		{"overnight morning of the start day", night, at(14, 3, 0), false},
		{"overnight from sunday into monday", sunday, at(14, 0, 30), true},
		{"overnight sunday evening", sunday, at(20, 23, 30), true},
	}
	for _, c := range cases {
		if got := inWindow(c.w, c.at, "Asia/Kuala_Lumpur"); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if inWindow(day, at(14, 9, 0), "Nowhere/Unknown") {
		t.Error("an unknown time zone never matches")
	}
}
