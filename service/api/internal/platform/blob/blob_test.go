package blob

import (
	"context"
	"errors"
	"testing"
)

func TestDir(t *testing.T) {
	dir := Dir(t.TempDir())

	ctx := context.Background()

	err := dir.Put(ctx, "a/b.png", "image/png", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	b, err := dir.Get(ctx, "a/b.png")
	if err != nil || string(b) != "x" {
		t.Fatalf("%q %v", b, err)
	}

	for _, key := range []string{"", "../x", "/abs"} {
		err := dir.Put(ctx, key, "", nil)
		if err == nil {
			t.Errorf("key %q accepted", key)
		}

		_, err = dir.Get(ctx, key)
		if err == nil {
			t.Errorf("get %q accepted", key)
		}
	}

	_, err = dir.Get(ctx, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing blob: %v", err)
	}

	t.Setenv("BLOB_DIR", "/tmp/x")

	if NewDirFromEnv() != "/tmp/x" {
		t.Error("env")
	}

	t.Setenv("BLOB_DIR", "")

	if NewDirFromEnv() == "" {
		t.Error("default dir")
	}
}
