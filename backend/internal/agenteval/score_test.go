package agenteval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMatchPath(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"backend/database/**", "backend/database/migrations/001.up.sql", true},
		{"backend/database/**", "backend/database/", true},
		{"backend/**/*_test.go", "backend/models/contact_test.go", true},
		{"backend/openapi*_test.go", "backend/openapi_request_test.go", true},
		{"backend/openapi*_test.go", "backend/openapi/sub_test.go", false},
		{"frontend/src/utils/graphBudget.ts", "frontend/src/utils/graphBudget.ts", true},
		{"frontend/**", "backend/AGENTS.md", false},
	}
	for _, c := range cases {
		if got := MatchPath(c.pattern, c.path); got != c.want {
			t.Errorf("MatchPath(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestClassifyPaths(t *testing.T) {
	changed := []string{
		"backend/database/migrations/078.up.sql",
		"backend/models/contact.go",
		"frontend/src/App.tsx",
		"README.md",
	}
	got := ClassifyPaths(changed, []string{"backend/database/**", "backend/models/**"}, []string{"README.md"})
	want := []string{
		"forbidden path: README.md",
		"outside expected paths: frontend/src/App.tsx",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ClassifyPaths() = %v, want %v", got, want)
	}
	if got := ClassifyPaths(nil, []string{"backend/**"}, nil); len(got) != 0 {
		t.Fatalf("empty change set should pass: %v", got)
	}
}

func TestDetectHandEdits(t *testing.T) {
	arts := []GeneratedArtifact{
		{Path: "frontend/src/generated/openapi.ts", Generator: "gen-ts-api", Sources: []string{"backend/openapi.yaml"}},
		{Path: "backend/testdata/contract-fixtures/", Generator: "gen-contract-fixtures", Sources: []string{"backend/openapi.yaml"}},
	}
	if got := DetectHandEdits([]string{"frontend/src/generated/openapi.ts"}, arts); len(got) != 1 {
		t.Fatalf("generated-only change = %v", got)
	}
	if got := DetectHandEdits([]string{"frontend/src/generated/openapi.ts", "backend/openapi.yaml"}, arts); len(got) != 0 {
		t.Fatalf("regenerated change flagged: %v", got)
	}
	if got := DetectHandEdits([]string{"backend/testdata/contract-fixtures/getContact.json", "backend/openapi.yaml"}, arts); len(got) != 0 {
		t.Fatalf("contract fixtures flagged: %v", got)
	}
	if got := DetectHandEdits([]string{"backend/models/contact.go"}, arts); len(got) != 0 {
		t.Fatalf("unrelated change flagged: %v", got)
	}
}

func TestVerifyGeneratorTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "backend", "Makefile"), []byte("gen-one:\n\t@true\ngen-two:\n\t@true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing, err := VerifyGeneratorTargets(root, []GeneratedArtifact{{Generator: "gen-one"}, {Generator: "gen-missing"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(missing, ",") != "gen-missing" {
		t.Fatalf("missing = %v", missing)
	}
	if _, err := VerifyGeneratorTargets(t.TempDir(), nil); err == nil {
		t.Fatal("missing Makefile should error")
	}
}

func TestFindingsSatisfied(t *testing.T) {
	ok, missing := FindingsSatisfied([]string{"ASVS/MASVS", "schema fixture"}, "Missing ASVS/MASVS row; add a schema fixture.")
	if !ok || len(missing) != 0 {
		t.Fatalf("findings not matched: %v", missing)
	}
	ok, missing = FindingsSatisfied([]string{"manual gate"}, "nothing")
	if ok || len(missing) != 1 || missing[0] != "manual gate" {
		t.Fatalf("missing finding not reported: %v", missing)
	}
}

func TestScore(t *testing.T) {
	run := RunResult{Status: "passed", SuiteCommands: []CommandResult{{Passed: true}}}
	Score(&run)
	if !run.Success {
		t.Fatal("passing run should score")
	}
	run.SuiteCommands[0].Passed = false
	Score(&run)
	if run.Success {
		t.Fatal("failing suite command should not score")
	}
}

