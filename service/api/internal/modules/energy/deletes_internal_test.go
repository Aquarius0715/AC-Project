package energy

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// A delete names its record and gives a reason of 1–1000 characters after trimming (DEC-103): a blank reason counts as
// none, and the limit counts characters, not bytes.
func TestDeleteInputValidate(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		id     uuid.UUID
		reason string
		want   string
	}{
		"no id":                {uuid.Nil, "made by mistake", "id=" + apperr.KeyRequired},
		"blank reason":         {uuid.New(), "  \n ", "reason=" + apperr.KeyLength},
		"1000 characters":      {uuid.New(), strings.Repeat("é", 1000), ""},
		"1001 characters":      {uuid.New(), strings.Repeat("é", 1001), "reason=" + apperr.KeyLength},
		"padded 1000 accepted": {uuid.New(), " " + strings.Repeat("x", 1000) + " ", ""},
	} {
		input := DeleteInput{ID: testCase.id, Reason: testCase.reason}

		fieldErrs := input.Validate()

		pairs := make([]string, 0, len(fieldErrs))
		for field, key := range fieldErrs {
			pairs = append(pairs, field+"="+key)
		}

		slices.Sort(pairs)

		if got := strings.Join(pairs, ","); got != testCase.want {
			t.Errorf("%s: %q, want %q", name, got, testCase.want)
		}
	}
}
