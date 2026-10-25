// Issue #1944 regression guards for third-party notices drift.
//
// PR #1942 put the `gennotices -check` step in license-compliance.yml, a
// workflow with no required status check, so a stale THIRD_PARTY_NOTICES.md
// could merge. The fix is a step inside the already-required
// `Docs & security-doc citations` job (the CLAUDE.md "adding a quality gate"
// default), path-gated on the shared `notices` filter.
//
// None of that is executable logic, so these tests parse the real workflow,
// filter, ruleset and registry files as ground truth -- the same shape as
// ci_claims_test.go / version_test.go. They fail if the step is moved back to
// an advisory workflow, loses a prerequisite step, stops running on the events
// that matter, or if the filter stops covering the generator's inputs.
package compatci

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"mycorrhizal/internal/governance"
	"mycorrhizal/internal/releasegates"
)

const (
	filtersRel            = "../../../.github/filters.yaml"
	releaseGatesRel       = "../../../.github/release-gates.json"
	mainProtectionRel     = "../../../.github/rulesets/main-protection.json"
	licenseAttributionRel = "../../../docs/security/license-attribution.md"

	noticesCheckRun = "go run ./cmd/gennotices -check"
	noticesGateName = "Docs & security-doc citations"
)

type ciWorkflow struct {
	Jobs map[string]ciJob `yaml:"jobs"`
}

type ciJob struct {
	Name    string            `yaml:"name"`
	If      string            `yaml:"if"`
	Needs   any               `yaml:"needs"`
	Env     map[string]any    `yaml:"env"`
	Outputs map[string]string `yaml:"outputs"`
	Steps   []ciStep          `yaml:"steps"`
}

type ciStep struct {
	Name             string         `yaml:"name"`
	If               string         `yaml:"if"`
	Run              string         `yaml:"run"`
	Uses             string         `yaml:"uses"`
	WorkingDirectory string         `yaml:"working-directory"`
	Env              map[string]any `yaml:"env"`
	With             map[string]any `yaml:"with"`
}

func loadCIWorkflow(t *testing.T, file string) ciWorkflow {
	t.Helper()
	path := filepath.Join(workflowsDirRel, file)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var wf ciWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return wf
}

