// Package nightlyalert checks the structural invariant behind
// nightly-failure-alert.yml: every scheduled workflow under
// .github/workflows/ must be listed by name in that workflow's
// `on.workflow_run.workflows` array, or a nightly (or weekly/monthly)
// regression on the un-registered workflow raises no alert.
//
// Nightly is the only zero-retry CI tier here (flakes on the PR/push path get
// retried; a scheduled run does not), and it is where mutation testing,
// large-dataset benchmarks, chaos injection, and the real-server CardDAV
// suite all live. A schedule trigger with nobody watching its result is a
// blind spot, so this package's check fails CI the moment a scheduled
// workflow is added (or renamed) without also updating the alert workflow's
// registration list.
package nightlyalert

import (
	"fmt"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// alertWorkflowFile is the path (relative to the workflows directory) of the
// alert workflow whose `workflows:` list this package cross-checks.
const AlertWorkflowFile = "nightly-failure-alert.yml"

type onBlock struct {
	Schedule    []any `yaml:"schedule"`
	WorkflowRun struct {
		Workflows []string `yaml:"workflows"`
		Types     []string `yaml:"types"`
	} `yaml:"workflow_run"`
}

type workflowFile struct {
	Name string  `yaml:"name"`
	On   onBlock `yaml:"on"`
}

// ScheduledWorkflowNames parses every file in files (path -> raw YAML bytes,
// path being the workflow's filename such as "unit-tests.yml") and returns
// the `name:` of every workflow that declares a top-level `schedule:`
// trigger. A parse failure is reported as a finding, not a panic.
func ScheduledWorkflowNames(files map[string][]byte) (names []string, findings []string) {
	seen := map[string]bool{}
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		var wf workflowFile
		if err := yaml.Unmarshal(files[p], &wf); err != nil {
			findings = append(findings, fmt.Sprintf("%s does not parse: %v", p, err))
			continue
		}
		if len(wf.On.Schedule) == 0 {
			continue
		}
		if strings.TrimSpace(wf.Name) == "" {
			findings = append(findings, p+" has a schedule: trigger but no name:")
			continue
		}
		if !seen[wf.Name] {
			seen[wf.Name] = true
			names = append(names, wf.Name)
		}
	}
	sort.Strings(names)
	return names, findings
}

// RegisteredWorkflowNames parses the alert workflow's raw YAML (the contents
// of nightly-failure-alert.yml) and returns its
// `on.workflow_run.workflows` list.
func RegisteredWorkflowNames(alertWorkflowYAML []byte) ([]string, error) {
	var wf workflowFile
	if err := yaml.Unmarshal(alertWorkflowYAML, &wf); err != nil {
		return nil, fmt.Errorf("%s does not parse: %w", AlertWorkflowFile, err)
	}
	out := append([]string(nil), wf.On.WorkflowRun.Workflows...)
	sort.Strings(out)
	return out, nil
}

// CheckDrift compares the two lists and returns one finding per mismatch:
// a scheduled workflow the alert file does not list, and a name the alert
// file lists that no longer has a schedule: trigger (a stale/renamed entry
// that alerts on nothing, or was never real).
func CheckDrift(scheduled, registered []string) []string {
	scheduledSet := map[string]bool{}
	for _, n := range scheduled {
		scheduledSet[n] = true
	}
	registeredSet := map[string]bool{}
	for _, n := range registered {
		registeredSet[n] = true
	}

	var findings []string
	for _, n := range scheduled {
		if !registeredSet[n] {
			findings = append(findings, fmt.Sprintf(
				"%s does not list %q in on.workflow_run.workflows, but that workflow has a schedule: trigger -- a nightly failure on it raises no alert",
				AlertWorkflowFile, n))
		}
	}
	for _, n := range registered {
		if !scheduledSet[n] {
			findings = append(findings, fmt.Sprintf(
				"%s lists %q in on.workflow_run.workflows, but no workflow with that name has a schedule: trigger anymore -- stale entry",
				AlertWorkflowFile, n))
		}
	}
	sort.Strings(findings)
	return findings
}
