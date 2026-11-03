package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The generated files are current: regenerating from the design documents changes neither catalog_gen.go nor the
// web apps' route table and contract types (run make gen after changing a catalog or service-contracts.ts).
func TestGeneratedFilesAreCurrent(t *testing.T) {
	t.Parallel()

	src, routesTS, contractsTS, operations := generate("../../../../../docs/02-design")
	if operations != 212 {
		t.Fatalf("%d operations", operations)
	}

	for path, want := range map[string][]byte{"../../../internal/ops/catalog_gen.go": src, "../../../../web/shared/lib/" +
		"routes.gen.ts": routesTS, "../../../../web/shared/lib/contracts.gen.ts": contractsTS} {
		got, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale: run make gen", path)
		}
	}
}
