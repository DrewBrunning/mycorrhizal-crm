package governance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGovernanceDriftIgnore(t *testing.T) {
	body := "# header comment\n\n" +
		"main-protection  # PAT rotation in progress, see #916\n" +
		"  tags-v   #   trailing spaces trimmed  \n"
	entries, findings := ParseGovernanceDriftIgnore(body)
	assert.Empty(t, findings)
	assert.Len(t, entries, 2)
	assert.Equal(t, "PAT rotation in progress, see #916", entries["main-protection"].Reason)
	assert.Equal(t, 3, entries["main-protection"].Line)
	assert.Equal(t, "trailing spaces trimmed", entries["tags-v"].Reason)
}

func TestParseGovernanceDriftIgnoreMalformedLine(t *testing.T) {
	_, findings := ParseGovernanceDriftIgnore("main-protection no hash reason here\n")
	assert.Len(t, findings, 1)
	assert.Contains(t, findings[0], "line 1")
	assert.Contains(t, findings[0], "expected `<ruleset name>  # <reason>`")
}

func TestParseGovernanceDriftIgnoreDuplicate(t *testing.T) {
	body := "main-protection  # first\nmain-protection  # second\n"
	entries, findings := ParseGovernanceDriftIgnore(body)
	assert.Len(t, entries, 1)
	assert.Equal(t, "first", entries["main-protection"].Reason)
	assert.Len(t, findings, 1)
	assert.Contains(t, findings[0], "duplicate entry")
	assert.Contains(t, findings[0], "line 2")
}

func TestParseGovernanceDriftIgnoreEmpty(t *testing.T) {
	entries, findings := ParseGovernanceDriftIgnore("")
	assert.Empty(t, entries)
	assert.Empty(t, findings)
}

// TestEvaluateGovernanceDrift_UnignoredDriftFails is the #916 regression
// test: before the fix, a ruleset that drifted with no ignore entry produced
// no finding at all -- the workflow only ever warned and always exited 0.
func TestEvaluateGovernanceDrift_UnignoredDriftFails(t *testing.T) {
	statuses := []RulesetStatus{{Name: "main-protection", Applied: true, Drifted: true}}
	findings, accepted := EvaluateGovernanceDrift(statuses, nil)
	assert.Empty(t, accepted)
	assert.Len(t, findings, 1)
	assert.Contains(t, findings[0], "main-protection")
	assert.Contains(t, findings[0], "drifted from .github/rulesets/")
}

// TestEvaluateGovernanceDrift_UnappliedFails is the same regression for a
// ruleset that was never applied (or was deleted from the live repository) --
// arguably the more severe case, since the protection simply does not exist.
func TestEvaluateGovernanceDrift_UnappliedFails(t *testing.T) {
	statuses := []RulesetStatus{{Name: "tags-v", Applied: false}}
	findings, accepted := EvaluateGovernanceDrift(statuses, nil)
	assert.Empty(t, accepted)
	assert.Len(t, findings, 1)
	assert.Contains(t, findings[0], "not applied to the repository")
}

func TestEvaluateGovernanceDrift_MatchingPasses(t *testing.T) {
	statuses := []RulesetStatus{
		{Name: "main-protection", Applied: true, Drifted: false},
		{Name: "tags-v", Applied: true, Drifted: false},
	}
	findings, accepted := EvaluateGovernanceDrift(statuses, nil)
	assert.Empty(t, findings)
	assert.Empty(t, accepted)
}

func TestEvaluateGovernanceDrift_IgnoredDriftAccepted(t *testing.T) {
	statuses := []RulesetStatus{{Name: "main-protection", Applied: true, Drifted: true}}
	ignore := map[string]IgnoreEntry{
		"main-protection": {Line: 5, Name: "main-protection", Reason: "PAT rotation, see #916"},
	}
	findings, accepted := EvaluateGovernanceDrift(statuses, ignore)
	assert.Empty(t, findings)
	assert.Len(t, accepted, 1)
	assert.Contains(t, accepted[0], "main-protection")
	assert.Contains(t, accepted[0], "PAT rotation, see #916")
}

// TestEvaluateGovernanceDrift_StaleIgnoreEntryFails is the bidirectional
// half of the ignore-list-with-justification convention (matching
// citation-drift.ignore / crypto-surface.ignore): an accepted entry for a
// ruleset that is now back in sync must fail, not just sit there.
func TestEvaluateGovernanceDrift_StaleIgnoreEntryFails(t *testing.T) {
	statuses := []RulesetStatus{{Name: "main-protection", Applied: true, Drifted: false}}
	ignore := map[string]IgnoreEntry{
		"main-protection": {Line: 5, Name: "main-protection", Reason: "no longer relevant"},
	}
	findings, accepted := EvaluateGovernanceDrift(statuses, ignore)
	assert.Empty(t, accepted)
	assert.Len(t, findings, 1)
	assert.Contains(t, findings[0], "back in sync")
	assert.Contains(t, findings[0], "line 5")
}

func TestEvaluateGovernanceDrift_UnknownIgnoreEntryFails(t *testing.T) {
	ignore := map[string]IgnoreEntry{
		"not-a-real-ruleset": {Line: 5, Name: "not-a-real-ruleset", Reason: "typo"},
	}
	findings, accepted := EvaluateGovernanceDrift(nil, ignore)
	assert.Empty(t, accepted)
	assert.Len(t, findings, 1)
	assert.Contains(t, findings[0], "not-a-real-ruleset")
	assert.Contains(t, findings[0], "does not match any committed")
}

func TestEvaluateGovernanceDrift_MultipleFindingsSorted(t *testing.T) {
	statuses := []RulesetStatus{
		{Name: "tags-v", Applied: false},
		{Name: "main-protection", Applied: true, Drifted: true},
	}
	findings, _ := EvaluateGovernanceDrift(statuses, nil)
	require := assert.New(t)
	require.Len(findings, 2)
	require.Contains(findings[0], "main-protection")
	require.Contains(findings[1], "tags-v")
}

func TestRulesetStatusMismatched(t *testing.T) {
	assert.True(t, RulesetStatus{Applied: false}.Mismatched())
	assert.True(t, RulesetStatus{Applied: true, Drifted: true}.Mismatched())
	assert.False(t, RulesetStatus{Applied: true, Drifted: false}.Mismatched())
}
