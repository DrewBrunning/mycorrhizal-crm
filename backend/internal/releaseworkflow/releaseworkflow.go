// Package releaseworkflow checks structural invariants of release.yml and
// promote-rc.yml that the YAML schema cannot express but ADR 0021 relies on: a
// cut must serialize against another cut of the same version, the
// release-readiness record must be emitted (issue #1164), and both
// workflows must run the final-release-only obligations through the shared
// script (issue #1195). `cmd/releasegatecheck` runs these against the committed
// workflows so a refactor cannot quietly drop any of them.
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
			Run  string `yaml:"run"`
			With struct {
				Name string `yaml:"name"`
			} `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// obligationsScript is the one definition of the two final-release-only gates
// (issue #1195); both release.yml's final path and promote-rc.yml's promotion
// must invoke each subcommand, or an RC-derived final skips them.
const obligationsScript = ".github/scripts/release-obligations.sh"

// checkObligations reports a workflow that fails to invoke either obligation.
func checkObligations(name string, w workflow) []string {
	asvs, adversarial := false, false
	for _, j := range w.Jobs {
		for _, s := range j.Steps {
			if strings.Contains(s.Run, obligationsScript+" asvs") {
				asvs = true
			}
			if strings.Contains(s.Run, obligationsScript+" adversarial") {
				adversarial = true
			}
		}
	}
	var findings []string
	if !asvs {
		findings = append(findings, name+" must run `"+obligationsScript+" asvs` -- the ASVS/MASVS re-verification obligation (issue #1195)")
	}
	if !adversarial {
		findings = append(findings, name+" must run `"+obligationsScript+" adversarial` -- the per-release adversarial-delta gate (issue #1195)")
	}
	return findings
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

	// issue #1195: the final-release path must still run both obligations
	// through the shared script after they were factored out of the inline
	// steps.
	findings = append(findings, checkObligations("release.yml", w)...)

	return findings
}

// CheckPromote reports structural problems with promote-rc.yml. text is the raw
// file contents.
//
// issue #1195: release.yml skips the two final-release-only obligations for an
// RC and relies on promotion to run them, so promote-rc.yml must (a) invoke the
// shared release-obligations.sh for both, and (b) declare the ack escape-hatch
// inputs, or an RC-derived final is never gated.
func CheckPromote(text string) []string {
	var w workflow
	if err := yaml.Unmarshal([]byte(text), &w); err != nil {
		return []string{fmt.Sprintf("promote-rc.yml does not parse: %v", err)}
	}

	findings := checkObligations("promote-rc.yml", w)

	if !strings.Contains(text, "ack_asvs_current:") {
		findings = append(findings, "promote-rc.yml must declare the ack_asvs_current input so a promotion records the ASVS escape (issue #1195)")
	}
	if !strings.Contains(text, "ack_adversarial_delta:") {
		findings = append(findings, "promote-rc.yml must declare the ack_adversarial_delta input so a promotion records the delta escape (issue #1195)")
	}

	return findings
}
