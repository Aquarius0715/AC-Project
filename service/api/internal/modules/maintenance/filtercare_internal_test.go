package maintenance

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestReminderEvidence covers the evidence of a filter cleaning reminder (Figma Client 06a): run hours since the last
// cleaning, run hours without a recorded cleaning, days since the last cleaning when the run time is unknown, or
// neither — always "not a fault".
func TestReminderEvidence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)

	hours, cleaned := 412.4, now.AddDate(0, 0, -95)
	for name, testCase := range map[string]struct {
		st   FilterStatus
		want string
	}{
		"hours since cleaning": {
			FilterStatus{
				RunHoursSinceCleaning: &hours, LastCleanedAt: &cleaned, UnitID: uuid.UUID{}, ThresholdHours: 0,
				FallbackDays: 0, LastCleanedBy: nil, LastCleaningJobID: nil, Status: "",
			}, "Cleaning due (412 h of run time since the last cleaning). Not a fault.",
		},
		"hours, never cleaned": {
			FilterStatus{
				RunHoursSinceCleaning: &hours, UnitID: uuid.UUID{}, ThresholdHours: 0, FallbackDays: 0,
				LastCleanedAt: nil, LastCleanedBy: nil, LastCleaningJobID: nil, Status: "",
			}, "Cleaning due (412 h of run time, no cleaning recorded). Not a fault.",
		},
		"days, no run time": {
			FilterStatus{
				LastCleanedAt: &cleaned, UnitID: uuid.UUID{}, RunHoursSinceCleaning: nil, ThresholdHours: 0,
				FallbackDays: 0, LastCleanedBy: nil, LastCleaningJobID: nil, Status: "",
			}, "Cleaning due (95 days since the last cleaning, run time unknown). Not a fault.",
		},
		"nothing known": {
			FilterStatus{
				UnitID: uuid.UUID{}, RunHoursSinceCleaning: nil, ThresholdHours: 0, FallbackDays: 0, LastCleanedAt: nil,
				LastCleanedBy: nil, LastCleaningJobID: nil, Status: "",
			}, "Cleaning due. Not a fault.",
		},
	} {
		if got := reminderEvidence(testCase.st, now); got != testCase.want {
			t.Errorf("%s: %q", name, got)
		}
	}
}
