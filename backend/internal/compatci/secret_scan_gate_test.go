// Issue #1877 follow-up: the app-token gitleaks rule was only enforced by the
// local pre-commit hook (`gitleaks protect --staged`). A token committed with
// `--no-verify` or through the web UI was invisible to TruffleHog/Trivy (they
// do not know the `mycorrhizal_` format) and GitHub push protection cannot take
// custom patterns on this repository.
//
// The gate lives inside the already-required, always-on
// `Docs & security-doc citations` job, so it blocks a merge without a new
// status context. These structural tests pin the properties that make it
// trustworthy: full history is fetched, the pinned tools.mod binary runs the
// scan, and the scan itself uses the base branch's config with only the
// app-token rule enabled.
package compatci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	appTokenScanScriptRel = "../../../.github/scripts/secret-scan-app-tokens.sh"
	appTokenRuleID        = "mycorrhizal-app-token"
)

func TestAppTokenHistoryScanRunsInRequiredJob(t *testing.T) {
	wf := loadCIWorkflow(t, "unit-tests.yml")
	job, ok := wf.Jobs["docs-citations"]
	if !ok {
		t.Fatal("unit-tests.yml has no docs-citations job")
	}
	if job.Name != noticesGateName {
		t.Fatalf("docs-citations job is named %q; must be %q (the release-gates/ruleset context)", job.Name, noticesGateName)
	}

	checkoutIndex := -1
	buildIndex := -1
	scanIndex := -1
	for i, step := range job.Steps {
		switch {
		case checkoutIndex < 0 && strings.HasPrefix(step.Uses, "actions/checkout@"):
			checkoutIndex = i
		case buildIndex < 0 && step.WorkingDirectory == "backend" &&
			strings.Contains(step.Run, "go build -modfile=tools.mod") &&
			// The published module still declares the legacy zricethezav path.
			strings.Contains(step.Run, "github.com/zricethezav/gitleaks/v8"):
			buildIndex = i
		case scanIndex < 0 && strings.Contains(step.Run, "secret-scan-app-tokens.sh"):
			scanIndex = i
		}
	}
	if checkoutIndex < 0 {
		t.Fatal("docs-citations has no actions/checkout step")
	}
	if buildIndex < 0 {
		t.Fatal("docs-citations has no pinned gitleaks build from backend/tools.mod")
	}
	if scanIndex < 0 {
		t.Fatal("docs-citations does not run .github/scripts/secret-scan-app-tokens.sh")
	}
	if !(checkoutIndex < buildIndex && buildIndex < scanIndex) {
		t.Fatalf("expected checkout (%d) < gitleaks build (%d) < scan (%d)", checkoutIndex, buildIndex, scanIndex)
	}

	checkout := job.Steps[checkoutIndex]
	if got := fmt.Sprint(checkout.With["fetch-depth"]); got != "0" {
		t.Fatalf("docs-citations checkout fetch-depth = %q; the full git history is required for the app-token history scan", got)
	}

	scan := job.Steps[scanIndex]
	wantEnv := map[string]string{
		"REPO_ROOT":      "${{ github.workspace }}",
		"DEFAULT_BRANCH": "${{ github.event.repository.default_branch }}",
		"GITLEAKS_BIN":   "${{ runner.temp }}/gitleaks",
	}
	for key, want := range wantEnv {
		if got := fmt.Sprint(scan.Env[key]); got != want {
			t.Errorf("scan step env %s = %q, want %q", key, got, want)
		}
	}

	// The pinned tool lives in the CI-only tools.mod graph, not backend/go.mod
	// (the shipped binary's module graph).
	toolsMod, err := os.ReadFile(filepath.Join(workflowsDirRel, "..", "..", "backend", "tools.mod"))
	if err != nil {
		t.Fatalf("reading backend/tools.mod: %v", err)
	}
	for _, want := range []string{
		// gitleaks v8.30.1 still declares the legacy zricethezav module path.
		"\tgithub.com/zricethezav/gitleaks/v8\n",
		"github.com/zricethezav/gitleaks/v8 v8.30.1",
	} {
		if !strings.Contains(string(toolsMod), want) {
			t.Errorf("backend/tools.mod is missing %q", want)
		}
	}

	// The script must derive the config from the default branch checkout, not
	// the PR's own copy, so a PR cannot weaken the rule and pass.
	scriptPath := filepath.Join(workflowsDirRel, "..", "..", ".github", "scripts", "secret-scan-app-tokens.sh")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("reading %s: %v", scriptPath, err)
	}
	for _, want := range []string{
		"origin/$DEFAULT_BRANCH:.gitleaks.toml",
		"is-shallow-repository",
		"--log-opts=--all",
		"--enable-rule " + appTokenRuleID,
		"--redact",
		`id = "` + appTokenRuleID + `"`,
	} {
		if !strings.Contains(string(script), want) {
			t.Errorf("%s does not contain %q", appTokenScanScriptRel, want)
		}
	}
}
