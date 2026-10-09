// Package blob stores uploaded files (S3 in staging/production; a local directory in development and tests).
package blob

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned by Get when no blob exists under the key (a stored row whose content was lost).
var ErrNotFound = errors.New("blob: not found")

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
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return b, err
}
