package scheduler

import "testing"

func TestResultChanges(t *testing.T) {
	empty := Result{
		ExpiredOffers:    0,
		ExpiredCommands:  0,
		Runs:             0,
		Confirmed:        0,
		ExpiredProposals: 0,
		FrozenHistories:  0,
		Jobs:             0,
		Skipped:          0,
	}
	if empty.changes() != 0 {
		t.Fatal("an empty tick changes nothing")
	}

	r := Result{
		ExpiredOffers:    1,
		ExpiredCommands:  2,
		Runs:             3,
		Confirmed:        4,
		ExpiredProposals: 5,
		FrozenHistories:  6,
		Jobs:             7, Skipped: 0,
	}
	if got := r.changes(); got != 28 {
		t.Fatalf("changes() = %d, want the sum of every counter (28)", got)
	}
}
