package services

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Storage-walk hard caps. A configured storage directory (profile photos,
// attachments) is normally small, but the admin system-status endpoint (issue
// #388) must not turn a pathological tree — a misconfigured root pointed at a
// huge volume, a symlink loop, millions of files — into an unbounded walk on
// an authenticated request. Hitting either cap returns the partial total with
// truncated=true rather than a wrong-looking zero or a hang. Package vars, not
// consts, only so a test can lower them to exercise the truncation path
// without materializing 200k files (same rationale as deepHealthCacheTTL).
var (
	storageWalkMaxEntries = 200_000
	storageWalkMaxDepth   = 8
)

// storageUsageCacheTTL bounds how often StorageUsage re-walks the configured
// directories. A package var, not a const, so tests can shorten or zero it
// (SetStorageUsageCacheTTL). Mirrors deepHealthCacheTTL.
var storageUsageCacheTTL = 5 * time.Minute

// DirectoryUsage is the recursive on-disk size of one configured storage
// directory. Truncated is true when a hard cap stopped the walk early, so the
// caller can show the total as a lower bound.
type DirectoryUsage struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	FileCount int    `json:"file_count"`
	Truncated bool   `json:"truncated"`
}

// storageUsageCache memoizes the last StorageUsage result. Same shape as
// deepHealthCache: a single slot, no keying on the input, because the set of
// directories is fixed for the process lifetime (it comes from config).
var storageUsageCache struct {
	mu  sync.Mutex
	at  time.Time
	val []DirectoryUsage
}

// StorageUsage walks each directory in dirs and reports its recursive byte
// total and file count. Empty entries in dirs are skipped. The result is
// memoized for storageUsageCacheTTL so repeated admin polling does not re-walk
// the filesystem every request.
//
// The walk skips symlinks entirely (never following one out of the configured
// root) and stops with Truncated=true after storageWalkMaxEntries entries or
// at depth storageWalkMaxDepth. An unreadable entry is skipped, not fatal.
func StorageUsage(dirs []string) []DirectoryUsage {
	storageUsageCache.mu.Lock()
	defer storageUsageCache.mu.Unlock()

	if !storageUsageCache.at.IsZero() && time.Since(storageUsageCache.at) < storageUsageCacheTTL {
		return storageUsageCache.val
	}

	out := make([]DirectoryUsage, 0, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		out = append(out, walkDirUsage(dir))
	}

	storageUsageCache.at = time.Now()
	storageUsageCache.val = out
	return out
}

// ResetStorageUsageCache clears the memoized result. Test helper; mirrors
// ResetDeepHealthCache.
func ResetStorageUsageCache() {
	storageUsageCache.mu.Lock()
	storageUsageCache.at = time.Time{}
	storageUsageCache.val = nil
	storageUsageCache.mu.Unlock()
}

// SetStorageUsageCacheTTL overrides the cache window (0 disables caching) and
// returns the previous value so a test can restore it. Mirrors
// SetDeepHealthCacheTTL.
func SetStorageUsageCacheTTL(d time.Duration) time.Duration {
	prev := storageUsageCacheTTL
	storageUsageCacheTTL = d
	return prev
}

func walkDirUsage(root string) DirectoryUsage {
	usage := DirectoryUsage{Path: root}

	entries := 0
	// The walk callback never returns a non-sentinel error, so WalkDir's
	// return is either nil or filepath.SkipAll — nothing to act on. A root
	// that cannot be walked at all (missing / not a directory) yields a
	// zero-byte usage with the path, which is a truthful answer for the
	// admin surface.
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Unreadable directory or vanished entry: skip that subtree and
			// keep going rather than abandoning the whole total.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		entries++
		if entries > storageWalkMaxEntries {
			usage.Truncated = true
			return filepath.SkipAll
		}

		depth := dirDepth(root, path)

		if d.IsDir() {
			if depth >= storageWalkMaxDepth {
				usage.Truncated = true
				return fs.SkipDir
			}
			return nil
		}

		// Never follow a symlink — it can point outside the configured root
		// or form a loop. Skipping it also keeps the byte total to real files
		// under this directory.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		usage.Bytes += info.Size()
		usage.FileCount++
		return nil
	})
	return usage
}

// dirDepth is how many path segments below root path sits: root itself is 0,
// its direct children are 1, and so on.
func dirDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}
