package agenteval

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CommandResult is one suite command (or the agent invocation) with a bounded
// output tail.
type CommandResult struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out"`
	Passed     bool   `json:"passed"`
	OutputTail string `json:"output_tail,omitempty"`
	Output     string `json:"-"`
}

// RunResult is one worktree's score and evidence.
type RunResult struct {
	Status              string          `json:"status"` // passed | failed | error
	Agent               CommandResult   `json:"agent"`
	SuiteCommands       []CommandResult `json:"suite_commands"`
	ChangedFiles        []string        `json:"changed_files"`
	ScopeViolations     []string        `json:"scope_violations,omitempty"`
	HandEditedGenerated []string        `json:"hand_edited_generated,omitempty"`
	ExpectedFindingsOK  bool            `json:"expected_findings_ok"`
	MissingFindings     []string        `json:"missing_findings,omitempty"`
	Success             bool            `json:"success"`
}

// ControlDelta records how the stripped-context control differed from the run
// with the full context.
type ControlDelta struct {
	ContextSuccess      bool   `json:"context_success"`
	ControlSuccess      bool   `json:"control_success"`
	CommandPassDelta    int    `json:"command_pass_delta"`
	ScopeViolationDelta int    `json:"scope_violation_delta"`
	HandEditDelta       int    `json:"hand_edit_delta"`
	ChangedFileDelta    int    `json:"changed_file_delta"`
	Summary             string `json:"summary"`
}

// Artifact is the JSON contract the workflow uploads.
type Artifact struct {
	Task         string              `json:"task"`
	BaseRef      string              `json:"base_ref"`
	Profile      string              `json:"profile,omitempty"`
	AgentName    string              `json:"agent_name,omitempty"`
	AgentModel   string              `json:"agent_model,omitempty"`
	AgentVersion string              `json:"agent_version,omitempty"`
	AgentCommand string              `json:"agent_command"`
	Status       string              `json:"status"` // passed | failed | skipped | error
	Reason       string              `json:"reason,omitempty"`
	StartedAt    time.Time           `json:"started_at"`
	FinishedAt   time.Time           `json:"finished_at"`
	DurationMS   int64               `json:"duration_ms"`
	Stripped     []string            `json:"stripped,omitempty"`
	Context      *RunResult          `json:"context,omitempty"`
	Control      *RunResult          `json:"control,omitempty"`
	Delta        *ControlDelta       `json:"delta,omitempty"`
	Baseline     *BaselineComparison `json:"baseline,omitempty"`
}

// GeneratedArtifact is one committed artifact and the editable sources it is
// derived from. Generator is a backend/Makefile target.
type GeneratedArtifact struct {
	Path      string
	Generator string
	Sources   []string
}

// GeneratedArtifacts mirrors backend/Makefile's generator inventory. Each row
// is the generator target plus the output it owns and the canonical sources a
// legitimate regeneration must touch.
var GeneratedArtifacts = []GeneratedArtifact{
	{Path: "backend/testdata/contract-fixtures/", Generator: "gen-contract-fixtures", Sources: []string{"backend/openapi.yaml"}},
	{Path: "frontend/src/generated/openapi.ts", Generator: "gen-ts-api", Sources: []string{"backend/openapi.yaml"}},
	{Path: "backend/internal/apibaseline/testdata/v1.json", Generator: "gen-api-baseline", Sources: []string{"backend/openapi.yaml", "backend/routes/", "backend/controllers/"}},
	{Path: "backend/database/testdata/schemas/", Generator: "gen-schema-fixtures", Sources: []string{"backend/database/migrations/", "backend/internal/schemafixture/releases.go"}},
	{Path: "docs/data-01-field-compatibility-matrix.md", Generator: "gen-compat-matrix", Sources: []string{"backend/correspondence/testdata/correspondence.tsv"}},
	{Path: "docs/int-01-integration-classification-matrix.md", Generator: "gen-integration-matrix", Sources: []string{"backend/integrations/"}},
	{Path: "docs/development/perf-benchmarks.md", Generator: "gen-perf-baseline", Sources: []string{"backend/internal/perfbench/", "backend/internal/largedata/"}},
	{Path: "docs/development/data-movement-benchmarks.md", Generator: "gen-perf-baseline", Sources: []string{"backend/internal/perfbench/", "backend/internal/largedata/"}},
	{Path: "backend/internal/perfbench/testdata/baseline.json", Generator: "gen-perf-baseline", Sources: []string{"backend/internal/perfbench/", "backend/internal/largedata/"}},
	{Path: "backend/internal/perfbench/testdata/datamovement-baseline.json", Generator: "gen-perf-baseline", Sources: []string{"backend/internal/perfbench/", "backend/internal/largedata/"}},
	{Path: "backend/internal/perfbench/testdata/wallclock-trend.json", Generator: "gen-perf-baseline", Sources: []string{"backend/internal/perfbench/", "backend/internal/largedata/"}},
	{Path: "backend/.gremlins/", Generator: "gen-mutation-scope", Sources: []string{"backend/internal/mutationscope/"}},
	{Path: "docs/configuration-reference.md", Generator: "gen-config-reference", Sources: []string{"backend/config/"}},
	{Path: "THIRD_PARTY_NOTICES.md", Generator: "gen-notices", Sources: []string{"backend/go.mod", "backend/go.sum", "frontend/package.json", "frontend/yarn.lock"}},
	{Path: "backend/internal/coverageratchet/testdata/baseline.json", Generator: "gen-coverage-baseline", Sources: []string{"backend/internal/coverageratchet/", "backend/**/*_test.go", "backend/internal/**/*_test.go"}},
}

