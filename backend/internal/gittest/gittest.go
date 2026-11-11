// Package gittest makes tests that build throwaway git repositories
// hermetic.
//
// git exports GIT_DIR, GIT_INDEX_FILE and GIT_WORK_TREE to hook
// subprocesses (and `git rebase --exec`, `git bisect run`, ...). A test that
// inherits them and runs `git init` + `git config user.name Test` in a temp
// dir does not touch the temp dir at all: every command acts on the REAL
// repository, so the fixture identity lands in the shared .git/config and
// every later commit in every worktree is signed off as
// "Test <test@example.com>". Isolate removes the repository-location
// variables and detaches git from the developer's global/system config.
//
// Use it from TestMain in any package whose tests shell out to git:
//
//	func TestMain(m *testing.M) { os.Exit(gittest.Main(m)) }
package gittest

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// inheritedRepoVars are the variables that redirect git to a repository other
// than the one in the command's working directory.
var inheritedRepoVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
	"GIT_PREFIX",
	"GIT_CEILING_DIRECTORIES",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM",
}

// Isolate unsets the repository-location variables for the whole process and
// points git's global and system config at nothing, so fixtures neither leak
// into nor pick up configuration from the developer's real repository.
func Isolate() {
	for _, key := range inheritedRepoVars {
		_ = os.Unsetenv(key)
	}
	_ = os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	_ = os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// Main is the TestMain body: isolate, run the package, return the exit code.
func Main(m *testing.M) int {
	Isolate()
	return m.Run()
}

// Run runs `git <args>` with dir as the working directory and returns the
// combined output, failing the test on error.
func Run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...) // #nosec G204 -- test-only helper; fixed binary, args come from test code
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
