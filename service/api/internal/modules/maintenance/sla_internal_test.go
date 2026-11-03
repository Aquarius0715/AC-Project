package maintenance

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// slaRow is a saved SlaTargets row: the plan type's response hours from a time on, with its version.
func slaRow(plan string, responseHours, version int, from time.Time) SlaTargets {
	return SlaTargets{
		PlanType:               plan,
		ResponseHours:          responseHours,
		EffectiveFrom:          from,
		Version:                version,
		ID:                     uuid.UUID{},
		TenantID:               uuid.UUID{},
		CreatedAt:              time.Time{},
		UpdatedAt:              time.Time{},
		ArrivalInWindowPercent: 0,
		FirstTimeFixPercent:    0,
	}
}

// TestTargetFor: a job is held to the plan's target in effect when it was created; without one, the default.
func TestTargetFor(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

	all := []SlaTargets{
		slaRow(planRTO, 2, 1, when.Add(-48*time.Hour)),
		slaRow(planRTO, 3, 2, when.Add(-time.Hour)),
		slaRow(planRTO, 1, 3, when.Add(time.Hour)), // starts later
		slaRow(planGeneral, 6, 1, when.Add(-time.Hour)),
	}
	if got := targetFor(all, planRTO, when); got.Version != 2 || got.ResponseHours != 3 {
		t.Errorf("rto in effect: %+v", got)
	}

	if got := targetFor(all, planRTO, when.Add(-24*time.Hour)); got.Version != 1 {
		t.Errorf("rto a day earlier: %+v", got)
	}

	if got := targetFor(
		all,
		planEnergy,
		when,
	); got.ResponseHours != defaultSLA().ResponseHours ||
		!got.EffectiveFrom.IsZero() {
		t.Errorf("energy without targets: %+v", got)
	}

	if got := targetFor(
		all,
		planRTO,
		when.Add(-time.Hour),
	); got.Version != 2 { // effective from exactly then
		t.Errorf("rto at its start: %+v", got)
	}
}

// testTargetViews are the target views of an rto row in effect, a general row that starts in three days and an rto
// row that starts tomorrow: one view per plan type and then the two later rows.
func testTargetViews(t *testing.T) []TargetView {
	t.Helper()

	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	all := []SlaTargets{
		slaRow(planRTO, 3, 2, now.Add(-time.Hour)),
		slaRow(planGeneral, 1, 2, now.Add(72*time.Hour)),
		slaRow(planRTO, 1, 3, now.Add(24*time.Hour)),
	}

	views := targetViews(all, now)
	if len(views) != len(planTypes())+2 {
		t.Fatalf("views: %+v", views)
	}

	return views
}

// TestTargetViews: per plan type the targets in effect, a saved row or the default without a version (IR236).
func TestTargetViews(t *testing.T) {
	t.Parallel()

	views := testTargetViews(t)
	if views[0].PlanType != planRTO || views[0].State != targetsInEffect || views[0].Version != 2 ||
		views[0].EffectiveFrom == nil {
		t.Errorf("rto in effect: %+v", views[0])
	}

	if views[1].PlanType != planGeneral || views[1].State != targetsDefault ||
		views[1].EffectiveFrom != nil ||
		views[1].ResponseHours != defaultSLA().ResponseHours {
		t.Errorf("general default: %+v", views[1])
	}
}

// TestTargetViewsScheduled: after the plan types' views, the rows that start later, earliest first (IR236).
func TestTargetViewsScheduled(t *testing.T) {
	t.Parallel()

	views := testTargetViews(t)

	later := views[len(planTypes()):]
	if later[0].PlanType != planRTO || later[0].Version != 3 || later[1].PlanType != planGeneral ||
		later[0].State != targetsScheduled ||
		later[1].State != targetsScheduled {
		t.Errorf("scheduled, earliest first: %+v", later)
	}
}

// shares are the Metrics of the response, arrival and first-time-fix shares (nil when there is nothing to measure)
// and the open overdue jobs.
func shares(response, arrival, firstTimeFix *float64, overdue int) Metrics {
	return Metrics{
		ResponseWithinTarget: response,
		ArrivalInWindow:      arrival,
		FirstTimeFix:         firstTimeFix,
		AverageRating:        nil,
		RatingCount:          0,
		OpenOverdue:          overdue,
	}
}

// TestCustomerStatus: on track at or above every target, at risk up to 10 points below, breached further below or with
// an open overdue job; a share without jobs says nothing (DD-A22).
func TestCustomerStatus(t *testing.T) {
	t.Parallel()

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
	for name, testCase := range map[string]struct {
		m    Metrics
		want string
	}{
		"all met":            {shares(new(float64(100)), new(float64(95)), new(float64(85)), 0), slaOnTrack},
		"nothing measured":   {shares(nil, nil, nil, 0), slaOnTrack},
		"arrival 5 below":    {shares(new(float64(100)), new(float64(85)), nil, 0), slaAtRisk},
		"response 10 below":  {shares(new(float64(90)), nil, nil, 0), slaAtRisk},
		"response 11 below":  {shares(new(float64(89)), nil, nil, 0), slaBreached},
		"risk then breach":   {shares(nil, new(float64(89)), new(float64(70)), 0), slaBreached},
		"breach then risk":   {shares(new(float64(50)), nil, new(float64(80)), 0), slaBreached},
		"overdue, all met":   {shares(new(float64(100)), nil, nil, 1), slaBreached},
		"first-time fix met": {shares(nil, nil, new(float64(85)), 0), slaOnTrack},
	} {
		if got := customerStatus(testCase.m, tgt); got != testCase.want {
			t.Errorf("%s: %s, want %s", name, got, testCase.want)
		}
	}
}
