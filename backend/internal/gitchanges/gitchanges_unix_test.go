//go:build unix

package gitchanges

import (
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestTempIndexWriteFailure forces the temporary-index write to fail with
// RLIMIT_FSIZE rather than adding a production seam solely for this branch.
func TestTempIndexWriteFailure(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	root := t.TempDir()
	write(t, filepath.Join(root, "idx"), "index")
	fakeGit(t, `case "$*" in
  *" merge-base "*) echo `+commit+` ;;
  *" rev-parse "*) echo idx ;;
esac
`)

	signal.Ignore(syscall.SIGXFSZ)
	defer signal.Reset(syscall.SIGXFSZ)
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Fatalf("get file-size limit: %v", err)
	}
	limited := old
	limited.Cur = 0
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limited); err != nil {
		t.Fatalf("limit file size: %v", err)
	}
	defer func() {
		if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
			t.Errorf("restore file-size limit: %v", err)
		}
	}()

	if _, err := UnifiedDiff(root, "HEAD", "*.go"); err == nil || !strings.Contains(err.Error(), "write temporary git index") {
		t.Fatalf("UnifiedDiff error = %v, want the temporary-index write failure", err)
	}
}
