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
}
