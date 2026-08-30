package schemafixture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDocsStateTheBreakingChangePolicy is the MAINT-02 (issue #491) companion
// to TestDocsStateTheFloor: the canonical breaking-change policy must still
// state the load-bearing claims the issue's verify list names — the covered
// surfaces, the breaking/non-breaking categories, the one-sentence /api/v1
// promise, and the 2.0.0-not-parallel-v2 versioning decision — so the
// published policy cannot silently lose a promise a 1.0.0 gate item (#525)
// depends on.
func TestDocsStateTheBreakingChangePolicy(t *testing.T) {
	dir, err := findRepoRoot()
	require.NoError(t, err)
	doc, err := os.ReadFile(filepath.Join(dir, "docs", "breaking-change-policy.md"))
	require.NoError(t, err)
	text := string(doc)

	// The /api/v1 promise, one quotable sentence.
	assert.Contains(t, text, "does not remove, rename,",
		"the doc must state the /api/v1 non-breaking promise")
	assert.Contains(t, text, "`1.0.0` or any later `1.x`",
		"the doc must bound the promise to shipped releases")

	// Covered surfaces.
	for _, surface := range []string{
		"`/api/v1` REST contract",
		"database schema",
		"Configuration variable names and semantics",
		"CLI commands and flags",
		"Export formats",
		"supported runtime matrix",
		"client/server",
	} {
		assert.Contains(t, text, surface, "the doc must list %q as a covered surface", surface)
	}

	// Breaking categories.
	for _, breakClass := range []string{
		"Removes or renames",
		"Narrows accepted input",
		"Changes a response's type or meaning",
		"Changes a default in a way that alters behavior",
		"Raises a supported-version minimum",
		"Loses user data",
	} {
		assert.Contains(t, text, breakClass, "the doc must list %q as a breaking change", breakClass)
	}

	// Non-breaking categories.
	for _, additive := range []string{
		"Adding an optional request field",
		"Adding a response field",
		"Adding an endpoint",
		"Adding an enum value",
		"Bug fixes that bring behavior into line with documentation",
	} {
		assert.Contains(t, text, additive, "the doc must list %q as non-breaking", additive)
	}

	// Client requirement + versioning decision.
	assert.Contains(t, text, "clients must ignore unknown response",
		"the doc must state the client unknown-field-tolerance requirement")
	assert.Contains(t, text, "**`2.0.0`, not a parallel `/api/v2`**",
		"the doc must record the versioning decision: 2.0.0, not parallel API versions")
	assert.Contains(t, text, "Within `1.x`, `/api/v1` does not break",
		"the doc must state that /api/v1 does not break within 1.x")
	assert.Contains(t, text, "issue #490",
		"the doc must tie removal windows to the deprecation policy (MAINT-01, issue #490)")
}
