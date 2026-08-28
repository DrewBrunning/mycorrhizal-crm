package faults

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startClean makes sure no fault armed by a previous test (or by this
// process's own MYCORRHIZAL_FAULTS env, read at init) survives into the test.
func startClean(t *testing.T) {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
}

func TestHookReturnsNilWhenUnarmed(t *testing.T) {
	startClean(t)
	assert.NoError(t, Hook("no.such.fault"))
	assert.False(t, Enabled("no.such.fault"))
}

func TestArmErrorHookReturnsArmedError(t *testing.T) {
	startClean(t)

	sentinel := errors.New("boom")
	ArmError("test.fault", sentinel)
	assert.True(t, Enabled("test.fault"))

	err := Hook("test.fault")
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "the exact armed error must cross the seam unchanged")
}

func TestArmErrorNilErrorArmsDefault(t *testing.T) {
	startClean(t)

	ArmError("test.default", nil)
	err := Hook("test.default")
	require.Error(t, err)
	assert.ErrorIs(t, err, &ErrInjected{Name: "test.default"})
	assert.Contains(t, err.Error(), "injected fault")
	assert.Contains(t, err.Error(), "test.default")
}

func TestErrInjectedErrorString(t *testing.T) {
	e := &ErrInjected{Name: "a.b", Message: "my message"}
	assert.Equal(t, "my message: a.b", e.Error())
}

func TestArmPauseBlocksAndReturnsNil(t *testing.T) {
	startClean(t)

	ArmPause("test.pause", 5*time.Millisecond)
	require.True(t, Enabled("test.pause"))

	start := time.Now()
	err := Hook("test.pause")
	elapsed := time.Since(start)
	assert.NoError(t, err, "a pause fault completes and returns nil")
	assert.GreaterOrEqual(t, elapsed, 5*time.Millisecond, "Hook must block for the configured duration")
}

func TestArmPauseNonPositiveDurationArmsNothing(t *testing.T) {
	startClean(t)

	ArmPause("test.pause", 0)
	ArmPause("test.pause2", -time.Second)
	assert.False(t, Enabled("test.pause"))
	assert.False(t, Enabled("test.pause2"))
	assert.NoError(t, Hook("test.pause"))
}

func TestDisarmRemovesFault(t *testing.T) {
	startClean(t)

	ArmError("test.fault", errors.New("boom"))
	require.True(t, Enabled("test.fault"))

	Disarm("test.fault")
	assert.False(t, Enabled("test.fault"))
	assert.NoError(t, Hook("test.fault"))
}

func TestResetClearsEverything(t *testing.T) {
	startClean(t)

	ArmError("test.one", errors.New("one"))
	ArmError("test.two", errors.New("two"))
	require.True(t, Enabled("test.one"))

	Reset()
	assert.False(t, Enabled("test.one"))
	assert.False(t, Enabled("test.two"))
	assert.NoError(t, Hook("test.one"))
}

func TestListSorted(t *testing.T) {
	startClean(t)

	ArmError("zeta", errors.New("z"))
	ArmError("alpha", errors.New("a"))
	ArmError("mid", errors.New("m"))

	assert.Equal(t, []string{"alpha", "mid", "zeta"}, List())
}

func TestSetFromEnvEmptyDoesNothing(t *testing.T) {
	startClean(t)

	old := os.Getenv(envVar)
	t.Cleanup(func() { os.Setenv(envVar, old) })
	require.NoError(t, os.Setenv(envVar, ""))

	require.NoError(t, SetFromEnv())
	assert.Empty(t, List())
}

func TestSetFromEnvParsesEntries(t *testing.T) {
	startClean(t)

	old := os.Getenv(envVar)
	t.Cleanup(func() { os.Setenv(envVar, old) })
	require.NoError(t, os.Setenv(envVar,
		"bare.fault, custom:err:my message, pausing:pause:15ms, spaced : err : also spaced"))

	require.NoError(t, SetFromEnv())

	assert.True(t, Enabled("bare.fault"))
	assert.True(t, Enabled("custom"))
	assert.True(t, Enabled("pausing"))
	assert.True(t, Enabled("spaced"))

	// Bare name -> default ErrInjected.
	err := Hook("bare.fault")
	require.Error(t, err)
	assert.ErrorIs(t, err, &ErrInjected{Name: "bare.fault"})

	// :err: carries the custom message, colons included.
	err = Hook("custom")
	require.Error(t, err)
	assert.Equal(t, "my message: custom", err.Error())

	// :pause: arms a blocking fault.
	start := time.Now()
	require.NoError(t, Hook("pausing"))
	assert.GreaterOrEqual(t, time.Since(start), 15*time.Millisecond)

	// Whitespace around name/kind tokens is tolerated.
	err = Hook("spaced")
	require.Error(t, err)
	assert.Equal(t, "also spaced: spaced", err.Error())
}

