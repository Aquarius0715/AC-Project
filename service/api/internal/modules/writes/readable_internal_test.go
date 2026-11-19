package writes

import (
	"encoding/json"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestTargetReadableWithoutTarget: a stored result of a scoped write that does not name its target as a string is
// not readable, so writes.getResult hides it instead of showing it without the scope recheck (IR724, fail closed);
// a write outside the table keeps its result, and a result that is not a JSON object is an error, not a pass. None of
// these cases reaches the read check, so the call has no transaction.
func TestTargetReadableWithoutTarget(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		operation string
		data      string
		readable  bool
		failed    bool
	}{
		"job write without its id":      {"jobs.cancel", `{"state":"cancelled"}`, false, false},
		"job write with an empty id":    {"jobs.hold", `{"id":""}`, false, false},
		"job write with a numeric id":   {"jobs.start", `{"id":42}`, false, false},
		"device write without deviceId": {"devices.calibrate", `{"id":"dev-1"}`, false, false},
		"null result of a scoped write": {"units.save", `null`, false, false},
		"scoped write, not an object":   {"units.save", `"unit-1"`, false, true},
		"jobs.accept keeps its receipt": {"jobs.accept", `{}`, true, false},
	}
	for name, want := range cases {
		readable, err := targetReadable(
			t.Context(),
			new(ops.Call),
			want.operation,
			json.RawMessage(want.data),
		)
		if readable != want.readable || (err != nil) != want.failed {
			t.Errorf(
				"%s: readable=%v err=%v, want readable=%v failed=%v",
				name,
				readable,
				err,
				want.readable,
				want.failed,
			)
		}
	}
}
