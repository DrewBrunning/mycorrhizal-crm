// Package diskspace is the free-space preflight primitive for capacity-under-
// constraint handling (issue #498). Operations that grow the database or the
// WAL — a backup's VACUUM INTO second copy, a bulk import's transaction, a
// migration that rebuilds a table — fail mid-write when SQLite cannot extend
// the file, and a mid-write ENOSPC is the worst moment to run out. A cheap
// statfs check before the operation turns that into a clear, actionable
// "insufficient disk space" refusal (issue #498 action 7: degrade before you
// die) while the on-disk data is still untouched.
//
// This is the check-before-operation primitive. It is deliberately separate
// from the observability-only free-space code that already exists —
// metrics.FilesystemBytes (the Prometheus gauge + the admin system-status
// block) and services' statfsDiskUsage (the percent-used alert condition with
// hysteresis). Those answer "how full is the disk right now"; this answers
// "does this specific operation have room to run". They could be folded onto
// this fold later; the duplication is a statfs call, not a second source of
// classification truth.
package diskspace

import (
	"fmt"
	"syscall"
)

// Probe reports the available and total bytes on the filesystem holding path.
// It is a package var so a test can force a constrained result without needing
// a real full filesystem; production never reassigns it. Mirrors the
// diskUsageFn seam in services/diagnostics.go.
var Probe = statfsProbe

// statfsProbe is the real implementation of Probe: one syscall.Statfs fold.
// Bsize is int64 on Linux, Bavail/Blocks are uint64; the arithmetic is done in
// uint64 after a sign/zero check so gosec's G115 is satisfied without a
// suppression (same reasoning as metrics.FilesystemBytes).
func statfsProbe(path string) (freeBytes, totalBytes uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs %q: %w", path, err)
	}
	if st.Bsize <= 0 || st.Blocks == 0 { // # pragma: no cover — a mounted filesystem never reports zero block size or zero blocks; the guard exists so the arithmetic below cannot divide-by-zero or wrap
		return 0, 0, fmt.Errorf("statfs %q returned unusable geometry (bsize=%d blocks=%d)", path, st.Bsize, st.Blocks)
	}
	bsize := uint64(st.Bsize)
	freeBytes = st.Bavail * bsize
	totalBytes = st.Blocks * bsize
	if freeBytes > totalBytes { // # pragma: no cover — Bavail <= Blocks on every real filesystem; a defensive clamp only
		freeBytes = totalBytes
	}
	return freeBytes, totalBytes, nil
}

// Available reports the free and total bytes on the filesystem holding path.
func Available(path string) (freeBytes, totalBytes uint64, err error) {
	return Probe(path)
}

// Require returns a non-nil *ErrInsufficientSpace when the filesystem holding
// path has fewer than needBytes free.
//
// A statfs that cannot be read at all is NOT treated as a failure: the check
// is a best-effort guard in front of an operation that has its own fail-closed
// error path, so a broken statfs must not be the thing that blocks a backup or
// an import. The operation runs and, if the disk really is full, fails closed
// the hard way.
func Require(path string, needBytes uint64) error {
	free, _, err := Probe(path)
	if err != nil {
		return nil
	}
	if free < needBytes {
		return &ErrInsufficientSpace{Path: path, NeedBytes: needBytes, FreeBytes: free}
	}
	return nil
}

// ErrInsufficientSpace is returned by Require when a preflight check finds the
// target filesystem does not have room for the operation. It is a stable,
// assertable sentinel: callers wrap it (database.BackupSnapshot,
// services import-confirm) and their tests match it with errors.As.
type ErrInsufficientSpace struct {
	Path      string
	NeedBytes uint64
	FreeBytes uint64
}

func (e *ErrInsufficientSpace) Error() string {
	return fmt.Sprintf(
		"insufficient disk space at %q: operation needs about %d MiB free, only %d MiB available. "+
			"Free space on that filesystem (or move the data directory) and retry.",
		e.Path, e.NeedBytes>>20, e.FreeBytes>>20,
	)
}

// StubForTest forces Probe to report exactly freeBytes available (with a large
// total) until the returned restore func runs. Test-only helper; call the
// restore in t.Cleanup.
func StubForTest(freeBytes uint64) (restore func()) {
	prev := Probe
	Probe = func(string) (uint64, uint64, error) { return freeBytes, 1 << 44, nil }
	return func() { Probe = prev }
}

// StubErrorForTest forces Probe to fail with err until the returned restore
// func runs, so a test can exercise the "statfs unreadable, do not block"
// branch. Test-only helper.
func StubErrorForTest(err error) (restore func()) {
	prev := Probe
	Probe = func(string) (uint64, uint64, error) { return 0, 0, err }
	return func() { Probe = prev }
}
