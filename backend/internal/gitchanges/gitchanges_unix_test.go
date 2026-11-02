//go:build unix

package gitchanges

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTempIndexWriteFailure forces the temporary-index write to fail on a
// read-only file. The earlier RLIMIT_FSIZE approach changed a process-wide
// limit and raced the Go test logger under -coverpkg=./..., producing
// "testlog.txt: file too large" instead of the expected error.
func TestTempIndexWriteFailure(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	root := t.TempDir()
	write(t, filepath.Join(root, "idx"), "index")
	fakeGit(t, `case "$*" in
  *" merge-base "*) echo `+commit+` ;;
  *" rev-parse "*) echo idx ;;
esac
`)

	orig := createTempIndex
	t.Cleanup(func() { createTempIndex = orig })
	createTempIndex = func(dir, pattern string) (*os.File, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		name := file.Name()
		if err := file.Close(); err != nil {
			return nil, err
		}
		return os.Open(name) // read-only: the subsequent Write fails
	}

	if _, err := UnifiedDiff(root, "HEAD", "*.go"); err == nil || !strings.Contains(err.Error(), "write temporary git index") {
		t.Fatalf("UnifiedDiff error = %v, want the temporary-index write failure", err)
	}
}
