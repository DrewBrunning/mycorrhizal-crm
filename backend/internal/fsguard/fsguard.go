// Package fsguard is the storage-filesystem-type preflight for issue #472
// (COMPAT-01, "make unsupported environments fail clearly"). SQLite's WAL
// mode depends on advisory byte-range locks that most network filesystems
// (NFS, SMB/CIFS, NCP, Coda, AFS) do not implement correctly across clients —
// running the database over one of them is a documented corruption risk (see
// docs/development/supported-runtime-matrix.md and docs/deployment.md), and
// it is exactly the kind of thing a self-hoster reaches for first (mount a
// NAS share, point SQLITE_DB_PATH at it).
//
// This package answers one question at startup: is the directory holding the
// database file on a filesystem type known to be unsafe for WAL SQLite? It is
// advisory-only by design (see NetworkFilesystemWarning's doc comment for
// why this warns instead of refusing to boot) — the mirror-image decision
// from internal/diskspace's Require, which fails an *operation* closed but
// never blocks on an unreadable probe either.
package fsguard

import "fmt"

// typeProbe reports the filesystem magic number (Linux's statfs(2) f_type)
// for the filesystem holding path. It is a package var so tests can force a
// result without a real mount of the filesystem in question; production
// never reassigns it. Mirrors diskspace.Probe's seam.
//
// The real implementation is platform-specific (fsguard_linux.go); the
// non-Linux stub (fsguard_other.go) always returns errUnsupported so this
// check degrades to a no-op off Linux rather than failing to build or
// misreporting from a struct layout that doesn't have this field.
var typeProbe = statfsType

// knownNetworkFilesystems maps a Linux statfs(2) f_type magic number to the
// human-readable filesystem name, for the types with no reliable local-disk
// use case. FUSE (0x65735546) is deliberately NOT included here even though
// network mounts (sshfs, rclone mount, etc.) commonly use it: Docker's
// fuse-overlayfs storage driver makes a container's entirely-local writable
// layer FUSE-typed too, so flagging FUSE would false-positive on ordinary
// containerized deployments. A FUSE-backed network mount is a real but
// undetectable-from-here case; the matrix doc says so explicitly rather than
// implying this check is exhaustive.
var knownNetworkFilesystems = map[int64]string{
	0x6969:     "NFS",
	0x517B:     "SMB",
	0xFF534D42: "CIFS", //nolint:staticcheck // magic number per linux/magic.h, not a typo
	0xFE534D42: "SMB2",
	0x564C:     "NCP",
	0x73757245: "Coda",
	0x5346414F: "AFS",
}

// Warning describes a storage path that sits on a filesystem type known to
// risk SQLite WAL corruption.
type Warning struct {
	Path           string
	FilesystemName string
}

func (w *Warning) String() string {
	return fmt.Sprintf(
		"database storage path %q is on a %s filesystem, which does not reliably support "+
			"the advisory locking SQLite's WAL mode depends on and is a known corruption risk. "+
			"Move the database to local disk. See docs/development/supported-runtime-matrix.md.",
		w.Path, w.FilesystemName,
	)
}

// NetworkFilesystemWarning inspects the filesystem holding path (typically
// the directory containing SQLITE_DB_PATH) and returns a non-nil *Warning
// when it is backed by a known-unsafe network filesystem type.
//
// It returns nil — no warning — when the filesystem is local, unrecognized,
// or its type could not be determined at all (statfs failure, non-Linux
// build). This is deliberately advisory rather than fatal: unlike the
// v0.6.0 upgrade-floor check, this is a *risk* signal, not a certainty of
// corruption, and refusing to boot could brick an already-running
// self-hosted instance with real production data on its next restart. A
// clear startup log line is the "fail clearly, not subtly" behavior this
// check exists for; making it fatal is a separate, deliberate decision this
// package does not make.
func NetworkFilesystemWarning(path string) *Warning {
	magic, err := typeProbe(path)
	if err != nil {
		return nil
	}
	name, known := knownNetworkFilesystems[magic]
	if !known {
		return nil
	}
	return &Warning{Path: path, FilesystemName: name}
}
