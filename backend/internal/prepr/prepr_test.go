package prepr

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"backend/**", "backend/go.mod", true},
		{"backend/**", "backend/internal/foo/bar.go", true},
		{"backend/**", "frontend/foo.ts", false},
		{"backend/**", "backend", false},
		{"frontend/**", "frontend/src/a.test.tsx", true},
		{"docs/golden-fixtures/**", "docs/golden-fixtures/card.vcf", true},
		{"android/**/build.gradle.kts", "android/build.gradle.kts", true},
		{"android/**/build.gradle.kts", "android/app/build.gradle.kts", true},
		{"android/**/gradle.lockfile", "android/feature/contacts/gradle.lockfile", true},
		{"docker-compose*.yml", "docker-compose.prod.yml", true},
		{"docker-compose*.yml", "docker-compose.yml", true},
		{"docker-compose*.yml", "docker/docker-compose.yml", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false},
		{".github/**", ".github/workflows/unit-tests.yml", true},
		{"testdata/**", "testdata/canonical-fixture/x.json", true},
		{"backend/services/immich_*.go", "backend/services/immich_client.go", true},
	}
	for _, tt := range tests {
		if got := Match(tt.pattern, tt.path); got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

const fixtureFilters = `
backend:
  - 'backend/**'
  - 'docs/golden-fixtures/**'
frontend:
  - 'frontend/**'
android:
  - 'android/**'
openapi:
  - 'backend/openapi.yaml'
infra:
  - 'Dockerfile'
  - 'docker-compose*.yml'
  - 'docker/**'
docs-only:
  - 'nope/**'
`

func loadFixture(t *testing.T) *Filters {
	t.Helper()
	f, err := LoadFilters(strings.NewReader(fixtureFilters))
	if err != nil {
		t.Fatalf("LoadFilters: %v", err)
	}
	return f
}

func TestLoadFiltersPreservesOrder(t *testing.T) {
	f := loadFixture(t)
	want := []string{"backend", "frontend", "android", "openapi", "infra", "docs-only"}
	if got := f.AreaNames(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("AreaNames() = %v, want %v", got, want)
	}
	if got := f.Globs["backend"]; len(got) != 2 || got[1] != "docs/golden-fixtures/**" {
		t.Fatalf("backend globs = %v", got)
	}
}

func TestLoadFiltersRejectsMalformed(t *testing.T) {
	for _, doc := range []string{
		"backend: not-a-list\n",
		"- just\n- a\n- list\n",
		"",
	} {
		if _, err := LoadFilters(strings.NewReader(doc)); err == nil {
			t.Errorf("LoadFilters(%q) succeeded, want error", doc)
		}
	}
}

func TestSelect(t *testing.T) {
	f := loadFixture(t)
	got := f.Select([]string{
		"backend/services/foo.go",
		"frontend/src/App.tsx",
		"README.md",
	})
	want := []string{"backend", "frontend"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Select() = %v, want %v", got, want)
	}
}

// TestEveryFilterHasAnArea is the drift gate: a new key in
// .github/filters.yaml without a prepr entry would otherwise be silently
// ignored by the local gate.
func TestEveryFilterHasAnArea(t *testing.T) {
	root := repoRoot(t)
	file, err := os.Open(filepath.Join(root, ".github", "filters.yaml"))
	if err != nil {
		t.Fatalf("open filters.yaml: %v", err)
	}
	defer file.Close()
	f, err := LoadFilters(file)
	if err != nil {
		t.Fatalf("LoadFilters: %v", err)
	}

	areas := map[string]bool{}
	for _, name := range AreaNames() {
		if areas[name] {
			t.Errorf("duplicate prepr area %q", name)
		}
		areas[name] = true
	}

	for _, name := range f.AreaNames() {
		if !areas[name] {
			t.Errorf("filter %q in .github/filters.yaml has no prepr area; add it to internal/prepr/areas.go", name)
		}
		delete(areas, name)
	}
	for name := range areas {
		t.Errorf("prepr area %q does not exist in .github/filters.yaml", name)
	}
}

func TestEveryAreaDoesSomething(t *testing.T) {
	for _, area := range Areas {
		if len(area.Fast) == 0 && len(area.Full) == 0 && len(area.Implies) == 0 && area.Note == "" {
			t.Errorf("area %q has no commands, implications, or note", area.Name)
		}
		if len(area.Fast) == 0 && len(area.Full) > 0 {
			t.Errorf("area %q has full-only commands; fast must be a subset of full", area.Name)
		}
	}
}

func TestBuildBackendFast(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{"backend/services/foo.go"}, ModeFast, "origin/main")

	if strings.Join(plan.Selected, ",") != "backend" {
		t.Fatalf("Selected = %v, want [backend]", plan.Selected)
	}
	labels := labelsFor(plan, "backend")
	for _, want := range []string{"backend: go build", "backend: go vet", "backend: golangci-lint", "backend: API baseline drift"} {
		if !contains(labels, want) {
			t.Errorf("fast backend plan missing %q; got %v", want, labels)
		}
	}
	for _, unwanted := range []string{"backend: go test ./...", "backend: per-file coverage ratchet"} {
		if contains(labels, unwanted) {
			t.Errorf("fast backend plan must not run %q", unwanted)
		}
	}
	if len(plan.Hints) != 0 {
		t.Errorf("fast mode should not print baseline hints, got %v", plan.Hints)
	}
}

