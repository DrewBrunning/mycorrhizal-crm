// Package releaseworkflow checks structural invariants of release.yml that the
// YAML schema cannot express but ADR 0021 relies on (issue #1164): a cut must
// serialize against another cut of the same version, and the release-readiness
// record must be emitted. `cmd/releasegatecheck` runs these against the
// committed workflow so a refactor cannot quietly drop either.
package releaseworkflow

import (
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

type workflow struct {
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress any    `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Steps []struct {
			Name string `yaml:"name"`
			With struct {
				Name string `yaml:"name"`
			} `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// CheckRelease reports structural problems with release.yml. text is the raw
// file contents.
func CheckRelease(text string) []string {
	var w workflow
	if err := yaml.Unmarshal([]byte(text), &w); err != nil {
		return []string{fmt.Sprintf("release.yml does not parse: %v", err)}
	}

	var findings []string

	// ADR 0021 (issue #1164): two overlapping cuts of one version must not
	// interleave. The concurrency group is keyed on the version input and never
	// cancels an in-flight cut (cancellation would drop a legitimate tag).
	if !strings.Contains(w.Concurrency.Group, "inputs.version") {
		findings = append(findings, "release.yml concurrency.group must include inputs.version so two cuts of one version serialize (issue #1164)")
	}
	if cancel, ok := w.Concurrency.CancelInProgress.(bool); ok && cancel {
		findings = append(findings, "release.yml concurrency.cancel-in-progress must be false; a cut must never cancel another (issue #1164)")
	}

	// ADR 0021 (issue #1164): the durable readiness record must be uploaded,
	// named per version so a re-dispatch can find it.
	ready := false
	for _, j := range w.Jobs {
		for _, s := range j.Steps {
			if strings.HasPrefix(s.Name, "Upload release-readiness artifact") &&
				strings.Contains(s.With.Name, "release-readiness-") {
				ready = true
			}
		}
	}
	if !ready {
		findings = append(findings, "release.yml must upload a release-readiness-<version> artifact (issue #1164)")
	}

	return findings
}