func needsHas(needs any, want string) bool {
	switch v := needs.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func findStep(job ciJob, predicate func(ciStep) bool) (ciStep, bool) {
	for _, step := range job.Steps {
		if predicate(step) {
			return step, true
		}
	}
	return ciStep{}, false
}

// TestThirdPartyNoticesDriftRunsInRequiredJob is the issue #1944 core guard:
// the drift command lives in the docs-citations job, that job's check context
// is a mandatory per-PR gate and a required main-protection check, and its
// path-gating covers the events that must always run it.
func TestThirdPartyNoticesDriftRunsInRequiredJob(t *testing.T) {
	wf := loadCIWorkflow(t, "unit-tests.yml")
	job, ok := wf.Jobs["docs-citations"]
	if !ok {
		t.Fatal("unit-tests.yml has no docs-citations job")
	}
	if job.Name != noticesGateName {
		t.Fatalf("docs-citations job is named %q; must be %q (the release-gates/ruleset context)", job.Name, noticesGateName)
	}
	if !needsHas(job.Needs, "changes") {
		t.Fatal("docs-citations must need the `changes` job so RUN_NOTICES can read the notices filter output")
	}

	runNotices, ok := job.Env["RUN_NOTICES"]
	if !ok {
		t.Fatal("docs-citations has no RUN_NOTICES env value; the notices steps have no shared condition")
	}
	condition := fmt.Sprint(runNotices)
	for _, want := range []string{
		"github.event_name == 'schedule'",
		"github.event_name == 'workflow_dispatch'",
		"github.event_name == 'workflow_call'",
		"needs.changes.outputs.notices == 'true'",
		"needs.changes.outputs.workflows == 'true'",
	} {
		if !strings.Contains(condition, want) {
			t.Errorf("RUN_NOTICES condition does not contain %q; the gate would miss an event that must run it.\ncondition: %s", want, condition)
		}
	}

	check, ok := findStep(job, func(s ciStep) bool { return strings.Contains(s.Run, noticesCheckRun) })
	if !ok {
		t.Fatalf("docs-citations has no step running %q", noticesCheckRun)
	}
	if check.WorkingDirectory != "backend" {
		t.Errorf("%s must run from backend/, got working-directory %q", check.Name, check.WorkingDirectory)
	}
	if check.If != "env.RUN_NOTICES == 'true'" {
		t.Errorf("notices check step condition = %q, want env.RUN_NOTICES == 'true' (path-gated, not always-on)", check.If)
	}

	// The check is useless without the resolved production node_modules and
	// the Go toolchain above it. Both prerequisites must be gated identically
	// to the check itself, or the check fails for the wrong reason on the
	// events that do run it.
	node, ok := findStep(job, func(s ciStep) bool { return strings.HasPrefix(s.Uses, "actions/setup-node@") })
	if !ok {
		t.Fatal("docs-citations has no setup-node step; license-checker cannot resolve npm licenses")
	}
	if node.If != check.If {
		t.Errorf("setup-node condition %q != notices check condition %q", node.If, check.If)
	}
	install, ok := findStep(job, func(s ciStep) bool {
		return strings.Contains(s.Run, "yarn install --frozen-lockfile") && s.WorkingDirectory == "frontend"
	})
	if !ok {
		t.Fatal("docs-citations has no `yarn install --frozen-lockfile` step in frontend/")
	}
	if install.If != check.If {
		t.Errorf("yarn install condition %q != notices check condition %q", install.If, check.If)
	}

	// The gate is only merge-blocking if its job name is a required context.
	gateData, err := os.ReadFile(releaseGatesRel)
	if err != nil {
		t.Fatalf("reading %s: %v", releaseGatesRel, err)
	}
	reg, findings := releasegates.Parse(gateData)
	if len(findings) > 0 {
		t.Fatalf("%s has findings: %v", releaseGatesRel, findings)
	}
	var gate *releasegates.Gate
	for i := range reg.Gates {
		if reg.Gates[i].Name == noticesGateName {
			gate = &reg.Gates[i]
			break
		}
	}
	if gate == nil {
		t.Fatalf("%s has no %q gate", releaseGatesRel, noticesGateName)
	}
	if gate.Tier != "per-pr" || !gate.Mandatory || !gate.ReleaseGate {
		t.Errorf("%q gate = tier %q mandatory %v release_gate %v; want per-pr/true/true so the check blocks merges and is re-checked at release",
			noticesGateName, gate.Tier, gate.Mandatory, gate.ReleaseGate)
	}

	rulesetData, err := os.ReadFile(mainProtectionRel)
	if err != nil {
		t.Fatalf("reading %s: %v", mainProtectionRel, err)
	}
	rs, findings := governance.ParseRuleset(mainProtectionRel, rulesetData)
	if len(findings) > 0 {
		t.Fatalf("%s has findings: %v", mainProtectionRel, findings)
	}
	required := governance.RequiredContexts(rs)
	found := false
	for _, c := range required {
		if c == noticesGateName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("%q is not a required main-protection check; notices drift would be advisory again. required=%v", noticesGateName, required)
	}
}

// TestThirdPartyNoticesDriftHasOneDefinition fails if the advisory
// license-compliance.yml job is reintroduced alongside the required step:
// two checks would be able to disagree, and one would be silently ignored.
func TestThirdPartyNoticesDriftHasOneDefinition(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(workflowsDirRel, "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(data), noticesCheckRun) {
			found = append(found, filepath.Base(path))
		}
	}
	sort.Strings(found)
	if len(found) != 1 || found[0] != "unit-tests.yml" {
		t.Fatalf("%q must exist in exactly one workflow (unit-tests.yml, the required job); found in %v", noticesCheckRun, found)
	}

	license := loadCIWorkflow(t, "license-compliance.yml")
	if _, ok := license.Jobs["notices-drift"]; ok {
		t.Fatal("license-compliance.yml still has the advisory notices-drift job; the check moved to unit-tests.yml (issue #1944)")
	}
}

// TestNoticesFilterCoversTheAttributionInputs pins the path filter and its
// wiring into the required job. It also asserts the deliberate absence of
// Dockerfile/docker/** -- the Alpine base layer is a manual-review boundary
// (docs/security/license-attribution.md), not something gennotices can see.
func TestNoticesFilterCoversTheAttributionInputs(t *testing.T) {
	data, err := os.ReadFile(filtersRel)
	if err != nil {
		t.Fatalf("reading %s: %v", filtersRel, err)
	}
	var filters map[string][]string
	if err := yaml.Unmarshal(data, &filters); err != nil {
		t.Fatalf("parsing %s: %v", filtersRel, err)
	}
	notices := filters["notices"]
	want := []string{
		"backend/go.mod",
		"backend/go.sum",
		"frontend/package.json",
		"frontend/yarn.lock",
		"backend/cmd/gennotices/**",
		"backend/internal/notices/**",
		"thirdparty/**",
		"THIRD_PARTY_NOTICES.md",
	}
	if len(notices) != len(want) {
		t.Fatalf("notices filter = %v; want exactly %v", notices, want)
	}
	seen := map[string]bool{}
	for _, p := range notices {
		seen[p] = true
	}
	for _, p := range want {
		if !seen[p] {
			t.Errorf("notices filter is missing %q", p)
		}
	}
	for _, p := range notices {
		if p == "Dockerfile" || strings.HasPrefix(p, "docker/") {
			t.Errorf("notices filter matches %q; the Alpine base layer is a documented manual-review boundary, not a gennotices input (issue #1944)", p)
		}
	}

	wf := loadCIWorkflow(t, "unit-tests.yml")
	changes, ok := wf.Jobs["changes"]
	if !ok {
		t.Fatal("unit-tests.yml has no changes job")
	}
	if got := changes.Outputs["notices"]; got != "${{ steps.resolve.outputs.notices }}" {
		t.Fatalf("changes.outputs.notices = %q, want the resolve step output", got)
	}
	filterStep, ok := findStep(changes, func(s ciStep) bool { return strings.HasPrefix(s.Uses, "dorny/paths-filter@") })
	if !ok {
		t.Fatal("changes job has no dorny/paths-filter step")
	}
	if got := fmt.Sprint(filterStep.With["filters"]); got != ".github/filters.yaml" {
		t.Fatalf("paths-filter reads %q, want .github/filters.yaml", got)
	}
	resolve, ok := findStep(changes, func(s ciStep) bool { return s.Name == "Resolve gating decision" })
	if !ok {
		t.Fatal("changes job has no Resolve gating decision step")
	}
	for _, want := range []string{`notices="$NOTICES_CHANGED"`, "notices=true"} {
		if !strings.Contains(resolve.Run, want) {
			t.Errorf("Resolve gating decision does not contain %q:\n%s", want, resolve.Run)
		}
	}
	if got := fmt.Sprint(resolve.Env["NOTICES_CHANGED"]); got != "${{ steps.filter.outputs.notices }}" {
		t.Errorf("NOTICES_CHANGED env = %q, want the paths-filter notices output", got)
	}
}