func TestBuildDocsOnlyRunsOnlyGovernance(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{"docs/development/workflow.md"}, ModeFast, "origin/main")

	if len(plan.Selected) != 0 {
		t.Fatalf("Selected = %v, want none", plan.Selected)
	}
	if len(plan.Steps) != len(GovernanceSteps) {
		t.Fatalf("steps = %d, want %d governance steps only", len(plan.Steps), len(GovernanceSteps))
	}
	for _, step := range plan.Steps {
		if step.Area != "docs" {
			t.Errorf("docs-only plan ran area %q", step.Area)
		}
	}
}

func TestBuildFullStressesChangedGoTestAndPrintsBaseline(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{"backend/services/foo_test.go"}, ModeFull, "origin/main")

	labels := labelsFor(plan, "backend")
	if !contains(labels, "backend: go test ./...") {
		t.Errorf("full plan missing go test ./...; got %v", labels)
	}
	if !contains(labels, "backend: per-file coverage ratchet") {
		t.Errorf("full plan missing coverage ratchet; got %v", labels)
	}
	if !contains(labels, "stress: go test ./services") {
		t.Errorf("full plan missing go stress step; got %v", labels)
	}
	if len(plan.Hints) != 1 || plan.Hints[0] != BaselineHints["backend"] {
		t.Errorf("Hints = %v, want [%s]", plan.Hints, BaselineHints["backend"])
	}
	patch := stepByLabel(plan, "backend: changed-line coverage warning (non-blocking)")
	if patch == nil {
		t.Fatal("patchcov warning step not found")
	}
	if !patch.Command.Warn {
		t.Error("patchcov step must be non-blocking")
	}
	if !contains(patch.Command.Argv, "origin/main") {
		t.Errorf("patchcov argv = %v, want the selected base", patch.Command.Argv)
	}

	stress := stepByLabel(plan, "stress: go test ./services")
	if stress == nil {
		t.Fatal("stress step not found")
	}
	want := []string{"go", "test", "-race", "-count=10", "-shuffle=on", "./services"}
	if strings.Join(stress.Command.Argv, " ") != strings.Join(want, " ") {
		t.Errorf("stress argv = %v, want %v", stress.Command.Argv, want)
	}
}

func TestBuildStressPlaywrightAndVitest(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{
		"frontend/e2e/auth.spec.ts",
		"frontend/src/App.test.tsx",
	}, ModeFull, "origin/main")

	pw := stepByLabel(plan, "stress: playwright e2e/auth.spec.ts")
	if pw == nil {
		t.Fatal("playwright stress step not found")
	}
	if !contains(pw.Command.Argv, "--repeat-each=5") {
		t.Errorf("playwright argv = %v, want --repeat-each=5", pw.Command.Argv)
	}

	vt := stepByLabel(plan, "stress: vitest src/App.test.tsx")
	if vt == nil {
		t.Fatal("vitest stress step not found")
	}
	if vt.Command.Repeat != 10 {
		t.Errorf("vitest Repeat = %d, want 10", vt.Command.Repeat)
	}
	if !contains(vt.Command.Argv, "--sequence.shuffle") {
		t.Errorf("vitest argv = %v, want --sequence.shuffle", vt.Command.Argv)
	}
}

