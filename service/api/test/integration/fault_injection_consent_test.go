package integration

import (
	"testing"
)

// TestConsentChangeRollsBackWhenItCannotBeWritten: a client's consent that cannot be written is not granted. The
// operation fails as a server error, and the consent reads back as it was before, version included, because the
// database rolls the transaction back (consents.update; the consent write fails). The consent is first revoked, since
// a grant of a consent already in force writes nothing and would not reach the failing statement.
func TestConsentChangeRollsBackWhenItCannotBeWritten(t *testing.T) {
	s := server(t)
	get := func() map[string]any {
		_, m := post(s, &customerB, "consents.get", `{"purpose":"location_automation"}`)
		return m
	}
	before := get()
	if data(before)["granted"] == true {
		if code, m := write(s, &customerB, "consents.update", `{"purpose":"location_automation","granted":false}`, ver(before)); code != 200 {
			t.Fatalf("revoke before the test: %d %v", code, m)
		}
		before = get()
	}

	broken := injectAt(t, "UPDATE identity.consents SET granted = ")
	code, m := write(broken, &customerB, "consents.update", `{"purpose":"location_automation","granted":true}`, ver(before))
	if code < 500 {
		t.Fatalf("consent with a failing write: %d %v, want a server error", code, m)
	}
	after := get()
	if data(after)["granted"] != false || data(after)["grantedAt"] != data(before)["grantedAt"] || ver(after) != ver(before) {
		t.Errorf("a failed consent change moved the consent: before %v, after %v", before, after)
	}
}
