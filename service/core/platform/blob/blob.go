// Package blob stores uploaded files (S3 in staging/production; a local directory in development and tests).
package blob

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store keeps blobs by key.
type Store interface {
	Put(ctx context.Context, key, mime string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

// Dir is a Store on the local file system.
type Dir string

// NewDirFromEnv uses BLOB_DIR or a directory under the system temp directory.
func NewDirFromEnv() Dir {
	if d := os.Getenv("BLOB_DIR"); d != "" {
		return Dir(d)
	}
	return Dir(filepath.Join(os.TempDir(), "ac-blobs"))
}

func (d Dir) path(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("blob: invalid key %q", key)
	}
	return filepath.Join(string(d), filepath.FromSlash(key)), nil
}

// Put implements Store.
func (d Dir) Put(_ context.Context, key, _ string, data []byte) error {
	p, err := d.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Get implements Store.
func (d Dir) Get(_ context.Context, key string) ([]byte, error) {
	p, err := d.path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}
