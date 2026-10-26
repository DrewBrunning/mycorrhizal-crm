package agenteval

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q", ".")
	runGit(t, root, "config", "user.email", "t@example.invalid")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "commit.gpgsign", "false")
	writeRepoFile(t, root, "AGENTS.md", "# Root\n\n## Project map\n\n| x | y |\n\n## Safety\n")
	writeRepoFile(t, root, "backend/AGENTS.md", "# Backend\n")
	writeRepoFile(t, root, ".agents/skills/demo/SKILL.md", "---\nname: demo\ndescription: x\n---\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "init")
	return root, strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func writeRepoFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func baseSpec(t *testing.T, root, sha string) Spec {
	t.Helper()
	return Spec{
		Name:         "demo",
		Prompt:       "do nothing",
		BaseRef:      sha,
		Suite:        "demo",
		ExpectPaths:  []string{"**"},
		AgentCommand: "true",
		TimeoutValue: 30 * time.Second,
		TaskDir:      t.TempDir(),
	}
}

func TestRunNoopWithControl(t *testing.T) {
	root, sha := initRepo(t)
	before, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := Run(context.Background(), Options{
		Root:         root,
		Spec:         baseSpec(t, root, sha),
		Suites:       []Suite{{Name: "demo", Commands: []string{"true"}}},
		AgentCommand: "true",
		RunControl:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "passed" || artifact.Context == nil || !artifact.Context.Success {
		t.Fatalf("artifact = %+v context=%+v control=%+v", artifact, artifact.Context, artifact.Control)
	}
	if artifact.Control == nil || !artifact.Control.Success {
		t.Fatalf("control = %+v", artifact.Control)
	}
	if artifact.Delta == nil || artifact.Delta.Summary != "both passed" {
		t.Fatalf("delta = %+v", artifact.Delta)
	}
	for _, want := range []string{".agents/skills", "backend/AGENTS.md", "AGENTS.md#project-map"} {
		if !contains(artifact.Stripped, want) {
			t.Fatalf("stripped %v missing %q", artifact.Stripped, want)
		}
	}
	after, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("caller's working tree was modified")
	}
	if got := strings.TrimSpace(runGit(t, root, "status", "--porcelain")); got != "" {
		t.Fatalf("caller's tree is dirty: %s", got)
	}
}

func TestRunSeedPatchFailureBeforeAgent(t *testing.T) {
	root, sha := initRepo(t)
	spec := baseSpec(t, root, sha)
	if err := os.WriteFile(filepath.Join(spec.TaskDir, "bad.patch"), []byte("not a patch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec.SeedPatch = "bad.patch"
	calls := 0
	artifact, err := Run(context.Background(), Options{
		Root:   root,
		Spec:   spec,
		Suites: []Suite{{Name: "demo", Commands: []string{"true"}}},
		LookPath: func(string) (string, error) {
			return "/bin/true", nil
		},
		CommandRunner: func(context.Context, string, string, time.Duration) CommandResult {
			calls++
			return CommandResult{Passed: true}
		},
	})
	if err == nil || !strings.Contains(err.Error(), "seed patch does not apply") {
		t.Fatalf("err = %v", err)
	}
	if calls != 0 {
		t.Fatalf("agent ran after a failed seed patch (calls=%d)", calls)
	}
	if artifact.Status != "error" {
		t.Fatalf("artifact status = %q", artifact.Status)
	}
}

func TestRunSkipsMissingTool(t *testing.T) {
	root, sha := initRepo(t)
	spec := baseSpec(t, root, sha)
	spec.Requires = []string{"go"}
	artifact, err := Run(context.Background(), Options{
		Root: root,
		Spec: spec,
		LookPath: func(string) (string, error) {
			return "", os.ErrNotExist
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "skipped" || !strings.Contains(artifact.Reason, "missing required tool") {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestStripRoutingTable(t *testing.T) {
	in := "before\n\n## Project map\n\n| x | y |\n\n## Safety\n\nafter\n"
	got := stripRoutingTable(in)
	if strings.Contains(got, "Project map") || !strings.Contains(got, "## Safety") || !strings.HasPrefix(got, "before") {
		t.Fatalf("stripRoutingTable() = %q", got)
	}
	if got := stripRoutingTable("no map\n"); got != "no map\n" {
		t.Fatalf("unchanged = %q", got)
	}
	if got := stripRoutingTable("## Project map\n\nonly\n"); strings.Contains(got, "Project map") {
		t.Fatalf("leading heading = %q", got)
	}
}

func TestApplySeedPatch(t *testing.T) {
	root, _ := initRepo(t)
	writeRepoFile(t, root, "backend/AGENTS.md", "# Backend changed\n")
	patchPath := filepath.Join(t.TempDir(), "seed.patch")
	diff := runGit(t, root, "diff", "--", "backend/AGENTS.md")
	if err := os.WriteFile(patchPath, []byte(diff), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "checkout", "--", "backend/AGENTS.md")
	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, root, "worktree", "add", "--detach", wt, "HEAD")
	defer runGit(t, root, "worktree", "remove", "--force", wt)
	if err := applySeedPatch(context.Background(), wt, patchPath); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(wt, "backend", "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "changed") {
		t.Fatalf("seed patch not applied: %s", body)
	}
}

func TestRunRecordsIntegrityViolations(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := Run(context.Background(), Options{
		Root:         root,
		Spec:         baseSpec(t, root, sha),
		Suites:       []Suite{{Name: "demo", Commands: []string{"true"}}},
		AgentCommand: `printf 'package x\n\nimport "testing"\n\nfunc TestX(t *testing.T) { t.Skip("x") }\n' > backend/x_test.go`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Context == nil {
		t.Fatal("missing context run")
	}
	if artifact.Status != "failed" || artifact.Context.Success {
		t.Fatalf("artifact = %+v context = %+v", artifact, artifact.Context)
	}
	if len(artifact.Context.IntegrityViolations) == 0 {
		t.Fatalf("integrity findings = %v", artifact.Context.IntegrityViolations)
	}
	if !strings.Contains(strings.Join(artifact.Context.IntegrityViolations, "\n"), "added test skip: backend/x_test.go") {
		t.Fatalf("integrity findings = %v", artifact.Context.IntegrityViolations)
	}
}

func TestRunFailsWhenSuiteCommandFails(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := Run(context.Background(), Options{
		Root:   root,
		Spec:   baseSpec(t, root, sha),
		Suites: []Suite{{Name: "demo", Commands: []string{"false"}}},
		LookPath: func(string) (string, error) {
			return "/bin/true", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || artifact.Context == nil || artifact.Context.Success {
		t.Fatalf("artifact = %+v", artifact)
	}
	if artifact.Context.Status != "failed" {
		t.Fatalf("context status = %q", artifact.Context.Status)
	}
}

func TestRunTempDirFailure(t *testing.T) {
	root, sha := initRepo(t)
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", filepath.Join(file, "nope"))
	artifact, err := Run(context.Background(), Options{
		Root: root,
		Spec: baseSpec(t, root, sha),
		LookPath: func(string) (string, error) {
			return "/bin/true", nil
		},
	})
	if err == nil || artifact.Status != "error" {
		t.Fatalf("err = %v, artifact = %+v", err, artifact)
	}
}

func TestRunWorktreeFailure(t *testing.T) {
	root, sha := initRepo(t)
	spec := baseSpec(t, root, sha)
	spec.BaseRef = strings.Repeat("d", 40)
	artifact, err := Run(context.Background(), Options{
		Root: root,
		Spec: spec,
		LookPath: func(string) (string, error) {
			return "/bin/true", nil
		},
	})
	if err == nil || artifact.Status != "error" {
		t.Fatalf("err = %v, artifact = %+v", err, artifact)
	}
	if err := addWorktree(context.Background(), root, filepath.Join(t.TempDir(), "wt"), "nope"); err == nil {
		t.Fatal("addWorktree should reject a non-SHA ref")
	}
}

func TestChangedFilesError(t *testing.T) {
	if _, err := changedFiles(context.Background(), t.TempDir(), "HEAD"); err == nil {
		t.Fatal("changedFiles should fail outside a git repository")
	}
}

func TestRunSuiteUnknown(t *testing.T) {
	opts := Options{Spec: Spec{Suite: "missing"}}
	if _, ok := runSuite(context.Background(), opts, t.TempDir()); ok {
		t.Fatal("unknown suite should not resolve")
	}
}

func TestFinalizeRun(t *testing.T) {
	r := RunResult{Agent: CommandResult{ExitCode: 1}}
	finalizeRun(&r, Spec{})
	if r.Status != "failed" || r.Success {
		t.Fatalf("agent failure = %+v", r)
	}
	r = RunResult{Agent: CommandResult{TimedOut: true}}
	finalizeRun(&r, Spec{})
	if r.Status != "failed" {
		t.Fatalf("timeout = %+v", r)
	}
	r = RunResult{Agent: CommandResult{ExitCode: 0}, SuiteCommands: []CommandResult{{Passed: false}}}
	finalizeRun(&r, Spec{})
	if r.Status != "failed" {
		t.Fatalf("suite failure = %+v", r)
	}
}

func TestSubtractAndFirstNonEmpty(t *testing.T) {
	if got := strings.Join(subtract([]string{"a", "b", "c"}, []string{"b"}), ","); got != "a,c" {
		t.Fatalf("subtract = %q", got)
	}
	if got := firstNonEmpty("", "x", "y"); got != "x" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("firstNonEmpty empty = %q", got)
	}
}

func TestStripContextRemovesSkillsAndSkipsBuildDirs(t *testing.T) {
	wt := t.TempDir()
	writeRepoFile(t, wt, ".agents/skills/x/SKILL.md", "x")
	writeRepoFile(t, wt, "backend/AGENTS.md", "x")
	writeRepoFile(t, wt, "node_modules/pkg/AGENTS.md", "keep")
	writeRepoFile(t, wt, "AGENTS.md", "# Root\n\n## Project map\n\nx\n\n## Safety\n")
	stripped, err := stripContext(wt)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(stripped, ".agents/skills") || !contains(stripped, "backend/AGENTS.md") {
		t.Fatalf("stripped = %v", stripped)
	}
	if _, err := os.Stat(filepath.Join(wt, "node_modules/pkg/AGENTS.md")); err != nil {
		t.Fatal("build/dependency AGENTS.md should be left alone")
	}
}

func TestStripContextError(t *testing.T) {
	wt := t.TempDir()
	writeRepoFile(t, wt, ".agents/skills/x/SKILL.md", "x")
	agentsDir := filepath.Join(wt, ".agents")
	if err := os.Chmod(agentsDir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(agentsDir, 0o755) }()
	if _, err := stripContext(wt); err == nil {
		t.Fatal("unremovable skills tree should error")
	}
}

func TestRunShellCommand(t *testing.T) {
	dir := t.TempDir()
	res := runShellCommand(context.Background(), dir, "false", time.Second)
	if res.Passed || res.ExitCode != 1 {
		t.Fatalf("false = %+v", res)
	}
	res = runShellCommand(context.Background(), dir, "sleep 2", 10*time.Millisecond)
	if !res.TimedOut || res.Passed {
		t.Fatalf("timeout = %+v", res)
	}
	res = runShellCommand(context.Background(), filepath.Join(dir, "missing"), "true", time.Second)
	if res.ExitCode != -1 || res.Passed {
		t.Fatalf("bad dir = %+v", res)
	}
}

func TestLimitedBuffer(t *testing.T) {
	var b limitedBuffer
	if _, err := b.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if b.String() != "hello" {
		t.Fatalf("String = %q", b.String())
	}
	big := strings.Repeat("a", outputLimit+10)
	if _, err := b.Write([]byte(big)); err != nil {
		t.Fatal(err)
	}
	if len(b.String()) != outputLimit {
		t.Fatalf("buffer len = %d", len(b.String()))
	}
	if !strings.Contains(b.Tail(), "(truncated)") {
		t.Fatalf("Tail not truncated: %d", len(b.Tail()))
	}
	var small limitedBuffer
	_, _ = small.Write([]byte("short"))
	if small.Tail() != "short" {
		t.Fatalf("small tail = %q", small.Tail())
	}
}

func TestRunSeesCommittedAgentChanges(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := Run(context.Background(), Options{
		Root:   root,
		Spec:   baseSpec(t, root, sha),
		Suites: []Suite{{Name: "demo", Commands: []string{"true"}}},
		AgentCommand: `printf 'package x\n\nimport "testing"\n\nfunc TestX(t *testing.T) { t.Skip("x") }\n' > backend/x_test.go && ` +
			`git add -A && git -c user.name=A -c user.email=a@b.invalid -c commit.gpgsign=false commit -qm agent`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Context == nil || artifact.Status != "failed" {
		t.Fatalf("artifact = %+v", artifact)
	}
	if !artifact.Context.HeadMoved {
		t.Fatal("committed agent change was not detected as HEAD movement")
	}
	if !strings.Contains(strings.Join(artifact.Context.IntegrityViolations, "\n"), "added test skip: backend/x_test.go") {
		t.Fatalf("integrity findings = %v", artifact.Context.IntegrityViolations)
	}
	if len(artifact.Context.ChangedFiles) == 0 {
		t.Fatal("committed change was hidden from changed files")
	}
}

func TestRunIgnoresSeedPatchIntegrityFindings(t *testing.T) {
	root, _ := initRepo(t)
	writeRepoFile(t, root, "backend/seed_test.go", "package x\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "seed target")
	sha := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	spec := baseSpec(t, root, sha)
	writeRepoFile(t, spec.TaskDir, "seed.patch", strings.Join([]string{
		"diff --git a/backend/seed_test.go b/backend/seed_test.go",
		"--- a/backend/seed_test.go",
		"+++ b/backend/seed_test.go",
		"@@ -1,1 +1,2 @@",
		" package x",
		"+func TestSeed(t *testing.T) { t.Skip(\"seed\") }",
		"",
	}, "\n"))
	spec.SeedPatch = "seed.patch"
	artifact, err := Run(context.Background(), Options{
		Root:   root,
		Spec:   spec,
		Suites: []Suite{{Name: "demo", Commands: []string{"true"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "passed" || artifact.Context == nil || !artifact.Context.Success {
		t.Fatalf("artifact = %+v", artifact)
	}
	if len(artifact.Context.IntegrityViolations) != 0 {
		t.Fatalf("seed patch attributed to agent: %v", artifact.Context.IntegrityViolations)
	}
}

func TestRenamedFilesReportsRename(t *testing.T) {
	root, _ := initRepo(t)
	writeRepoFile(t, root, "backend/old_test.go", "package x\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "add test")
	base := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "mv", "backend/old_test.go", "backend/old.go")
	got, err := renamedFiles(context.Background(), root, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Old != "backend/old_test.go" || got[0].New != "backend/old.go" {
		t.Fatalf("renames = %+v", got)
	}
}

func TestRunWorktreeReportsIntegrityGitErrors(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"head lookup", "rev-parse", "rev-parse"},
		{"patch scan", "--unified=0", "integrity scan"},
		{"deleted files", "--diff-filter=D", "diff-filter=D"},
		{"renamed files", "--name-status", "name-status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, sha := initRepo(t)
			marker := filepath.Join(t.TempDir(), "marker")
			installGitShim(t, tt.token, "fail", marker)
			_, _, err := runWorktree(context.Background(), Options{
				Root: root,
				Spec: baseSpec(t, root, sha),
				CommandRunner: func(context.Context, string, string, time.Duration) CommandResult {
					touchMarker(t, marker)
					return CommandResult{Passed: true}
				},
			}, filepath.Join(t.TempDir(), "wt"), false, "true")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunWorktreeMarksTruncatedIntegrityScan(t *testing.T) {
	root, sha := initRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	installGitShim(t, "--unified=0", "huge", marker)
	result, _, err := runWorktree(context.Background(), Options{
		Root: root,
		Spec: baseSpec(t, root, sha),
		CommandRunner: func(context.Context, string, string, time.Duration) CommandResult {
			touchMarker(t, marker)
			return CommandResult{Passed: true}
		},
	}, filepath.Join(t.TempDir(), "wt"), false, "true")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(result.IntegrityViolations, "integrity scan truncated: diff is too large to verify") {
		t.Fatalf("violations = %v", result.IntegrityViolations)
	}
	if !contains(result.IntegrityWarnings, "integrity scan truncated at 16 MiB") {
		t.Fatalf("warnings = %v", result.IntegrityWarnings)
	}
}
