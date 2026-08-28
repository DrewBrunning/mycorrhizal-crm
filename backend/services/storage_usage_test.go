package services

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile creates a file of exactly n bytes under dir/name (creating parent
// dirs), and returns nothing — the caller knows the size it asked for.
func writeFile(t *testing.T, path string, n int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, make([]byte, n), 0o644))
}

func TestStorageUsage_SumsBytesAndFileCount(t *testing.T) {
	ResetStorageUsageCache()
	t.Cleanup(ResetStorageUsageCache)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.bin"), 100)
	writeFile(t, filepath.Join(root, "sub", "b.bin"), 250)
	writeFile(t, filepath.Join(root, "sub", "deep", "c.bin"), 25)

	got := StorageUsage([]string{root})
	require.Len(t, got, 1)
	assert.Equal(t, root, got[0].Path)
	assert.EqualValues(t, 375, got[0].Bytes)
	assert.Equal(t, 3, got[0].FileCount)
	assert.False(t, got[0].Truncated)
}

func TestStorageUsage_SkipsEmptyDirEntriesAndMissingRoots(t *testing.T) {
	ResetStorageUsageCache()
	t.Cleanup(ResetStorageUsageCache)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.bin"), 10)
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	got := StorageUsage([]string{root, "", missing})
	// "" is skipped entirely; the missing root still gets an entry, with a
	// zero total, so the caller can see it was asked for.
	require.Len(t, got, 2)
	assert.Equal(t, root, got[0].Path)
	assert.EqualValues(t, 10, got[0].Bytes)
	assert.Equal(t, missing, got[1].Path)
	assert.EqualValues(t, 0, got[1].Bytes)
	assert.Equal(t, 0, got[1].FileCount)
}

func TestStorageUsage_DoesNotFollowSymlinks(t *testing.T) {
	ResetStorageUsageCache()
	t.Cleanup(ResetStorageUsageCache)

	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.bin"), 9999)

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "real.bin"), 5)
	if err := os.Symlink(outside, filepath.Join(root, "link-to-outside")); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.bin"), filepath.Join(root, "link-to-file")); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	got := StorageUsage([]string{root})
	require.Len(t, got, 1)
	assert.EqualValues(t, 5, got[0].Bytes, "the walk must not count bytes reached through a symlink")
	assert.Equal(t, 1, got[0].FileCount)
	assert.False(t, got[0].Truncated)
}

func TestStorageUsage_TruncatesOnEntryCap(t *testing.T) {
	ResetStorageUsageCache()
	t.Cleanup(ResetStorageUsageCache)

	prevEntries := storageWalkMaxEntries
	storageWalkMaxEntries = 5
	t.Cleanup(func() { storageWalkMaxEntries = prevEntries })

	root := t.TempDir()
	for i := 0; i < 50; i++ {
		writeFile(t, filepath.Join(root, strconv.Itoa(i)+".bin"), 1)
	}

	start := time.Now()
	got := StorageUsage([]string{root})
	elapsed := time.Since(start)

	require.Len(t, got, 1)
	assert.True(t, got[0].Truncated, "hitting the entry cap must set Truncated")
	assert.Less(t, got[0].FileCount, 50, "the walk must have stopped early")
	assert.Less(t, elapsed, 5*time.Second, "a capped walk must return within a bounded wall-clock time")
}

func TestStorageUsage_TruncatesOnDepthCap(t *testing.T) {
	ResetStorageUsageCache()
	t.Cleanup(ResetStorageUsageCache)

	prevDepth := storageWalkMaxDepth
	storageWalkMaxDepth = 2
	t.Cleanup(func() { storageWalkMaxDepth = prevDepth })

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "top.bin"), 3)                         // depth 1
	writeFile(t, filepath.Join(root, "d1", "mid.bin"), 3)                   // depth 2
	writeFile(t, filepath.Join(root, "d1", "d2", "d3", "buried.bin"), 1000) // below the cap

	got := StorageUsage([]string{root})
	require.Len(t, got, 1)
	assert.True(t, got[0].Truncated)
	assert.EqualValues(t, 6, got[0].Bytes, "files below the depth cap must not be counted")
}

func TestStorageUsage_CachesWithinTTLThenReWalksAfterReset(t *testing.T) {
	ResetStorageUsageCache()
	prev := SetStorageUsageCacheTTL(time.Minute)
	t.Cleanup(func() { SetStorageUsageCacheTTL(prev); ResetStorageUsageCache() })

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.bin"), 100)

	first := StorageUsage([]string{root})
	require.Len(t, first, 1)
	require.EqualValues(t, 100, first[0].Bytes)

	// Grow the tree; a second call inside the TTL must NOT see it (no re-walk).
	writeFile(t, filepath.Join(root, "b.bin"), 900)
	second := StorageUsage([]string{root})
	assert.EqualValues(t, 100, second[0].Bytes, "a call inside the cache TTL must return the memoized result, not re-walk")

	// After an explicit reset the walk runs again and picks up the new file.
	ResetStorageUsageCache()
	third := StorageUsage([]string{root})
	assert.EqualValues(t, 1000, third[0].Bytes)
}