// ClassifyPaths returns one violation per changed file that is forbidden or
// outside the task's expected paths. An empty change set is allowed.
func ClassifyPaths(changed, expect, forbid []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, path := range changed {
		path = filepath.ToSlash(path)
		forbidden := false
		for _, pattern := range forbid {
			if MatchPath(pattern, path) {
				forbidden = true
				break
			}
		}
		if forbidden {
			if !seen[path] {
				out = append(out, "forbidden path: "+path)
				seen[path] = true
			}
			continue
		}
		allowed := false
		for _, pattern := range expect {
			if MatchPath(pattern, path) {
				allowed = true
				break
			}
		}
		if !allowed && !seen[path] {
			out = append(out, "outside expected paths: "+path)
			seen[path] = true
		}
	}
	sort.Strings(out)
	return out
}

// DetectHandEdits flags a changed generated artifact when none of its editable
// sources changed. A legitimate regeneration always has a source diff beside
// the generated diff.
func DetectHandEdits(changed []string, artifacts []GeneratedArtifact) []string {
	changed = SortStrings(unique(changed))
	var out []string
	for _, art := range artifacts {
		if !anyMatchesArtifact(changed, art.Path) {
			continue
		}
		sourceChanged := false
		for _, src := range art.Sources {
			for _, path := range changed {
				if MatchPath(src, path) || strings.HasPrefix(path, strings.TrimSuffix(src, "/")+"/") {
					sourceChanged = true
					break
				}
			}
			if sourceChanged {
				break
			}
		}
		if !sourceChanged {
			out = append(out, fmt.Sprintf("hand-edited generated artifact: %s (regenerate with `make %s`)", art.Path, art.Generator))
		}
	}
	sort.Strings(out)
	return out
}

