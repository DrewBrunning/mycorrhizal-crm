package compatci

// Issue #1551: CI Go tools are `tool` directives in backend/go.mod, run as
// `go tool <name>`, not `go run <module>@<version>` (a live module-proxy +
// sum.golang.org fetch per run, no retry — it reddened the required
// Backend (Go) gate on a checksum-DB read). These guards fail if a workflow
// (or the pre-commit hook) goes back to the fetch-at-test-time form, or a
// file that uses `go tool` loses its retried `go mod download`.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var goRunAtVersion = regexp.MustCompile(`go run [^\s]+@`)

// goToolFindings returns one finding per problem in a workflow source.
// requireDownload is false for the pre-commit hook (a developer's module
// cache, no CI download step).
func goToolFindings(name, src string, requireDownload bool) []string {
	var out []string
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if goRunAtVersion.MatchString(line) {
			out = append(out, name+":"+strconv.Itoa(i+1)+": `go run <module>@<version>` — use a go.mod tool directive + `go tool`")
		}
	}
	if requireDownload && usesGoTool(src) && !strings.Contains(src, "go mod download") {
		out = append(out, name+": uses `go tool` but has no retried `go mod download` step")
	}
	return out
}

func usesGoTool(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, "go tool ") {
			return true
		}
	}
	return false
}

func TestNoGoRunAtVersionInCI(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(workflowsDirRel, "*.yml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		require.NoError(t, err)
		assert.Empty(t, goToolFindings(filepath.Base(f), string(b), true))
	}
	hook, err := os.ReadFile(filepath.Join(workflowsDirRel, "..", "..", ".githooks", "pre-commit"))
	require.NoError(t, err)
	assert.Empty(t, goToolFindings("pre-commit", string(hook), false))
}

func TestGoToolFindingsMutations(t *testing.T) {
	assert.Len(t, goToolFindings("w.yml", "run: go run gotest.tools/gotestsum@v1.13.0 ./...", true), 1)
	assert.Len(t, goToolFindings("w.yml", "run: go tool gotestsum", true), 1)
	assert.Empty(t, goToolFindings("w.yml", "run: go tool gotestsum", false))
	assert.Empty(t, goToolFindings("w.yml", "# go run x/y@v1\n# go tool z\nrun: go run ./cmd/foo", true))
	assert.Empty(t, goToolFindings("w.yml", "run: go mod download\nrun: go tool gotestsum", true))
}
