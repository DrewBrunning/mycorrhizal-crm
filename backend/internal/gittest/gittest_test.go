package gittest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestIsolateStopsInheritedGitDirLeakingConfig is the regression test for the
// "everything is committed as Test <test@example.com>" bug. Git exports
// GIT_DIR to hook subprocesses; a test that then ran `git init` +
// `git config user.name Test` in a temp dir wrote into the REAL repository's
// config instead, silently changing the identity of every later commit.
func TestIsolateStopsInheritedGitDirLeakingConfig(t *testing.T) {
	Isolate() // so creating the victim itself is not redirected by an outer GIT_DIR
	victim := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", victim).CombinedOutput(); err != nil {
		t.Fatalf("init victim: %v: %s", err, out)
	}
	t.Setenv("GIT_DIR", victim+"/.git")
	t.Setenv("GIT_WORK_TREE", victim)
	t.Setenv("GIT_INDEX_FILE", victim+"/.git/index")

	Isolate()

	for _, key := range inheritedRepoVars {
		if v, ok := os.LookupEnv(key); ok {
			t.Errorf("%s still set to %q after Isolate", key, v)
		}
	}

	dir := t.TempDir()
	Run(t, dir, "init", "-q")
	Run(t, dir, "config", "user.name", "Test")

	cfg, err := os.ReadFile(victim + "/.git/config")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), "Test") {
		t.Fatalf("temp-repo config leaked into the ambient GIT_DIR repo:\n%s", cfg)
	}
	if got := strings.TrimSpace(Run(t, dir, "config", "--local", "user.name")); got != "Test" {
		t.Fatalf("temp repo user.name = %q, want Test", got)
	}
}

func TestIsolateDetachesFromGlobalAndSystemConfig(t *testing.T) {
	Isolate()
	if got := os.Getenv("GIT_CONFIG_GLOBAL"); got != os.DevNull {
		t.Errorf("GIT_CONFIG_GLOBAL = %q, want %q", got, os.DevNull)
	}
	if got := os.Getenv("GIT_CONFIG_NOSYSTEM"); got != "1" {
		t.Errorf("GIT_CONFIG_NOSYSTEM = %q, want 1", got)
	}
}

func TestMain(m *testing.M) { os.Exit(Main(m)) }

// recorder captures Fatalf so Run's failure path can be exercised without
// failing the real test.
type recorder struct {
	testing.TB
	msg string
}

func (r *recorder) Helper()                  {}
func (r *recorder) Context() context.Context { return context.Background() }
func (r *recorder) Fatalf(format string, args ...any) {
	r.msg = fmt.Sprintf(format, args...)
}

func TestRunReportsGitFailure(t *testing.T) {
	r := &recorder{TB: t}
	Run(r, t.TempDir(), "not-a-git-subcommand")
	if !strings.Contains(r.msg, "not-a-git-subcommand") {
		t.Fatalf("Fatalf message = %q, want it to name the failing command", r.msg)
	}
}
