package integration

import (
	"testing"

	"github.com/google/uuid"
)

// TestMRVReadRefusals: an MRV report that does not exist is NOT_FOUND; a version below 1 and a nil id are refused as
// VALIDATION on id before any row is read (energy mrv.get, mrv.versions; the input rules of GetInput and VersionsInput).
func TestMRVReadRefusals(t *testing.T) {
	s := server(t)
	missing := uuid.NewString()
	code, m := post(s, &hq, "mrv.get", `{"id":"`+missing+`"}`)
	if code != 404 || m["messageKey"] != "error.notFound" {
		t.Fatalf("mrv.get of a missing report: %d %v, want 404 NOT_FOUND", code, m)
	}
	code, m = post(s, &hq, "mrv.get", `{"id":"`+missing+`","reportVersion":0}`)
	refusal(t, "mrv.get reportVersion 0", code, m, "id=error.invalid")
	code, m = post(s, &hq, "mrv.versions", `{"id":"00000000-0000-0000-0000-000000000000"}`)
	refusal(t, "mrv.versions nil id", code, m, "id=error.required")
}
