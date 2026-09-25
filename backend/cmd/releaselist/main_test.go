package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"mycorrhizal/internal/schemafixture"

	"github.com/stretchr/testify/require"
)

// failingWriter always errors, so tests can exercise run's write-failure
// branch without touching the real os.Stdout (which does not fail this
// write in practice).
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("boom")
}

// TestRunEmitsEverySupportedRelease is the contract migration-tests.yml relies
// on: the matrix has exactly one leg per SupportedReleases entry, in registry
// order, and every leg's label names its tag.
func TestRunEmitsEverySupportedRelease(t *testing.T) {
	var buf bytes.Buffer
	require.Equal(t, 0, run(&buf))

	var got []matrixEntry
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, len(schemafixture.SupportedReleases))

	for i, r := range schemafixture.SupportedReleases {
		require.Equalf(t, r.Tag, got[i].Release, "matrix leg %d is out of registry order", i)
		require.Containsf(t, got[i].Entry, r.Tag, "leg %s label must name its tag", r.Tag)
	}
}

// TestRunLabelsTheFloorAsLongestSkip pins the one leg with a distinct name:
// floor -> current is the longest supported skip and must stay labelled as such
// (matching the name the hand-written matrix used).
func TestRunLabelsTheFloorAsLongestSkip(t *testing.T) {
	var buf bytes.Buffer
	require.Equal(t, 0, run(&buf))

	var got []matrixEntry
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))

	for _, e := range got {
		if e.Release == schemafixture.FloorTag {
			require.Equal(t, schemafixture.FloorTag+" → current (longest supported skip)", e.Entry)
			return
		}
	}
	t.Fatalf("floor release %s not found in matrix output", schemafixture.FloorTag)
}

// TestRunEmitsSingleLineJSON keeps the output usable as
// `echo "matrix=$(go run ./cmd/releaselist)" >> "$GITHUB_OUTPUT"` — one line,
// parseable by fromJSON.
func TestRunEmitsSingleLineJSON(t *testing.T) {
	var buf bytes.Buffer
	require.Equal(t, 0, run(&buf))

	require.Equal(t, 1, bytes.Count(buf.Bytes(), []byte("\n")), "output must be exactly one line")
	require.True(t, json.Valid(bytes.TrimSpace(buf.Bytes())))
}

// TestRunReturns2OnWriteFailure proves the checker actually fails closed
// when it cannot report its result — a checker that swallows a write error
// and exits 0 would silently look like success in CI.
func TestRunReturns2OnWriteFailure(t *testing.T) {
	require.Equal(t, 2, run(failingWriter{}))
}

// TestMainExitsZero drives main() itself (not just run()) through the
// osExit seam, so the real process-entry wiring is exercised too.
func TestMainExitsZero(t *testing.T) {
	origExit := osExit
	origStdout := os.Stdout
	defer func() {
		osExit = origExit
		os.Stdout = origStdout
	}()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	var gotCode int
	exited := false
	osExit = func(code int) { gotCode = code; exited = true }

	main()

	require.NoError(t, w.Close())
	os.Stdout = origStdout

	require.True(t, exited, "main must call osExit")
	require.Equal(t, 0, gotCode)

	buf, err := io.ReadAll(r)
	require.NoError(t, err)
	require.True(t, json.Valid(bytes.TrimSpace(buf)))
}
