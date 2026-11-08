package maintenance

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/money"
)

// The amounts HQ and contractors enter stay within JavaScript's safe-integer range (Money in
// implementation-contracts.md): the web shows a larger one rounded, and a sum of such amounts can wrap around.
func TestAmountsStayInSafeRange(t *testing.T) {
	t.Parallel()

	over, last := int(money.MaxMinor+1), int(money.MaxMinor)

	for name, testCase := range map[string]struct {
		field string
		check func(amount int) map[string]string
	}{
		"cost line": {"costLines", func(a int) map[string]string {
			var costs CostInput

			costs.CostLines = make([]CostLine, 1)
			costs.CostLines[0].AmountMinor, costs.CostLines[0].Currency, costs.CostLines[0].Kind = a, currencyMYR, "actual"
			costs.CostLines[0].Description, costs.CostLines[0].Visibility = "x", visibilityInternal

			return costs.Validate()
		}},
		"warranty claim": {"amountMinor", func(a int) map[string]string {
			var in ClaimInput

			in.AmountMinor = a

			return in.Validate()
		}},
		"rate card line": {"lines", func(a int) map[string]string {
			var in RateCardInput

			in.Lines = make([]RateLine, 1)
			in.Lines[0].WorkType, in.Lines[0].AmountMinor = workTypes()[0], a

			return in.Validate()
		}},
	} {
		if got := testCase.check(over)[testCase.field]; got == "" {
			t.Errorf("%s over the range is accepted", name)
		}

		if got := testCase.check(last)[testCase.field]; got != "" {
			t.Errorf("%s at the range's end is refused: %q", name, got)
		}
	}
}
