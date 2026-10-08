package logtest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"mycorrhizal/logger"
)

// fakeT records failures instead of failing the real test, so the guard's
// failure path can itself be asserted. Cleanups run in LIFO order via finish.
type fakeT struct {
	testing.TB
	cleanups []func()
	errors   []string
	fatals   []string
	logs     []string
	failed   bool
}

// Name is unique per owning test: AllowWarnings records the opt-out in the
// package-global allowSet keyed by name, so a constant name let
// TestAllowWarningsOptsOut's entry silently disable every later Guard test
// when -shuffle or -count=2 ran it first (nightly #1572).
func (f *fakeT) Name() string            { return "TestFake/" + f.TB.Name() }
func (f *fakeT) Helper()                 {}
func (f *fakeT) Failed() bool            { return f.failed }
func (f *fakeT) Cleanup(fn func())       { f.cleanups = append(f.cleanups, fn) }
func (f *fakeT) Logf(s string, a ...any) { f.logs = append(f.logs, fmt.Sprintf(s, a...)) }
func (f *fakeT) Errorf(s string, a ...any) {
	f.failed = true
	f.errors = append(f.errors, fmt.Sprintf(s, a...))
}
func (f *fakeT) Fatal(a ...any) {
	f.failed = true
	f.fatals = append(f.fatals, fmt.Sprint(a...))
}
func (f *fakeT) finish() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

func TestCaptureRecordsAndRestores(t *testing.T) {
	before := logger.Logger
	f := &fakeT{TB: t}
	rec := Capture(f)

	logger.Info().Msg("hello")
	logger.Warn().Msg("careful")
	logger.Error().Msg("boom")
	rec.Write([]byte("not json\n\n"))

	require.Len(t, rec.Records(), 3, "non-JSON and blank lines are skipped")
	w := rec.Warnings()
	require.Len(t, w, 2)
	require.Equal(t, "careful", w[0].Message)
	require.Equal(t, "error", w[1].Level)
	require.Contains(t, rec.String(), "hello")

	f.finish()
	require.Equal(t, before, logger.Logger, "global logger restored")
}

func TestAssertNoWarnings(t *testing.T) {
	f := &fakeT{TB: t}
	rec := Capture(f)
	logger.Info().Msg("fine")
	rec.AssertNoWarnings(f)
	require.Empty(t, f.errors)

	logger.Warn().Msg("expected one")
	logger.Error().Msg("unexpected one")
	rec.AssertNoWarnings(f, "expected one")
	require.Len(t, f.errors, 1)
	require.Contains(t, f.errors[0], "unexpected one")
	f.finish()
}

func TestSeverity(t *testing.T) {
	for _, l := range []string{"warn", "error", "fatal", "panic"} {
		require.True(t, severe(l), l)
	}
	for _, l := range []string{"", "info", "debug", "trace"} {
		require.False(t, severe(l), l)
	}
}

func TestGuardFailsOnWarning(t *testing.T) {
	f := &fakeT{TB: t}
	Guard(f)
	logger.Warn().Msg("audit: failed to persist")
	f.finish()
	require.Len(t, f.errors, 1)
	require.Contains(t, f.errors[0], "audit: failed to persist")
}

func TestGuardPassesWhenQuiet(t *testing.T) {
	f := &fakeT{TB: t}
	Guard(f)
	logger.Info().Msg("all good")
	f.finish()
	require.Empty(t, f.errors)
}

func TestGuardSkipsWhenTestAlreadyFailed(t *testing.T) {
	f := &fakeT{TB: t}
	Guard(f)
	logger.Warn().Msg("noise")
	f.failed = true
	f.finish()
	require.Empty(t, f.errors, "a failing test reports its own failure, not log noise")
}

func TestAllowWarningsOptsOut(t *testing.T) {
	f := &fakeT{TB: t}
	Guard(f)
	AllowWarnings(f, "exercises the error path")
	logger.Warn().Msg("expected noise")
	f.finish()
	require.Empty(t, f.errors)
	require.Len(t, f.logs, 1)
	require.Contains(t, f.logs[0], "exercises the error path")
}

func TestAllowWarningsRequiresReason(t *testing.T) {
	f := &fakeT{TB: t}
	Guard(f)
	AllowWarnings(f, "  ")
	require.Len(t, f.fatals, 1)
	f.finish()
}

func TestAllowedCoversSubtestsOnly(t *testing.T) {
	allowMu.Lock()
	allowSet["TestParent"] = true
	allowMu.Unlock()
	defer func() {
		allowMu.Lock()
		delete(allowSet, "TestParent")
		allowMu.Unlock()
	}()
	require.True(t, allowed("TestParent"))
	require.True(t, allowed("TestParent/sub/deeper"))
	require.False(t, allowed("TestParentOther"))
	require.False(t, allowed("TestOther/sub"))
}
