package agentcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func appendFile(t *testing.T, root, rel, extra string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(body, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDeadSkillPathFails(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "dead-path")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "path reference missing: backend/does/not/exist.go") {
		t.Fatalf("dead skill path not reported: %s", got)
	}
}

func TestInlineCodeTokensAreNotMisclassified(t *testing.T) {
	root := validTree(t)
	appendFile(t, root, "frontend/AGENTS.md", "\nTokens: `user_id`, `suggested`, `normal|private|secret`, `GET /api/v1/contacts`.\n")
	if got := findings(root); len(got) != 0 {
		t.Fatalf("non-path tokens were flagged: %s", strings.Join(got, "\n"))
	}
}

func TestDeadAgentsPathFails(t *testing.T) {
	root := validTree(t)
	appendFile(t, root, "frontend/AGENTS.md", "\nSee `src/does/not/exist.ts`.\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "path reference missing: src/does/not/exist.ts") {
		t.Fatalf("dead path not reported: %s", got)
	}
}

func TestDeadAnchorFails(t *testing.T) {
	root := validTree(t)
	appendFile(t, root, "docs/AGENTS.md", "\n[bad anchor](../backend/AGENTS.md#not-a-heading)\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "anchor not found: ../backend/AGENTS.md#not-a-heading") {
		t.Fatalf("dead anchor not reported: %s", got)
	}
}

func TestDeadGoRunTargetFails(t *testing.T) {
	root := validTree(t)
	appendFile(t, root, "backend/AGENTS.md", "\nRun `go run ./cmd/doesnotexist`.\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "go run target missing: cmd/doesnotexist") {
		t.Fatalf("dead go run target not reported: %s", got)
	}
}

func TestRenamedMakeTargetFails(t *testing.T) {
	root := validTree(t)
	appendFile(t, root, "backend/AGENTS.md", "\nRun `make not-a-target`.\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "make target missing from backend/Makefile: not-a-target") {
		t.Fatalf("dead make target not reported: %s", got)
	}
}

func TestMissingYarnScriptFails(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "frontend/package.json", `{"scripts":{"lint":"eslint ."},"devDependencies":{"vitest":"1"}}`)
	appendFile(t, root, "frontend/AGENTS.md", "\nRun `yarn not-a-script` and `yarn install --frozen-lockfile`.\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "yarn script missing from frontend/package.json: not-a-script") {
		t.Fatalf("missing yarn script not reported: %s", got)
	}
	if strings.Contains(got, "install") {
		t.Fatalf("yarn builtin install should be allowed: %s", got)
	}
}

func TestNpxResolution(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "frontend/package.json", `{"scripts":{},"devDependencies":{"vitest":"1"}}`)
	appendFile(t, root, "frontend/AGENTS.md", "\nRun `npx vitest run`, `npx tsc --noEmit`, `npx playwright test`, and `npx not-a-binary`.\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "npx binary is neither allowlisted nor a frontend devDependency: not-a-binary") {
		t.Fatalf("unknown npx binary not reported: %s", got)
	}
	for _, allowed := range []string{"vitest", "tsc", "playwright"} {
		if strings.Contains(got, allowed+":") {
			t.Fatalf("%s should be allowed: %s", allowed, got)
		}
	}
}

func TestTestReferenceResolution(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/agentcontext_ref_test.go", "package backend\n\nfunc TestExisting(t *testing.T) {}\n")
	appendFile(t, root, "backend/AGENTS.md", "\n`backend/agentcontext_ref_test.go#TestExisting`\n")
	if got := findings(root); len(got) != 0 {
		t.Fatalf("valid test reference should pass: %s", strings.Join(got, "\n"))
	}

	appendFile(t, root, "backend/AGENTS.md", "\n`backend/nope_test.go#TestMissing` and `backend/agentcontext_ref_test.go#TestGone`\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "test file reference missing: backend/nope_test.go") {
		t.Fatalf("missing test file not reported: %s", got)
	}
	if !strings.Contains(got, "test function not found in backend/agentcontext_ref_test.go: TestGone") {
		t.Fatalf("missing test function not reported: %s", got)
	}
}

func TestFencedCommandIsResolved(t *testing.T) {
	root := validTree(t)
	appendFile(t, root, "backend/AGENTS.md", "\n```bash\ncd backend && go run ./cmd/missing\n```\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "go run target missing: backend/cmd/missing") {
		t.Fatalf("fenced command target not reported: %s", got)
	}
}

func TestPackageJSONErrorsAreReported(t *testing.T) {
	root := validTree(t)
	appendFile(t, root, "frontend/AGENTS.md", "\n`yarn lint`\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "cannot read frontend/package.json") {
		t.Fatalf("missing package.json not reported: %s", got)
	}
	writeFile(t, root, "frontend/package.json", "not json")
	got = strings.Join(findings(root), "\n")
	if !strings.Contains(got, "cannot parse frontend/package.json") {
		t.Fatalf("invalid package.json not reported: %s", got)
	}
}

func TestMakefileErrorIsReported(t *testing.T) {
	root := validTree(t)
	if err := os.Remove(filepath.Join(root, "backend", "Makefile")); err != nil {
		t.Fatal(err)
	}
	appendFile(t, root, "backend/AGENTS.md", "\n`make gen-contract-fixtures`\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "cannot read backend/Makefile") {
		t.Fatalf("missing Makefile not reported: %s", got)
	}
}
