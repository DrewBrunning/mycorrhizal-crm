package imaging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Store writes image variants beneath a configured root, one subdirectory per
// user. Names are always server-generated UUIDs; the only other path input is
// the authenticated numeric user id, so request input never reaches
// filepath.Join (ADR 0040 decision 1).
type Store struct {
	Dir string
}

// Seams for fault-injection tests (a filesystem that fails only on the second
// write, or on rename, cannot be expressed with a real temp directory).
var (
	writeFile  = os.WriteFile
	renameFile = os.Rename
)

// StoredPath resolves a stored name under the user's directory, rejecting
// traversal/absolute attempts the same way attachments.StoredPath does.
func (s Store) StoredPath(userID uint, storedName string) (string, error) {
	if s.Dir == "" {
		return "", fmt.Errorf("imaging: empty image directory")
	}
	if storedName == "" || storedName != filepath.Base(storedName) || strings.Contains(storedName, "..") {
		return "", fmt.Errorf("imaging: invalid stored name")
	}
	return filepath.Join(s.Dir, fmt.Sprintf("u%d", userID), storedName), nil
}

// Save writes both variants to disk and returns their stored names. It writes
// to a temporary name and renames, so a reader never observes a partial file.
func (s Store) Save(userID uint, full, thumb []byte, ext, thumbExt string) (string, string, error) {
	if s.Dir == "" {
		return "", "", fmt.Errorf("imaging: empty image directory")
	}
	dir := filepath.Join(s.Dir, fmt.Sprintf("u%d", userID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("imaging: create directory: %w", err)
	}
	id := uuid.New().String()
	fullName := id + "." + ext
	thumbName := id + "_t." + thumbExt
	if err := writeAtomic(s.Dir, userID, fullName, full); err != nil {
		return "", "", err
	}
	if err := writeAtomic(s.Dir, userID, thumbName, thumb); err != nil {
		_ = s.Remove(userID, fullName)
		return "", "", err
	}
	return fullName, thumbName, nil
}

func writeAtomic(dir string, userID uint, name string, data []byte) error {
	store := Store{Dir: dir}
	path, err := store.StoredPath(userID, name)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := writeFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("imaging: write file: %w", err)
	}
	if err := renameFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("imaging: rename file: %w", err)
	}
	return nil
}

// Remove deletes a stored file; a missing file is not an error.
func (s Store) Remove(userID uint, storedName string) error {
	path, err := s.StoredPath(userID, storedName)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("imaging: remove file: %w", err)
	}
	return nil
}

// Read returns a stored file's bytes after the stored-name validation.
func (s Store) Read(userID uint, storedName string) ([]byte, error) {
	path, err := s.StoredPath(userID, storedName)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path) // #nosec G304 -- path is a validated bare name under the configured directory
}
