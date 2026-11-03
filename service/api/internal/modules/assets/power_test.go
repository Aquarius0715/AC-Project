package assets

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEffectivePower(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	powerOn, off := true, false
	fromNow := func(d time.Duration) *time.Time {
		x := now.Add(d)

		return &x
	}
	newUnit := func(conn string, p *bool, obs *time.Time) *Unit {
		return &Unit{
			ACUnit: ACUnit{
				Connection: conn,
				ObservedState: ObservedState{
					Power:      p,
					ObservedAt: obs,
					Celsius:    nil,
					Mode:       nil,
					FanLevel:   nil,
				},
				ID:                  uuid.UUID{},
				TenantID:            uuid.UUID{},
				Version:             0,
				CreatedAt:           time.Time{},
				UpdatedAt:           time.Time{},
				CustomerOrgID:       uuid.UUID{},
				PropertyID:          uuid.UUID{},
				SpaceID:             nil,
				DisplayName:         "",
				ModelID:             uuid.UUID{},
				Type:                "",
				InstalledAt:         nil,
				ServiceScope:        nil,
				WarrantyEndsAt:      nil,
				AlertPolicyIDs:      nil,
				Archived:            false,
				CapabilityVersion:   0,
				ObservedRestriction: nil,
				LastSeenAt:          nil,
			}, EffectivePowerState: "", LatestMeasurements: nil, ActiveAlertCount: 0,
		}
	}

	cases := []struct {
		u           *Unit
		fresh, have bool
		want        string
	}{
		{newUnit("online", &powerOn, fromNow(-30*time.Second)), true, true, "on"},
		{newUnit("online", &off, fromNow(-120*time.Second)), true, true, "off"},
		{newUnit("online", &powerOn, fromNow(-121*time.Second)), true, true, "unknown"}, // too old
		{
			newUnit("online", &powerOn, fromNow(time.Second)),
			true,
			true,
			"unknown",
		}, // future observation
		{newUnit("offline", &powerOn, fromNow(-time.Second)), true, true, "unknown"},
		{newUnit("online", nil, fromNow(-time.Second)), true, true, "unknown"},
		{newUnit("online", &powerOn, nil), true, true, "unknown"},
		{
			newUnit("online", &powerOn, fromNow(-time.Second)),
			false,
			true,
			"unknown",
		}, // measured power stale / invalid
		{
			newUnit("online", &powerOn, fromNow(-time.Second)),
			true,
			false,
			"unknown",
		}, // no power measurement at all
	}
	for i, c := range cases {
		if got := EffectivePower(c.u, now, c.fresh, c.have); got != c.want {
			t.Errorf("case %d: got %s want %s", i, got, c.want)
		}
	}
}