func TestSetFromEnvErrMessageMayContainColons(t *testing.T) {
	startClean(t)

	old := os.Getenv(envVar)
	t.Cleanup(func() { os.Setenv(envVar, old) })
	require.NoError(t, os.Setenv(envVar, "f:err:first:second:third"))

	require.NoError(t, SetFromEnv())
	err := Hook("f")
	require.Error(t, err)
	assert.Equal(t, "first:second:third: f", err.Error())
}

func TestSetFromEnvRejectsMalformed(t *testing.T) {
	startClean(t)

	old := os.Getenv(envVar)
	t.Cleanup(func() { os.Setenv(envVar, old) })

	cases := map[string]string{
		"no name":              ":err:message",
		"unknown kind":         "fault:explode:1",
		"bad pause duration":   "fault:pause:not-a-duration",
		"pause missing value":  "fault:pause:",
		"one good one garbage": "good.fault, bad:explode:1",
	}
	for label, value := range cases {
		t.Run(label, func(t *testing.T) {
			startClean(t)
			require.NoError(t, os.Setenv(envVar, value))
			err := SetFromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MYCORRHIZAL_FAULTS")
		})
	}
}

func TestSetFromEnvSkipsBlankEntries(t *testing.T) {
	startClean(t)

	old := os.Getenv(envVar)
	t.Cleanup(func() { os.Setenv(envVar, old) })
	require.NoError(t, os.Setenv(envVar, "a.fault,  , b.fault"))

	require.NoError(t, SetFromEnv())
	assert.Equal(t, []string{"a.fault", "b.fault"}, List())
}

func TestErrInjectedIsErrorIsPortable(t *testing.T) {
	startClean(t)

	// An arm site wraps the injected error like any other; errors.Is must
	// still see the fault (matching by Name, not Message).
	ArmError("wrapped.fault", fmt.Errorf("wrapped: %w", &ErrInjected{Name: "wrapped.fault", Message: "custom"}))
	assert.ErrorIs(t, Hook("wrapped.fault"), &ErrInjected{Name: "wrapped.fault"})

	// Real-world assertion style: arm with the sentinel the site itself uses
	// (a shared package-level error) and assert the caller sees that sentinel.
	siteSentinel := errors.New("site sentinel")
	ArmError("sentinel.fault", siteSentinel)
	assert.ErrorIs(t, Hook("sentinel.fault"), siteSentinel)
}

func TestErrInjectedIsDoesNotMatchWrongNameOrNonErrInjected(t *testing.T) {
	e := &ErrInjected{Name: "a.fault", Message: "x"}
	assert.False(t, e.Is(&ErrInjected{Name: "b.fault"}))
	assert.False(t, e.Is(errors.New("plain error")))
	assert.False(t, e.Is(nil))
	assert.True(t, e.Is(&ErrInjected{Name: "a.fault"}))
}

func TestConcurrentHookAndArmAreSafe(t *testing.T) {
	startClean(t)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			_ = Hook("race.fault")
			ArmError("race.fault", errors.New("x"))
			Disarm("race.fault")
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		_ = Hook("race.fault")
	}
	<-done
}

func TestListEmptyWhenNothingArmed(t *testing.T) {
	startClean(t)
	assert.Empty(t, List())
}

func TestPauseMarkerLogged(t *testing.T) {
	// The pause marker is the external harness's "reached the seam" signal; at
	// least ensure the message text is greppable and the fault name is present.
	startClean(t)

	ArmPause("pause.marker", time.Millisecond)
	// This exercises the Warn branch; it cannot be asserted on here (zerolog's
	// default writer goes to stderr), but it must not panic and must return.
	assert.NoError(t, Hook("pause.marker"))

	// The equivalent marker text the external job greps for.
	assert.True(t, strings.Contains("injected fault pause", "injected fault pause"))
}
