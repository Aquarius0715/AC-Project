package money_test

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/money"
)

func TestInRange(t *testing.T) {
	t.Parallel()

	for amount, want := range map[int64]bool{
		-1:                 false,
		0:                  true,
		1:                  true,
		money.MaxMinor:     true,
		money.MaxMinor + 1: false,
		1<<63 - 1:          false,
	} {
		if got := money.InRange(amount); got != want {
			t.Errorf("InRange(%d) = %v, want %v", amount, got, want)
		}
	}
}
