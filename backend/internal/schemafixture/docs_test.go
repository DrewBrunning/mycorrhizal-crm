package schemafixture

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDocsStateTheFloor is the DOC-04-style documentation check (issue #529
// verify, DOC-02's publication step): the canonical supported-upgrade
// statement must name the same floor this package and the database package
// enforce, so the published policy cannot drift from the code.
//
// The test asserts the FULL policy surface the issue records, not just the
// floor version: version-skipping within the range, the post-1.0 rule, the
// downgrade position, and the two-step instruction, so a doc edit that
// softens any part of the promise (not only the floor number) fails here.
func TestDocsStateTheFloor(t *testing.T) {
	dir, err := findRepoRoot()
	require.NoError(t, err)
	doc, err := os.ReadFile(filepath.Join(dir, "docs", "upgrade-compatibility.md"))
	require.NoError(t, err)
	text := string(doc)

	assert.Contains(t, text, "v0.6.0", "the doc must state the floor release")
	assert.Contains(t, text, fmt.Sprintf("migration %d", FloorVersion), "the doc must state the floor migration version")
	assert.Contains(t, text, "Upgrade this instance to v0.6.0 first", "the doc must state the two-step instruction")
	assert.Contains(t, text, "MYCORRHIZAL_ALLOW_SUB_FLOOR_MIGRATION", "the doc must document the one-time bridge override")

	// The supported range: version-skipping is a promise, not a convenience.
	assert.Contains(t, text, "Version-skipping within the range is supported",
		"the doc must state that skipping versions within the range is supported")
	assert.Contains(t, text, "v0.6.0 → current",
		"the doc must name the longest supported skip explicitly")
	assert.Contains(t, text, "Downgrade is unsupported",
		"the doc must state the downgrade position (the companion decision)")

	// The post-1.0 rule: the floor only moves at a major version.
	assert.Contains(t, text, "any `1.x` upgrade from any earlier `1.x`",
		"the doc must state that 1.x upgrades are supported from any earlier 1.x")
	assert.Contains(t, text, "from the final\n  `0.9.x`",
		"the doc must state that the final 0.9.x is the 1.0 boundary")
	assert.Contains(t, text, "The floor moves only at a major version",
		"the doc must state that the floor cannot move within a major line")
}