func TestBuildOpenAPIImpliesClients(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{"backend/openapi.yaml"}, ModeFast, "origin/main")

	for _, want := range []string{"android", "backend", "frontend", "openapi"} {
		if !contains(plan.Selected, want) {
			t.Errorf("Selected = %v, missing implied area %q", plan.Selected, want)
		}
	}
	if len(labelsFor(plan, "openapi")) != 0 {
		t.Error("openapi area itself should not run commands")
	}
	if !contains(plan.Notes, "openapi implies backend, frontend, android") {
		t.Errorf("Notes = %v, missing openapi implication", plan.Notes)
	}
}

func TestBuildSkipsCIOnlyAreaWithReason(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{"docker-compose.yml"}, ModeFast, "origin/main")

	if !contains(plan.Selected, "infra") {
		t.Fatalf("Selected = %v, want infra", plan.Selected)
	}
	found := false
	for _, skip := range plan.Skipped {
		if skip.Area == "infra" && strings.Contains(skip.Reason, "CI-only") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Skipped = %v, want infra with a CI-only reason", plan.Skipped)
	}
}

func TestAndroidTestTarget(t *testing.T) {
	module, class := androidTestTarget("android/feature/contacts/src/test/kotlin/com/mycorrhizal/crm/feature/contacts/ContactValueRowsTest.kt")
	if module != ":feature:contacts" {
		t.Errorf("module = %q, want :feature:contacts", module)
	}
	if class != "com.mycorrhizal.crm.feature.contacts.ContactValueRowsTest" {
		t.Errorf("class = %q", class)
	}

	module, class = androidTestTarget("android/app/src/androidTest/kotlin/com/mycorrhizal/crm/security/ScreenResilienceTest.kt")
	if module != ":app" || class != "com.mycorrhizal.crm.security.ScreenResilienceTest" {
		t.Errorf("got module=%q class=%q", module, class)
	}
}

