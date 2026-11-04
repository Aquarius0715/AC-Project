package monitoring

import (
	"testing"
	"time"
)

func TestInWindow(t *testing.T) {
	t.Parallel()

	kl := time.FixedZone("MYT", 8*3600)
	localTime := func(day, h, m int) time.Time { return time.Date(2026, 9, day, h, m, 0, 0, kl) } // 2026-09-14 is a Monday
	day := &ActiveWindow{Weekdays: []int{1, 2, 3, 4, 5}, StartLocal: "08:00", EndLocal: "19:00"}
	night := &ActiveWindow{Weekdays: []int{1}, StartLocal: "22:00", EndLocal: "06:00"}
	sunday := &ActiveWindow{Weekdays: []int{7}, StartLocal: "23:00", EndLocal: "01:00"}

	cases := []struct {
		name string
		w    *ActiveWindow
		at   time.Time
		want bool
	}{
		{"no window", nil, localTime(14, 3, 0), true},
		{"weekday inside", day, localTime(14, 8, 0), true},
		{"weekday end is exclusive", day, localTime(14, 19, 0), false},
		{"saturday", day, localTime(19, 10, 0), false},
		{"overnight start day evening", night, localTime(14, 22, 30), true},
		{"overnight next morning", night, localTime(15, 5, 59), true},
		{"overnight end is exclusive", night, localTime(15, 6, 0), false},
		{"overnight before the start", night, localTime(14, 21, 59), false},
		{"overnight morning of the start day", night, localTime(14, 3, 0), false},
		{"overnight from sunday into monday", sunday, localTime(14, 0, 30), true},
		{"overnight sunday evening", sunday, localTime(20, 23, 30), true},
	}
	for _, c := range cases {
		if got := inWindow(c.w, c.at, "Asia/Kuala_Lumpur"); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	if inWindow(day, localTime(14, 9, 0), "Nowhere/Unknown") {
		t.Error("an unknown time zone never matches")
	}
}

// TestCompareAndRecover covers the four threshold operators at the boundary and the recovery side of each (D08): a high
// limit (gt / gte) recovers below its recovery threshold, a low limit (lt / lte) above it.
func TestCompareAndRecover(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		op         string
		at, beyond bool // the value equal to the threshold, and one past it on the limit's side
	}{
		{"gt", false, true}, {opGTE, true, true}, {"lt", false, true}, {"lte", true, true},
	} {
		past := 29.0
		if testCase.op == "lt" || testCase.op == "lte" {
			past = 27.0
		}

		if got := cmp(testCase.op, 28, 28); got != testCase.at {
			t.Errorf("%s at the threshold: %v", testCase.op, got)
		}

		if got := cmp(testCase.op, past, 28); got != testCase.beyond {
			t.Errorf("%s past the threshold: %v", testCase.op, got)
		}
	}

	high, low := AlertCondition{
		Operator:          opGTE,
		Threshold:         28,
		RecoveryThreshold: 26, Metric: "", DurationSeconds: 0, ActiveWindow: nil, Severity: "",
	}, AlertCondition{
		Operator:          "lt",
		Threshold:         16,
		RecoveryThreshold: 18, Metric: "", DurationSeconds: 0, ActiveWindow: nil, Severity: "",
	}
	if !recovered(high, 25.9) || recovered(high, 26) || recovered(high, 27) {
		t.Error("a high limit recovers below its recovery threshold only")
	}

	if !recovered(low, 18.1) || recovered(low, 18) || recovered(low, 17) {
		t.Error("a low limit recovers above its recovery threshold only")
	}
}
