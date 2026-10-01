package releasegates

import (
	"fmt"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// Issue #1396: docker-publish.yml's documented manual fallback is a
// workflow_dispatch run. A job guarded by `github.event_name == 'push'` is
// SKIPPED on that path, and a skipped job does not fail the run — so a
// mandatory release-internal gate with a push-only guard silently vanishes on
// the fallback while the run concludes success. CheckDispatchPath makes that
// a build failure.

// dispatchWorkflow is the one workflow whose dispatch fallback is policed.
const dispatchWorkflow = "docker-publish.yml"

// pushGuard is the substring that marks a job as push-only.
const pushGuard = "github.event_name == 'push'"

// PushOnlyAllowance records why a mandatory gate may legitimately be
// push-only AND which job makes its absence a defined failure on dispatch.
type PushOnlyAllowance struct {
	// Reason is the written-down justification.
	Reason string
	// Compensator is the job that must run on dispatch (no push guard) and
	// fail when this gate did not run.
	Compensator string
	// Marker is a string the Compensator's step scripts must contain together
	// with `::error::` — the assertion that turns the gate's absence red.
	Marker string
}

// PushOnlyAllowlist is deliberately tiny. Adding an entry is a reviewed
// decision: it needs a reason and a failing check on the dispatch path.
var PushOnlyAllowlist = map[string]PushOnlyAllowance{
	"apk-provenance": {
		Reason: "the slsa-github-generator records the triggering ref, and a dispatch from a branch is a branch ref that `slsa-verifier --source-tag` rejects",
		// verify-release-assets asserts mycorrhizal-apk.intoto.jsonl is on the
		// Release; with no provenance on dispatch the run concludes red.
		Compensator: "verify-release-assets",
		Marker:      "mycorrhizal-apk.intoto.jsonl",
	},
}

type dispatchWorkflowDoc struct {
	Jobs map[string]struct {
		If    string `yaml:"if"`
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// CheckDispatchPath reports every mandatory release-internal gate of
// docker-publish.yml that carries a push-only guard without a matching
// allowlist entry plus a defined failing check on the dispatch path, and every
// allowlist entry whose compensating check is missing, push-guarded itself, or
// no longer asserts. workflowText is docker-publish.yml's content.
func CheckDispatchPath(reg Registry, workflowText string) []string {
	var doc dispatchWorkflowDoc
	if err := yaml.Unmarshal([]byte(workflowText), &doc); err != nil {
		return []string{fmt.Sprintf("%s does not parse as YAML: %v", dispatchWorkflow, err)}
	}
	var findings []string
	var names []string
	for _, g := range reg.Gates {
		if g.Workflow == dispatchWorkflow && g.Tier == "release-internal" && g.Mandatory {
			names = append(names, g.Name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		job, ok := doc.Jobs[name]
		if !ok {
			findings = append(findings, fmt.Sprintf(
				"mandatory gate %q names no job in %s (issue #1396: its dispatch-path behaviour cannot be checked)", name, dispatchWorkflow))
			continue
		}
		if !strings.Contains(job.If, pushGuard) {
			continue
		}
		allow, listed := PushOnlyAllowlist[name]
		if !listed {
			findings = append(findings, fmt.Sprintf(
				"mandatory gate %q in %s is guarded by `%s`: it is skipped on the workflow_dispatch fallback while the run still concludes success (issue #1396). Remove the guard, or add a reasoned releasegates.PushOnlyAllowlist entry with a failing dispatch-path check",
				name, dispatchWorkflow, pushGuard))
			continue
		}
		comp, ok := doc.Jobs[allow.Compensator]
		switch {
		case !ok:
			findings = append(findings, fmt.Sprintf(
				"push-only gate %q relies on job %q to fail the dispatch path, but %s has no such job (issue #1396)", name, allow.Compensator, dispatchWorkflow))
		case strings.Contains(comp.If, pushGuard):
			findings = append(findings, fmt.Sprintf(
				"push-only gate %q relies on job %q to fail the dispatch path, but that job is itself push-only (issue #1396)", name, allow.Compensator))
		default:
			asserts := false
			for _, s := range comp.Steps {
				if strings.Contains(s.Run, allow.Marker) && strings.Contains(s.Run, "::error::") {
					asserts = true
					break
				}
			}
			if !asserts {
				findings = append(findings, fmt.Sprintf(
					"push-only gate %q relies on job %q to fail the dispatch path, but no step of that job asserts %q with an ::error:: (issue #1396)", name, allow.Compensator, allow.Marker))
			}
		}
	}
	return findings
}