// TestLicenseAttributionRecordsDockerManualReviewBoundary pins the recorded
// decision the issue asks for: since the filter cannot see the base image,
// the docs must say so and call it a manual review.
func TestLicenseAttributionRecordsDockerManualReviewBoundary(t *testing.T) {
	data, err := os.ReadFile(licenseAttributionRel)
	if err != nil {
		t.Fatalf("reading %s: %v", licenseAttributionRel, err)
	}
	doc := strings.Join(strings.Fields(string(data)), " ")
	for _, want := range []string{
		"manual-review boundary",
		"deliberately does not match `Dockerfile` or `docker/**`",
		"belongs in the same review as this notice",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("%s does not record %q; the Docker/base-image decision must be written down (issue #1944)", licenseAttributionRel, want)
		}
	}
}

// TestContainerShipsBundledContentNotices pins the issue #1880 follow-up: the
// image must carry THIRD_PARTY_SOURCES.md (bundled content and full OFL texts)
// as well as THIRD_PARTY_NOTICES.md, both in /app and served over HTTP. The
// root .dockerignore ignores every markdown file, so the source file needs an
// explicit negation or it never reaches the build context.
func TestContainerShipsBundledContentNotices(t *testing.T) {
	dockerfileData, err := os.ReadFile("../../../Dockerfile")
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	dockerfile := string(dockerfileData)
	for _, want := range []string{
		"COPY THIRD_PARTY_NOTICES.md /app/THIRD_PARTY_NOTICES.md",
		"COPY THIRD_PARTY_NOTICES.md /usr/share/nginx/html/third-party-notices.txt",
		"COPY THIRD_PARTY_SOURCES.md /app/THIRD_PARTY_SOURCES.md",
		"COPY THIRD_PARTY_SOURCES.md /usr/share/nginx/html/third-party-sources.txt",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("Dockerfile is missing %q", want)
		}
	}

	ignoreData, err := os.ReadFile("../../../.dockerignore")
	if err != nil {
		t.Fatalf("reading .dockerignore: %v", err)
	}
	if !strings.Contains(string(ignoreData), "!THIRD_PARTY_SOURCES.md") {
		t.Error(".dockerignore does not un-ignore THIRD_PARTY_SOURCES.md; it would never reach the build context")
	}
}

// TestAndroidPackagesGoAndFontNotices pins the app-side half: :app copies the
// two notices and both font OFL files into asset/thirdparty so the licenses
// screen can show the Go standard library and fonts that AboutLibraries cannot
// see in the Gradle graph.
func TestAndroidPackagesGoAndFontNotices(t *testing.T) {
	data, err := os.ReadFile("../../../android/app/build.gradle.kts")
	if err != nil {
		t.Fatalf("reading app/build.gradle.kts: %v", err)
	}
	build := string(data)
	for _, want := range []string{
		"copyThirdPartyAttribution",
		"../THIRD_PARTY_NOTICES.md",
		"../THIRD_PARTY_SOURCES.md",
		"../assets/fonts/eb-garamond/OFL.txt",
		"../assets/fonts/ibm-plex/OFL.txt",
		`assets.directories.add(thirdPartyAttributionAssets.get().asFile.absolutePath)`,
	} {
		if !strings.Contains(build, want) {
			t.Errorf("android/app/build.gradle.kts is missing %q", want)
		}
	}
}