func TestExecuteFailureNamesExactCommand(t *testing.T) {
	plan := &Plan{Mode: ModeFast, Steps: []Step{
		{Area: "backend", Command: Command{Label: "pass", Dir: "backend", Argv: []string{"go", "build", "./..."}}},
		{Area: "frontend", Command: Command{Label: "fail", Dir: "frontend", Argv: []string{"yarn", "lint"}}},
	}}
	var out bytes.Buffer
	code := Execute(plan, &out, func(step Step) error {
		if step.Command.Label == "fail" {
			return os.ErrNotExist
		}
		return nil
	})
	if code != 1 {
		t.Fatalf("Execute() = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "(cd frontend && yarn lint)") {
		t.Errorf("summary does not name the failing command:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "2 command(s) run") {
		t.Errorf("summary missing command count:\n%s", out.String())
	}
}

func TestExecuteWarningDoesNotFailTheGate(t *testing.T) {
	plan := &Plan{Mode: ModeFull, Steps: []Step{
		{Area: "backend", Command: Command{Label: "warn", Dir: "backend", Argv: []string{"go", "run", "./cmd/patchcov"}, Warn: true}},
	}}
	var out bytes.Buffer
	if code := Execute(plan, &out, func(Step) error { return os.ErrNotExist }); code != 0 {
		t.Fatalf("Execute() = %d, want 0 for a warning\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "WARN (does not fail the gate)") {
		t.Errorf("output missing warning:\n%s", out.String())
	}
}

func TestExecuteAllPass(t *testing.T) {
	plan := &Plan{Mode: ModeFast, Steps: []Step{
		{Area: "docs", Command: Command{Label: "ok", Dir: "backend", Argv: []string{"go", "run", "./cmd/docscheck"}}},
	}}
	var out bytes.Buffer
	if code := Execute(plan, &out, func(Step) error { return nil }); code != 0 {
		t.Fatalf("Execute() = %d, want 0\n%s", code, out.String())
	}
}

func countStress(plan *Plan, area string) int {
	n := 0
	for _, step := range plan.Steps {
		if step.Area == area && strings.HasPrefix(step.Command.Label, "stress:") {
			n++
		}
	}
	return n
}

func labelsFor(plan *Plan, area string) []string {
	var out []string
	for _, step := range plan.Steps {
		if step.Area == area {
			out = append(out, step.Command.Label)
		}
	}
	return out
}

func stepByLabel(plan *Plan, label string) *Step {
	for i := range plan.Steps {
		if plan.Steps[i].Command.Label == label {
			return &plan.Steps[i]
		}
	}
	return nil
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = parent
	}
}

func TestParseMode(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want Mode
		ok   bool
	}{
		{"fast", ModeFast, true},
		{"FULL", ModeFull, true},
		{"", "", false},
		{"slow", "", false},
	} {
		got, err := ParseMode(tt.in)
		if tt.ok && (err != nil || got != tt.want) {
			t.Errorf("ParseMode(%q) = (%v, %v), want %v", tt.in, got, err, tt.want)
		}
		if !tt.ok && err == nil {
			t.Errorf("ParseMode(%q) succeeded, want error", tt.in)
		}
	}
}

func TestMatchingPaths(t *testing.T) {
	f := loadFixture(t)
	changed := []string{"frontend/src/b.tsx", "backend/a.go", "backend/b.go", "README.md"}
	got := f.MatchingPaths("backend", changed)
	want := []string{"backend/a.go", "backend/b.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("MatchingPaths = %v, want %v", got, want)
	}
}

func TestLookupMissing(t *testing.T) {
	if _, ok := Lookup("nope"); ok {
		t.Fatal("Lookup(nope) = ok")
	}
}

func TestStepDisplay(t *testing.T) {
	cases := []struct {
		step Step
		want string
	}{
		{Step{Command: Command{Argv: []string{"go", "build", "./..."}}}, "go build ./..."},
		{Step{Command: Command{Shell: "echo hi"}}, "echo hi"},
		{Step{Command: Command{Reminder: "do the thing"}}, "do the thing"},
	}
	for _, tc := range cases {
		if got := tc.step.Display(); got != tc.want {
			t.Errorf("Display() = %q, want %q", got, tc.want)
		}
	}
}

func TestBuildAndroidTests(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{
		"android/feature/contacts/src/test/kotlin/com/x/ContactTest.kt",
		"android/app/src/androidTest/kotlin/com/x/ScreenTest.kt",
	}, ModeFull, "origin/main")

	unit := stepByLabel(plan, "stress: gradle com.x.ContactTest")
	if unit == nil {
		t.Fatalf("Android unit stress step missing; got %v", labelsFor(plan, "android"))
	}
	if unit.Command.Repeat != 5 {
		t.Errorf("Android unit Repeat = %d, want 5", unit.Command.Repeat)
	}
	if !contains(unit.Command.Argv, ":feature:contacts:testDebugUnitTest") {
		t.Errorf("Android unit argv = %v", unit.Command.Argv)
	}

	inst := stepByLabel(plan, "stress: Android instrumented test android/app/src/androidTest/kotlin/com/x/ScreenTest.kt")
	if inst == nil || inst.Command.Reminder == "" {
		t.Fatalf("Android instrumented reminder missing; got %v", labelsFor(plan, "android"))
	}
	if !contains(plan.Hints, BaselineHints["android"]) {
		t.Errorf("Hints = %v, want android baseline", plan.Hints)
	}
}

func TestBuildStressRootGoTest(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{"backend/root_test.go"}, ModeFull, "origin/main")
	if stepByLabel(plan, "stress: go test ./") == nil {
		t.Fatalf("root package stress step missing; got %v", labelsFor(plan, "backend"))
	}
}

