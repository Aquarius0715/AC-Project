package authz_test

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

func TestPermissionsMatchContracts(t *testing.T) {
	t.Parallel()

	b, err := os.ReadFile("../../../../../docs/02-design/service-contracts.ts")
	if err != nil {
		t.Skip(err)
	}

	m := regexp.MustCompile(`export type Permission = ([^;]+);`).FindSubmatch(b)

	vals := regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(m[1], -1)
	if len(vals) != 39 || len(authz.Permissions()) != 39 {
		t.Fatalf("want 39 permissions, contracts %d, authz %d", len(vals), len(authz.Permissions()))
	}

	for _, v := range vals {
		if !authz.IsPermission(string(v[1])) {
			t.Errorf("missing %s", v[1])
		}
	}
}

// Every catalog authorization must parse; the predicate vocabulary is printed so new names are reviewed.
func TestCatalogParses(t *testing.T) {
	t.Parallel()

	preds := map[string]int{}

	for _, spec := range ops.Catalog {
		expr, err := authz.Parse(spec.Authorization)
		if err != nil {
			t.Errorf("%s: %v", spec.Name, err)

			continue
		}

		if spec.Mode == ops.Write {
			checkNotGrantedByRead(t, spec.Name, expr)
		}

		for _, a := range expr.Alternatives {
			for _, p := range a.Predicates {
				preds[p]++
			}
		}
	}

	names := slices.Sorted(maps.Keys(preds))
	t.Logf("%d predicates: %s", len(names), strings.Join(names, " "))
}

// checkNotGrantedByRead fails a write operation that an admin read permission grants without the read-only scope
// predicate.
func checkNotGrantedByRead(t *testing.T, name string, expr authz.Expr) {
	t.Helper()

	for _, a := range expr.Alternatives {
		if a.Role == "admin" && strings.HasSuffix(a.Permission, ".read") &&
			!contains(a.Predicates, "scope-candidate-read-only") {
			t.Errorf("%s: write op granted by read permission %s", name, a.Permission)
		}
	}
}

func contains(xs []string, s string) bool {
	return slices.Contains(xs, s)
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	for _, column := range []string{
		"", "nobody:self", "admin:self", "client:foo.bar", "client:Bad", "client:self | ",
		"IR01:only",
	} {
		_, err := authz.Parse(column)
		if err == nil {
			t.Errorf("%q should fail", column)
		}
	}
}

func TestCandidatesAndAllowed(t *testing.T) {
	t.Parallel()

	expr, err := authz.Parse(
		"client:self-customer:owner | admin:alert.policy.write | IR01:same-key-receipt; note",
	)
	if err != nil {
		t.Fatal(err)
	}

	if expr.Remark != "note" || len(expr.Notes) != 1 || len(expr.Alternatives) != 2 {
		t.Fatalf("%+v", expr)
	}

	owner := &authz.Principal{Role: "client", ClientRole: "owner", Permissions: map[string]bool{}}

	c := expr.Candidates(owner)
	if len(c) != 1 || !authz.Allowed(c, authz.Facts{"self-customer": true, "owner": true}) ||
		authz.Allowed(c, authz.Facts{"self-customer": true}) {
		t.Fatal("client owner rules")
	}

	checkAdminNeedsPermission(t, expr)
	checkPublicAndAuthenticated(t, owner)
}

func checkAdminNeedsPermission(t *testing.T, expr authz.Expr) {
	t.Helper()

	admin := &authz.Principal{
		Role:        "admin",
		Permissions: map[string]bool{"alert.policy.read": true},
		ClientRole:  "",
	}
	if len(expr.Candidates(admin)) != 0 {
		t.Fatal("admin without write permission must not be a candidate")
	}

	admin.Permissions["alert.policy.write"] = true
	if !authz.Allowed(expr.Candidates(admin), authz.Facts{}) {
		t.Fatal("admin with permission and no predicates is allowed")
	}
}

func checkPublicAndAuthenticated(t *testing.T, signedIn *authz.Principal) {
	t.Helper()

	pub, _ := authz.Parse("public:demo-only | authenticated:own-session")
	if got := len(pub.Candidates(nil)); got != 1 {
		t.Fatalf("anonymous gets public only, got %d", got)
	}

	if got := len(pub.Candidates(signedIn)); got != 2 {
		t.Fatalf("signed-in gets both, got %d", got)
	}
}
