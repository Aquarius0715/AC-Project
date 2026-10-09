package scheduler

import "testing"

func TestResultChanges(t *testing.T) {
	if (Result{}).changes() != 0 {
		t.Fatal("an empty tick changes nothing")
	}
	r := Result{ExpiredOffers: 1, ExpiredCommands: 2, Runs: 3, Confirmed: 4, ExpiredProposals: 5, FrozenHistories: 6, Jobs: 7}
	if got := r.changes(); got != 28 {
		t.Fatalf("changes() = %d, want the sum of every counter (28)", got)
	}
}
