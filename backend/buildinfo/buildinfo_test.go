package buildinfo

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"
)

// saveBuildVars restores the package-level ldflags-injectable variables after
// a test, since they are process-global.
func saveBuildVars(t *testing.T) {
	t.Helper()
	origV, origC, origD := Version, Commit, BuildDate
	t.Cleanup(func() { Version, Commit, BuildDate = origV, origC, origD })
}

func TestGetReturnsLDFlagsVerbatim(t *testing.T) {
	saveBuildVars(t)
	Version = "v0.6.3"
	Commit = "deadbeef00"
	BuildDate = "2026-08-29T00:00:00Z"

	info := Get()

	// When ldflags supplied every value, Get must return them unchanged —
	// nothing may be re-derived or truncated.
	require.Equal(t, "v0.6.3", info.Version)
	require.Equal(t, "deadbeef00", info.Commit)
	require.Equal(t, "2026-08-29T00:00:00Z", info.BuildDate)
}

// TestGetReturnsLDFlagsVerbatimEvenWhenVersionEmpty pins the "Commit non-empty
// short-circuits" rule: a partial ldflags injection (just the commit) must not
// fall through to the VCS stamp and clobber the injected value.
func TestGetShortCircuitsOnCommit(t *testing.T) {
	saveBuildVars(t)
	Commit = "injected-commit"

	info := Get()
	require.Equal(t, "injected-commit", info.Commit)
	// Version/BuildDate keep their package defaults — no VCS derivation ran.
	require.Equal(t, "dev", info.Version)
}

// TestGetWithoutLDFlags matches Get's output against whatever VCS stamp the
// test binary was actually built with, so the assertion is deterministic in
// any build environment (CI checks out real git history; local test binaries
// may or may not carry the stamp).
func TestGetWithoutLDFlags(t *testing.T) {
	saveBuildVars(t)
	Commit = ""

	info := Get()

	var want string
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				if len(s.Value) > 12 {
					want = s.Value[:12]
				} else {
					want = s.Value
				}
			}
		}
	}
	require.Equal(t, want, info.Commit)
}

func TestApplyVCSSettings(t *testing.T) {
	longRevision := "0123456789abcdef0123456789abcdef01234567"

	t.Run("long revision truncated to 12", func(t *testing.T) {
		info := applyVCSSettings(Info{}, []debug.BuildSetting{
			{Key: "vcs.revision", Value: longRevision},
		})
		require.Equal(t, longRevision[:12], info.Commit)
	})

	t.Run("short revision kept whole", func(t *testing.T) {
		info := applyVCSSettings(Info{}, []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
		})
		require.Equal(t, "abc123", info.Commit)
	})

	t.Run("dirty working tree flags the commit", func(t *testing.T) {
		info := applyVCSSettings(Info{}, []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.modified", Value: "true"},
		})
		require.Equal(t, "abc123-dirty", info.Commit)
	})

	t.Run("clean working tree leaves commit untouched", func(t *testing.T) {
		info := applyVCSSettings(Info{}, []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123"},
			{Key: "vcs.modified", Value: "false"},
		})
		require.Equal(t, "abc123", info.Commit)
	})

	t.Run("vcs time fills empty build date", func(t *testing.T) {
		info := applyVCSSettings(Info{}, []debug.BuildSetting{
			{Key: "vcs.time", Value: "2026-08-29T00:00:00Z"},
		})
		require.Equal(t, "2026-08-29T00:00:00Z", info.BuildDate)
	})

	t.Run("vcs time does not override an injected build date", func(t *testing.T) {
		info := applyVCSSettings(Info{BuildDate: "injected"}, []debug.BuildSetting{
			{Key: "vcs.time", Value: "2026-08-29T00:00:00Z"},
		})
		require.Equal(t, "injected", info.BuildDate)
	})

	t.Run("unrelated settings ignored", func(t *testing.T) {
		info := applyVCSSettings(Info{}, []debug.BuildSetting{
			{Key: "go.mod", Value: "module example"},
		})
		require.Empty(t, info.Commit)
		require.Empty(t, info.BuildDate)
	})
}
