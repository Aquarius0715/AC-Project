package billing

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/money"
)

// The amounts HQ enters stay within JavaScript's safe-integer range (Money in implementation-contracts.md): the web
// shows a larger one rounded, and a sum of such amounts can wrap around.
func TestAmountsStayInSafeRange(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		amount int64
		field  string
		check  func(amount int64) map[string]string
	}{
		"invoice": {money.MaxMinor + 1, "amountMinor", func(a int64) map[string]string {
			var in InvoiceInput

			in.AmountMinor = a

			return in.Validate()
		}},
		"contract price": {money.MaxMinor + 1, "priceMinor", func(a int64) map[string]string {
			var in ContractInput

			in.PriceMinor = a

			return in.Validate()
		}},
	} {
		if got := testCase.check(testCase.amount)[testCase.field]; got != apperr.KeyRange {
			t.Errorf("%s over the range: %q", name, got)
		}

		if got := testCase.check(money.MaxMinor)[testCase.field]; got != "" {
			t.Errorf("%s at the range's end is refused: %q", name, got)
		}
	}
}
