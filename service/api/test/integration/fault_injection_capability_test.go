package integration_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestCapabilityVersionRollsBackWhenThePreviousVersionCannotBeRetired: a new version of a device capability is not kept
// when the previous version cannot be retired as current. The operation fails as a server error, and the capability
// still has only its first version (capabilities.save; the is_current update fails).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCapabilityVersionRollsBackWhenThePreviousVersionCannotBeRetired(t *testing.T) {
	s := server(t)
	model := "FI-" + uuid.NewString()[:8]

	code, res := write(s, hq(), "capabilities.save", capBody("", model, ""), 0)
	if code != 200 || ver(t, res) != 1 {
		t.Fatalf("create: %d %v", code, res)
	}

	capabilityID := asStr(t, data(res)["id"])

	versions := func() (int, int) {
		var total, top int
		ownerScan(
			t,
			`SELECT count(*), max(version) FROM devices.capabilities WHERE id = $1`,
			[]any{capabilityID},
			&total,
			&top,
		)

		return total, top
	}
	versionCount, top := versions()

	broken := injectAt(t, "UPDATE devices.capabilities SET is_current = false")
	body := strings.Replace(
		capBody(capabilityID, model, `,"changeReason":"added fan mode"`),
		`["cool","dry"]`,
		`["cool","dry","fan"]`,
		1,
	)

	code, res = write(broken, hq(), "capabilities.save", body, 1)
	if code < 500 {
		t.Fatalf("new version with a failing retire: %d %v, want a server error", code, res)
	}

	if n2, top2 := versions(); n2 != versionCount || top2 != top {
		t.Errorf(
			"a failed save kept a version: before %d versions (top %d), after %d (top %d)",
			versionCount,
			top,
			n2,
			top2,
		)
	}
}
