package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingReader always errors, so tests can exercise run's stdin-read
// failure branch without depending on real stdin behavior.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

// failingWriter always errors, so tests can exercise run's write-failure
// branch.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("boom")
}

func TestRunAssemblesFromStdin(t *testing.T) {
	in := strings.NewReader(`[
	  {"number": 812, "title": "add column", "url": "https://x/pull/812",
	   "body": "## Summary\nx\n\n## Upgrade notes\nRebuild the search index after upgrading.\n"},
	  {"number": 813, "title": "refactor", "url": "https://x/pull/813", "body": "no-changelog: internal only\n"}
	]`)
	var out bytes.Buffer

	require.Equal(t, 0, run(in, &out))
	got := out.String()
	assert.Contains(t, got, "## Upgrade notes")
	assert.Contains(t, got, "**[#812](https://x/pull/812)** — Rebuild the search index after upgrading.")
	assert.NotContains(t, got, "#813")
}

func TestRunEmptyArrayStillEmitsUpgradeNotes(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 0, run(strings.NewReader(`[]`), &out))
	assert.Contains(t, out.String(), "## Upgrade notes")
	assert.Contains(t, out.String(), "no action required")
}

func TestRunRejectsNonJSON(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 2, run(strings.NewReader("not json"), &out))
	assert.Empty(t, out.String())
}

// TestRunReturns2OnReadFailure proves the checker fails closed when stdin
// itself cannot be read, rather than silently treating it as empty input.
func TestRunReturns2OnReadFailure(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 2, run(failingReader{}, &out))
	assert.Empty(t, out.String())
}

// TestRunReturns2OnWriteFailure proves the checker fails closed when it
// cannot write its assembled output, rather than silently exiting 0.
func TestRunReturns2OnWriteFailure(t *testing.T) {
	require.Equal(t, 2, run(strings.NewReader(`[]`), failingWriter{}))
}

// TestMainExitsZero drives main() itself through the osExit seam.
func TestMainExitsZero(t *testing.T) {
	origExit := osExit
	origStdin := os.Stdin
	origStdout := os.Stdout
	defer func() {
		osExit = origExit
		os.Stdin = origStdin
		os.Stdout = origStdout
	}()

	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	_, err = inW.WriteString(`[]`)
	require.NoError(t, err)
	require.NoError(t, inW.Close())
	os.Stdin = inR

	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = outW

	var gotCode int
	exited := false
	osExit = func(code int) { gotCode = code; exited = true }

	main()

	require.NoError(t, outW.Close())
	os.Stdout = origStdout

	require.True(t, exited, "main must call osExit")
	require.Equal(t, 0, gotCode)

	buf, err := io.ReadAll(outR)
	require.NoError(t, err)
	assert.Contains(t, string(buf), "## Upgrade notes")
}
