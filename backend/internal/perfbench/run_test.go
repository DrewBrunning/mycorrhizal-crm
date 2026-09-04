package perfbench

import (
	"testing"

	"mycorrhizal/internal/largedata"

	"github.com/stretchr/testify/assert"
)

func TestIsAtScale(t *testing.T) {
	assert.False(t, isAtScale(largedata.Smoke))
	assert.False(t, isAtScale(largedata.Typical))
	assert.True(t, isAtScale(largedata.Large))
	assert.True(t, isAtScale(largedata.Stress))
	// Keyed on the contact target, not the name.
	assert.True(t, isAtScale(largedata.Profile{Name: "xlarge", Contacts: largedata.Large.Contacts + 1}))
	assert.False(t, isAtScale(largedata.Profile{Name: "tiny", Contacts: largedata.Large.Contacts - 1}))
}

func TestOperationsForProfile_SkipAtScale(t *testing.T) {
	full := Registry()

	// Below `large`, every registered operation runs.
	for _, p := range []largedata.Profile{largedata.Smoke, largedata.Typical} {
		assert.Len(t, operationsForProfile(p), len(full), "profile %q must run the full registry", p.Name)
	}

	// At `large` and above, the SkipAtScale operations are omitted and nothing
	// else is.
	var skipped []string
	for _, op := range full {
		if op.SkipAtScale {
			skipped = append(skipped, op.Name)
		}
	}
	assert.NotEmpty(t, skipped, "at least one operation is expected to be SkipAtScale (the graph.traverse_* blow-ups)")

	for _, p := range []largedata.Profile{largedata.Large, largedata.Stress} {
		got := operationsForProfile(p)
		assert.Len(t, got, len(full)-len(skipped), "profile %q must drop exactly the SkipAtScale operations", p.Name)
		names := map[string]bool{}
		for _, op := range got {
			names[op.Name] = true
			assert.Falsef(t, op.SkipAtScale, "profile %q kept SkipAtScale operation %q", p.Name, op.Name)
		}
		for _, s := range skipped {
			assert.Falsef(t, names[s], "profile %q kept skipped operation %q", p.Name, s)
		}
	}
}

// TestGraphTraversalDeepIsSkippedAtScale pins the specific operations the fix
// (PERF-02 large-dataset CI timeout) removes from the `large` profile — a
// walk-enumeration recursive CTE over the synthetically dense PERF-01 graph
// does not terminate in a sane wall-clock there, and its only deterministic
// signal (a constant query count) is already pinned at smoke + typical.
func TestGraphTraversalDeepIsSkippedAtScale(t *testing.T) {
	atScale := map[string]bool{}
	for _, op := range operationsForProfile(largedata.Large) {
		atScale[op.Name] = true
	}
	assert.False(t, atScale["graph.traverse_deep"], "graph.traverse_deep must be SkipAtScale")
	assert.False(t, atScale["graph.traverse_hub"], "graph.traverse_hub must be SkipAtScale")
	// The bounded, low-degree traversal stays — it is the at-scale data point.
	assert.True(t, atScale["graph.traverse_shallow"], "graph.traverse_shallow must still run at scale")
	// The other recorded super-linear finding terminates, so it stays in.
	assert.True(t, atScale["duplicates.find_pairs"], "duplicates.find_pairs must still run at scale")
}
