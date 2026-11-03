package blob

import (
	"context"
	"errors"
	"testing"
)

func TestDir(t *testing.T) {
	d := Dir(t.TempDir())

	ctx := context.Background()
	err := d.Put(ctx, "a/b.png", "image/png", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	b, err := d.Get(ctx, "a/b.png")
	if err != nil || string(b) != "x" {
		t.Fatalf("%q %v", b, err)
	}

	for _, k := range []string{"", "../x", "/abs"} {
		err := d.Put(ctx, k, "", nil)
		if err == nil {
			t.Errorf("key %q accepted", k)
		}

		_, err = d.Get(ctx, k)
		if err == nil {
			t.Errorf("get %q accepted", k)
		}
	}

	_, err = d.Get(ctx, "missing")
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
