package assets

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

func TestUnitGetInputNeedsID(t *testing.T) {
	t.Parallel()

	missing := new(UnitGetInput)
	if got := missing.Validate(); got["id"] != apperr.KeyRequired || len(got) != 1 {
		t.Errorf("units.get without id: %v", got)
	}

	given := new(UnitGetInput)
	given.ID = uuid.New()

	if got := given.Validate(); got != nil {
		t.Errorf("units.get with id: %v", got)
	}
}
