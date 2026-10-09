package blob

import (
	"context"
	"errors"
	"testing"
)

func TestDir(t *testing.T) {
	d := Dir(t.TempDir())
	ctx := context.Background()
	if err := d.Put(ctx, "a/b.png", "image/png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b, err := d.Get(ctx, "a/b.png"); err != nil || string(b) != "x" {
		t.Fatalf("%q %v", b, err)
	}
	for _, k := range []string{"", "../x", "/abs"} {
		if err := d.Put(ctx, k, "", nil); err == nil {
			t.Errorf("key %q accepted", k)
		}
		if _, err := d.Get(ctx, k); err == nil {
			t.Errorf("get %q accepted", k)
		}
	}
	if _, err := d.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
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
