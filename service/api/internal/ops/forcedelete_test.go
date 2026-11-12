package ops_test

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

func callWith(perms map[string]bool) *ops.Call {
	p := new(ops.Principal)
	p.Permissions = perms

	c := new(ops.Call)
	c.Principal = p

	return c
}

func TestCheckForce(t *testing.T) {
	t.Parallel()

	held := callWith(map[string]bool{ops.ForceDeletePermission: true})
	other := callWith(map[string]bool{"data.delete": true})

	err := ops.CheckForce(held, true)
	if err != nil {
		t.Fatalf("holder asking force: %v", err)
	}

	err = ops.CheckForce(other, false)
	if err != nil {
		t.Fatalf("no force needs no permission: %v", err)
	}

	for name, c := range map[string]*ops.Call{"other permission": other, "nil permissions": callWith(nil)} {
		err := ops.CheckForce(c, true)
		if err == nil {
			t.Fatalf("%s: force allowed", name)
		}

		e := apperr.From(err)
		if e.Code != apperr.Forbidden || e.MessageKey != ops.KeyForceNotAllowed {
			t.Fatalf("%s: got %v, want FORBIDDEN %s", name, err, ops.KeyForceNotAllowed)
		}
	}
}

func TestForcedReason(t *testing.T) {
	t.Parallel()

	if got := ops.ForcedReason("closed", true); got != "[forced] closed" {
		t.Fatalf("forced: %q", got)
	}

	if got := ops.ForcedReason("closed", false); got != "closed" {
		t.Fatalf("not forced: %q", got)
	}
}
