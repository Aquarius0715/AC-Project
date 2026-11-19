package unitscope_test

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// A role the scope does not know reads no unit: the predicate is FALSE and binds no argument, in both modes. The
// integration suites only sign in with the five known roles, so this deny branch has no other test.
func TestSQLUnknownRoleReadsNothing(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"", "authenticated", "public", "partner"} {
		for _, mode := range []unitscope.Mode{unitscope.List, unitscope.Equipment} {
			principal := new(ops.Principal)
			principal.Role = role
			call := new(ops.Call)
			call.Principal = principal

			var args []any

			got := unitscope.SQL(call, &args, "u.id", mode)
			if got != "FALSE" || len(args) != 0 {
				t.Errorf(
					"role %q mode %d: SQL = %q with %d args, want FALSE with none",
					role,
					mode,
					got,
					len(args),
				)
			}
		}
	}
}
