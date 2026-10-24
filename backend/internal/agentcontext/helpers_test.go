package agentcontext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsResolvablePathToken(t *testing.T) {
	cases := []struct {
		tok  string
		want bool
	}{
		{"", false},
		{"/api/v1/contacts", false},
		{"-flag", false},
		{"backend/.gremlins/*.yaml", false},
		{"http://example.com/x.go", false},
		{"user_id", false},
		{"lib/typescript.js", false},
		{"backend/AGENTS.md", true},
		{"docs/", true},
		{"./backend/AGENTS.md", true},
	}
	for _, c := range cases {
		if got := isResolvablePathToken(c.tok); got != c.want {
			t.Errorf("isResolvablePathToken(%q) = %v, want %v", c.tok, got, c.want)
		}
	}
}

func TestIsCommandLine(t *testing.T) {
	for _, line := range []string{"$ go test ./...", "cd backend", "make x", "yarn lint", "npx tsc", "go run ./cmd/x"} {
		if !isCommandLine(line) {
			t.Errorf("%q should be a command", line)
		}
	}
	if isCommandLine("some prose") {
		t.Error("prose should not be a command")
	}
}

func TestCheckCommandLineEdgeCases(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/cmd/ok/main.go", "package main\n")
	writeFile(t, root, "backend/Makefile", "gen-contract-fixtures:\n\t@echo ok\n")
	writeFile(t, root, "frontend/package.json", `{"scripts":{"lint":"x"},"devDependencies":{"custom":"1"}}`)

	cases := []struct {
		line string
		want string
	}{
		{"", ""},
		{"# comment", ""},
		{"cd", ""},
		{"go", ""},
		{"make", ""},
		{"yarn", ""},
		{"npx", ""},
		{"go build ./...", ""},
		{"cd /tmp && go run ./cmd/doesnotexist", "go run target missing"},
		{"go run -tags x ./cmd/ok", ""},
		{"go run $TARGET", ""},
		{"go run ./scripts/thing", ""},
		{"go run ./cmd/...", ""},
		{"go run ./cmd/ok", ""},
		{"make -n", ""},
		{"make $TARGET", ""},
		{"make gen-contract-fixtures", ""},
		{"yarn -s", ""},
		{"yarn run", ""},
		{"yarn lint", ""},
		{"npx -y", ""},
		{"npx custom", ""},
	}
	for _, c := range cases {
		got := checkCommandLine(root, "AGENTS.md", 1, c.line)
		if c.want == "" && len(got) != 0 {
			t.Errorf("checkCommandLine(%q) = %v, want no findings", c.line, got)
		}
		if c.want != "" {
			if len(got) == 0 || !contains(got[0].Message, c.want) {
				t.Errorf("checkCommandLine(%q) = %v, want %q", c.line, got, c.want)
			}
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestResolveCWDAndPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "backend/AGENTS.md", "# Backend\n")
	if got := resolveCWD(root, root, "/tmp"); got != "/tmp" {
		t.Errorf("absolute cwd = %q", got)
	}
	if got := resolvePath(root, "docs/AGENTS.md", "backend/AGENTS.md"); got != filepath.Join(root, "backend", "AGENTS.md") {
		t.Errorf("resolvePath() = %q", got)
	}
	if got := resolvePath(root, "docs/AGENTS.md", filepath.Join(root, "backend", "AGENTS.md")); got != filepath.Join(root, "backend", "AGENTS.md") {
		t.Errorf("absolute resolvePath() = %q", got)
	}
	if got := resolvePath(root, "docs/AGENTS.md", "nope.go"); got != filepath.Join(root, "nope.go") {
		t.Errorf("missing resolvePath() = %q", got)
	}
	if !resolvePathExists(root, "docs/AGENTS.md", filepath.Join(root, "backend", "AGENTS.md")) {
		t.Error("absolute resolvePathExists should pass")
	}
	if resolvePathExists(root, "docs/AGENTS.md", "nope.go") {
		t.Error("missing resolvePathExists should fail")
	}
}

func TestTrimLineColSuffix(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"plain":             "plain",
		"backend/x.go:12":   "backend/x.go",
		"backend/x.go:12-3": "backend/x.go",
		"backend/x.go:12x":  "backend/x.go:12x",
		"backend/x.go:":     "backend/x.go:",
	}
	for in, want := range cases {
		if got := trimLineColSuffix(in); got != want {
			t.Errorf("trimLineColSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInlineCodeSpanEdgeCases(t *testing.T) {
	if got := inlineCodeSpans("no backticks"); len(got) != 0 {
		t.Errorf("prose spans = %v", got)
	}
	if got := inlineCodeSpans("`unclosed"); len(got) != 0 {
		t.Errorf("unclosed span = %v", got)
	}
	if got := inlineCodeSpans("``a`b`` and `c`"); len(got) != 2 || got[0].Text != "a`b" || got[1].Text != "c" {
		t.Errorf("mixed spans = %v", got)
	}
}

func TestExtractCodeSpansTildeFence(t *testing.T) {
	spans := extractCodeSpans("~~~bash\ncd backend && go test ./...\n~~~\n`backend/AGENTS.md`\n")
	if len(spans) != 2 {
		t.Fatalf("spans = %v", spans)
	}
	if spans[0].Text != "cd backend && go test ./..." || spans[1].Text != "backend/AGENTS.md" {
		t.Fatalf("spans = %v", spans)
	}
}

func TestCheckCodeSpanEmpty(t *testing.T) {
	if got := checkCodeSpan(t.TempDir(), contextFile{Path: "AGENTS.md"}, codeSpan{Text: "   "}); len(got) != 0 {
		t.Fatalf("empty span = %v", got)
	}
}

func TestDiscoverSkillsErrors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, SkillsRoot, "not a directory\n")
	if _, got := discoverSkills(root, SkillsRoot); len(got) == 0 {
		t.Fatal("non-directory skills root should be reported")
	}

	root2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root2, SkillsRoot, "broken", "SKILL.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, got := discoverSkills(root2, SkillsRoot); len(got) == 0 {
		t.Fatal("unreadable SKILL.md should be reported")
	}
}

