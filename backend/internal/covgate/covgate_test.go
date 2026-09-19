package covgate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validCodecovYAML = `
coverage:
  status:
    patch:
      backend:
        target: 95%
        threshold: 5%
        flags: [backend]
        only_pulls: true
      frontend:
        target: 90%
        threshold: 10%
        flags: [frontend]
        only_pulls: true
`

func TestParsePatchAreas(t *testing.T) {
	areas, f := ParsePatchAreas([]byte(validCodecovYAML), "codecov.yml")
	require.Empty(t, f)
	require.Len(t, areas, 2)
	assert.Equal(t, PatchArea{Target: "95%", Threshold: "5%", Flags: []string{"backend"}, OnlyPulls: true}, areas["backend"])
	assert.Equal(t, PatchArea{Target: "90%", Threshold: "10%", Flags: []string{"frontend"}, OnlyPulls: true}, areas["frontend"])

	_, f = ParsePatchAreas([]byte("not: [valid"), "bad.yml")
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "does not parse as YAML")

	_, f = ParsePatchAreas([]byte("coverage:\n  status:\n    patch: {}\n"), "empty.yml")
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "has no coverage.status.patch entries")
}

func TestCrossCheckPatchAreas(t *testing.T) {
	same := map[string]PatchArea{
		"backend": {Target: "95%", Threshold: "5%", Flags: []string{"backend"}, OnlyPulls: true},
	}
	assert.Empty(t, CrossCheckPatchAreas(same, same))

	// codecov.yml narrows a target with no matching doc change.
	codecov := map[string]PatchArea{
		"backend": {Target: "5%", Threshold: "5%", Flags: []string{"backend"}, OnlyPulls: true},
	}
	doc := map[string]PatchArea{
		"backend": {Target: "95%", Threshold: "5%", Flags: []string{"backend"}, OnlyPulls: true},
	}
	f := CrossCheckPatchAreas(codecov, doc)
	require.Len(t, f, 1)
	assert.Contains(t, f[0], `target "5%" != docs/development/coverage.md target "95%"`)

	// threshold, only_pulls, and flags each independently flagged.
	codecov2 := map[string]PatchArea{
		"backend": {Target: "95%", Threshold: "50%", Flags: []string{"backend", "extra"}, OnlyPulls: false},
	}
	doc2 := map[string]PatchArea{
		"backend": {Target: "95%", Threshold: "5%", Flags: []string{"backend"}, OnlyPulls: true},
	}
	f = CrossCheckPatchAreas(codecov2, doc2)
	joined := join(f)
	assert.Contains(t, joined, `threshold "50%" != docs/development/coverage.md threshold "5%"`)
	assert.Contains(t, joined, "only_pulls=false != docs/development/coverage.md only_pulls=true")
	assert.Contains(t, joined, "flags")

	// A new area added to codecov.yml only (no matching doc block) -> a
	// silently widened ignore surface, same "no doc change" shape.
	extraArea := map[string]PatchArea{
		"backend": same["backend"],
		"android": {Target: "1%", Threshold: "99%", Flags: []string{"android"}, OnlyPulls: true},
	}
	f = CrossCheckPatchAreas(extraArea, same)
	require.Len(t, f, 1)
	assert.Contains(t, f[0], `codecov.yml has patch area "android" with no matching block`)

	// The reverse: doc claims an area codecov.yml no longer has.
	f = CrossCheckPatchAreas(same, extraArea)
	require.Len(t, f, 1)
	assert.Contains(t, f[0], `documents patch area "android", which codecov.yml does not have`)
}

func TestExtractStubAreas(t *testing.T) {
	areas, f := ExtractStubAreas([]byte("jobs:\n  codecov-patch-stub:\n    steps:\n      - env:\n          PATCH_AREAS: backend frontend android\n        run: echo\n"))
	require.Empty(t, f)
	assert.Equal(t, []string{"backend", "frontend", "android"}, areas)

	_, f = ExtractStubAreas([]byte("jobs:\n  other:\n    steps: []\n"))
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "no jobs.codecov-patch-stub job")

	_, f = ExtractStubAreas([]byte("jobs:\n  codecov-patch-stub:\n    steps:\n      - run: echo\n"))
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "no step with a PATCH_AREAS env")

	_, f = ExtractStubAreas([]byte("not: [valid"))
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "does not parse as YAML")
}

func TestCrossCheckStubAreas(t *testing.T) {
	codecov := map[string]PatchArea{
		"backend":  {Target: "95%"},
		"frontend": {Target: "90%"},
		"android":  {Target: "80%"},
	}
	assert.Empty(t, CrossCheckStubAreas(codecov, []string{"backend", "frontend", "android"}))

	// A codecov.yml area missing from the stub: its required context would be
	// stranded on a no-upload PR (issue #1188).
	f := CrossCheckStubAreas(codecov, []string{"backend", "frontend"})
	require.Len(t, f, 1)
	assert.Contains(t, f[0], `"android"`)
	assert.Contains(t, f[0], "does not list it")

	// An area in the stub that codecov.yml does not define.
	f = CrossCheckStubAreas(codecov, []string{"backend", "frontend", "android", "ios"})
	require.Len(t, f, 1)
	assert.Contains(t, f[0], `"ios"`)
	assert.Contains(t, f[0], "no patch area for")

	// A duplicate entry in the stub.
	f = CrossCheckStubAreas(codecov, []string{"backend", "backend", "frontend", "android"})
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "lists \"backend\" twice")
}

