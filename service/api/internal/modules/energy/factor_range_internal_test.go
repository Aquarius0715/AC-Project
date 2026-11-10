package energy

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// A factor the numeric(10,6) column would round to zero is refused on its field, not left to the database's check
// (IR660); the smallest value that rounds to 0.000001 and the largest allowed value are accepted.
func TestFactorRangeFollowsTheColumn(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		value float64
		want  string
	}{
		"rounds to zero":    {4.9e-7, apperr.KeyRange},
		"zero":              {0, apperr.KeyRange},
		"smallest stored":   {5e-7, ""},
		"one millionth":     {1e-6, ""},
		"largest allowed":   {10, ""},
		"above the largest": {10.000001, apperr.KeyRange},
	} {
		input := FactorInput{
			ID:           nil,
			Region:       "Test",
			Year:         2030,
			KgCO2ePerKWh: testCase.value,
			Source:       "test",
			IsDemo:       true,
		}

		if got := input.Validate()["kgCO2ePerKWh"]; got != testCase.want {
			t.Errorf("%s: %q, want %q", name, got, testCase.want)
		}
	}
}
