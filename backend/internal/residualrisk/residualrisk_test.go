package residualrisk

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFixtureTree lays down every declared accept list plus the two ledgers,
// so Summarize has a complete, self-consistent tree to read.
func writeFixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		".trivyignore": "# a comment\n\nDS-0002\n",
		".grype.yml":   "ignore: []\n",
		"zap/dast.ignore": "# comment\n" +
			"40045 * .* # Spring4Shell does not apply\n" +
			"10055 * .* # bounded diagnostic\n",
		"schemathesis/schemathesis.ignore": "# no entries at present\n",
		"docker/cis-hardening.ignore":      "# comment\n4.1 WARN\n",
		"android/.mobsf":                   "ignore-paths:\n  - build\nignore-rules:\n  - android_certificate_pinning\n  - android_ssl_pinning\n  - android_task_hijacking1\n",
		"docs/security/citation-drift.ignore": "# comment\n" +
			"docs/security/asvs-l2.md | `a.go:1` | a.go  # false positive\n" +
			"docs/security/asvs-l2.md | `b.go:2` | b.go  # false positive\n",
		"docs/security/crypto-surface.ignore": "# comment\n" +
			"backend/services/x.go  # comparison helper\n",
		"docs/security/governance-drift.ignore": "# no entries\n",
		"docs/security/dependency-exceptions.ignore": "# comment\n" +
			"GHSA-aaaa-bbbb-cccc | npm | example-pkg | 2026-09-01 | 2026-09-01 | 2026-09-25 | 0 | drew | no upstream fix yet\n" +
			"CVE-2026-0001 | go | example/mod | 2026-08-01 | 2026-08-01 | 2026-10-30 | 0 | drew | blocked by a pin\n",
		ReportFile: "> **ASVS Level 2, with 23 documented exceptions.** **MASVS-L1, with 1 documented exception** (plus two\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return root
}

func TestSummarize(t *testing.T) {
	root := writeFixtureTree(t)
	now := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)

	got, err := Summarize(root, now)
	require.NoError(t, err)

	// 1 trivy + 0 grype + 2 zap + 0 schemathesis + 1 cis + 3 mobsf + 2 drift + 1 crypto + 0 governance
	assert.Equal(t, 10, got.AcceptItems.Total)
	bySource := map[string]int{}
	for _, s := range got.AcceptItems.BySource {
		bySource[s.Source] = s.Entries
	}
	assert.Equal(t, 1, bySource[".trivyignore"])
	assert.Equal(t, 0, bySource[".grype.yml"])
	assert.Equal(t, 2, bySource["zap/dast.ignore"])
	assert.Equal(t, 3, bySource["android/.mobsf"])
	assert.Equal(t, 2, bySource["docs/security/citation-drift.ignore"])
	assert.Equal(t, 1, bySource["docs/security/crypto-surface.ignore"])

	assert.Equal(t, 2, got.DependencyExceptions.Open)
	assert.Equal(t, 1, got.DependencyExceptions.SoonExpiring)
	assert.Equal(t, "2026-09-25", got.DependencyExceptions.SoonestExpiry)
	require.Len(t, got.DependencyExceptions.Entries, 2)
	assert.Equal(t, "GHSA-aaaa-bbbb-cccc", got.DependencyExceptions.Entries[0].Advisory)
	assert.Equal(t, 7, got.DependencyExceptions.Entries[0].DaysRemaining)
	assert.True(t, got.DependencyExceptions.Entries[0].SoonExpiring)
	assert.False(t, got.DependencyExceptions.Entries[1].SoonExpiring)

	assert.Equal(t, 23, got.Exceptions.ASVS)
	assert.Equal(t, 1, got.Exceptions.MASVS)
}

func TestSummarizeMissingAcceptListFails(t *testing.T) {
	root := writeFixtureTree(t)
	require.NoError(t, os.Remove(filepath.Join(root, ".trivyignore")))

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".trivyignore")
}

func TestSummarizeMalformedDependencyLineFails(t *testing.T) {
	root := writeFixtureTree(t)
	p := filepath.Join(root, ExceptionsFile)
	require.NoError(t, os.WriteFile(p, []byte("not | enough | fields\n"), 0o644))

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "9 pipe-delimited fields")
}

func TestSummarizeMissingExceptionsFileFails(t *testing.T) {
	root := writeFixtureTree(t)
	require.NoError(t, os.Remove(filepath.Join(root, ExceptionsFile)))

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ExceptionsFile)
}

func TestSummarizeBadExpiryDateFails(t *testing.T) {
	root := writeFixtureTree(t)
	line := "GHSA-aaaa-bbbb-cccc | npm | example-pkg | 2026-09-01 | 2026-09-01 | not-a-date | 0 | drew | reason\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ExceptionsFile), []byte(line), 0o644))

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-date")
}

func TestSummarizeUnknownSourceKindFails(t *testing.T) {
	root := writeFixtureTree(t)
	before := acceptSources
	t.Cleanup(func() { acceptSources = before })
	acceptSources = append(append([]acceptSource{}, before...), acceptSource{Path: ".trivyignore", Kind: countKind(99)})

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown count kind")
}

func TestSummarizeMissingReportFails(t *testing.T) {
	root := writeFixtureTree(t)
	require.NoError(t, os.Remove(filepath.Join(root, ReportFile)))

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ReportFile)
}

func TestSummarizeMissingMasvsClaimFails(t *testing.T) {
	root := writeFixtureTree(t)
	claim := "> **ASVS Level 2, with 23 documented exceptions.**\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ReportFile), []byte(claim), 0o644))

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MASVS")
}

func TestSummarizeMissingClaimFails(t *testing.T) {
	root := writeFixtureTree(t)
	p := filepath.Join(root, ReportFile)
	require.NoError(t, os.WriteFile(p, []byte("no claim here\n"), 0o644))

	_, err := Summarize(root, time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ASVS")
}

func TestCountAcceptEntriesUnknownKind(t *testing.T) {
	_, err := countAcceptEntries(nil, countKind(99))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown count kind")
}

func TestReadRepoFileRejectsTraversal(t *testing.T) {
	_, err := readRepoFile(t.TempDir(), "../escape")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the repository root")
}

func TestCountNonCommentLines(t *testing.T) {
	body := "# comment\n\nflag\n  indented flag\n# another\n   \n"
	assert.Equal(t, 2, countNonCommentLines([]byte(body)))
}

func TestCountYAMLListItems(t *testing.T) {
	t.Run("empty inline list", func(t *testing.T) {
		assert.Equal(t, 0, countYAMLListItems([]byte("ignore: []\n"), "ignore:"))
	})
	t.Run("comments inside list are not items", func(t *testing.T) {
		body := "ignore-rules:\n  # why\n  - one\n  - two\nother: value\n  - not counted\n"
		assert.Equal(t, 2, countYAMLListItems([]byte(body), "ignore-rules:"))
	})
	t.Run("indented key is not the top-level key", func(t *testing.T) {
		body := "outer:\n  ignore: []\nignore:\n  - x\n"
		assert.Equal(t, 1, countYAMLListItems([]byte(body), "ignore:"))
	})
}