func TestExtractDocPatchStatusBlock(t *testing.T) {
	doc := "prose\n<!-- codecov-patch-status:begin -->\n```yaml\ncoverage:\n  status:\n    patch:\n      backend:\n        target: 95%\n```\n<!-- codecov-patch-status:end -->\nmore prose\n"
	block, f := ExtractDocPatchStatusBlock(doc)
	require.Empty(t, f)
	assert.Contains(t, block, "target: 95%")

	_, f = ExtractDocPatchStatusBlock("no markers here")
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "missing the")

	noFence := "<!-- codecov-patch-status:begin -->\nno fenced block\n<!-- codecov-patch-status:end -->\n"
	_, f = ExtractDocPatchStatusBlock(noFence)
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "no fenced")
}

func TestCheckIgnoreEntriesJustified(t *testing.T) {
	// Own head comment -> justified.
	good := "ignore:\n  # a good reason\n  - \"a/**\"\n"
	assert.Empty(t, CheckIgnoreEntriesJustified([]byte(good)))

	// Own trailing comment -> justified.
	goodTrailing := "ignore:\n  - \"a/**\"  # a good reason\n"
	assert.Empty(t, CheckIgnoreEntriesJustified([]byte(goodTrailing)))

	// Bare entry, no comment at all -> flagged.
	bare := "ignore:\n  - \"a/**\"\n"
	f := CheckIgnoreEntriesJustified([]byte(bare))
	require.Len(t, f, 1)
	assert.Contains(t, f[0], `entry "a/**"`)
	assert.Contains(t, f[0], "no justifying comment")

	// A bare entry appended right after an already-justified one must NOT
	// ride along on the neighbor's comment -- this is the exact attack the
	// checker exists to catch (issue #979).
	appended := "ignore:\n  # justified\n  - \"a/**\"\n  - \"b/**\"\n"
	f = CheckIgnoreEntriesJustified([]byte(appended))
	require.Len(t, f, 1)
	assert.Contains(t, f[0], `entry "b/**"`)

	// No ignore: list at all -- nothing to justify.
	assert.Empty(t, CheckIgnoreEntriesJustified([]byte("coverage:\n  status: {}\n")))

	// Unparseable YAML.
	f = CheckIgnoreEntriesJustified([]byte("not: [valid"))
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "does not parse as YAML")
}

// TestCommittedCodecovYAMLIsConsistent runs the real files, like
// cmd/codecovcheck does, and hand-verifies the checker actually fires by
// mutating a copy (never the real files) and confirming it goes red.
func TestCommittedCodecovYAMLIsConsistent(t *testing.T) {
	root := repoRoot(t)
	codecovBytes, err := os.ReadFile(filepath.Join(root, "codecov.yml"))
	require.NoError(t, err)
	docBytes, err := os.ReadFile(filepath.Join(root, "docs/development/coverage.md"))
	require.NoError(t, err)
	workflowBytes, err := os.ReadFile(filepath.Join(root, ".github/workflows/unit-tests.yml"))
	require.NoError(t, err)

	codecovAreas, f := ParsePatchAreas(codecovBytes, "codecov.yml")
	require.Empty(t, f)
	docBlock, f := ExtractDocPatchStatusBlock(string(docBytes))
	require.Empty(t, f)
	docAreas, f := ParsePatchAreas([]byte(docBlock), "docs/development/coverage.md")
	require.Empty(t, f)
	stubAreas, f := ExtractStubAreas(workflowBytes)
	require.Empty(t, f)

	assert.Empty(t, CrossCheckPatchAreas(codecovAreas, docAreas))
	assert.Empty(t, CheckIgnoreEntriesJustified(codecovBytes))
	assert.Empty(t, CrossCheckStubAreas(codecovAreas, stubAreas))

	// Hand-verify: a target drift the doc doesn't reflect must be caught.
	mutatedAreas := map[string]PatchArea{}
	for k, v := range codecovAreas {
		mutatedAreas[k] = v
	}
	backend := mutatedAreas["backend"]
	backend.Target = "5%"
	mutatedAreas["backend"] = backend
	assert.NotEmpty(t, CrossCheckPatchAreas(mutatedAreas, docAreas))
}

func join(ss []string) string {
	out := ""
	for _, s := range ss {
		out += s + "\n"
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir
		}
		p := filepath.Dir(dir)
		if p == dir {
			t.Fatal("repo root not found")
		}
		dir = p
	}
}