func TestCompare(t *testing.T) {
	ctx := &RunResult{Success: true, SuiteCommands: []CommandResult{{Passed: true}}, ChangedFiles: []string{"a"}}
	ctrl := &RunResult{Success: false, SuiteCommands: []CommandResult{{Passed: false}}, ScopeViolations: []string{"x"}}
	d := Compare(ctx, ctrl)
	if !d.ContextSuccess || d.ControlSuccess {
		t.Fatalf("success flags = %+v", d)
	}
	if d.CommandPassDelta != 1 || d.ScopeViolationDelta != -1 || d.ChangedFileDelta != 1 {
		t.Fatalf("delta = %+v", d)
	}
	if d.Summary != "context passed; control failed" {
		t.Fatalf("summary = %q", d.Summary)
	}
	both := Compare(&RunResult{Success: true}, &RunResult{Success: true})
	if both.Summary != "both passed" {
		t.Fatalf("both summary = %q", both.Summary)
	}
	neither := Compare(&RunResult{}, &RunResult{})
	if neither.Summary != "both failed" {
		t.Fatalf("neither summary = %q", neither.Summary)
	}
	if got := Compare(nil, nil); got.Summary != "both failed" {
		t.Fatalf("nil summary = %q", got.Summary)
	}
}

func TestMarkdown(t *testing.T) {
	a := Artifact{
		Task: "demo", Status: "passed", BaseRef: sha, AgentName: "a", AgentModel: "m", AgentVersion: "1", AgentCommand: "true", DurationMS: 5,
		Context:  &RunResult{Status: "passed", Success: true, SuiteCommands: []CommandResult{{Command: "true", Passed: true}}, ScopeViolations: []string{"v"}, HandEditedGenerated: []string{"h"}, MissingFindings: []string{"f"}},
		Control:  &RunResult{Status: "passed", Success: true},
		Delta:    &ControlDelta{Summary: "both passed"},
		Stripped: []string{".agents/skills"},
	}
	md := Markdown(a)
	for _, want := range []string{"# Agent eval: demo", "Context run", "Control run", "Scope: v", "Generated: h", "Missing finding: f", "Stripped for control"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	if a.Delta == nil || a.Delta.Summary != "both passed" {
		t.Fatal("delta")
	}
	if !strings.HasPrefix(Markdown(Artifact{Task: "x"}), "# Agent eval: x") {
		t.Fatal("minimal markdown")
	}
	_ = time.Second
}

func TestScoreScopeAndFindings(t *testing.T) {
	run := RunResult{Status: "passed", ScopeViolations: []string{"x"}}
	Score(&run)
	if run.Success {
		t.Fatal("scope violation should not score")
	}
	run = RunResult{Status: "passed", HandEditedGenerated: []string{"x"}}
	Score(&run)
	if run.Success {
		t.Fatal("hand edit should not score")
	}
	run = RunResult{Status: "passed", MissingFindings: []string{"x"}}
	Score(&run)
	if run.Success {
		t.Fatal("missing finding should not score")
	}
}

func TestCompareControlOnlyPassed(t *testing.T) {
	d := Compare(&RunResult{Success: false}, &RunResult{Success: true})
	if d.Summary != "control passed; context failed" {
		t.Fatalf("summary = %q", d.Summary)
	}
}

func TestMarkdownReason(t *testing.T) {
	md := Markdown(Artifact{Task: "x", Status: "skipped", Reason: "missing go"})
	if !strings.Contains(md, "Reason: missing go") {
		t.Fatalf("markdown = %s", md)
	}
}

func TestMatchPathQuestionMark(t *testing.T) {
	if !MatchPath("backend/v?/x.go", "backend/v1/x.go") {
		t.Fatal("? should match one non-slash character")
	}
	if MatchPath("backend/v?/x.go", "backend/v12/x.go") {
		t.Fatal("? must match exactly one character")
	}
}

func TestFindingsSatisfiedAny(t *testing.T) {
	ok, missing := FindingsSatisfiedAny([]string{"data loss", "lose data"}, "Dropping the column would lose data for every contact.")
	if !ok || len(missing) != 0 {
		t.Fatalf("ok=%v missing=%v", ok, missing)
	}
	ok, missing = FindingsSatisfiedAny([]string{"deprecation", "breaking change"}, "I implemented the change.")
	if ok || len(missing) != 1 {
		t.Fatalf("ok=%v missing=%v", ok, missing)
	}
}
