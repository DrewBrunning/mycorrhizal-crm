package database

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRootFromTestFile resolves the repository root from this test file's own
// path (backend/database/disaster_recovery_docs_test.go -> repo root), so the
// checks below do not depend on the process working directory. Same pattern as
// backup_test.go / backup_immutability_test.go.
func repoRootFromTestFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestDisasterRecoveryDocStatesTheBoundaries is the DOC-04-style drift guard for
// the disaster-recovery boundaries document (BACKUP-03, issue #455). The
// document's value is in the limits it states without hedging, so the test
// asserts those limits are actually present — a future edit that softens or
// drops one fails here rather than shipping a runbook that quietly over-promises.
//
// It pins the document against the code where a real constant exists (the
// upgrade floor tag, the pre-migration backup directory knob) so the two cannot
// drift, and against the issue #455 checklist otherwise (one procedure per
// scenario, RPO/RTO stated, an explicit "not recoverable" list).
func TestDisasterRecoveryDocStatesTheBoundaries(t *testing.T) {
	root := repoRootFromTestFile(t)
	path := filepath.Join(root, "docs", "operations", "disaster-recovery.md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "the disaster-recovery boundaries document must exist at docs/operations/disaster-recovery.md")
	doc := string(raw)

	// --- Pinned to code: these strings are enforced constants, not prose. ---

	assert.Contains(t, doc, SupportedUpgradeFloorTag,
		"the doc must name the supported upgrade floor tag the code enforces (database.SupportedUpgradeFloorTag)")
	assert.Contains(t, doc, preMigrationBackupDirEnvVar,
		"the doc must name the env var that moves the pre-migration backup directory (database.preMigrationBackupDirEnvVar)")
	assert.Contains(t, doc, "pre-migration/",
		"the doc must name the pre-migration snapshot directory")

	// --- The single-instance reality (issue #455 action 3). ---

	for _, phrase := range []string{"no replica", "no failover"} {
		assert.Contains(t, doc, phrase,
			"the doc must state the single-instance reality without hedging: %q", phrase)
	}

	// --- RPO / RTO are stated, per mechanism (issue #455 action 2, issue #506). ---

	assert.Contains(t, doc, "RPO", "the doc must state RPO")
	assert.Contains(t, doc, "RTO", "the doc must state RTO")
	assert.Regexp(t, `(?i)no backup scheduler|ships no backup scheduler|no default schedule|no default number`, doc,
		"the doc must state honestly that routine backup cadence — and therefore RPO — is operator-owned")

	// --- The soft-delete undo window is a stated guarantee (issue #455 action 2). ---

	assert.Contains(t, doc, "DELETED_RETENTION_DAYS",
		`the doc must name the retention env var (config/config.go: getIntEnv("DELETED_RETENTION_DAYS", 30))`)
	assert.Contains(t, doc, "30",
		"the doc must state the default soft-delete retention window (30 days)")

	// --- One documented procedure per scenario from issue #455 action 1. ---

	for _, scenario := range []string{
		"database corruption",
		"migration interrupted",
		"bad release",
		"accidental deletion",
		"loss of the host",
		"attachment or photo directory",
		"corrupted or incomplete backup",
		"lost `JWT_SECRET_KEY`",
	} {
		assert.Regexp(t, `(?i)`+regexp.QuoteMeta(scenario), doc,
			"issue #455 requires a documented scenario for: %q", scenario)
	}

	// --- An explicit, unhedged "not recoverable" statement (issue #455 action 2). ---

	assert.Regexp(t, `(?i)not recoverable`, doc,
		"the doc must have an explicit section stating what is simply not recoverable")
	assert.Contains(t, doc, "DATA_ENCRYPTION_KEY",
		"the not-recoverable list must call out a lost at-rest master key")

	// --- The drill: the milestone bar is 'exercised by following the documentation'. ---

	assert.Regexp(t, `(?i)the drill|exercised by following`, doc,
		"the doc must include a drill section so the runbook can be exercised from the document alone")

	// --- Cross-references resolve (issue #455 verify: internal links resolve). ---

	for _, rel := range []string{
		filepath.Join("docs", "operations", "migration-recovery.md"),
		filepath.Join("docs", "deployment.md"),
		filepath.Join("docs", "security", "incident-response.md"),
		filepath.Join("docs", "security", "data-retention-lifecycle.md"),
		filepath.Join("docs", "development", "scale-testing.md"),
	} {
		assert.FileExists(t, filepath.Join(root, rel),
			"the disaster-recovery doc references %s — it must exist", rel)
	}

	// --- The docs index links the new document. ---

	index, err := os.ReadFile(filepath.Join(root, "docs", "index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(index), "operations/disaster-recovery.md",
		"docs/index.md must link the disaster-recovery boundaries document")
}
