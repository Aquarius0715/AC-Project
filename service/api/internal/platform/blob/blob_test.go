package blob_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/blob"
)

func TestDir(t *testing.T) {
	dir := blob.Dir(t.TempDir())

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
	if !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("missing blob: %v", err)
	}

	t.Setenv("BLOB_DIR", "/tmp/x")

	if blob.NewDirFromEnv() != "/tmp/x" {
		t.Error("env")
	}

	t.Setenv("BLOB_DIR", "")

	if blob.NewDirFromEnv() == "" {
		t.Error("default dir")
	}
}

// A symbolic link inside the directory does not lead the store to a file outside it.
func TestDirStaysInside(t *testing.T) {
	t.Parallel()

	dir := blob.Dir(t.TempDir())
	outside := t.TempDir()
	ctx := t.Context()

	symlinkErr := os.Symlink(outside, filepath.Join(string(dir), "link"))
	if symlinkErr != nil {
		t.Fatal(symlinkErr)
	}

	err := dir.Put(ctx, "link/x", "", []byte("x"))
	if err == nil {
		t.Error("put through the link accepted")
	}

	_, statErr := os.Stat(filepath.Join(outside, "x"))
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("put wrote outside the directory: %v", statErr)
	}

	writeErr := os.WriteFile(filepath.Join(outside, "y"), []byte("y"), 0o600)
	if writeErr != nil {
		t.Fatal(writeErr)
	}

	body, err := dir.Get(ctx, "link/y")
	if err == nil {
		t.Errorf("get through the link read %q", body)
	}
}
