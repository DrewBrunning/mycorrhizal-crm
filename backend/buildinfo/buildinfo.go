// Package buildinfo carries the identity of the running binary: which version
// it is, which commit it was built from, and when.
//
// This exists because /health hardcoded `Version: "0.1.0"` as a string
// literal. Every build reported the same version forever, so a bug report
// could not be tied to a build — the one thing you most need when an alpha
// user says "it broke" and you have three candidate builds in the wild.
//
// Values are injected at link time (see the Dockerfile and backend/Makefile):
//
//	go build -ldflags "-X mycorrhizal/buildinfo.Version=v0.2.0 \
//	                   -X mycorrhizal/buildinfo.Commit=$(git rev-parse --short HEAD) \
//	                   -X mycorrhizal/buildinfo.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
//
// A plain `go build` or `go test` leaves the defaults, which deliberately read
// as "unknown" rather than pretending to be a release.
package buildinfo

import "runtime/debug"

// Injected via -ldflags at build time; see the package comment.
var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

// Info is the wire shape returned by /health and surfaced in the frontend's
// About dialog.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

// Get returns the build identity, falling back to Go's embedded VCS stamp for
// the commit when ldflags were not supplied. `go build` records the revision
// automatically inside a git checkout, so a developer build still reports a
// usable commit without anyone having to remember the flags.
func Get() Info {
	info := Info{Version: Version, Commit: Commit, BuildDate: BuildDate}
	if info.Commit != "" {
		return info
	}

	build, ok := debug.ReadBuildInfo()
	if !ok { // # pragma: no cover — never false for go-built binaries (only non-module builds hit it)
		return info
	}
	return applyVCSSettings(info, build.Settings)
}

// applyVCSSettings merges Go's embedded VCS stamp into an Info that was not
// fully populated by ldflags. Extracted from Get so the stamp-merging rules
// can be unit-tested against hand-built settings instead of depending on how
// the current test/CI binary happened to be built.
func applyVCSSettings(info Info, settings []debug.BuildSetting) Info {
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			if len(setting.Value) > 12 {
				info.Commit = setting.Value[:12]
			} else {
				info.Commit = setting.Value
			}
		case "vcs.modified":
			// A dirty working tree means the commit alone does not identify
			// the source; say so rather than reporting a clean commit hash.
			if setting.Value == "true" {
				info.Commit += "-dirty"
			}
		case "vcs.time":
			if info.BuildDate == "" {
				info.BuildDate = setting.Value
			}
		}
	}
	return info
}
