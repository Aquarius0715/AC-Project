package authz_test

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	. "github.com/pradita/ac-project/service/api/internal/platform/authz"
)

func TestPermissionsMatchContracts(t *testing.T) {
	b, err := os.ReadFile("../../../../../docs/02-design/service-contracts.ts")
	if err != nil {
		t.Skip(err)
	}

	m := regexp.MustCompile(`export type Permission = ([^;]+);`).FindSubmatch(b)

	vals := regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(m[1], -1)
	if len(vals) != 38 || len(Permissions) != 38 {
		t.Fatalf("want 38 permissions, contracts %d, authz %d", len(vals), len(Permissions))
	}

	for _, v := range vals {
		if !Permissions[string(v[1])] {
			t.Errorf("missing %s", v[1])
		}
	}
}

// Every catalog authorization must parse; the predicate vocabulary is printed so new names are reviewed.
func TestCatalogParses(t *testing.T) {
	preds := map[string]int{}

	for _, spec := range ops.Catalog {
		expr, err := Parse(spec.Authorization)
		if err != nil {
			t.Errorf("%s: %v", spec.Name, err)

			continue
		}

		if spec.Mode == ops.Write {
			for _, a := range expr.Alternatives {
				if a.Role == "admin" && strings.HasSuffix(a.Permission, ".read") &&
					!contains(a.Predicates, "scope-candidate-read-only") {
					t.Errorf("%s: write op granted by read permission %s", spec.Name, a.Permission)
				}
			}
		}

		for _, a := range expr.Alternatives {
			for _, p := range a.Predicates {
				preds[p]++
			}
		}
	}

	var names []string
	for k := range preds {
		names = append(names, k)
	}

	sort.Strings(names)
	t.Logf("%d predicates: %s", len(names), strings.Join(names, " "))
}

func contains(xs []string, s string) bool {
	return slices.Contains(xs, s)
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{
		"", "nobody:self", "admin:self", "client:foo.bar", "client:Bad", "client:self | ",
		"IR01:only",
	} {
		_, err := Parse(s)
		if err == nil {
			t.Errorf("%q should fail", s)
		}
	}
}

func TestCandidatesAndAllowed(t *testing.T) {
	expr, err := Parse(
		"client:self-customer:owner | admin:alert.policy.write | IR01:same-key-receipt; note",
	)
	if err != nil {
		t.Fatal(err)
	}

	if expr.Remark != "note" || len(expr.Notes) != 1 || len(expr.Alternatives) != 2 {
		t.Fatalf("%+v", expr)
	}

	owner := &Principal{Role: "client", ClientRole: "owner", Permissions: map[string]bool{}}

	c := expr.Candidates(owner)
	if len(c) != 1 || !Allowed(c, Facts{"self-customer": true, "owner": true}) ||
		Allowed(c, Facts{"self-customer": true}) {
		t.Fatal("client owner rules")
	}

	admin := &Principal{
		Role:        "admin",
		Permissions: map[string]bool{"alert.policy.read": true},
		ClientRole:  "",
	}
	if len(expr.Candidates(admin)) != 0 {
		t.Fatal("admin without write permission must not be a candidate")
	}

	admin.Permissions["alert.policy.write"] = true
	if !Allowed(expr.Candidates(admin), Facts{}) {
		t.Fatal("admin with permission and no predicates is allowed")
	}

	pub, _ := Parse("public:demo-only | authenticated:own-session")
	if got := len(pub.Candidates(nil)); got != 1 {
		t.Fatalf("anonymous gets public only, got %d", got)
	}

	if got := len(pub.Candidates(owner)); got != 2 {
		t.Fatalf("signed-in gets both, got %d", got)
	}
}
