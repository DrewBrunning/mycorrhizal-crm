package releaseworkflow

import (
	"fmt"
	"strings"
)

// Issue #1486: a gate only a human can discharge has no CI job that can go red,
// so the ONLY thing standing between "forgot" and "released anyway" is release.yml
// refusing to proceed without an attestation. That refusal is wiring across two
// workflows and one script family, and it rots quietly -- a preflight step that
// gained an `if:`, a `continue-on-error`, an input that stopped being required,
// or a dry-run rehearsal that stopped passing the input all leave the gate
// looking present while enforcing nothing. CheckManualGates pins it.

// Files and names CheckManualGates reads.
const (
	DryRunFile      = "release-dry-run.yml"
	ManualGatesFile = ".github/manual-gates.json"
	manualInput     = "attest_manual_gates"
	manualTool      = "./cmd/manualgatecheck"
	manualFactsSh   = ".github/scripts/manual-gate-facts.sh"
)

// CheckManualGates reports every way the manual-gate enforcement is broken.
// release is release.yml, dryRun is release-dry-run.yml.
func CheckManualGates(release, dryRun string) []string {
	rel, errs := parseDoc(ReleaseFile, release)
	if errs != nil {
		return errs
	}
	dry, errs := parseDoc(DryRunFile, dryRun)
	if errs != nil {
		return errs
	}
	var findings []string

	input := asMap(asMap(asMap(asMap(rel["on"])["workflow_dispatch"])["inputs"])[manualInput])
	switch {
	case input == nil:
		findings = append(findings, ReleaseFile+" must declare the `"+manualInput+"` dispatch input (issue #1486)")
	case asStr(input["required"]) != "true":
		findings = append(findings, ReleaseFile+" input `"+manualInput+"` must be `required: true`: an optional input lets a dispatch forget the manual gate silently (issue #1486)")
	}

	pre := jobOf(rel, "preflight")
	var step anyMap
	for _, s := range stepsOf(pre) {
		if asStr(s["id"]) == "manual" {
			step = s
		}
	}
	if step == nil {
		findings = append(findings, ReleaseFile+" preflight needs a step `manual` that enforces the manual-gate attestations (issue #1486)")
	} else {
		run := asStr(step["run"])
		if !strings.Contains(run, "go run "+manualTool+" check") {
			findings = append(findings, ReleaseFile+" preflight step `manual` must enforce with `go run "+manualTool+" check` (issue #1486)")
		}
		if !strings.Contains(run, manualFactsSh) {
			findings = append(findings, ReleaseFile+" preflight step `manual` must compute the git facts (attested commit ancestry + changed paths) with "+manualFactsSh+" (issue #1486)")
		}
		if !strings.Contains(run, "-facts ") || !strings.Contains(run, `-kind "$kind"`) {
			findings = append(findings, ReleaseFile+" preflight step `manual` must pass the git facts and the release kind (-facts, -kind) to the check, or a stale attestation and an RC/final mix-up pass unnoticed (issue #1486)")
		}
		if strings.Contains(run, "inputs."+manualInput) {
			findings = append(findings, ReleaseFile+" preflight step `manual` interpolates inputs."+manualInput+" into the script; pass it through `env:` (a skip reason is free text -- script injection) (issue #1486)")
		}
		if !strings.Contains(asStr(asMap(step["env"])["ATTEST_MANUAL_GATES"]), "inputs."+manualInput) {
			findings = append(findings, ReleaseFile+" preflight step `manual` must take ATTEST_MANUAL_GATES from `inputs."+manualInput+"` (issue #1486)")
		}
		if asStr(step["if"]) != "" {
			findings = append(findings, ReleaseFile+" preflight step `manual` must be unconditional: an `if:` lets a dispatch bypass the gate (issue #1486)")
		}
		if asStr(step["continue-on-error"]) == "true" {
			findings = append(findings, ReleaseFile+" preflight step `manual` must not be `continue-on-error`: a failed attestation has to stop the release (issue #1486)")
		}
	}
	if !strings.Contains(asStr(asMap(pre["outputs"])["manual_gates"]), "steps.manual.outputs.manual_gates") {
		findings = append(findings, ReleaseFile+" preflight must output `manual_gates` from `steps.manual.outputs.manual_gates` (issue #1486)")
	}

	rj := jobOf(rel, "release")
	if !strings.Contains(runBlob(rj), "manual_gates:$manual_gates") {
		findings = append(findings, ReleaseFile+" release-readiness.json must record the decisions as `manual_gates` -- who attested or skipped which manual gate, and why (issue #1486)")
	}
	readsOutput := false
	for _, s := range stepsOf(rj) {
		if strings.HasPrefix(asStr(s["name"]), "Emit release-readiness artifact") &&
			strings.Contains(asStr(asMap(s["env"])["MANUAL_GATES"]), "needs.preflight.outputs.manual_gates") {
			readsOutput = true
		}
	}
	if !readsOutput {
		findings = append(findings, ReleaseFile+" the `Emit release-readiness artifact` step must read MANUAL_GATES from `needs.preflight.outputs.manual_gates` (issue #1486)")
	}

	if !strings.Contains(runBlob(jobOf(dry, "dry-run")), manualInput+"=") {
		findings = append(findings, fmt.Sprintf("%s must dispatch %s with `%s=...`: the input is required, so a rehearsal without it fails for the wrong reason (issue #1486)", DryRunFile, ReleaseFile, manualInput))
	}
	return findings
}
