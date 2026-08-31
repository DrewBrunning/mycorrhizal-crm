package differential

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestContactCorpus assembles the shared corpus and checks its invariants:
// stable IDs, no duplicates, and every contact parseable. The corpus is the
// differential's input, so a corpus regression (a fixture card that stops
// importing, a seed file that goes stale) must fail here, not silently shrink
// the differential's coverage.
func TestContactCorpus(t *testing.T) {
	corpus, err := ContactCorpus()
	require.NoError(t, err)
	require.NotEmpty(t, corpus)

	seen := map[string]bool{}
	for _, e := range corpus {
		require.NotEmpty(t, e.ID, "corpus entry with empty ID")
		require.NotNil(t, e.Record, "corpus entry %s with nil record", e.ID)
		require.False(t, seen[e.ID], "duplicate corpus ID %s", e.ID)
		seen[e.ID] = true
	}

	// Every source contributes: the canonical fixture (TEST-02), the golden
	// fixtures (ADR-0003) and the generated seeds (#435).
	var fixture, golden, seed int
	for id := range seen {
		switch {
		case len(id) >= len("fixture/") && id[:len("fixture/")] == "fixture/":
			fixture++
		case len(id) >= len("golden/") && id[:len("golden/")] == "golden/":
			golden++
		case len(id) >= len("seed/") && id[:len("seed/")] == "seed/":
			seed++
		}
	}
	require.Positive(t, fixture, "corpus must include the TEST-02 canonical fixture contacts")
	require.Positive(t, golden, "corpus must include the ADR-0003 golden fixtures")
	require.Positive(t, seed, "corpus must include the pinned generated seeds (#435)")
}

// TestPinsReferenceRealCorpusEntries is the pin table's integrity guard: every
// divergence pin must name a corpus entry that actually exists, so a renamed
// or removed corpus card (which would silently invalidate a pin) fails here
// instead of leaving a dangling pin that the drift check can never see.
func TestPinsReferenceRealCorpusEntries(t *testing.T) {
	corpus, err := ContactCorpus()
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, e := range corpus {
		ids[e.ID] = true
	}

	// Register every format's pin table so the registry is fully populated
	// regardless of which differential tests ran before (each registrar is
	// idempotent).
	registerVCardPins()
	registerJSContactPins()

	require.NotEmpty(t, registry, "pin registry must not be empty once the pin tables are registered")
	for _, pin := range registry {
		require.True(t, ids[pin.CorpusID], "pin references unknown corpus entry %q", pin.CorpusID)
		require.NotEmpty(t, pin.Reason, "pin for %s/%s/%s has no written reason", pin.CorpusID, pin.Format, pin.Dir)
		require.NotEmpty(t, pin.Concepts, "pin for %s/%s/%s pins no concepts", pin.CorpusID, pin.Format, pin.Dir)
	}
}

// TestClassifyLegDriftDetection exercises the pin filter directly: an
// unpinned disagreement fails, a pinned one is absorbed, and a pin that no
// longer reproduces is reported stale. This is the unit test for the drift
// mechanism the real legs depend on. It snapshot/restores the shared registry
// so it cannot destroy the pins the differential tests registered.
func TestClassifyLegDriftDetection(t *testing.T) {
	saved := registry
	defer func() { registry = saved }()
	registry = []DivergencePin{
		{CorpusID: "fixture/ada", Format: "vcard4", Dir: dirOursToRef, Concepts: []string{"adr"}, Reason: "test pin"},
	}

	mkDiff := func(concept string) Difference {
		return Difference{Concept: concept, presentA: true}
	}

	// A pinned disagreement is absorbed; an unpinned one still fails,
	// naming the concept.
	res := classifyLeg([]Difference{mkDiff("adr"), mkDiff("email")}, "fixture/ada", "vcard4", dirOursToRef)
	require.Len(t, res.Unexpected, 1)
	require.Equal(t, "email", res.Unexpected[0].Concept)
	require.Empty(t, res.Stale)

	// A pin that no longer reproduces is reported stale (drift detection).
	res = classifyLeg(nil, "fixture/ada", "vcard4", dirOursToRef)
	require.Empty(t, res.Unexpected)
	require.Equal(t, []string{"adr"}, res.Stale)

	// A pin for a different entry/format/direction never matches.
	res = classifyLeg([]Difference{mkDiff("adr")}, "fixture/bob", "vcard4", dirOursToRef)
	require.Len(t, res.Unexpected, 1)

	// Multiple unexpected differences are reported in deterministic order.
	res = classifyLeg([]Difference{mkDiff("email"), mkDiff("phone")}, "fixture/ada", "vcard4", dirOursToRef)
	require.Equal(t, []string{"email", "phone"}, []string{res.Unexpected[0].Concept, res.Unexpected[1].Concept})
}

// TestDiffText covers the failure renderer (named concept + values, never
// "outputs differ") and its join helper.
func TestDiffText(t *testing.T) {
	loss := DiffText(Difference{Concept: "email", presentA: true, ValuesA: []string{"a@example.com"}})
	require.Contains(t, loss, "concept \"email\"")
	require.Contains(t, loss, "a@example.com")

	changed := DiffText(Difference{Concept: "phone", presentA: true, presentB: true, ValuesA: []string{"1"}, ValuesB: []string{"2"}})
	require.Contains(t, changed, "values differ")

	gained := DiffText(Difference{Concept: "name.full", presentB: true, ValuesB: []string{"Derived"}})
	require.Contains(t, gained, "synthesized")
}

// TestInScopeFiltersPassthroughAndNoHome verifies the comparison scope: only
// correspondence concepts with a home in the format are compared, and the
// passthrough concepts are never (the references do not preserve our exact
// unknown-property shapes).
func TestInScopeFiltersPassthroughAndNoHome(t *testing.T) {
	for _, f := range []formatName{formatVCard3, formatVCard4, formatJSContact} {
		require.False(t, f.inScope("pt.vcard"), "%s must exclude passthrough", f.label)
		require.False(t, f.inScope("pt.jscontact"), "%s must exclude passthrough", f.label)
	}
	// uid has a home in all three.
	require.True(t, formatVCard3.inScope("uid"))
	require.True(t, formatVCard4.inScope("uid"))
	require.True(t, formatJSContact.inScope("uid"))
	// kind has no v3 home.
	require.False(t, formatVCard3.inScope("kind"))
	require.True(t, formatVCard4.inScope("kind"))
	// A concept with no correspondence row is out of scope.
	require.False(t, formatVCard4.inScope("no-such-concept"))
}
