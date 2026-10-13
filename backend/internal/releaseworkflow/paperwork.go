package releaseworkflow

import (
	"strings"
)

// Dry-run handling of the release *paperwork* (process) gates.
//
// release-dry-run.yml rehearses release.yml weekly to catch pipeline defects.
// The two gates that check release paperwork rather than pipeline behaviour --
// the ASVS/MASVS section-10 changelog row and the per-release adversarial delta
// (release-obligations.sh) -- are expected to be unsatisfied mid-cycle, so
// under `dry_run` they run and REPORT (warning + "Pending release paperwork"
// summary section) but never fail. A real release is unchanged: they stay hard
// failures. CheckDryRunPaperwork pins both halves.
//
// The other gates in release.yml (citecheck, releasegatecheck, the manual-gate
// attestations, the validate battery) are deliberately NOT downgraded: citecheck
// and releasegatecheck are deterministic repo checks that also gate every PR, and
// the manual-gate step already takes a recorded skip from the rehearsal.
const (
	paperworkASVSStep = "Gate - ASVS/MASVS re-verification obligation"
	paperworkAdvStep  = "Gate - per-release adversarial delta"
	paperworkFileEnv  = "PENDING_PAPERWORK_FILE"
	dryRunGuard       = `if [ "$DRY_RUN" = "true" ]; then`
)

// CheckDryRunPaperwork reports every way the dry-run/real split of the
// paperwork gates is broken. release is release.yml, dryRun is
// release-dry-run.yml.
func CheckDryRunPaperwork(release, dryRun string) []string {
	rel, errs := parseDoc(ReleaseFile, release)
	if errs != nil {
		return errs
	}
	dry, errs := parseDoc(DryRunFile, dryRun)
	if errs != nil {
		return errs
	}
	var findings []string
	rj := jobOf(rel, "release")

	for _, g := range []struct{ name, sub string }{
		{paperworkASVSStep, obligationsScript + " asvs"},
		{paperworkAdvStep, obligationsScript + " adversarial"},
	} {
		var step anyMap
		for _, s := range stepsOf(rj) {
			if asStr(s["name"]) == g.name {
				step = s
			}
		}
		if step == nil {
			findings = append(findings, ReleaseFile+" release job needs the step `"+g.name+"`")
			continue
		}
		if !branchesCorrect(asStr(step["run"]), g.sub) {
			findings = append(findings, ReleaseFile+" step `"+g.name+"` must invoke `"+g.sub+"` exactly once with --dry-run inside `"+dryRunGuard+"` and exactly once without it in the else branch (a swap would downgrade a real release)")
		}
		if !strings.Contains(asStr(asMap(step["env"])["DRY_RUN"]), "inputs.dry_run") {
			findings = append(findings, ReleaseFile+" step `"+g.name+"` must take DRY_RUN from `inputs.dry_run`")
		}
		if !strings.Contains(asStr(asMap(step["env"])[paperworkFileEnv]), "runner.temp") {
			findings = append(findings, ReleaseFile+" step `"+g.name+"` must set "+paperworkFileEnv+" so a dry-run would-block reaches the summary")
		}
		// Real-mode integrity: no step-level escape that would soften a real cut.
		if strings.Contains(asStr(step["if"]), "dry_run") {
			findings = append(findings, ReleaseFile+" step `"+g.name+"` must not be skipped via `if: ...dry_run...`: the dry run has to execute and report it")
		}
		if asStr(step["continue-on-error"]) == "true" {
			findings = append(findings, ReleaseFile+" step `"+g.name+"` must not be continue-on-error: a real release has to block on it")
		}
	}

	summaryOK := false
	for _, s := range stepsOf(rj) {
		if asStr(s["name"]) == "Dry run - summary" &&
			strings.Contains(asStr(s["run"]), "Pending release paperwork") &&
			strings.Contains(asStr(asMap(s["env"])[paperworkFileEnv]), "runner.temp") {
			summaryOK = true
		}
	}
	if !summaryOK {
		findings = append(findings, ReleaseFile+" `Dry run - summary` must render a \"Pending release paperwork\" section from "+paperworkFileEnv)
	}

	// The rehearsal must not pass the ack escape hatches: an ack is a recorded
	// skip; the downgrade is the explicit dry-run branch instead.
	blob := runBlob(jobOf(dry, "dry-run"))
	for _, ack := range []string{"ack_asvs_current", "ack_adversarial_delta"} {
		if strings.Contains(blob, ack) {
			findings = append(findings, DryRunFile+" must not pass `"+ack+"`: paperwork gates are downgraded by release.yml's dry-run branch, not by a recorded ack")
		}
	}
	return findings
}

// branchesCorrect reports whether run invokes sub exactly once with --dry-run,
// inside the DRY_RUN-true branch, and exactly once without it, in the else.
func branchesCorrect(run, sub string) bool {
	inIf, inElse := false, false
	dry, real := 0, 0
	for _, l := range strings.Split(run, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case t == dryRunGuard:
			inIf = true
		case t == "else":
			inElse = true
		case t == "fi":
			inIf, inElse = false, false
		case strings.Contains(t, sub):
			if strings.Contains(t, "--dry-run") {
				if !inIf || inElse {
					return false
				}
				dry++
			} else {
				if !inElse {
					return false
				}
				real++
			}
		}
	}
	return dry == 1 && real == 1
}
