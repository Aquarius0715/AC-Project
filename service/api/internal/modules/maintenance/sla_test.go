package maintenance

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestTargetFor: a job is held to the plan's target in effect when it was created; without one, the default.
func TestTargetFor(t *testing.T) {
	at := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

	all := []SlaTargets{
		{
			PlanType:               "rto",
			ResponseHours:          2,
			EffectiveFrom:          at.Add(-48 * time.Hour),
			Version:                1,
			ID:                     uuid.UUID{},
			TenantID:               uuid.UUID{},
			CreatedAt:              time.Time{},
			UpdatedAt:              time.Time{},
			ArrivalInWindowPercent: 0,
			FirstTimeFixPercent:    0,
		},
		{
			PlanType:               "rto",
			ResponseHours:          3,
			EffectiveFrom:          at.Add(-time.Hour),
			Version:                2,
			ID:                     uuid.UUID{},
			TenantID:               uuid.UUID{},
			CreatedAt:              time.Time{},
			UpdatedAt:              time.Time{},
			ArrivalInWindowPercent: 0,
			FirstTimeFixPercent:    0,
		},
		{
			PlanType:               "rto",
			ResponseHours:          1,
			EffectiveFrom:          at.Add(time.Hour),
			Version:                3,
			ID:                     uuid.UUID{},
			TenantID:               uuid.UUID{},
			CreatedAt:              time.Time{},
			UpdatedAt:              time.Time{},
			ArrivalInWindowPercent: 0,
			FirstTimeFixPercent:    0,
		}, // starts later
		{
			PlanType:               "general",
			ResponseHours:          6,
			EffectiveFrom:          at.Add(-time.Hour),
			Version:                1,
			ID:                     uuid.UUID{},
			TenantID:               uuid.UUID{},
			CreatedAt:              time.Time{},
			UpdatedAt:              time.Time{},
			ArrivalInWindowPercent: 0,
			FirstTimeFixPercent:    0,
		},
	}
	if got := targetFor(all, "rto", at); got.Version != 2 || got.ResponseHours != 3 {
		t.Errorf("rto in effect: %+v", got)
	}

	if got := targetFor(all, "rto", at.Add(-24*time.Hour)); got.Version != 1 {
		t.Errorf("rto a day earlier: %+v", got)
	}

	if got := targetFor(
		all,
		"energy",
		at,
	); got.ResponseHours != DefaultSLA.ResponseHours ||
		!got.EffectiveFrom.IsZero() {
		t.Errorf("energy without targets: %+v", got)
	}

	if got := targetFor(
		all,
		"rto",
		at.Add(-time.Hour),
	); got.Version != 2 { // effective from exactly then
		t.Errorf("rto at its start: %+v", got)
	}
}

// TestTargetViews: per plan type the targets in effect (a saved row or the default without a version), then the rows
// that start later, earliest first (IR236).
func TestTargetViews(t *testing.T) {
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	all := []SlaTargets{
		{
			PlanType:               "rto",
			ResponseHours:          3,
			EffectiveFrom:          now.Add(-time.Hour),
			Version:                2,
			ID:                     uuid.UUID{},
			TenantID:               uuid.UUID{},
			CreatedAt:              time.Time{},
			UpdatedAt:              time.Time{},
			ArrivalInWindowPercent: 0,
			FirstTimeFixPercent:    0,
		},
		{
			PlanType:               "general",
			ResponseHours:          1,
			EffectiveFrom:          now.Add(72 * time.Hour),
			Version:                2,
			ID:                     uuid.UUID{},
			TenantID:               uuid.UUID{},
			CreatedAt:              time.Time{},
			UpdatedAt:              time.Time{},
			ArrivalInWindowPercent: 0,
			FirstTimeFixPercent:    0,
		},
		{
			PlanType:               "rto",
			ResponseHours:          1,
			EffectiveFrom:          now.Add(24 * time.Hour),
			Version:                3,
			ID:                     uuid.UUID{},
			TenantID:               uuid.UUID{},
			CreatedAt:              time.Time{},
			UpdatedAt:              time.Time{},
			ArrivalInWindowPercent: 0,
			FirstTimeFixPercent:    0,
		},
	}

	v := targetViews(all, now)
	if len(v) != len(planTypes)+2 {
		t.Fatalf("views: %+v", v)
	}

	if v[0].PlanType != "rto" || v[0].State != "in_effect" || v[0].Version != 2 ||
		v[0].EffectiveFrom == nil {
		t.Errorf("rto in effect: %+v", v[0])
	}

	if v[1].PlanType != "general" || v[1].State != "default" || v[1].EffectiveFrom != nil ||
		v[1].ResponseHours != DefaultSLA.ResponseHours {
		t.Errorf("general default: %+v", v[1])
	}

	later := v[len(planTypes):]
	if later[0].PlanType != "rto" || later[0].Version != 3 || later[1].PlanType != "general" ||
		later[0].State != "scheduled" ||
		later[1].State != "scheduled" {
		t.Errorf("scheduled, earliest first: %+v", later)
	}
}

