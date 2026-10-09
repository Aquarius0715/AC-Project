package main

import (
	"bytes"
	"os"
	"testing"
)

// The generated files are current: regenerating from the design documents changes neither catalog_gen.go nor the
// web apps' route table (run make gen after changing a catalog).
func TestGeneratedFilesAreCurrent(t *testing.T) {
	src, ts, n := generate("../../../../../docs/02-design")
	if n != 199 {
		t.Fatalf("%d operations", n)
	}
	for path, want := range map[string][]byte{"../../../internal/ops/catalog_gen.go": src, "../../../../web/shared/lib/routes.gen.ts": ts} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is stale: run make gen", path)
		}
	}
}
