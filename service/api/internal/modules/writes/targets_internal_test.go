package writes

import (
	"encoding/json"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestTargetsReadTheirTarget: every write of the recheck table is a catalog write whose result carries the ID field
// it is read from, and its read is a catalog read whose input has the field it fills (IR724). A wrong name would
// make writes.getResult hide a result the caller may read, or refuse its own read check as malformed input.
func TestTargetsReadTheirTarget(t *testing.T) {
	t.Parallel()

	results := map[string]struct {
		Fields []string `json:"fields"`
	}{}
	inputs := map[string][]string{}

	lib := os.DirFS("../../../../web/shared/lib") // the generated contracts of the web apps
	for file, into := range map[string]any{
		"contractResults.gen.json": &results,
		"contractInputs.gen.json":  &inputs,
	} {
		raw, err := fs.ReadFile(lib, file)
		if err != nil {
			t.Fatal(err)
		}

		err = json.Unmarshal(raw, into)
		if err != nil {
			t.Fatal(err)
		}
	}

	specs := ops.SpecByName()
	for write, target := range targets {
		if specs[write].Mode != ops.Write || !slices.Contains(results[write].Fields, target.from) {
			t.Errorf(
				"%s: not a write whose result has %s (%v)",
				write,
				target.from,
				results[write].Fields,
			)
		}

		if specs[target.read].Mode != ops.Read ||
			!slices.Contains(inputs[target.read], target.field) {
			t.Errorf(
				"%s: %s is not a read with the input %s (%v)",
				write,
				target.read,
				target.field,
				inputs[target.read],
			)
		}
	}
}
