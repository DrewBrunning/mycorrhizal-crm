package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAgainstCommittedFiles(t *testing.T) {
	var out bytes.Buffer
	code := run(&out)
	require.Equalf(t, 0, code, "codecovcheck failed:\n%s", out.String())
	assert.Contains(t, out.String(), "codecov.yml OK")
}

func TestFindRepoRoot(t *testing.T) {
	root, err := findRepoRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root)
}

func TestRunSuccessMessageShape(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 0, run(&out))
	line := strings.TrimSpace(out.String())
	assert.True(t, strings.HasPrefix(line, "codecov.yml OK:"), line)
}

const fixtureDoc = "<!-- codecov-patch-status:begin -->\n\n```yaml\ncoverage:\n  status:\n    patch:\n      backend:\n        target: 95%\n        threshold: 5%\n        flags: [backend]\n        only_pulls: true\n```\n\n<!-- codecov-patch-status:end -->\n"

// TestRunAtReportsTargetDrift exercises the "target changed without a
// matching doc change" finding end to end through runAt, the same fixture
// pattern cmd/governancecheck's test uses.
func TestRunAtReportsTargetDrift(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", `coverage:
  status:
    patch:
      backend:
        target: 50%
        threshold: 5%
        flags: [backend]
        only_pulls: true
ignore:
  # a bare entry, no per-entry comment
  - "a/**"
  - "b/**"
`)
	write("docs/development/coverage.md", fixtureDoc)

	var out bytes.Buffer
	code := runAt(&out, root)
	require.Equal(t, 1, code)
	s := out.String()
	assert.Contains(t, s, `target "50%" != docs/development/coverage.md target "95%"`)
	assert.Contains(t, s, `entry "b/**"`)
	assert.Contains(t, s, "codecov.yml finding(s)")
}

func TestRunAtMissingFileExits2(t *testing.T) {
	var out bytes.Buffer
	assert.Equal(t, 2, runAt(&out, t.TempDir()))
	assert.Contains(t, out.String(), "cannot read")
}

func TestRunAtCleanFixturePasses(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("codecov.yml", `coverage:
  status:
    patch:
      backend:
        target: 95%
        threshold: 5%
        flags: [backend]
        only_pulls: true
ignore:
  # justified
  - "a/**"
`)
	write("docs/development/coverage.md", fixtureDoc)

	var out bytes.Buffer
	require.Equal(t, 0, runAt(&out, root))
}