func anyMatchesArtifact(changed []string, artifact string) bool {
	for _, path := range changed {
		if artifact == path || strings.HasPrefix(path, strings.TrimSuffix(artifact, "/")+"/") {
			return true
		}
	}
	return false
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// VerifyGeneratorTargets reports any generated-artifact row whose Makefile
// target no longer exists. It keeps the hand-edit map honest as generators move.
func VerifyGeneratorTargets(root string, artifacts []GeneratedArtifact) ([]string, error) {
	body, err := os.ReadFile(filepath.Join(root, "backend", "Makefile")) // #nosec G304 -- fixed repository path
	if err != nil {
		return nil, err
	}
	targets := map[string]bool{}
	re := regexp.MustCompile(`(?m)^([A-Za-z0-9_-]+):`)
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		targets[m[1]] = true
	}
	var missing []string
	for _, art := range artifacts {
		if !targets[art.Generator] {
			missing = append(missing, art.Generator)
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// Score computes Success from the deterministic evidence.
func Score(run *RunResult) {
	run.Success = run.Status == "passed"
	for _, cmd := range run.SuiteCommands {
		if !cmd.Passed {
			run.Success = false
		}
	}
	if len(run.ScopeViolations) > 0 || len(run.HandEditedGenerated) > 0 || len(run.MissingFindings) > 0 {
		run.Success = false
	}
}

// Compare produces the context-vs-control delta.
func Compare(contextRun, controlRun *RunResult) ControlDelta {
	d := ControlDelta{}
	if contextRun != nil {
		d.ContextSuccess = contextRun.Success
		d.CommandPassDelta = passedCount(contextRun)
		d.ScopeViolationDelta = len(contextRun.ScopeViolations)
		d.HandEditDelta = len(contextRun.HandEditedGenerated)
		d.ChangedFileDelta = len(contextRun.ChangedFiles)
	}
	if controlRun != nil {
		d.ControlSuccess = controlRun.Success
		d.CommandPassDelta -= passedCount(controlRun)
		d.ScopeViolationDelta -= len(controlRun.ScopeViolations)
		d.HandEditDelta -= len(controlRun.HandEditedGenerated)
		d.ChangedFileDelta -= len(controlRun.ChangedFiles)
	}
	switch {
	case d.ContextSuccess && !d.ControlSuccess:
		d.Summary = "context passed; control failed"
	case !d.ContextSuccess && d.ControlSuccess:
		d.Summary = "control passed; context failed"
	case d.ContextSuccess && d.ControlSuccess:
		d.Summary = "both passed"
	default:
		d.Summary = "both failed"
	}
	return d
}

func passedCount(r *RunResult) int {
	n := 0
	for _, cmd := range r.SuiteCommands {
		if cmd.Passed {
			n++
		}
	}
	return n
}

// Markdown renders the human summary uploaded beside the JSON artifact.
func Markdown(a Artifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent eval: %s\n\n", a.Task)
	fmt.Fprintf(&b, "- Status: **%s**\n", a.Status)
	if a.Reason != "" {
		fmt.Fprintf(&b, "- Reason: %s\n", a.Reason)
	}
	fmt.Fprintf(&b, "- Base ref: `%s`\n", a.BaseRef)
	if a.Profile != "" {
		fmt.Fprintf(&b, "- Profile: `%s`\n", a.Profile)
	}
	fmt.Fprintf(&b, "- Agent: %s / %s / %s\n", a.AgentName, a.AgentModel, a.AgentVersion)
	fmt.Fprintf(&b, "- Command: `%s`\n", a.AgentCommand)
	fmt.Fprintf(&b, "- Duration: %dms\n\n", a.DurationMS)
	if a.Context != nil {
		b.WriteString("## Context run\n\n")
		b.WriteString(renderRun(*a.Context))
	}
	if a.Control != nil {
		b.WriteString("\n## Control run (stripped context)\n\n")
		b.WriteString(renderRun(*a.Control))
	}
	if a.Delta != nil {
		fmt.Fprintf(&b, "\n## Delta\n\n- %s\n- command pass delta: %d\n- scope violation delta: %d\n- hand-edit delta: %d\n- changed-file delta: %d\n",
			a.Delta.Summary, a.Delta.CommandPassDelta, a.Delta.ScopeViolationDelta, a.Delta.HandEditDelta, a.Delta.ChangedFileDelta)
	}
	if a.Baseline != nil {
		fmt.Fprintf(&b, "\n## Baseline\n\n- Profile/task: %s / %s\n", a.Baseline.Profile, a.Baseline.Task)
		if !a.Baseline.Present {
			b.WriteString("- No committed baseline entry\n")
		} else {
			b.WriteString("- Committed baseline present\n")
		}
		for _, r := range a.Baseline.Regressions {
			fmt.Fprintf(&b, "- Regression: %s\n", r)
		}
	}
	if len(a.Stripped) > 0 {
		fmt.Fprintf(&b, "\nStripped for control: %s\n", strings.Join(a.Stripped, ", "))
	}
	return b.String()
}

func renderRun(r RunResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Success: **%v** (status %s)\n", r.Success, r.Status)
	fmt.Fprintf(&b, "- Agent exit: %d (%dms)\n", r.Agent.ExitCode, r.Agent.DurationMS)
	fmt.Fprintf(&b, "- Changed files: %d\n", len(r.ChangedFiles))
	for _, c := range r.SuiteCommands {
		fmt.Fprintf(&b, "- `%s`: %v (%dms)\n", c.Command, c.Passed, c.DurationMS)
	}
	for _, v := range r.ScopeViolations {
		fmt.Fprintf(&b, "- Scope: %s\n", v)
	}
	for _, v := range r.HandEditedGenerated {
		fmt.Fprintf(&b, "- Generated: %s\n", v)
	}
	for _, v := range r.MissingFindings {
		fmt.Fprintf(&b, "- Missing finding: %s\n", v)
	}
	return b.String()
}

// FindingsSatisfied reports which expected findings are absent from output.
// Matching is case-insensitive substring, sufficient for the release-readiness
// judgment task.
func FindingsSatisfied(expected []string, output string) (bool, []string) {
	lower := strings.ToLower(output)
	var missing []string
	for _, want := range expected {
		if !strings.Contains(lower, strings.ToLower(want)) {
			missing = append(missing, want)
		}
	}
	return len(missing) == 0, missing
}
