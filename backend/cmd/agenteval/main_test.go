package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/agenteval"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func fixtureRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", ".")
	git(t, root, "config", "user.email", "t@example.invalid")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "commit.gpgsign", "false")
	var makefile strings.Builder
	for _, art := range agenteval.GeneratedArtifacts {
		makefile.WriteString(art.Generator + ":\n\t@true\n")
	}
	writeFile(t, root, "backend/Makefile", makefile.String())
	writeFile(t, root, "AGENTS.md", "# Root\n\n## Project map\n\nx\n\n## Safety\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "init")
	sha := git(t, root, "rev-parse", "HEAD")
	spec := "name: demo\nprompt: do nothing\nbase_ref: " + sha + "\nsuite: release-readiness\ntimeout: 10s\nrequires: [go]\nagent_command: \"true\"\nexpect_paths: [\"**\"]\n"
	writeFile(t, root, "testdata/agent-eval/demo/spec.yaml", spec)
	return root, sha
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunFlagErrors(t *testing.T) {
	if code, _, _ := runCLI(t, "-h"); code != 0 {
		t.Errorf("-h = %d", code)
	}
	if code, _, _ := runCLI(t, "-bogus"); code != 2 {
		t.Errorf("unknown flag = %d", code)
	}
	if code, _, _ := runCLI(t, ""); code != 2 {
		t.Errorf("missing task = %d", code)
	}
}

func TestRunWritesArtifactAndFails(t *testing.T) {
	root, _ := fixtureRepo(t)
	out := t.TempDir()
	code, stdout, stderr := runCLI(t, "-root", root, "-task", "demo", "-out", out, "-no-control")
	if code != 1 {
		t.Fatalf("run = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Status: **failed**") || !strings.Contains(stdout, "Artifacts:") {
		t.Fatalf("stdout = %q", stdout)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("artifacts = %v", entries)
	}
	var foundJSON, foundMD bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			foundJSON = true
			body, err := os.ReadFile(filepath.Join(out, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var art agenteval.Artifact
			if err := json.Unmarshal(body, &art); err != nil {
				t.Fatal(err)
			}
			if art.Task != "demo" || art.Status != "failed" {
				t.Fatalf("artifact = %+v", art)
			}
		}
		if strings.HasSuffix(e.Name(), ".md") {
			foundMD = true
		}
	}
	if !foundJSON || !foundMD {
		t.Fatalf("missing artifact files: %v", entries)
	}
}

func TestRunWithDryRunProfile(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  dry-run:\n    command: \"true\"\n    name: dry-run\n    model: none\n    version: \"0\"\n    dry_run: true\n")
	out := t.TempDir()
	code, stdout, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", sha, "-profile", "dry-run", "-out", out, "-no-control")
	if code != 1 {
		t.Fatalf("run = %d, stderr %q stdout %q", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "- Profile: `dry-run`") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunBaselineRequiresProfile(t *testing.T) {
	root, _ := fixtureRepo(t)
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-baseline", "testdata/agent-eval/baseline.json")
	if code != 2 || !strings.Contains(stderr, "require -profile") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunWritesAndComparesBaseline(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  pinned:\n    command: \": {prompt}\"\n    name: pinned\n    model: test-model\n    version: \"1.0.0\"\n")
	out := t.TempDir()
	args := []string{"-root", root, "-task", "demo", "-base-ref", sha, "-profile", "pinned", "-out", out, "-no-control"}
	if code, _, stderr := runCLI(t, append(args, "-write-baseline", "testdata/agent-eval/baseline.json")...); code != 1 || !strings.Contains(stderr, "refusing to write") {
		t.Fatalf("unguarded write = %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "testdata/agent-eval/baseline.json")); !os.IsNotExist(err) {
		t.Fatalf("baseline written from a failing run: %v", err)
	}
	if code, _, stderr := runCLI(t, append(args, "-write-baseline", "testdata/agent-eval/baseline.json", "-force-baseline")...); code != 1 {
		t.Fatalf("forced write = %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "testdata/agent-eval/baseline.json")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, append(args, "-baseline", "testdata/agent-eval/baseline.json")...)
	if code != 1 {
		t.Fatalf("compare run = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Baseline: success=") {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestRunRejectsDryRunBaseline(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  dry-run:\n    command: \"true\"\n    name: dry-run\n    model: none\n    version: \"0\"\n    dry_run: true\n")
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", sha, "-profile", "dry-run", "-baseline", "testdata/agent-eval/baseline.json")
	if code != 2 || !strings.Contains(stderr, "dry-run profiles") {
		t.Fatalf("dry-run baseline = %d, stderr %q", code, stderr)
	}
}

func TestRunRejectsProfileOverrides(t *testing.T) {
	root, _ := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  dry-run:\n    command: \"true\"\n    name: dry-run\n    model: none\n    version: \"0\"\n    dry_run: true\n")
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-profile", "dry-run", "-agent-model", "other")
	if code != 2 || !strings.Contains(stderr, "cannot be combined") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunRejectsUnknownProfile(t *testing.T) {
	root, _ := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles: {}\n")
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-profile", "missing")
	if code != 1 || !strings.Contains(stderr, "not defined") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunMissingTask(t *testing.T) {
	root, _ := fixtureRepo(t)
	code, _, stderr := runCLI(t, "-root", root, "-task", "missing")
	if code != 1 || !strings.Contains(stderr, "read task spec") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunRejectsBranchBaseRef(t *testing.T) {
	root, _ := fixtureRepo(t)
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", "main")
	if code != 2 || !strings.Contains(stderr, "full commit SHA") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunRejectsMissingGeneratorTarget(t *testing.T) {
	root, _ := fixtureRepo(t)
	writeFile(t, root, "backend/Makefile", "gen-contract-fixtures:\n\t@true\n")
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo")
	if code != 1 || !strings.Contains(stderr, "missing Makefile target") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestWriteArtifactRelativeOut(t *testing.T) {
	root := t.TempDir()
	art := agenteval.Artifact{Task: "demo", Status: "skipped", Reason: "test"}
	jsonPath, mdPath, err := writeArtifact(root, "out", art)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(jsonPath, root) || !strings.HasPrefix(mdPath, root) {
		t.Fatalf("paths = %q %q", jsonPath, mdPath)
	}
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatal(err)
	}
}

func TestRunRootDiscoveryFailure(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "-task", "demo")
	if code != 1 || !strings.Contains(stderr, "could not locate repository root") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunVerifyGeneratorError(t *testing.T) {
	root := t.TempDir()
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo")
	if code != 1 || !strings.Contains(stderr, "agenteval:") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunBaseRefOverride(t *testing.T) {
	root, sha := fixtureRepo(t)
	out := t.TempDir()
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-out", out, "-no-control", "-base-ref", sha)
	if code != 1 {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunInvalidButWellFormedBaseRef(t *testing.T) {
	root, _ := fixtureRepo(t)
	out := t.TempDir()
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-out", out, "-no-control", "-base-ref", strings.Repeat("d", 40))
	if code != 1 || !strings.Contains(stderr, "git worktree add") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunWriteArtifactError(t *testing.T) {
	root, _ := fixtureRepo(t)
	file := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-out", filepath.Join(file, "sub"), "-no-control")
	if code != 1 || !strings.Contains(stderr, "write artifact") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestWriteArtifactWriteFailures(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "out")
	if err := os.MkdirAll(out, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(out, 0o755) }()
	if _, _, err := writeArtifact(root, out, agenteval.Artifact{Task: "demo"}); err == nil {
		t.Fatal("read-only output directory should fail")
	}

	out2 := t.TempDir()
	art := agenteval.Artifact{Task: "demo"}
	base := "demo-00010101T000000Z"
	if err := os.MkdirAll(filepath.Join(out2, base+".md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writeArtifact(root, out2, art); err == nil {
		t.Fatal("markdown path that is a directory should fail")
	}
}

func TestRunRequireBaselineNeedsBaseline(t *testing.T) {
	root, _ := fixtureRepo(t)
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-require-baseline")
	if code != 2 || !strings.Contains(stderr, "requires -baseline") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunBaselineLoadError(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  pinned:\n    command: \": {prompt}\"\n    name: pinned\n    model: test-model\n    version: \"1.0.0\"\n")
	writeFile(t, root, "testdata/agent-eval/baseline.json", "{")
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", sha, "-profile", "pinned",
		"-baseline", "testdata/agent-eval/baseline.json", "-out", t.TempDir(), "-no-control")
	if code != 1 || !strings.Contains(stderr, "parse baseline") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunBaselineRegression(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  pinned:\n    command: \": {prompt}\"\n    name: pinned\n    model: test-model\n    version: \"1.0.0\"\n")
	writeFile(t, root, "testdata/agent-eval/baseline.json", `{"version":1,"profiles":{"pinned":{"demo":{"status":"passed","success":true,"commands_passed":3,"commands_total":3}}}}`)
	code, stdout, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", sha, "-profile", "pinned",
		"-baseline", "testdata/agent-eval/baseline.json", "-out", t.TempDir(), "-no-control")
	if code != 1 || !strings.Contains(stdout, "Regression: task passed before; now failed") {
		t.Fatalf("run = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestRunWriteBaselineLoadError(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  pinned:\n    command: \": {prompt}\"\n    name: pinned\n    model: test-model\n    version: \"1.0.0\"\n")
	writeFile(t, root, "testdata/agent-eval/baseline.json", "{")
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", sha, "-profile", "pinned",
		"-write-baseline", "testdata/agent-eval/baseline.json", "-force-baseline", "-out", t.TempDir(), "-no-control")
	if code != 1 || !strings.Contains(stderr, "parse baseline") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestRunWriteBaselineWriteError(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  pinned:\n    command: \": {prompt}\"\n    name: pinned\n    model: test-model\n    version: \"1.0.0\"\n")
	if err := os.MkdirAll(filepath.Join(root, "testdata", "agent-eval", "baseline.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", sha, "-profile", "pinned",
		"-write-baseline", "testdata/agent-eval/baseline.json", "-force-baseline", "-out", t.TempDir(), "-no-control")
	if code != 1 || !strings.Contains(stderr, "write baseline") {
		t.Fatalf("run = %d, stderr = %q", code, stderr)
	}
}

func TestResolvePath(t *testing.T) {
	if got := resolvePath("/root", "/abs/baseline.json"); got != "/abs/baseline.json" {
		t.Fatalf("absolute path = %q", got)
	}
	if got := resolvePath("/root", "testdata/baseline.json"); got != filepath.Join("/root", "testdata", "baseline.json") {
		t.Fatalf("relative path = %q", got)
	}
}

func TestRunRequiresBaselineEntry(t *testing.T) {
	root, sha := fixtureRepo(t)
	writeFile(t, root, "testdata/agent-eval/agents.yaml", "version: 1\nprofiles:\n  pinned:\n    command: \": {prompt}\"\n    name: pinned\n    model: test-model\n    version: \"1.0.0\"\n")
	code, _, stderr := runCLI(t, "-root", root, "-task", "demo", "-base-ref", sha, "-profile", "pinned",
		"-baseline", "testdata/agent-eval/baseline.json", "-require-baseline", "-no-control")
	if code != 1 || !strings.Contains(stderr, "no committed baseline entry") {
		t.Fatalf("run = %d, stderr %q", code, stderr)
	}
}
