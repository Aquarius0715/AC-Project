package integration_test

import (
	"testing"
)

// TestConsentChangeRollsBackWhenItCannotBeWritten: a client's consent that cannot be written is not granted. The
// operation fails as a server error, and the consent reads back as it was before, version included, because the
// database rolls the transaction back (consents.update; the consent write fails). The consent is first revoked, since
// a grant of a consent already in force writes nothing and would not reach the failing statement.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestConsentChangeRollsBackWhenItCannotBeWritten(t *testing.T) {
	srv := server(t)
	get := func() map[string]any {
		_, m := post(srv, customerB(), "consents.get", `{"purpose":"location_automation"}`)

		return m
	}

	before := get()
	if data(before)["granted"] == true {
		if code, resp := write(
			srv,
			customerB(),
			"consents.update",
			`{"purpose":"location_automation","granted":false}`,
			ver(t, before),
		); code != 200 {
			t.Fatalf("revoke before the test: %d %v", code, resp)
		}

		before = get()
	}

	broken := injectAt(t, "UPDATE identity.consents SET granted = ")

	code, res := write(
		broken,
		customerB(),
		"consents.update",
		`{"purpose":"location_automation","granted":true}`,
		ver(t, before),
	)
	if code < 500 {
		t.Fatalf("consent with a failing write: %d %v, want a server error", code, res)
	}

	after := get()
	if data(after)["granted"] != false || data(after)["grantedAt"] != data(before)["grantedAt"] ||
		ver(t, after) != ver(t, before) {
		t.Errorf("a failed consent change moved the consent: before %v, after %v", before, after)
	}
}
