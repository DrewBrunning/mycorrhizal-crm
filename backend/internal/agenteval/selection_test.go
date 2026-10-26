package agenteval

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadTriggerSet(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agents/skills/demo/SKILL.md", "---\nname: demo\n---\n")
	writeRepoFile(t, root, ".agents/skills/other/SKILL.md", "---\nname: other\n---\n")
	writeRepoFile(t, root, "backend/AGENTS.md", "# Backend\n")
	writeRepoFile(t, root, "testdata/agent-eval/triggers.yaml", strings.Join([]string{
		"version: 1",
		"ambiguous_pairs:",
		"  - skills: [demo, other]",
		"  - skill: demo",
		"    context_file: backend/AGENTS.md",
		"prompts:",
		"  - prompt: do demo",
		"    expect: demo",
		"    not: [other]",
	}, "\n"))

	set, err := LoadTriggerSet(root, "testdata/agent-eval/triggers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if set.Version != 1 || len(set.Prompts) != 1 || set.Prompts[0].Expect != "demo" {
		t.Fatalf("set = %+v", set)
	}
	if len(set.AmbiguousPairs) != 2 {
		t.Fatalf("pairs = %+v", set.AmbiguousPairs)
	}
}

func TestLoadTriggerSetRejectsInvalid(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agents/skills/demo/SKILL.md", "---\nname: demo\n---\n")
	writeRepoFile(t, root, "backend/AGENTS.md", "# Backend\n")
	path := "testdata/agent-eval/triggers.yaml"

	cases := []struct {
		name string
		body string
		want string
	}{
		{"bad version", "version: 0\nprompts:\n  - prompt: x\n    expect: demo\n", "version"},
		{"no prompts", "version: 1\nprompts: []\n", "no prompts"},
		{"empty prompt", "version: 1\nprompts:\n  - prompt: ' '\n    expect: demo\n", "empty"},
		{"unknown skill", "version: 1\nprompts:\n  - prompt: x\n    expect: missing\n", "missing"},
		{"expect in not", "version: 1\nprompts:\n  - prompt: x\n    expect: demo\n    not: [demo]\n", "non-match"},
		{"unknown not", "version: 1\nprompts:\n  - prompt: x\n    expect: demo\n    not: [missing]\n", "missing"},
		{"duplicate prompt", "version: 1\nprompts:\n  - prompt: x\n    expect: demo\n  - prompt: x\n    expect: demo\n", "duplicate"},
		{"missing context file", "version: 1\nambiguous_pairs:\n  - skill: demo\n    context_file: backend/MISSING.md\nprompts:\n  - prompt: x\n    expect: demo\n", "MISSING"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			writeRepoFile(t, root, path, c.body)
			_, err := LoadTriggerSet(root, path)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}

func TestLoadRepositoryTriggerSet(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTriggerSet(root, "testdata/agent-eval/triggers.yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeSelection(t *testing.T) {
	skills, contextFiles, unknown := NormalizeSelection(SelectionOutput{
		Skills: []string{
			".agents/skills/demo/SKILL.md",
			"demo",
			"frontend/src/App.tsx",
			"../escape",
		},
		ContextFiles: []string{"backend/AGENTS.md", ".agents/skills/other/SKILL.md"},
	}, map[string]bool{"demo": true, "other": true})
	if strings.Join(skills, ",") != "demo,other" {
		t.Fatalf("skills = %v", skills)
	}
	if strings.Join(contextFiles, ",") != "backend/AGENTS.md" {
		t.Fatalf("context files = %v", contextFiles)
	}
	if len(unknown) != 2 {
		t.Fatalf("unknown = %v", unknown)
	}
}

func TestScoreSelectionRejectsExtraAndUnknown(t *testing.T) {
	out := SelectionOutcome{Expect: "demo"}
	out.Skills = []string{"demo", "unrelated"}
	out.Unknown = []string{"not-a-skill"}
	ScoreSelection(&out, nil)
	if len(out.ExtraSkills) != 1 || out.ExtraSkills[0] != "unrelated" {
		t.Fatalf("extra = %v", out.ExtraSkills)
	}
	if !strings.Contains(out.Reason, "extra skill") {
		t.Fatalf("reason = %q", out.Reason)
	}

	unknown := SelectionOutcome{Expect: "demo", Skills: []string{"demo"}, Unknown: []string{"made-up"}}
	ScoreSelection(&unknown, nil)
	if !strings.Contains(unknown.Reason, "unknown") {
		t.Fatalf("reason = %q", unknown.Reason)
	}
}

func TestScoreSelection(t *testing.T) {
	pairs := []TriggerPair{{Skills: []string{"demo", "other"}}, {Skill: "demo", ContextFile: "backend/AGENTS.md"}}
	out := SelectionOutcome{Expect: "demo", Not: []string{"other"}}
	out.Skills = []string{"demo", "other"}
	out.ContextFiles = []string{"backend/AGENTS.md"}
	out.RepoChanges = []string{"backend/x.go"}
	ScoreSelection(&out, pairs)
	if !out.Matched {
		t.Fatal("expected match")
	}
	if len(out.Unexpected) != 1 || out.Unexpected[0] != "other" {
		t.Fatalf("unexpected = %v", out.Unexpected)
	}
	if len(out.Ambiguous) != 2 {
		t.Fatalf("ambiguous = %v", out.Ambiguous)
	}
	if !strings.Contains(out.Reason, "non-matching") {
		t.Fatalf("reason = %q", out.Reason)
	}
}

func TestRunSelectionPasses(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := RunSelection(context.Background(), SelectionOptions{
		Root:         root,
		BaseRef:      sha,
		TriggersPath: "testdata/agent-eval/triggers.yaml",
		Set: TriggerSet{Version: 1, Prompts: []TriggerPrompt{{
			Prompt: "do demo",
			Expect: "demo",
			Not:    []string{"other"},
		}}},
		AgentCommand: "true",
		CommandRunner: func(_ context.Context, dir, _ string, _ time.Duration) CommandResult {
			writeRepoFile(t, dir, selectionReportFile, `{"skills":[".agents/skills/demo/SKILL.md"]}`)
			return CommandResult{Passed: true}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "passed" || artifact.Passed != 1 || artifact.Total != 1 {
		t.Fatalf("artifact = %+v", artifact)
	}
	if !artifact.Outcomes[0].Matched {
		t.Fatalf("outcome = %+v", artifact.Outcomes[0])
	}
	if got := strings.TrimSpace(runGit(t, root, "status", "--porcelain")); got != "" {
		t.Fatalf("caller's tree is dirty: %s", got)
	}
}

func TestRunSelectionFailsWithoutReport(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := RunSelection(context.Background(), SelectionOptions{
		Root:         root,
		BaseRef:      sha,
		Set:          TriggerSet{Version: 1, Prompts: []TriggerPrompt{{Prompt: "do demo", Expect: "demo"}}},
		AgentCommand: "true",
		CommandRunner: func(context.Context, string, string, time.Duration) CommandResult {
			return CommandResult{Passed: true}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || !strings.Contains(artifact.Outcomes[0].Reason, "did not write") {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestRunSelectionFailsOnRepoChanges(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := RunSelection(context.Background(), SelectionOptions{
		Root:         root,
		BaseRef:      sha,
		Set:          TriggerSet{Version: 1, Prompts: []TriggerPrompt{{Prompt: "do demo", Expect: "demo"}}},
		AgentCommand: "true",
		CommandRunner: func(_ context.Context, dir, _ string, _ time.Duration) CommandResult {
			writeRepoFile(t, dir, selectionReportFile, `{"skills":["demo"]}`)
			writeRepoFile(t, dir, "backend/changed.txt", "x\n")
			return CommandResult{Passed: true}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || len(artifact.Outcomes[0].RepoChanges) != 1 {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestRunSelectionFailsOnWrongSkill(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := RunSelection(context.Background(), SelectionOptions{
		Root:         root,
		BaseRef:      sha,
		Set:          TriggerSet{Version: 1, Prompts: []TriggerPrompt{{Prompt: "do demo", Expect: "demo", Not: []string{"other"}}}},
		AgentCommand: "true",
		CommandRunner: func(_ context.Context, dir, _ string, _ time.Duration) CommandResult {
			writeRepoFile(t, dir, selectionReportFile, `{"skills":["other"]}`)
			return CommandResult{Passed: true}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || artifact.Outcomes[0].Matched {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestResolveCommit(t *testing.T) {
	root, sha := initRepo(t)
	got, err := ResolveCommit(root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got != sha {
		t.Fatalf("ResolveCommit = %q, want %q", got, sha)
	}
	for _, ref := range []string{"--upload-pack=evil", "HEAD with space"} {
		if _, err := ResolveCommit(root, ref); err == nil {
			t.Fatalf("ResolveCommit(%q) accepted", ref)
		}
	}
	if _, err := ResolveCommit(root, "missing"); err == nil {
		t.Fatal("ResolveCommit(missing) accepted")
	}
}

func TestRunSelectionRejectsMissingCommandAndBadRef(t *testing.T) {
	root, sha := initRepo(t)
	if _, err := RunSelection(context.Background(), SelectionOptions{Root: root, BaseRef: sha, Set: TriggerSet{Version: 1, Prompts: []TriggerPrompt{{Prompt: "x", Expect: "demo"}}}}); err == nil {
		t.Fatal("missing agent command accepted")
	}
	art, err := RunSelection(context.Background(), SelectionOptions{
		Root:         root,
		BaseRef:      "main",
		Set:          TriggerSet{Version: 1, Prompts: []TriggerPrompt{{Prompt: "x", Expect: "demo"}}},
		AgentCommand: "true",
	})
	if err == nil || art.Status != "error" {
		t.Fatalf("bad ref: err=%v artifact=%+v", err, art)
	}
}

func TestRunSelectionFailsOnCommittedRepoChanges(t *testing.T) {
	root, sha := initRepo(t)
	artifact, err := RunSelection(context.Background(), SelectionOptions{
		Root:         root,
		BaseRef:      sha,
		Set:          TriggerSet{Version: 1, Prompts: []TriggerPrompt{{Prompt: "do demo", Expect: "demo"}}},
		AgentCommand: "true",
		CommandRunner: func(_ context.Context, dir, _ string, _ time.Duration) CommandResult {
			writeRepoFile(t, dir, selectionReportFile, `{"skills":["demo"]}`)
			writeRepoFile(t, dir, "backend/changed.txt", "x\n")
			runGit(t, dir, "add", ".")
			runGit(t, dir, "-c", "user.name=A", "-c", "user.email=a@b.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "agent")
			return CommandResult{Passed: true}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || len(artifact.Outcomes[0].RepoChanges) == 0 {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestRunSelectionRejectsBaseWithoutExpectedSkill(t *testing.T) {
	root, _ := initRepo(t)
	runGit(t, root, "rm", "-r", ".agents/skills/demo")
	runGit(t, root, "commit", "-q", "-m", "drop skill")
	sha := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	artifact, err := RunSelection(context.Background(), SelectionOptions{
		Root:         root,
		BaseRef:      sha,
		Set:          TriggerSet{Version: 1, Prompts: []TriggerPrompt{{Prompt: "do demo", Expect: "demo"}}},
		AgentCommand: "true",
		CommandRunner: func(context.Context, string, string, time.Duration) CommandResult {
			return CommandResult{Passed: true}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || !strings.Contains(artifact.Outcomes[0].Reason, "base ref does not contain") {
		t.Fatalf("artifact = %+v", artifact)
	}
}
