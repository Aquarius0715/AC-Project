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

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

const (
	dirPerm  = 0o700 // directories of the store, for the API's user only
	filePerm = 0o600 // stored files, for the API's user only
)

// ErrNotFound is returned by Get when no blob exists under the key (a stored row whose content was lost).
var ErrNotFound = errors.New("blob: not found")

// errInvalidKey refuses a key that is empty, absolute or climbs out of the directory.
var errInvalidKey = errors.New("blob: invalid key")

// Store keeps blobs by key.
type Store interface {
	Put(ctx context.Context, key, mime string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}

// Dir is a Store on the local file system. Its files are read and written through an os.Root on the directory, so
// neither a key nor a symbolic link inside the directory reaches a file outside it.
type Dir string

// NewDirFromEnv uses BLOB_DIR or a directory under the system temp directory.
func NewDirFromEnv() Dir {
	if d := os.Getenv("BLOB_DIR"); d != "" {
		return Dir(d)
	}

	return Dir(filepath.Join(os.TempDir(), "ac-blobs"))
}

// Put implements Store.
func (d Dir) Put(_ context.Context, key, _ string, data []byte) error {
	name, err := fileName(key)
	if err != nil {
		return err
	}

	mkdirAllErr := os.MkdirAll(string(d), dirPerm)
	if mkdirAllErr != nil {
		return apperr.Wrap(mkdirAllErr, "os.MkdirAll")
	}

	root, err := os.OpenRoot(string(d))
	if err != nil {
		return apperr.Wrap(err, "os.OpenRoot")
	}

	defer func() { _ = root.Close() }()

	mkdirAllErr = root.MkdirAll(filepath.Dir(name), dirPerm)
	if mkdirAllErr != nil {
		return apperr.Wrap(mkdirAllErr, "os.Root.MkdirAll")
	}

	return apperr.Wrap(root.WriteFile(name, data, filePerm), "os.Root.WriteFile")
}

// Get implements Store.
func (d Dir) Get(_ context.Context, key string) ([]byte, error) {
	name, err := fileName(key)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(string(d))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}

	if err != nil {
		return nil, apperr.Wrap(err, "os.OpenRoot")
	}

	defer func() { _ = root.Close() }()

	body, err := root.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}

	return body, apperr.Wrap(err, "os.Root.ReadFile")
}

// fileName checks a key and returns its file name inside the directory.
func fileName(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("%w %q", errInvalidKey, key)
	}

	return filepath.Clean(filepath.FromSlash(key)), nil
}
