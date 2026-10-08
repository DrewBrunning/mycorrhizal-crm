package compatci

// Issue #1551: CI Go tools are `tool` directives in backend/tools.mod — a
// separate modfile — run as `go tool -modfile=tools.mod <name>`, not
// `go run <module>@<version>` (a live module-proxy + sum.golang.org fetch per
// run, no retry — it reddened the required Backend (Go) gate on a checksum-DB
// read). They live in tools.mod rather than backend/go.mod because tool
// dependencies join the main module's version selection: in go.mod they moved
// two dependencies of the shipped server binary (purego, protobuf) and made
// every image build's `go mod download` fetch ~200 tool-only modules.
//
// These guards fail if a workflow (or the pre-commit hook) goes back to the
// fetch-at-test-time form, runs a tool without -modfile=tools.mod, lacks the
// retried download of tools.mod, or if backend/go.mod grows a tool block.

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

var (
	goRunAtVersion = regexp.MustCompile(`go run [^\s]+@`)
	// goToolCall matches `go tool <name>` and `go tool -modfile=... <name>`;
	// group 1 is the -modfile flag if present, group 2 the tool name.
	goToolCall = regexp.MustCompile(`go tool (-modfile=\S+ )?([a-z][\w-]*)`)
	// builtinGoTools ship with the Go toolchain and need no modfile.
	builtinGoTools = map[string]bool{"cover": true, "pprof": true, "trace": true, "nm": true, "objdump": true, "dist": true, "covdata": true}
)

const toolsModfileFlag = "-modfile=tools.mod "

// goToolFindings returns one finding per problem in a workflow source.
// requireDownload is false for the pre-commit hook (a developer's module
// cache, no CI download step).
func goToolFindings(name, src string, requireDownload bool) []string {
	var out []string
	usesTools := false
	for i, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		where := name + ":" + strconv.Itoa(i+1)
		if goRunAtVersion.MatchString(line) {
			out = append(out, where+": `go run <module>@<version>` — use a tools.mod tool directive + `go tool -modfile=tools.mod`")
		}
		for _, m := range goToolCall.FindAllStringSubmatch(line, -1) {
			if builtinGoTools[m[2]] {
				continue
			}
			usesTools = true
			if m[1] != toolsModfileFlag {
				out = append(out, where+": `go tool "+m[2]+"` must run as `go tool -modfile=tools.mod "+m[2]+"`")
			}
		}
	}
	if requireDownload && usesTools && !strings.Contains(src, "go mod download -modfile=tools.mod") {
		out = append(out, name+": runs tools.mod tools but has no retried `go mod download -modfile=tools.mod` step")
	}
	return out
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

// TestToolDirectivesStayOutOfTheMainModule pins the split: the tools are in
// tools.mod, and backend/go.mod (whose graph builds the shipped binary and
// feeds every image build's `go mod download`) has no tool block.
func TestToolDirectivesStayOutOfTheMainModule(t *testing.T) {
	backend := filepath.Join(workflowsDirRel, "..", "..", "backend")
	goMod, err := os.ReadFile(filepath.Join(backend, "go.mod"))
	require.NoError(t, err)
	assert.NotRegexp(t, `(?m)^tool[ (]`, string(goMod), "backend/go.mod must not carry tool directives; add them to backend/tools.mod (go get -modfile=tools.mod -tool ...)")

	toolsMod, err := os.ReadFile(filepath.Join(backend, "tools.mod"))
	require.NoError(t, err)
	for _, tool := range []string{
		"gotest.tools/gotestsum",
		"golang.org/x/vuln/cmd/govulncheck",
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
		"github.com/go-gremlins/gremlins/cmd/gremlins",
	} {
		assert.Contains(t, string(toolsMod), "\t"+tool+"\n", "tools.mod must declare %s", tool)
	}
	_, err = os.Stat(filepath.Join(backend, "tools.sum"))
	assert.NoError(t, err, "tools.mod needs its tools.sum committed")
}

func TestGoToolFindingsMutations(t *testing.T) {
	assert.Len(t, goToolFindings("w.yml", "run: go run gotest.tools/gotestsum@v1.13.0 ./...", true), 1)
	// No -modfile: wrong module graph (and no download step).
	assert.Len(t, goToolFindings("w.yml", "run: go mod download -modfile=tools.mod\nrun: go tool gotestsum", true), 1)
	// Correct invocation but no tools.mod download.
	assert.Len(t, goToolFindings("w.yml", "run: go tool -modfile=tools.mod gotestsum", true), 1)
	assert.Empty(t, goToolFindings("w.yml", "run: go tool -modfile=tools.mod gotestsum", false))
	assert.Empty(t, goToolFindings("w.yml", "# go run x/y@v1\n# go tool z\nrun: go run ./cmd/foo", true))
	assert.Empty(t, goToolFindings("w.yml", "run: go mod download -modfile=tools.mod\nrun: go tool -modfile=tools.mod gotestsum", true))
	// Toolchain built-ins need neither.
	assert.Empty(t, goToolFindings("w.yml", "run: go tool cover -func=c.out", true))
}