// TestCustomerStatus: on track at or above every target, at risk up to 10 points below, breached further below or with
// an open overdue job; a share without jobs says nothing (DD-A22).
func TestCustomerStatus(t *testing.T) {
	tgt := SlaTargets{
		ArrivalInWindowPercent: 90,
		FirstTimeFixPercent:    85,
		ID:                     uuid.UUID{},
		TenantID:               uuid.UUID{},
		Version:                0,
		CreatedAt:              time.Time{},
		UpdatedAt:              time.Time{},
		PlanType:               "",
		ResponseHours:          0,
		EffectiveFrom:          time.Time{},
	}
	for name, c := range map[string]struct {
		m    Metrics
		want string
	}{
		"all met": {
			Metrics{
				ResponseWithinTarget: new(float64(100)), ArrivalInWindow: new(float64(95)), FirstTimeFix: new(float64(85)),
				AverageRating: nil, RatingCount: 0, OpenOverdue: 0,
			}, "on_track",
		},
		"nothing measured": {
			Metrics{
				ResponseWithinTarget: nil, ArrivalInWindow: nil, FirstTimeFix: nil, AverageRating: nil, RatingCount: 0,
				OpenOverdue: 0,
			}, "on_track",
		},
		"arrival 5 below": {
			Metrics{
				ResponseWithinTarget: new(float64(100)), ArrivalInWindow: new(float64(85)), FirstTimeFix: nil,
				AverageRating: nil, RatingCount: 0, OpenOverdue: 0,
			}, "at_risk",
		},
		"response 10 below": {
			Metrics{
				ResponseWithinTarget: new(float64(90)), ArrivalInWindow: nil, FirstTimeFix: nil, AverageRating: nil,
				RatingCount: 0, OpenOverdue: 0,
			}, "at_risk",
		},
		"response 11 below": {
			Metrics{
				ResponseWithinTarget: new(float64(89)), ArrivalInWindow: nil, FirstTimeFix: nil, AverageRating: nil,
				RatingCount: 0, OpenOverdue: 0,
			}, "breached",
		},
		"risk then breach": {
			Metrics{
				ArrivalInWindow: new(float64(89)), FirstTimeFix: new(float64(70)), ResponseWithinTarget: nil,
				AverageRating: nil, RatingCount: 0, OpenOverdue: 0,
			}, "breached",
		},
		"breach then risk": {
			Metrics{
				ResponseWithinTarget: new(float64(50)), FirstTimeFix: new(float64(80)), ArrivalInWindow: nil,
				AverageRating: nil, RatingCount: 0, OpenOverdue: 0,
			}, "breached",
		},
		"overdue, all met": {
			Metrics{
				ResponseWithinTarget: new(float64(100)), OpenOverdue: 1, ArrivalInWindow: nil, FirstTimeFix: nil,
				AverageRating: nil, RatingCount: 0,
			}, "breached",
		},
		"first-time fix met": {
			Metrics{
				FirstTimeFix: new(float64(85)), ResponseWithinTarget: nil, ArrivalInWindow: nil, AverageRating: nil,
				RatingCount: 0, OpenOverdue: 0,
			}, "on_track",
		},
	} {
		if got := customerStatus(c.m, tgt); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}
