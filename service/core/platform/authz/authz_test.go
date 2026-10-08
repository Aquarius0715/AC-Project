package authz_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/core/ops"
	. "github.com/pradita/ac-project/service/core/platform/authz"
)

func TestPermissionsMatchContracts(t *testing.T) {
	b, err := os.ReadFile("../../../../docs/02-design/service-contracts.ts")
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
	for _, s := range ops.Catalog {
		e, err := Parse(s.Authorization)
		if err != nil {
			t.Errorf("%s: %v", s.Name, err)
			continue
		}
		if s.Mode == ops.Write {
			for _, a := range e.Alternatives {
				if a.Role == "admin" && strings.HasSuffix(a.Permission, ".read") && !contains(a.Predicates, "scope-candidate-read-only") {
					t.Errorf("%s: write op granted by read permission %s", s.Name, a.Permission)
				}
			}
		}
		for _, a := range e.Alternatives {
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
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestParseErrors(t *testing.T) {
	for _, s := range []string{"", "nobody:self", "admin:self", "client:foo.bar", "client:Bad", "client:self | ", "IR01:only"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("%q should fail", s)
		}
	}
}

func TestCandidatesAndAllowed(t *testing.T) {
	e, err := Parse("client:self-customer:owner | admin:alert.policy.write | IR01:same-key-receipt; note")
	if err != nil {
		t.Fatal(err)
	}
	if e.Remark != "note" || len(e.Notes) != 1 || len(e.Alternatives) != 2 {
		t.Fatalf("%+v", e)
	}
	owner := &Principal{Role: "client", ClientRole: "owner", Permissions: map[string]bool{}}
	c := e.Candidates(owner)
	if len(c) != 1 || !Allowed(c, Facts{"self-customer": true, "owner": true}) || Allowed(c, Facts{"self-customer": true}) {
		t.Fatal("client owner rules")
	}
	admin := &Principal{Role: "admin", Permissions: map[string]bool{"alert.policy.read": true}}
	if len(e.Candidates(admin)) != 0 {
		t.Fatal("admin without write permission must not be a candidate")
	}
	admin.Permissions["alert.policy.write"] = true
	if !Allowed(e.Candidates(admin), Facts{}) {
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
