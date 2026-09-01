package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"mycorrhizal/internal/schemafixture"

	"github.com/stretchr/testify/require"
)

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
