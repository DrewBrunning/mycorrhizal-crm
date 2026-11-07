package covgate

import (
	"fmt"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// UploadSuiteFilters are the .github/filters.yaml names whose match makes a
// coverage-uploading suite run. They are the same five filters the
// `codecov-patch-stub` job in unit-tests.yml requires to be false before it
// posts its no-op statuses; codecovcheck keeps the two lists equal.
var UploadSuiteFilters = []string{"backend", "frontend", "android", "openapi", "workflows"}

// TriggersCoverageUpload reports whether any changed file matches one of the
// UploadSuiteFilters in filtersYAML (the contents of .github/filters.yaml).
// It is the trusted re-derivation of `Detect Changes` used by the
// workflow_run poster for PRs whose own pull_request run cannot post commit
// statuses (fork and Dependabot PRs): it must not trust anything the PR's
// own workflow run emitted.
//
// Fail-safe: an empty file list returns true (assume an upload may happen),
// and a filters file missing any upload-suite filter is an error rather than
// a filter that silently never matches.
func TriggersCoverageUpload(filtersYAML []byte, files []string) (bool, error) {
	var filters map[string][]string
	if err := yaml.Unmarshal(filtersYAML, &filters); err != nil {
		return false, fmt.Errorf(".github/filters.yaml does not parse as YAML: %w", err)
	}
	for _, name := range UploadSuiteFilters {
		if _, ok := filters[name]; !ok {
			return false, fmt.Errorf(".github/filters.yaml has no %q filter", name)
		}
	}
	if len(files) == 0 {
		return true, nil
	}
	for _, name := range UploadSuiteFilters {
		for _, pattern := range filters[name] {
			for _, f := range files {
				if matchGlob(pattern, f) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// matchGlob matches a repo-relative path against a filters.yaml pattern with
// the subset of picomatch semantics the file uses: `**` crosses directories
// (and `**/` also matches zero directories), `*` and `?` stay within one
// path segment, and dotfiles match (paths-filter runs with dot: true).
func matchGlob(pattern, path string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String()).MatchString(path)
}

// PosterWorkflowFile is the workflow_run poster for PRs whose own run cannot
// post commit statuses (fork and Dependabot PRs).
const PosterWorkflowFile = ".github/workflows/codecov-patch-stub-workflow-run.yml"

type posterWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Env map[string]string `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// ExtractPosterAreas pulls the PATCH_AREAS env of the `post` job in
// PosterWorkflowFile. Like the unit-tests.yml stub, it must list exactly
// codecov.yml's patch areas (CrossCheckStubAreas compares them).
func ExtractPosterAreas(workflow []byte) ([]string, []string) {
	var wf posterWorkflow
	if err := yaml.Unmarshal(workflow, &wf); err != nil {
		return nil, []string{fmt.Sprintf("%s does not parse as YAML: %v", PosterWorkflowFile, err)}
	}
	job, ok := wf.Jobs["post"]
	if !ok {
		return nil, []string{PosterWorkflowFile + " has no jobs.post job (issue #1188)"}
	}
	for _, step := range job.Steps {
		if areas, ok := step.Env["PATCH_AREAS"]; ok {
			fields := strings.Fields(areas)
			if len(fields) == 0 {
				return nil, []string{"codecov-patch-stub-workflow-run's PATCH_AREAS env is empty (issue #1188)"}
			}
			return fields, nil
		}
	}
	return nil, []string{PosterWorkflowFile + " post job has no step with a PATCH_AREAS env (issue #1188)"}
}

// CheckStubGuardsCoverUploadFilters asserts unit-tests.yml's
// `codecov-patch-stub` job `if:` requires every UploadSuiteFilters output to
// be false. The poster derives its verdict from that same list, so a filter
// dropped from one side only would make the two paths disagree about which
// PRs upload coverage.
func CheckStubGuardsCoverUploadFilters(workflow []byte) []string {
	var wf struct {
		Jobs map[string]struct {
			If string `yaml:"if"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(workflow, &wf); err != nil {
		return []string{fmt.Sprintf(".github/workflows/unit-tests.yml does not parse as YAML: %v", err)}
	}
	job, ok := wf.Jobs["codecov-patch-stub"]
	if !ok {
		return []string{".github/workflows/unit-tests.yml has no jobs.codecov-patch-stub job (issue #1188)"}
	}
	var findings []string
	for _, name := range UploadSuiteFilters {
		if !strings.Contains(job.If, "needs.changes.outputs."+name+" != 'true'") {
			findings = append(findings, fmt.Sprintf("codecov-patch-stub's if: does not require needs.changes.outputs.%s != 'true', but covgate.UploadSuiteFilters (the workflow_run poster's verdict) includes %q (issue #1188)", name, name))
		}
	}
	return findings
}
