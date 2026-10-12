package integration

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestCapabilityVersionRollsBackWhenThePreviousVersionCannotBeRetired: a new version of a device capability is not kept
// when the previous version cannot be retired as current. The operation fails as a server error, and the capability
// still has only its first version (capabilities.save; the is_current update fails).
func TestCapabilityVersionRollsBackWhenThePreviousVersionCannotBeRetired(t *testing.T) {
	s := server(t)
	model := "FI-" + uuid.NewString()[:8]
	code, m := write(s, &hq, "capabilities.save", capBody("", model, ""), 0)
	if code != 200 || ver(m) != 1 {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)

	versions := func() (int, int) {
		var n, top int
		ownerScan(t, `SELECT count(*), max(version) FROM devices.capabilities WHERE id = $1`, []any{id}, &n, &top)
		return n, top
	}
	n, top := versions()

	broken := injectAt(t, "UPDATE devices.capabilities SET is_current = false")
	body := strings.Replace(capBody(id, model, `,"changeReason":"added fan mode"`), `["cool","dry"]`, `["cool","dry","fan"]`, 1)
	code, m = write(broken, &hq, "capabilities.save", body, 1)
	if code < 500 {
		t.Fatalf("new version with a failing retire: %d %v, want a server error", code, m)
	}
	if n2, top2 := versions(); n2 != n || top2 != top {
		t.Errorf("a failed save kept a version: before %d versions (top %d), after %d (top %d)", n, top, n2, top2)
	}
}
