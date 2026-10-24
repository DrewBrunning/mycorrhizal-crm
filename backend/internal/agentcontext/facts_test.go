package agentcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func factsFindings(root string, facts []Fact) string {
	var out []string
	for _, f := range checkFacts(root, facts) {
		out = append(out, f.String())
	}
	return strings.Join(out, "\n")
}

func TestValidTreeSatisfiesFacts(t *testing.T) {
	root := validTree(t)
	if got := factsFindings(root, Facts()); got != "" {
		t.Fatalf("valid tree facts failed:\n%s", got)
	}
}

func TestFloorFactFollowsCanonicalSource(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "backend/database/AGENTS.md", "Upgrades are supported from `v0.6.0` (migration `000031`) and later.\n")
	got := factsFindings(root, Facts())
	if !strings.Contains(got, `fact "upgrade-floor"`) || !strings.Contains(got, "v1.0.0") || !strings.Contains(got, "000057") {
		t.Fatalf("floor mismatch not reported: %s", got)
	}
}

func TestLocaleFactFollowsTheLocaleDirectory(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "frontend/src/i18n/locales/nl.json", "{}\n")
	got := factsFindings(root, Facts())
	if !strings.Contains(got, `fact "locale-set"`) || !strings.Contains(got, "missing nl") {
		t.Fatalf("added locale not reported: %s", got)
	}
	writeFile(t, root, "frontend/AGENTS.md",
		"All six locale files (`en`, `de`, `es`, `fr`, `it`, `nl`) are translated.\n"+
			"The e2e setup hardcodes `http://localhost:7300`.\n")
	if got := factsFindings(root, Facts()); got != "" {
		t.Fatalf("prose should follow the locale directory:\n%s", got)
	}
}

func TestE2EPortFactFollowsTheCanonicalConfig(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, "frontend/e2e/global-setup.ts", "export const APP_ORIGIN = 'http://localhost:7400';\n")
	got := factsFindings(root, Facts())
	if !strings.Contains(got, `fact "e2e-port"`) || !strings.Contains(got, "missing 7400") {
		t.Fatalf("changed port not reported: %s", got)
	}
	writeFile(t, root, "frontend/AGENTS.md",
		"All five locale files (`en`, `de`, `es`, `fr`, `it`) are translated.\n"+
			"The e2e setup hardcodes `http://localhost:7400`.\n")
	if got := factsFindings(root, Facts()); got != "" {
		t.Fatalf("prose should follow the port config:\n%s", got)
	}
}

func TestE2EPortFallsBackToPlaywrightConfig(t *testing.T) {
	root := validTree(t)
	if err := os.Remove(filepath.Join(root, "frontend", "e2e", "global-setup.ts")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "frontend/playwright.config.ts", "baseURL: 'http://localhost:7300',\n")
	if got := factsFindings(root, Facts()); got != "" {
		t.Fatalf("playwright.config.ts fallback failed:\n%s", got)
	}
}

func TestRequiredGateFactFollowsBothSources(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, ".github/rulesets/main-protection.json", `{"rules":[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Backend (Go)"},{"context":"New Gate"}]}}]}`)
	writeFile(t, root, ".github/release-gates.json", `{"gates":[{"tier":"per-pr","mandatory":true,"check_context":"Backend (Go)"},{"tier":"per-pr","mandatory":true,"check_context":"New Gate"}]}`)
	writeFile(t, root, SkillsRoot+"/add-quality-gate/SKILL.md",
		"---\nname: add-quality-gate\ndescription: fixture\n---\n\n- `Backend (Go)`\n")
	got := factsFindings(root, Facts())
	if !strings.Contains(got, `fact "required-gate-jobs"`) || !strings.Contains(got, "missing New Gate") {
		t.Fatalf("new required gate not reported: %s", got)
	}
	writeFile(t, root, SkillsRoot+"/add-quality-gate/SKILL.md",
		"---\nname: add-quality-gate\ndescription: fixture\n---\n\n- `Backend (Go)`\n- `New Gate`\n")
	if got := factsFindings(root, Facts()); got != "" {
		t.Fatalf("prose should follow both gate sources:\n%s", got)
	}
}

func TestFactErrors(t *testing.T) {
	root := validTree(t)
	if _, err := factValues(root, "not-a-source"); err == nil {
		t.Fatal("unknown source should error")
	}
	if got := factsFindings(root, []Fact{{Name: "bad", Source: "upgrade-floor", Pattern: "(["}}); !strings.Contains(got, "invalid pattern") {
		t.Fatalf("invalid pattern not reported: %s", got)
	}
	if got := factsFindings(root, []Fact{{Name: "bad-source", Source: "nope", Pattern: "x"}}); !strings.Contains(got, "cannot read canonical source") {
		t.Fatalf("source error not reported: %s", got)
	}
}

func TestLocaleSetErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := localeSet(root); err == nil {
		t.Fatal("missing locale directory should error")
	}
	writeFile(t, root, "frontend/src/i18n/locales/json.d.ts", "declare const x: never;\n")
	if _, err := localeSet(root); err == nil {
		t.Fatal("directory with no locale json should error")
	}
}

func TestRequiredGateJobsErrors(t *testing.T) {
	root := validTree(t)
	writeFile(t, root, ".github/rulesets/main-protection.json", "not json")
	if _, err := requiredGateJobs(root); err == nil {
		t.Fatal("invalid ruleset should error")
	}
	if err := os.Remove(filepath.Join(root, ".github", "rulesets", "main-protection.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := requiredGateJobs(root); err == nil {
		t.Fatal("missing ruleset should error")
	}
}

func TestE2EPortErrorWhenNoConfig(t *testing.T) {
	root := t.TempDir()
	if _, err := e2ePort(root); err == nil {
		t.Fatal("missing config should error")
	}
}
