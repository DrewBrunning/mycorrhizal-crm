package agenteval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/agentcontext"
)

const sha = "22ac64d049f07f2eaac134a4d195b8a126f00a7c"

func writeTaskFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseSpecDefaultsTimeout(t *testing.T) {
	dir := t.TempDir()
	spec, err := parseSpec([]byte("name: demo\nprompt: do it\nbase_ref: "+sha+"\nsuite: database-migration\nagent_command: \"true\"\nexpect_paths: [backend/**]\n"), dir, "demo", []string{"database-migration"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.TimeoutValue != DefaultTimeout {
		t.Fatalf("timeout = %v, want %v", spec.TimeoutValue, DefaultTimeout)
	}
	if spec.TaskDir != dir {
		t.Fatalf("TaskDir = %q", spec.TaskDir)
	}
}

func TestParseSpecReadsTaskMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeTaskFile(t, dir, "task.md", "  fix the thing\n")
	spec, err := parseSpec([]byte("name: demo\nbase_ref: "+sha+"\nsuite: database-migration\nagent_command: \"true\"\nexpect_paths: [backend/**]\n"), dir, "demo", []string{"database-migration"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Prompt != "fix the thing" {
		t.Fatalf("prompt = %q", spec.Prompt)
	}
}

func TestParseSpecErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing name", "prompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\nexpect_paths: [backend/**]\n", "spec.name is required"},
		{"name mismatch", "name: other\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\nexpect_paths: [backend/**]\n", "must equal the task directory"},
		{"missing prompt", "name: demo\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\nexpect_paths: [backend/**]\n", "task.md is unreadable"},
		{"branch base", "name: demo\nprompt: x\nbase_ref: main\nsuite: database-migration\nagent_command: true\nexpect_paths: [backend/**]\n", "full commit SHA"},
		{"missing suite", "name: demo\nprompt: x\nbase_ref: " + sha + "\nagent_command: true\nexpect_paths: [backend/**]\n", "spec.suite is required"},
		{"unknown suite", "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: nope\nagent_command: true\nexpect_paths: [backend/**]\n", "not in internal/agentcontext.Suites"},
		{"missing expect", "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\n", "expect_paths must name"},
		{"bad glob", "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\nexpect_paths: ['[']\n", "invalid path glob"},
		{"bad timeout", "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\ntimeout: nope\nagent_command: true\nexpect_paths: [backend/**]\n", "not a positive Go duration"},
		{"bad require", "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nrequires: [rust]\nagent_command: true\nexpect_paths: [backend/**]\n", "go/node/docker/gh"},
		{"missing agent", "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nexpect_paths: [backend/**]\n", "agent_command is required"},
		{"bad yaml", "name: [", "parse spec.yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSpec([]byte(tc.body), t.TempDir(), "demo", []string{"database-migration"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseSpecAllowsNoChangeWithoutExpectPaths(t *testing.T) {
	dir := t.TempDir()
	body := "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\nexpect_no_change: true\n"
	spec, err := parseSpec([]byte(body), dir, "demo", []string{"database-migration"})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.ExpectNoChange {
		t.Fatal("expect_no_change not parsed")
	}
}

func TestParseSpecChecksSeedAndControlFiles(t *testing.T) {
	dir := t.TempDir()
	base := "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\nexpect_paths: [backend/**]\n"
	if _, err := parseSpec([]byte(base+"seed_patch: seed.patch\n"), dir, "demo", []string{"database-migration"}); err == nil || !strings.Contains(err.Error(), "seed_patch") {
		t.Fatalf("missing seed patch not reported: %v", err)
	}
	writeTaskFile(t, dir, "seed.patch", "patch")
	if _, err := parseSpec([]byte(base+"seed_patch: seed.patch\n"), dir, "demo", []string{"database-migration"}); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSpec([]byte(base+"control: missing\n"), dir, "demo", []string{"database-migration"}); err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("missing control not reported: %v", err)
	}
}

func TestMissingRequires(t *testing.T) {
	spec := Spec{Requires: []string{"go", "docker"}}
	missing := MissingRequires(spec, func(name string) (string, error) {
		if name == "go" {
			return "/usr/bin/go", nil
		}
		return "", os.ErrNotExist
	})
	if strings.Join(missing, ",") != "docker" {
		t.Fatalf("missing = %v", missing)
	}
}

func TestExpandPromptQuotesThePrompt(t *testing.T) {
	got := ExpandPrompt("agent run {prompt}", "it's a task; rm -rf /")
	want := "agent run 'it'\\''s a task; rm -rf /'"
	if got != want {
		t.Fatalf("ExpandPrompt() = %q, want %q", got, want)
	}
	if got := ExpandPrompt("true", "ignored"); got != "true" {
		t.Fatalf("command without placeholder = %q", got)
	}
}

func TestRedactCommand(t *testing.T) {
	cases := map[string]string{
		"agent --api-key sk-secret":                "agent --api-key [redacted]",
		"agent --token=abc":                        "agent --token=[redacted]",
		"AGENT_API_KEY=abc agent run":              "AGENT_API_KEY=[redacted] agent run",
		"agent -H 'Authorization: Bearer abc.def'": "agent -H 'Authorization: Bearer [redacted]'",
	}
	for in, want := range cases {
		if got := RedactCommand(in); got != want {
			t.Errorf("RedactCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateBaseRef(t *testing.T) {
	for _, ok := range []string{sha, strings.Repeat("a", 64)} {
		if err := ValidateBaseRef(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"main", "v1.0.0", "abc", "22ac64d", strings.Repeat("z", 40)} {
		if err := ValidateBaseRef(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParseSpecParsesTimeout(t *testing.T) {
	dir := t.TempDir()
	spec, err := parseSpec([]byte("name: demo\nprompt: x\nbase_ref: "+sha+"\nsuite: database-migration\ntimeout: 90s\nagent_command: true\nexpect_paths: [backend/**]\n"), dir, "demo", []string{"database-migration"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.TimeoutValue != 90*time.Second {
		t.Fatalf("timeout = %v", spec.TimeoutValue)
	}
}

func TestLoadTaskErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := LoadTask(root, "testdata/agent-eval", "missing", []string{"database-migration"}); err == nil || !strings.Contains(err.Error(), "read task spec") {
		t.Fatalf("missing task = %v", err)
	}
}

func TestParseSpecEmptyPromptFile(t *testing.T) {
	dir := t.TempDir()
	writeTaskFile(t, dir, "task.md", "   \n")
	_, err := parseSpec([]byte("name: demo\nbase_ref: "+sha+"\nsuite: database-migration\nagent_command: true\nexpect_paths: [backend/**]\n"), dir, "demo", []string{"database-migration"})
	if err == nil || !strings.Contains(err.Error(), "prompt or task.md is required") {
		t.Fatalf("empty task.md = %v", err)
	}
}

func TestCommittedTasksLoad(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(agentcontext.Suites()))
	for _, s := range agentcontext.Suites() {
		names = append(names, s.Name)
	}
	entries, err := os.ReadDir(filepath.Join(root, "testdata", "agent-eval"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "controls" {
			continue
		}
		if _, err := LoadTask(root, "testdata/agent-eval", e.Name(), names); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
		}
	}
}

func TestRepositoryTasksCoverEverySuite(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	suites := agentcontext.Suites()
	names := make([]string, 0, len(suites))
	for _, s := range suites {
		names = append(names, s.Name)
	}
	entries, err := os.ReadDir(filepath.Join(root, "testdata", "agent-eval"))
	if err != nil {
		t.Fatal(err)
	}
	covered := map[string]bool{}
	tasks := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "testdata", "agent-eval", e.Name(), "spec.yaml")); err != nil {
			continue
		}
		spec, err := LoadTask(root, "testdata/agent-eval", e.Name(), names)
		if err != nil {
			t.Errorf("%s: %v", e.Name(), err)
			continue
		}
		tasks++
		covered[spec.Suite] = true
	}
	if tasks == 0 {
		t.Fatal("no eval tasks found")
	}
	for _, s := range suites {
		if !covered[s.Name] {
			t.Errorf("suite %q has no eval task", s.Name)
		}
	}
}
