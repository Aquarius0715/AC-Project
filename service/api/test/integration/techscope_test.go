package integration

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestInternalTechnicianScopeKinds: an internal technician reads the units within Membership.scopes, whatever their kind
// (D01, unitscope): a property scope gives that property's units, an organization scope its customer's units, and no
// scope none at all. The seed technicians hold unit scopes only, so the property and organization branches and the
// empty scope had no test (IR381).
func TestInternalTechnicianScopeKinds(t *testing.T) {
	s := server(t)
	as := func(name string, scopes map[string]string) actor {
		id := seed.ID(name)
		owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from)
			VALUES ($1,$2,$3,$4,'technician','internal',$5) ON CONFLICT DO NOTHING`, id, seed.ID("tenant-a"), seed.ID("user-tech-internal-a"), seed.ID("org-operator-a"), clock.Add(-24*time.Hour))
		owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission)
			SELECT tenant_id, $1, permission FROM identity.membership_permissions WHERE membership_id = $2 ON CONFLICT DO NOTHING`, id, seed.ID("tech-internal-a"))
		for kind, ref := range scopes {
			owner(t, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, seed.ID("tenant-a"), id, kind, seed.ID(ref))
		}
		t.Cleanup(func() {
			owner(t, `DELETE FROM identity.membership_scopes WHERE membership_id = $1`, id)
			owner(t, `DELETE FROM identity.membership_permissions WHERE membership_id = $1`, id)
			owner(t, `DELETE FROM identity.memberships WHERE id = $1`, id)
		})
		return actor{"tok-ti", name} // the internal technician's user, signed in with this membership
	}
	seedUnits := []string{"unit-online-rto", "unit-non-rto", "unit-offline-rto", "unit-limited", "unit-other-customer"}
	ids := make([]string, len(seedUnits))
	for i, u := range seedUnits {
		ids[i] = `"` + seed.ID(u).String() + `"`
	}
	name := map[string]string{}
	for _, u := range seedUnits {
		name[seed.ID(u).String()] = u
	}
	listed := func(a actor) string {
		code, m := post(s, &a, "units.list", `{"limit":100,"filters":{"unitIds":[`+strings.Join(ids, ",")+`]}}`)
		if code != 200 {
			t.Fatalf("units.list as %s: %d %v", a.membership, code, m)
		}
		var got []string
		for _, it := range items(m) {
			got = append(got, name[it["id"].(string)])
		}
		sort.Strings(got)
		return strings.Join(got, ",")
	}
	property := as("test-tech-property", map[string]string{"property": "property-home-a"})
	organization := as("test-tech-organization", map[string]string{"organization": "org-customer-a"})
	none := as("test-tech-no-scope", nil)
	for _, c := range []struct {
		who  actor
		want string
	}{
		{property, "unit-non-rto,unit-online-rto"},
		{organization, "unit-limited,unit-non-rto,unit-offline-rto,unit-online-rto"},
		{none, ""},
	} {
		if got := listed(c.who); got != c.want {
			t.Errorf("%s lists %q, want %q", c.who.membership, got, c.want)
		}
	}
	// one unit outside the scope is NOT_FOUND, one inside is read
	if code, _ := post(s, &property, "units.get", `{"id":"`+seed.ID("unit-offline-rto").String()+`"}`); code != 404 {
		t.Errorf("a unit of another property: %d", code)
	}
	if code, m := post(s, &property, "units.get", `{"id":"`+seed.ID("unit-online-rto").String()+`"}`); code != 200 {
		t.Errorf("a unit of the scoped property: %d %v", code, m)
	}
	if code, _ := post(s, &none, "units.get", `{"id":"`+seed.ID("unit-online-rto").String()+`"}`); code != 404 {
		t.Errorf("a unit without any scope: %d", code)
	}
}
