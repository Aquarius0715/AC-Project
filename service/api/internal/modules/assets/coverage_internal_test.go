package assets

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

func TestCoverageStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	daysFromNow := func(days int) *time.Time {
		x := now.Add(time.Duration(days) * 24 * time.Hour)

		return &x
	}

	cases := []struct {
		ends      *time.Time
		contracts int
		want      string
	}{
		{daysFromNow(10), 1, "contract"},
		{nil, 1, "contract"},
		{daysFromNow(91), 0, "under_warranty"},
		{daysFromNow(90), 0, coverageExpiring},
		{daysFromNow(1), 0, coverageExpiring},
		{daysFromNow(0), 0, coverageNone},
		{daysFromNow(-5), 0, coverageNone},
		{nil, 0, coverageNone},
	}
	for i, c := range cases {
		if got := CoverageStatus(now, c.ends, c.contracts); got != c.want {
			t.Errorf("case %d: %s want %s", i, got, c.want)
		}
	}
}

func TestClaimable(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	recent, older, later := uuid.New(), uuid.New(), uuid.New()
	jobs := map[uuid.UUID]time.Time{
		recent: now.Add(-time.Hour),
		older:  now.Add(-48 * time.Hour),
		later:  now.Add(time.Hour),
	}
	ends := now

	got := claimable(jobs, &ends)
	if len(got) != 2 || got[0] != older || got[1] != recent {
		t.Fatalf("completed while the warranty ran, earliest first: %v", got)
	}

	if got := claimable(jobs, nil); len(got) != 0 {
		t.Fatalf("no warranty → nothing claimable: %v", got)
	}

	exact := now.Add(-time.Hour)
	if got := claimable(map[uuid.UUID]time.Time{recent: exact}, &exact); len(got) != 1 {
		t.Fatalf("completion on the last warranty instant counts: %v", got)
	}
}

// TestWarrantyStage: the latest BR-A19 reminder a warranty end has reached — 90 or 30 days before it, each from that
// very instant — and none earlier than 90 days (IR340).
func TestWarrantyStage(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

	day := 24 * time.Hour
	for name, testCase := range map[string]struct {
		ends time.Time
		want int
	}{
		"91 days":        {now.Add(91 * day), 0},
		"a second early": {now.Add(90*day + time.Second), 0},
		"90 days":        {now.Add(90 * day), 90},
		"31 days":        {now.Add(31 * day), 90},
		"30 days":        {now.Add(30 * day), 30},
		"an hour":        {now.Add(time.Hour), 30},
	} {
		if got := warrantyStage(testCase.ends, now); got != testCase.want {
			t.Errorf("%s: %d, want %d", name, got, testCase.want)
		}
	}
}

// TestSortCoverage: each sort field follows its direction — id and name ascending or descending, dueAt by warranty end
// with the units without one last either way — and ties go by unit ID.
func TestSortCoverage(t *testing.T) {
	t.Parallel()

	const asc, desc = "asc", "desc"

	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	soon, later := now.Add(24*time.Hour), now.Add(48*time.Hour)
	first, second, third, fourth := uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		uuid.MustParse("00000000-0000-0000-0000-000000000003"),
		uuid.MustParse("00000000-0000-0000-0000-000000000004")
	unit := func(id uuid.UUID, name string, ends *time.Time) UnitCoverage {
		return UnitCoverage{
			UnitID:          id,
			CustomerID:      uuid.Nil,
			ModelID:         uuid.Nil,
			WarrantyEndsAt:  ends,
			ContractIDs:     nil,
			Status:          "",
			ClaimableJobIDs: nil,
			name:            name,
		}
	}
	units := func() []UnitCoverage {
		return []UnitCoverage{
			unit(second, "b", &later),
			unit(fourth, "a", nil),
			unit(first, "c", &soon),
			unit(third, "d", &later),
		}
	}

	for name, testCase := range map[string]struct {
		field, direction string
		want             []uuid.UUID
	}{
		"id ascending":    {"id", asc, []uuid.UUID{first, second, third, fourth}},
		"id descending":   {"id", desc, []uuid.UUID{fourth, third, second, first}},
		"name ascending":  {keyName, asc, []uuid.UUID{fourth, second, first, third}},
		"name descending": {keyName, desc, []uuid.UUID{third, first, second, fourth}},
		"dueAt ascending": {keyDueAt, asc, []uuid.UUID{first, second, third, fourth}},
		// ties (second and third end together) go by unit ID in both directions; the unit without a warranty stays last
		"dueAt descending": {keyDueAt, desc, []uuid.UUID{second, third, first, fourth}},
	} {
		got := units()
		sortCoverage(got, &paging.SortSpec{Field: testCase.field, Direction: testCase.direction})

		for i, want := range testCase.want {
			if got[i].UnitID != want {
				t.Errorf("%s: position %d is %v, want %v", name, i, got[i].UnitID, want)

				break
			}
		}
	}
}
