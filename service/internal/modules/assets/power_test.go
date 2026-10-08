package assets

import (
	"testing"
	"time"
)

func TestEffectivePower(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	on, off := true, false
	at := func(d time.Duration) *time.Time { x := now.Add(d); return &x }
	mk := func(conn string, p *bool, obs *time.Time) *Unit {
		return &Unit{Connection: conn, ObservedState: ObservedState{Power: p, ObservedAt: obs}}
	}
	cases := []struct {
		u           *Unit
		fresh, have bool
		want        string
	}{
		{mk("online", &on, at(-30*time.Second)), true, true, "on"},
		{mk("online", &off, at(-120*time.Second)), true, true, "off"},
		{mk("online", &on, at(-121*time.Second)), true, true, "unknown"}, // too old
		{mk("online", &on, at(time.Second)), true, true, "unknown"},      // future observation
		{mk("offline", &on, at(-time.Second)), true, true, "unknown"},
		{mk("online", nil, at(-time.Second)), true, true, "unknown"},
		{mk("online", &on, nil), true, true, "unknown"},
		{mk("online", &on, at(-time.Second)), false, true, "unknown"}, // measured power stale / invalid
		{mk("online", &on, at(-time.Second)), true, false, "unknown"}, // no power measurement at all
	}
	for i, c := range cases {
		if got := EffectivePower(c.u, now, c.fresh, c.have); got != c.want {
			t.Errorf("case %d: got %s want %s", i, got, c.want)
		}
	}
}
