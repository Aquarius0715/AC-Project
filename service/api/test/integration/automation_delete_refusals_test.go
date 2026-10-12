package integration

import (
	"testing"
)

// TestAutomationDeleteNeedsAnID: deleting an automation without an id is refused as VALIDATION on id, before any rule
// of the owner or the scope is checked (control automations.delete; AutomationIDInput.Validate).
func TestAutomationDeleteNeedsAnID(t *testing.T) {
	s := server(t)
	code, m := post(s, &hq, "automations.delete", `{"id":"00000000-0000-0000-0000-000000000000"}`)
	refusal(t, "automations.delete nil id", code, m, "id=error.required")
}
