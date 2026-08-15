// Package attachments holds the disk-I/O helpers for contact file/document
// attachments (N7, N7), mirroring
// the photostore package's shape: server-generated UUID filenames only, and
// every path back to disk goes through a stored name that is validated against
// traversal before reaching filepath.Join.
//
// The security invariant is that user input never reaches a filesystem path:
// uploads are written under a fresh UUID (the original name is display-only),
// and downloads/deletes look up the stored UUID from the database row and pass
// it through StoredPath, which rejects anything containing ".." or an absolute
// path.
package attachments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// StoredPath resolves a server-generated stored name to an absolute path
// inside dir, rejecting traversal/absolute attempts. dir and storedName are
// both validated so a corrupt database row (or a caller mistake) can never
// escape the attachments directory.
func StoredPath(dir, storedName string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("attachments: empty directory")
	}
	if storedName == "" || storedName != filepath.Base(storedName) || strings.Contains(storedName, "..") {
		return "", fmt.Errorf("attachments: invalid stored name")
	}
	return filepath.Join(dir, storedName), nil
}

// Save writes data to disk under a fresh UUID name inside dir and returns the
// stored name. The directory is created if needed.
func Save(data []byte, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("attachments: create directory: %w", err)
	}
	storedName := uuid.New().String()
	path, err := StoredPath(dir, storedName)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("attachments: write file: %w", err)
	}
	return storedName, nil
}

// Remove deletes the file for a stored name. A missing file is not an error
// (the metadata row may outlive a manual cleanup); any other removal failure
// is.
func Remove(dir, storedName string) error {
	path, err := StoredPath(dir, storedName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("attachments: remove file: %w", err)
	}
	return nil
}
