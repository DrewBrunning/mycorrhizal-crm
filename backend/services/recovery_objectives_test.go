package services

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"mycorrhizal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvaluateRestoreDrillRTOBudget covers the non-fatal RTO-budget annotation
// on the restore drill's success path (issue #506). The measured duration is a
// plain parameter here, so both branches are exercised deterministically
// without needing a genuinely slow drill.
func TestEvaluateRestoreDrillRTOBudget(t *testing.T) {
	ctx := context.Background()

	t.Run("no budget configured returns no detail", func(t *testing.T) {
		got := evaluateRestoreDrillRTOBudget(ctx, config.Config{DBRestoreDrillMaxDurationSeconds: 0}, 999_999)
		assert.Empty(t, got)
	})

	t.Run("within budget returns no detail", func(t *testing.T) {
		got := evaluateRestoreDrillRTOBudget(ctx, config.Config{DBRestoreDrillMaxDurationSeconds: 10}, 9_000)
		assert.Empty(t, got)
	})

	t.Run("exactly at budget is not over", func(t *testing.T) {
		got := evaluateRestoreDrillRTOBudget(ctx, config.Config{DBRestoreDrillMaxDurationSeconds: 10}, 10_000)
		assert.Empty(t, got)
	})

	t.Run("over budget returns a detail naming the budget and the duration", func(t *testing.T) {
		got := evaluateRestoreDrillRTOBudget(ctx, config.Config{DBRestoreDrillMaxDurationSeconds: 5}, 12_500)
		assert.Contains(t, got, "12500ms")
		assert.Contains(t, got, "5s RTO budget")
		assert.Contains(t, got, "DB_RESTORE_DRILL_MAX_DURATION_SECONDS")
	})
}

// TestRecoveryObjectivesDocMatchesShippedDefaults pins the numbers in
// docs/deployment.md's "Recovery objectives (RPO and RTO)" section against the
// code they are derived from (issue #506, "derive rather than assert"):
//
//   - the backup-freshness ceiling stated in the RPO table must equal
//     2 x config.DefaultDBRestoreDrillIntervalHours — the default
//     ALERT_BACKUP_MAX_AGE_HOURS. Lengthen the restore-drill cadence and this
//     assertion fails until the documented number is updated to match, which is
//     the hand-verify CLAUDE.md asks for.
//   - the RTO table's migration figure must still appear in
//     docs/development/scale-testing.md, so a re-measure there cannot silently
//     leave the RTO derivation quoting a stale number.
func TestRecoveryObjectivesDocMatchesShippedDefaults(t *testing.T) {
	deployment := readRepoDoc(t, "docs/deployment.md")

	mustContain(t, deployment, "### Recovery objectives (RPO and RTO)",
		"the RPO/RTO section must exist in docs/deployment.md")

	// RPO: the stated freshness ceiling is derived, not asserted.
	ceilingHours := 2 * config.DefaultDBRestoreDrillIntervalHours
	mustContain(t, deployment, fmt.Sprintf("= **%d h**", ceilingHours),
		fmt.Sprintf("the RPO table must state the %d h backup-freshness ceiling "+
			"(2 x DefaultDBRestoreDrillIntervalHours); update docs/deployment.md if the cadence changed",
			ceilingHours))

	// RPO: the planned-upgrade row must state a zero objective.
	if !regexp.MustCompile(`(?s)Planned upgrade rollback.*?\*\*0\*\*`).MatchString(deployment) {
		t.Error("the RPO table must state RPO 0 for the planned-upgrade-rollback scenario")
	}

	// RTO: the number quoted in deployment.md must still be the one recorded in
	// scale-testing.md (issue #495), so the derivation cannot age silently.
	const migrationFigure = "6.5 s"
	mustContain(t, deployment, migrationFigure,
		"the RTO table quotes the 100k-contact migration duration from scale-testing.md")
	mustContain(t, readRepoDoc(t, "docs/development/scale-testing.md"), migrationFigure,
		fmt.Sprintf("docs/development/scale-testing.md no longer records %q — re-derive the RTO figure "+
			"in docs/deployment.md's Recovery objectives section", migrationFigure))

	// The section must point operators at the drift-tracking knob.
	mustContain(t, deployment, "DB_RESTORE_DRILL_MAX_DURATION_SECONDS",
		"the RTO section must document the restore-drill duration budget")
}

// mustContain fails with a short message (not a dump of the whole haystack,
// which is a ~30 KB doc here).
func mustContain(t *testing.T, haystack, needle, msg string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s\n  missing substring: %q", msg, needle)
	}
}

// readRepoDoc reads a doc file addressed from the repository root. Backend
// package tests run with the working directory at backend/<pkg>, so the repo
// root is two levels up.
func readRepoDoc(t *testing.T, relPath string) string {
	t.Helper()
	b, err := os.ReadFile("../../" + relPath)
	require.NoError(t, err, "read %s", relPath)
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}