func TestCheckDoDUnreadableSkill(t *testing.T) {
	root := validTree(t)
	got := checkDoDCommands(root, Skill{File: SkillsRoot + "/missing/SKILL.md"}, Suites())
	if len(got) != 0 {
		t.Fatalf("unreadable skill should not add a DoD finding: %v", got)
	}
}

func TestDoDHeadingEndsSection(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "valid")
	writeFile(t, root, SkillsRoot+"/valid/SKILL.md",
		"---\nname: valid\ndescription: x\n---\n# Valid\n\n## DoD\n\nText only.\n\n## Next\n\n```bash\ncd backend && go test ./database/... ./internal/dbtest/...\n```\n")
	if got := checkDoDCommands(root, Skill{File: SkillsRoot + "/valid/SKILL.md"}, Suites()); len(got) != 0 {
		t.Fatalf("commands after the DoD section must not be checked: %v", got)
	}
}

func TestCommandCoveredEmpty(t *testing.T) {
	if !commandCovered("", nil) {
		t.Fatal("empty command should be treated as covered")
	}
}

func TestWorkflowJobsMissingDirectory(t *testing.T) {
	if _, err := workflowJobs(t.TempDir()); err == nil {
		t.Fatal("missing .github/workflows should error")
	}
}

func TestFactsProseWithoutPattern(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/database/AGENTS.md", "No floor statement here.\n")
	got := factsFindings(root, Facts())
	if !contains(got, "prose does not state") {
		t.Fatalf("unmatched fact prose not reported: %s", got)
	}
}

func TestRequiredGateJobsMoreErrors(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, ".github/release-gates.json", "not json")
	if _, err := requiredGateJobs(root); err == nil {
		t.Fatal("invalid release-gates should error")
	}
	writeFile(t, root, ".github/release-gates.json", `{"gates":[]}`)
	writeFile(t, root, ".github/rulesets/main-protection.json", `{"rules":[]}`)
	if _, err := requiredGateJobs(root); err == nil {
		t.Fatal("empty gate sources should error")
	}
	if err := os.Remove(filepath.Join(root, ".github", "release-gates.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := requiredGateJobs(root); err == nil {
		t.Fatal("missing release-gates should error")
	}
}

func TestFactCapturesEmpty(t *testing.T) {
	if got := factCaptures("locale-set", "   "); got != nil {
		t.Fatalf("empty capture = %v", got)
	}
}
