// Package mainalert checks the structural invariant behind the red-`main`
// failure alarm (issue #1568): every workflow that carries a mandatory per-PR
// gate in .github/release-gates.json must be listed by `name:` in
// .github/workflows/main-failure-alert.yml's `on.workflow_run.workflows`
// array, or a red post-merge `push` run of that workflow alerts no one.
//
// A PR that merges before its required checks finish (or an admin bypass)
// leaves the post-merge `main` push run as the only verification the change
// ever gets. Unlike the nightly tier, nothing watched that run before this
// workflow existed -- a red `main` was visible only to whoever opened the
// Actions tab. main-failure-alert.yml is the alarm, and this package is the
// drift gate that keeps its registration list honest as the per-PR set
// changes.
package mainalert

import (
	"fmt"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"

	"mycorrhizal/internal/releasegates"
)

// AlertWorkflowFile is the path (relative to the workflows directory) of the
// alert workflow whose `workflows:` list this package cross-checks. It is
// deliberately separate from nightly-failure-alert.yml: that file's
// `workflows:` list must equal the set of scheduled workflows (enforced by
// cmd/nightlyalertcheck), and its security posture depends on reacting only
// to `schedule` runs.
const AlertWorkflowFile = "main-failure-alert.yml"

// RegistryFile is the path (relative to the repository root) of the
// machine-readable release-gate registry this package derives the expected
// workflow set from.
const RegistryFile = ".github/release-gates.json"

type workflowDoc struct {
	Name string `yaml:"name"`
	On   struct {
		WorkflowRun struct {
			Workflows []string `yaml:"workflows"`
		} `yaml:"workflow_run"`
	} `yaml:"on"`
}

// ExpectedWorkflowNames returns the distinct `name:` of every workflow named
// by a mandatory per-pr gate in reg. files maps a workflow filename (as it
// appears in a gate's `workflow` field) to that file's raw YAML. A gate whose
// workflow file is missing, unparseable or nameless is reported as a finding
// rather than silently dropping from the expected set.
func ExpectedWorkflowNames(reg releasegates.Registry, files map[string][]byte) (names, findings []string) {
	var gateFiles []string
	seenFile := map[string]bool{}
	for _, g := range reg.Gates {
		if g.Tier != "per-pr" || !g.Mandatory || g.Workflow == "" {
			continue
		}
		if !seenFile[g.Workflow] {
			seenFile[g.Workflow] = true
			gateFiles = append(gateFiles, g.Workflow)
		}
	}
	sort.Strings(gateFiles)

	seenName := map[string]bool{}
	for _, f := range gateFiles {
		raw, ok := files[f]
		if !ok {
			findings = append(findings, fmt.Sprintf(
				"a mandatory per-pr gate names workflow %q, but .github/workflows/%s does not exist -- %s cannot alert on it",
				f, f, AlertWorkflowFile))
			continue
		}
		var doc workflowDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			findings = append(findings, fmt.Sprintf("%s does not parse: %v", f, err))
			continue
		}
		if strings.TrimSpace(doc.Name) == "" {
			findings = append(findings, f+" carries a mandatory per-pr gate but has no name:")
			continue
		}
		if !seenName[doc.Name] {
			seenName[doc.Name] = true
			names = append(names, doc.Name)
		}
	}
	sort.Strings(names)
	return names, findings
}

// RegisteredWorkflowNames parses the alert workflow's raw YAML (the contents
// of main-failure-alert.yml) and returns its `on.workflow_run.workflows` list.
func RegisteredWorkflowNames(alertWorkflowYAML []byte) ([]string, error) {
	var doc workflowDoc
	if err := yaml.Unmarshal(alertWorkflowYAML, &doc); err != nil {
		return nil, fmt.Errorf("%s does not parse: %w", AlertWorkflowFile, err)
	}
	out := append([]string(nil), doc.On.WorkflowRun.Workflows...)
	sort.Strings(out)
	return out, nil
}

// CheckDrift returns one finding per expected workflow whose `name:` is absent
// from the alert workflow's registration list. The reverse direction is
// deliberately not reported: the list is allowed to carry release-relevant
// workflows beyond the mandatory per-pr set (e.g. ARM64 Tests, Docker Build
// Check), so an extra entry is not stale by construction.
func CheckDrift(expected, registered []string) []string {
	registeredSet := map[string]bool{}
	for _, n := range registered {
		registeredSet[n] = true
	}
	var findings []string
	for _, n := range expected {
		if !registeredSet[n] {
			findings = append(findings, fmt.Sprintf(
				"%s does not list %q in on.workflow_run.workflows, but a workflow with that name carries a mandatory per-pr gate -- a red push:main run of it raises no alert (issue #1568)",
				AlertWorkflowFile, n))
		}
	}
	sort.Strings(findings)
	return findings
}