func TestAndroidTestTargetFallbacks(t *testing.T) {
	if module, class := androidTestTarget("android/app/src/test/java/com/x/A.java"); module != ":app" || class != "com.x.A" {
		t.Errorf("java target = (%q, %q)", module, class)
	}
	if module, class := androidTestTarget("android/app/src/test/kotlin/A.kt"); module != ":app" || class != "A" {
		t.Errorf("file without package dirs = (%q, %q)", module, class)
	}
	if module, class := androidTestTarget("android/src/test/kotlin/A.kt"); module != "" || class != "" {
		t.Errorf("missing module = (%q, %q)", module, class)
	}
	if module, class := androidTestTarget("android/app/src/test/resources/A.kt"); module != ":app" || class != "" {
		t.Errorf("no kotlin/java dir = (%q, %q)", module, class)
	}
}

func TestExecutePrintsReminderNotesAndHints(t *testing.T) {
	plan := &Plan{
		Mode:      ModeFast,
		Steps:     []Step{{Area: "android", Command: Command{Label: "reminder", Reminder: "run it on an emulator"}}},
		Skipped:   []Skip{{Area: "infra", Reason: "CI-only"}},
		Notes:     []string{"openapi implies backend"},
		Unmatched: []string{"frontend", "android"},
		Hints:     []string{"cd backend && make gen-coverage-baseline"},
	}
	var out bytes.Buffer
	if code := Execute(plan, &out, func(Step) error { return nil }); code != 0 {
		t.Fatalf("Execute = %d, want 0", code)
	}
	for _, want := range []string{"run it on an emulator", "skipped infra: CI-only", "openapi implies backend", "no changed paths: frontend, android", "make gen-coverage-baseline"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestLoadFiltersMoreMalformed(t *testing.T) {
	for _, doc := range []string{
		"backend:\n  - [a]\n",
		"? [a, b]\n: [x]\n",
		"{}\n",
	} {
		if _, err := LoadFilters(strings.NewReader(doc)); err == nil {
			t.Errorf("LoadFilters(%q) succeeded, want error", doc)
		}
	}
}

func TestMatchQuestionMark(t *testing.T) {
	if !Match("a?c", "abc") {
		t.Error("? should match one non-slash character")
	}
	if Match("a?c", "a/c") {
		t.Error("? must not match a slash")
	}
}

func TestBuildUnknownFilterIsSkipped(t *testing.T) {
	f, err := LoadFilters(strings.NewReader("unknown:\n  - 'unknown/**'\n"))
	if err != nil {
		t.Fatalf("LoadFilters: %v", err)
	}
	plan := Build(f, []string{"unknown/x.txt"}, ModeFast, "origin/main")
	if len(plan.Skipped) != 1 || plan.Skipped[0].Area != "unknown" {
		t.Fatalf("Skipped = %v, want the unknown filter", plan.Skipped)
	}
}

func TestBuildAndroidTestWithoutModuleIsIgnored(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{"android/src/test/kotlin/A.kt"}, ModeFull, "origin/main")
	if got := countStress(plan, "android"); got != 0 {
		t.Errorf("stress steps = %d, want 0", got)
	}
}

func TestBuildDeduplicatesChangedTests(t *testing.T) {
	f := loadFixture(t)
	plan := Build(f, []string{
		"frontend/e2e/auth.spec.ts",
		"frontend/e2e/auth.spec.ts",
	}, ModeFull, "origin/main")
	if got := countStress(plan, "frontend"); got != 1 {
		t.Errorf("frontend stress steps = %d, want 1", got)
	}
}

func TestExecuteFailureWithRootDir(t *testing.T) {
	plan := &Plan{Mode: ModeFast, Steps: []Step{
		{Area: "docs", Command: Command{Label: "fail", Argv: []string{"false"}}},
	}}
	var out bytes.Buffer
	if code := Execute(plan, &out, func(Step) error { return os.ErrNotExist }); code != 1 {
		t.Fatalf("Execute = %d, want 1", code)
	}
	if !strings.Contains(out.String(), "(cd . && false)") {
		t.Errorf("output missing root-dir command:\n%s", out.String())
	}
}
