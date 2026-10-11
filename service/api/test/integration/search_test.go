package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestSearchIsLiteral: filters.search finds the term as written, anywhere and in any case, in every list that has it
// (IR388). A %, _ or \ in the term is not a wildcard: "%" alone finds only values with a "%" in them, not every row.
func TestSearchIsLiteral(t *testing.T) {
	s := server(t)
	// a part and a contractor whose names hold a % and an _ (the seed's do not)
	owner(t, `INSERT INTO maintenance.parts_catalog (code, tenant_id, name) VALUES ('SEAL_50', $1, 'Seal kit 50% grade') ON CONFLICT DO NOTHING`, seed.ID("tenant-a"))
	org := seed.ID("org-test-search")
	owner(t, `INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email, delegation)
		VALUES ($1, $2, 'Search Test 50%_Cooling', 'active', 'REG_TEST', 'search@partner.example', tstzrange($3, $4)) ON CONFLICT (organization_id) DO NOTHING`,
		seed.ID("tenant-a"), org, clock.Add(-time.Hour), clock.Add(1000*time.Hour))
	t.Cleanup(func() {
		owner(t, `DELETE FROM maintenance.parts_catalog WHERE code = 'SEAL_50'`)
		owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, org)
	})

	search := func(op, term string) []map[string]any {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"filters": map[string]any{"search": term}, "limit": 100})
		code, m := post(s, &hq, op, string(b))
		if code != 200 {
			t.Fatalf("%s search %q: %d %v", op, term, code, m)
		}
		return items(m)
	}
	text := func(item map[string]any) string { b, _ := json.Marshal(item); return strings.ToLower(string(b)) }

	// each list: a term taken from its own first row finds that row, in other case; a lone wildcard character finds
	// only the rows that contain it
	for op, field := range map[string]string{"clientUsers.list": "email", "contractors.list": "name", "parts.list": "name", "units.coverage": "unitId"} {
		all := search(op, "")
		if len(all) == 0 {
			t.Fatalf("%s lists nothing to search", op)
		}
		value, _ := all[0][field].(string)
		if len(value) < 3 {
			t.Fatalf("%s: no %s to search in %v", op, field, all[0])
		}
		if got := search(op, strings.ToUpper(value[1:len(value)-1])); len(got) == 0 {
			t.Errorf("%s: the middle of %q in other case finds nothing", op, value)
		}
		for _, wildcard := range []string{"%", "_", `\`} {
			for _, item := range search(op, wildcard) {
				if !strings.Contains(text(item), strings.ToLower(wildcard)) {
					t.Errorf("%s search %q found %v", op, wildcard, item[field])
				}
			}
		}
	}

	// the term's own % and _ are matched as written
	found := func(op, field, term string) (out []string) {
		for _, item := range search(op, term) {
			out = append(out, item[field].(string))
		}
		return out
	}
	for _, c := range []struct{ op, field, term, want string }{
		{"parts.list", "code", "50% grade", "SEAL_50"},
		{"parts.list", "code", "l_5", "SEAL_50"},
		{"parts.list", "code", "SEAL%", ""},
		{"parts.list", "code", "seal_kit", ""},
		{"parts.list", "code", "50%%", ""},
		{"contractors.list", "name", "50%_c", "Search Test 50%_Cooling"},
		{"contractors.list", "name", "50%c", ""},
		{"contractors.list", "name", "s_arch", ""},
		{"contractors.list", "name", "reg_t", "Search Test 50%_Cooling"},
	} {
		if got := strings.Join(found(c.op, c.field, c.term), ","); got != c.want {
			t.Errorf("%s search %q: %q, want %q", c.op, c.term, got, c.want)
		}
	}
}
