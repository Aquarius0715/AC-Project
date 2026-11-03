package integration

import (
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestSaveOfAMissingUnitIsNotFound: updating a unit that does not exist is NOT_FOUND, and it is refused before any
// other rule (assets units.save with an id; writes.go unitsSaveUnit, the load of the current row).
func TestSaveOfAMissingUnitIsNotFound(t *testing.T) {
	srv := server(t)
	body := `{"id":"` + uuid.NewString() + `","customerOrgId":"` + seed.ID("org-customer-b").
		String() +
		`","propertyId":"` + seed.ID("property-home-b").
		String() +
		`","spaceId":null,"displayName":"Ghost AC",` +
		`"modelId":"` + seed.ID("ventilation-demo").
		String() +
		`","type":"split","installedAt":null,"serviceScope":["indoor"]}`

	code, m := write(srv, &hq, "units.save", body, 1)
	if code != 404 || m["messageKey"] != apperr.KeyNotFound {
		t.Fatalf("update of a missing unit: %d %v, want 404 NOT_FOUND", code, m)
	}
}
