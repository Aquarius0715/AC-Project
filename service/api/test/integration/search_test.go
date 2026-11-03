package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestSearchIsLiteral: filters.search finds the term as written, anywhere and in any case, in every list that has it
// (IR388). A %, _ or \ in the term is not a wildcard: "%" alone finds only values with a "%" in them, not every row.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestSearchIsLiteral(t *testing.T) {
	srv := server(t)
	// a part and a contractor whose names hold a % and an _ (the seed's do not)
	owner(
		t,
		`INSERT INTO maintenance.parts_catalog (code, tenant_id, name) VALUES ('SEAL_50', $1, 'Seal kit 50% grade') ON
			CONFLICT DO NOTHING`,
		seed.ID("tenant-a"),
	)
	org := seed.ID("org-test-search")
	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation)
		VALUES ($1, $2, 'Search Test 50%_Cooling', 'active', 'REG_TEST', 'search@partner.example', tstzrange($3, $4)) ON
		CONFLICT (organization_id) DO NOTHING`,
		seed.ID("tenant-a"),
		org,
		clock().Add(-time.Hour),
		clock().Add(1000*time.Hour),
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM maintenance.parts_catalog WHERE code = 'SEAL_50'`)
		owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, org)
	})

	search := func(operation, term string) []map[string]any {
		t.Helper()

		b := jsonBytes(t,
			map[string]any{fieldFilters: map[string]any{"search": term}, "limit": 100},
		)

		code, m := post(srv, hq(), operation, string(b))
		if code != 200 {
			t.Fatalf("%s search %q: %d %v", operation, term, code, m)
		}

		return items(t, m)
	}
	text := func(item map[string]any) string {
		b := jsonBytes(t, item)

		return strings.ToLower(string(b))
	}

	// each list: a term taken from its own first row finds that row, in other case; a lone wildcard character finds
	// only the rows that contain it
	for operation, field := range map[string]string{
		opClientUsersList: fieldEmail, opContractorsList: fieldName, opPartsList: fieldName,
		"units.coverage": fieldUnitID,
	} {
		all := search(operation, "")
		if len(all) == 0 {
			t.Fatalf("%s lists nothing to search", operation)
		}

		value, _ := all[0][field].(string)
		if len(value) < 3 {
			t.Fatalf("%s: no %s to search in %v", operation, field, all[0])
		}

		if got := search(operation, strings.ToUpper(value[1:len(value)-1])); len(got) == 0 {
			t.Errorf("%s: the middle of %q in other case finds nothing", operation, value)
		}

		for _, wildcard := range []string{"%", "_", `\`} {
			for _, item := range search(operation, wildcard) {
				if !strings.Contains(text(item), strings.ToLower(wildcard)) {
					t.Errorf("%s search %q found %v", operation, wildcard, item[field])
				}
			}
		}
	}

	// the term's own % and _ are matched as written
	found := func(op, field, term string) []string {
		rows := search(op, term)

		out := make([]string, 0, len(rows))
		for _, item := range rows {
			out = append(out, asStr(t, item[field]))
		}

		return out
	}
	for _, testCase := range []struct{ op, field, term, want string }{
		{opPartsList, fieldCode, "50% grade", "SEAL_50"},
		{opPartsList, fieldCode, "l_5", "SEAL_50"},
		{opPartsList, fieldCode, "SEAL%", ""},
		{opPartsList, fieldCode, "seal_kit", ""},
		{opPartsList, fieldCode, "50%%", ""},
		{opContractorsList, fieldName, "50%_c", "Search Test 50%_Cooling"},
		{opContractorsList, fieldName, "50%c", ""},
		{opContractorsList, fieldName, "s_arch", ""},
		{opContractorsList, fieldName, "reg_t", "Search Test 50%_Cooling"},
	} {
		if got := strings.Join(found(testCase.op, testCase.field, testCase.term), ","); got != testCase.want {
			t.Errorf("%s search %q: %q, want %q", testCase.op, testCase.term, got, testCase.want)
		}
	}
}
