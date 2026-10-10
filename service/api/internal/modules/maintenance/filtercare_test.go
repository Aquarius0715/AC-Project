package maintenance

import (
	"testing"
	"time"
)

// TestReminderEvidence covers the evidence of a filter cleaning reminder (Figma Client 06a): run hours since the last
// cleaning, run hours without a recorded cleaning, days since the last cleaning when the run time is unknown, or
// neither — always "not a fault".
func TestReminderEvidence(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	hours, cleaned := 412.4, now.AddDate(0, 0, -95)
	for name, c := range map[string]struct {
		st   FilterStatus
		want string
	}{
		"hours since cleaning": {FilterStatus{RunHoursSinceCleaning: &hours, LastCleanedAt: &cleaned}, "Cleaning due (412 h of run time since the last cleaning). Not a fault."},
		"hours, never cleaned": {FilterStatus{RunHoursSinceCleaning: &hours}, "Cleaning due (412 h of run time, no cleaning recorded). Not a fault."},
		"days, no run time":    {FilterStatus{LastCleanedAt: &cleaned}, "Cleaning due (95 days since the last cleaning, run time unknown). Not a fault."},
		"nothing known":        {FilterStatus{}, "Cleaning due. Not a fault."},
	} {
		if got := reminderEvidence(c.st, now); got != c.want {
			t.Errorf("%s: %q", name, got)
		}
	}
}
