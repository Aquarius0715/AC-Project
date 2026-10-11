package billing

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestEndedPeriod: the monthly drafts are for the month that ended last in Kuala Lumpur — UTC 16:00 on the last day
// is already the 1st there — including across a year (IR341).
func TestEndedPeriod(t *testing.T) {
	for name, c := range map[string]struct {
		now  time.Time
		want string
	}{
		"mid month":                {time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC), "2026-08"},
		"the 1st in Kuala Lumpur":  {time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC), "2026-09"},
		"the last day there":       {time.Date(2026, 9, 30, 15, 59, 0, 0, time.UTC), "2026-08"},
		"January":                  {time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC), "2026-12"},
		"New Year in Kuala Lumpur": {time.Date(2026, 12, 31, 16, 30, 0, 0, time.UTC), "2026-12"},
	} {
		if got := endedPeriod(c.now); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

// TestTransitionInput: payouts.transition approves or marks paid, with an optional reason up to 1000 characters. Over
// REST the action comes from the route (/approve, /mark-paid), so an unknown action reaches this rule only from the
// other callers of the operation.
func TestTransitionInput(t *testing.T) {
	long := strings.Repeat("x", 1001)
	blank := "  ok  "
	for name, c := range map[string]struct {
		in   TransitionInput
		want map[string]string
	}{
		"approve":        {TransitionInput{StatementID: uuid.New(), Action: "approve"}, map[string]string{}},
		"mark paid":      {TransitionInput{StatementID: uuid.New(), Action: "mark_paid", Reason: &blank}, map[string]string{}},
		"unknown action": {TransitionInput{StatementID: uuid.New(), Action: "refund"}, map[string]string{"action": "error.invalid"}},
		"no statement":   {TransitionInput{Action: "approve"}, map[string]string{"statementId": "error.required"}},
		"long reason":    {TransitionInput{StatementID: uuid.New(), Action: "approve", Reason: &long}, map[string]string{"reason": "error.length"}},
	} {
		in := c.in
		if got := in.Validate(); !maps.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	in := TransitionInput{StatementID: uuid.New(), Action: "approve", Reason: &blank}
	if in.Validate(); *in.Reason != "ok" {
		t.Errorf("the reason is trimmed: %q", *in.Reason)
	}
}
