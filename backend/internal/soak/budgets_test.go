package soak

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every budget carries a reason and a coherent shape: a number with no recorded
// why is a number nobody can safely change.
func TestBudgets_AreWellFormed(t *testing.T) {
	require.NotEmpty(t, Budgets)
	seen := map[string]bool{}
	for _, b := range Budgets {
		assert.NotEmpty(t, strings.TrimSpace(b.Reason), "%s needs a reason", b.Signal)
		assert.NotEmpty(t, b.Unit, "%s needs a unit", b.Signal)
		assert.Greater(t, b.Limit, 0.0, "%s limit must be positive", b.Signal)
		assert.False(t, seen[b.Signal], "duplicate budget for %s", b.Signal)
		seen[b.Signal] = true
		if b.Kind == KindDegradation {
			assert.Greater(t, b.Ratio, 1.0, "%s degradation needs a ratio > 1", b.Signal)
		}
	}
}

// Every signal a failure-class issue #1496 names has a failing (non-advisory)
// budget, so removing one is a visible diff.
func TestBudgets_CoverTheIssueSignals(t *testing.T) {
	failing := map[string]bool{}
	for _, b := range Budgets {
		if !b.Advisory {
			failing[b.Signal] = true
		}
	}
	for _, sig := range []string{SigRSS, SigHeapInuse, SigGoroutines, SigOpenFDs, SigWALBytes, SigLimiterEntries, SigP95Read, SigP95Write} {
		assert.True(t, failing[sig], "%s must have a failing budget", sig)
	}
	for _, sig := range RequiredSignals {
		found := false
		for _, b := range Budgets {
			if b.Signal == sig {
				found = true
			}
		}
		assert.True(t, found, "required signal %s has no budget", sig)
	}
}

func TestDefaultBudgets_ReturnsACopy(t *testing.T) {
	c := DefaultBudgets()
	c[0].Limit = 1e12
	assert.NotEqual(t, 1e12, Budgets[0].Limit, "mutating the copy must not touch the committed table")
}

func TestFmtLimitAndKindLabels(t *testing.T) {
	assert.Equal(t, "16 MiB", fmtLimit(Budget{Unit: "bytes", Limit: 16 * mib}))
	assert.Equal(t, "4 x + 0.25 s", fmtLimit(Budget{Kind: KindDegradation, Ratio: 4, Limit: 0.25, Unit: "s"}))
	assert.Equal(t, "12 goroutines", fmtLimit(Budget{Kind: KindGrowth, Limit: 12, Unit: "goroutines"}))
	assert.Contains(t, kindLabel(KindCeiling), "ceiling")
	assert.Contains(t, kindLabel(KindDegradation), "degradation")
	assert.Contains(t, kindLabel(KindGrowth), "growth")
}

const (
	docBegin = DocBudgetBegin
	docEnd   = DocBudgetEnd
)

// docs/development/soak-baseline.md carries the committed budget table between
// markers; it is rendered from Budgets, and this test fails until the doc is
// regenerated (`go run ./cmd/soak -write-budgets-doc`), so a threshold can
// never change without a reviewable doc diff — the soak counterpart of the
// perf-budgets drift test.
func TestSoakBaselineDoc_BudgetTableIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "development", "soak-baseline.md")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	doc := string(raw)
	i := strings.Index(doc, docBegin)
	j := strings.Index(doc, docEnd)
	require.True(t, i >= 0 && j > i, "soak-baseline.md must carry the %s ... %s markers", docBegin, docEnd)
	got := strings.TrimSpace(doc[i+len(docBegin) : j])
	assert.Equal(t, strings.TrimSpace(BudgetsMarkdown()), got,
		"soak-baseline.md's budget table is stale; regenerate with `cd backend && go run ./cmd/soak -write-budgets-doc ../docs/development/soak-baseline.md`")
}

func TestReplaceBudgetBlock(t *testing.T) {
	doc := "intro\n" + docBegin + "\nOLD\n" + docEnd + "\noutro\n"
	out, err := ReplaceBudgetBlock(doc)
	require.NoError(t, err)
	assert.Contains(t, out, BudgetsMarkdown())
	assert.NotContains(t, out, "OLD")
	assert.True(t, strings.HasPrefix(out, "intro\n"))
	assert.True(t, strings.HasSuffix(out, "outro\n"))

	_, err = ReplaceBudgetBlock("no markers here")
	assert.Error(t, err)
}
