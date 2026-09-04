package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDocsStateTheDependencyUpgradePolicy is the COMPAT-03 (issue #474)
// companion to schemafixture's TestDocsStateTheFloor and
// TestDocsStateTheBreakingChangePolicy: the canonical dependency-upgrade
// policy must still state the load-bearing claims the issue's verify list
// names — the per-ecosystem tiers and response times, the pinning rules, the
// #331 gap, the Gradle-lockfile decision, the exception ledger this package
// enforces, and the tie to the supported-runtime matrix — so the published
// policy cannot silently lose a promise this command (or a reviewer) relies
// on.
func TestDocsStateTheDependencyUpgradePolicy(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	doc, err := os.ReadFile(filepath.Join(root, "docs", "dependency-upgrade-policy.md"))
	require.NoError(t, err)
	text := string(doc)

	// Ecosystems covered.
	for _, eco := range []string{
		"Go modules", "npm/Yarn", "Gradle", "Docker base images", "GitHub Actions",
	} {
		assert.Contains(t, text, eco, "the doc must cover %q as an ecosystem", eco)
	}

	// The three tiers, named.
	for _, tier := range []string{
		"Security fixes", "Patch and minor", "Major",
	} {
		assert.Contains(t, text, tier, "the doc must name the %q tier", tier)
	}

	// Response-time numbers, tied to numbers this project already commits to
	// elsewhere (SECURITY.md's 5-business-day acknowledgment, MAINT-01's
	// 90-day deprecation window) rather than inventing new ones.
	assert.Contains(t, text, "5 business days", "the doc must state the security-advisory triage target")
	assert.Contains(t, text, "90 days", "the doc must state the exception window ceiling")

	// Pinning rules and what actually enforces them.
	assert.Contains(t, text, "Go toolchain", "the doc must state the Go toolchain pinning rule")
	assert.Contains(t, text, "commit SHA", "the doc must state the GitHub Actions SHA-pinning rule")
	assert.Contains(t, text, "digest", "the doc must state the Docker base-image digest-pinning rule")
	assert.Contains(t, text, "zizmor", "the doc must name what mechanically enforces Actions pinning")
	assert.Contains(t, text, "hadolint", "the doc must name what mechanically enforces Docker digest pinning")

	// The known pinning gap.
	assert.Contains(t, text, "issue #331", "the doc must record the #331 Dockerfile fallback gap")

	// The Gradle lockfile decision.
	assert.Contains(t, text, "gradle.lockfile", "the doc must record the Gradle-lockfile decision")

	// The exception ledger this command enforces.
	assert.Contains(t, text, "dependency-exceptions.ignore", "the doc must name the exception ledger")
	assert.Contains(t, text, "depexceptions", "the doc must name the command that enforces the ledger")

	// The tie to the supported-runtime matrix and the breaking-change policy.
	assert.Contains(t, text, "issue #472", "the doc must tie a floor-raising bump to the supported-runtime matrix")
	assert.Contains(t, text, "issue #491", "the doc must tie a floor-raising bump to the breaking-change policy")
}
