package integration_test

import (
	"testing"
)

// TestFilterCareMarkNeedsAUnit: marking a filter cleaned without a unit is refused as VALIDATION on unitId, before the
// unit or the customer is looked up (filterCare.markCleaned; MarkInput.Validate).
func TestFilterCareMarkNeedsAUnit(t *testing.T) {
	s := server(t)
	code, res := write(
		s,
		customerB(),
		"filterCare.markCleaned",
		`{"unitId":"00000000-0000-0000-0000-000000000000"}`,
		0,
	)
	refusal(t, "markCleaned nil unit", code, res, "unitId=error.required")
}

// TestFilterCareSettingsAreForCustomersOnly: HQ reads and saves no customer's filter care settings; the client scope
// gate refuses it FORBIDDEN before the customer lookup.
func TestFilterCareSettingsAreForCustomersOnly(t *testing.T) {
	srv := server(t)
	if code, _ := post(srv, hq(), "filterCare.getSettings", `{}`); code != 403 {
		t.Errorf("hq reads filter care settings: %d, want 403", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"filterCare.saveSettings",
		`{"thresholdHours":null,"fallbackDays":30,"recipients":"owners","channels":["inApp"]}`,
		0,
	); code != 403 {
		t.Errorf("hq saves filter care settings: %d, want 403", code)
	}
}
