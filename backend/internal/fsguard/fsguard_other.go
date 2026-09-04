//go:build !linux

package fsguard

import "errors"

// errUnsupported is returned on every non-Linux build: this platform's
// syscall package does not expose a comparable numeric filesystem-type magic
// (Darwin's Statfs_t reports a name string in a differently-shaped struct,
// and Windows has no statfs equivalent at all). The check degrades to a
// silent no-op here rather than attempting a platform-specific
// classification this package has not verified — this repo ships and
// documents a Linux-only Docker deployment target (see
// docs/development/supported-runtime-matrix.md), so the check that matters
// in production always runs on fsguard_linux.go's path.
var errUnsupported = errors.New("fsguard: filesystem-type detection is not supported on this platform")

// statfsType always fails on non-Linux platforms; see errUnsupported.
func statfsType(string) (int64, error) {
	return 0, errUnsupported
}
