package identity

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// The set-up link of an account returns to its own app's client, so the sign-in page names that service (IR790).
func TestAccountAppsClientPerRole(t *testing.T) {
	t.Parallel()

	apps := AccountApps{Customer: "c", Partner: "p", Technician: "t", Admin: "a"}
	cases := map[string][2]string{
		authz.RoleClient:     {"c", "ac-customer-web"},
		authz.RoleContractor: {"p", "ac-partner-web"},
		authz.RoleTechnician: {"t", "ac-technician-web"},
		authz.RoleAdmin:      {"a", "ac-admin-web"},
	}

	for role, want := range cases {
		url, client := apps.of(role)
		if url != want[0] || client != want[1] {
			t.Errorf("%s: got %s %s, want %s %s", role, url, client, want[0], want[1])
		}
	}
}
