package releaseworkflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Issue #1487: the release battery's flake exposure was cut three ways -- a
// per-gate ledger with same-commit reuse at tag time, `rerun_gates` on
// release.yml, and bounded retries for the two suites whose failures were
// scanner/timing flakes. Each is spread over several workflows and a script
// family, and each fails QUIETLY if it rots: a gate job that loses its skip
// condition is simply re-run every time (the exposure returns, nothing goes
// red); a retry wrapper that is bypassed re-rolls nothing. CheckResilience pins
// the wiring so any such regression is a build failure.

// Files CheckResilience reads, as basenames or repo-relative paths.
const (
	ReleaseFile     = "release.yml"
	ZapFile         = "zap-dast.yml"
	MinVersionFile  = "min-version-tests.yml"
	ZapScript       = ".github/scripts/zap-scan-gate.sh"
	GoRetryScript   = ".github/scripts/go-test-retry-failed.sh"
	ZapgateSource   = "backend/cmd/zapgate/main.go"
	resultsJob      = "results"
	reuseJob        = "reuse-decision"
	skipInput       = "skip_gates"
	carriedInput    = "carried_ledger"
	ledgerFileName  = "release-gate-ledger.json"
	releaseplanTool = "./cmd/releaseplan"
)

// ResilienceFiles is every input CheckResilience reads.
type ResilienceFiles struct {
	Composer   string // release-validate.yml
	Release    string // release.yml
	Publish    string // docker-publish.yml
	Zap        string // zap-dast.yml
	MinVersion string // min-version-tests.yml
	ZapScript  string // .github/scripts/zap-scan-gate.sh
	ZapgateSrc string // backend/cmd/zapgate/main.go
}

var (
	zapExitBlindRE   = regexp.MustCompile(`(?m)^const exitBlind\s*=\s*(\d+)`)
	zapScriptBlindRE = regexp.MustCompile(`(?m)^exit_blind=(\d+)`)
)

// gateJobs returns the composer's gate jobs (those that call a workflow).
func gateJobs(comp anyMap) []string {
	var out []string
	for n, j := range asMap(comp["jobs"]) {
		if asStr(asMap(j)["uses"]) != "" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// CheckResilience reports every way the #1487 wiring is broken.
func CheckResilience(f ResilienceFiles) []string {
	comp, errs := parseDoc(ComposerFile, f.Composer)
	if errs != nil {
		return errs
	}
	rel, errs := parseDoc(ReleaseFile, f.Release)
	if errs != nil {
		return errs
	}
	pub, errs := parseDoc(PublishFile, f.Publish)
	if errs != nil {
		return errs
	}
	var findings []string
	findings = append(findings, checkComposerSkips(comp)...)
	findings = append(findings, checkReleaseRerun(rel)...)
	findings = append(findings, checkPublishReuse(pub)...)
	findings = append(findings, checkRetries(f)...)
	return findings
}

func checkComposerSkips(comp anyMap) []string {
	var findings []string
	inputs := asMap(asMap(asMap(comp["on"])["workflow_call"])["inputs"])
	for _, in := range []string{skipInput, carriedInput} {
		if inputs[in] == nil {
			findings = append(findings, ComposerFile+" must declare workflow_call input `"+in+"` (issue #1487: a rerun / tag-time reuse cannot skip a gate without it)")
		}
	}

	gates := gateJobs(comp)
	for _, g := range gates {
		job := jobOf(comp, g)
		cond := asStr(job["if"])
		if !strings.Contains(cond, "inputs."+skipInput) || !strings.Contains(cond, "',"+g+",'") {
			findings = append(findings, fmt.Sprintf("%s gate `%s` must carry an `if:` that skips it when inputs.%s lists `%s` (contains(format(',{0},', inputs.%s), ',%s,')) -- otherwise a rerun or a tag-time reuse still re-rolls it (issue #1487)", ComposerFile, g, skipInput, g, skipInput, g))
		}
		// A gate that needs ANOTHER gate would be implicitly skipped whenever
		// that one is skipped; it must say explicitly that a skipped need is fine.
		for _, n := range needsOf(job) {
			if n == candidateJob || !contains(gates, n) {
				continue
			}
			if !strings.Contains(cond, "!cancelled()") ||
				!strings.Contains(cond, "needs."+n+".result == 'success'") ||
				!strings.Contains(cond, "needs."+n+".result == 'skipped'") {
				findings = append(findings, fmt.Sprintf("%s gate `%s` needs gate `%s`, so its `if:` must use `!cancelled()` and accept needs.%s.result of 'success' or 'skipped' -- else carrying `%s` forward skips `%s` too (issue #1487)", ComposerFile, g, n, n, n, g))
			}
		}
	}

	// build-candidate always runs: the digest docker-publish.yml re-tags comes
	// from it, and a rerun's image-running gates pull it.
	if cb := jobOf(comp, candidateJob); cb != nil && asStr(cb["if"]) != "" {
		findings = append(findings, ComposerFile+" job `"+candidateJob+"` must not be conditional: the published digest comes from it (issue #1487)")
	}

	results := jobOf(comp, resultsJob)
	if !strings.Contains(runBlob(results), releaseplanTool+" ledger") {
		findings = append(findings, ComposerFile+" `results` job must record the gate ledger with `go run "+releaseplanTool+" ledger` (issue #1487)")
	}
	for _, name := range []string{skipInput, carriedInput} {
		if !strings.Contains(runEnv(results), "inputs."+name) {
			findings = append(findings, ComposerFile+" `results` job must pass inputs."+name+" to the ledger step so a carried gate is re-recorded with the run that earned it (issue #1487)")
		}
	}
	uploaded := false
	for _, s := range stepsOf(results) {
		if stepUses(s, "actions/upload-artifact") && strings.Contains(asStr(withOf(s)["path"]), ledgerFileName) {
			uploaded = true
		}
	}
	if !uploaded {
		findings = append(findings, ComposerFile+" `results` job must upload "+ledgerFileName+" in the release-gate-results artifact (issue #1487)")
	}
	return findings
}

// runEnv concatenates every `env:` value of a job's steps.
func runEnv(job anyMap) string {
	var b strings.Builder
	for _, s := range stepsOf(job) {
		for _, v := range asMap(s["env"]) {
			b.WriteString(asStr(v))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func checkReleaseRerun(rel anyMap) []string {
	var findings []string
	inputs := asMap(asMap(asMap(rel["on"])["workflow_dispatch"])["inputs"])
	if inputs["rerun_gates"] == nil {
		findings = append(findings, ReleaseFile+" must declare the `rerun_gates` dispatch input (issue #1487)")
	}

	pre := jobOf(rel, "preflight")
	planned := false
	for _, s := range stepsOf(pre) {
		if asStr(s["id"]) == "rerun" && strings.Contains(asStr(s["run"]), releaseplanTool+" rerun") {
			planned = true
		}
	}
	if !planned {
		findings = append(findings, ReleaseFile+" preflight needs a step `rerun` that plans the rerun with `go run "+releaseplanTool+" rerun` (issue #1487)")
	}
	outs := asMap(pre["outputs"])
	for _, o := range []string{"skip_gates", "carried_ledger"} {
		if asStr(outs[o]) == "" {
			findings = append(findings, ReleaseFile+" preflight must output `"+o+"` for the validate call (issue #1487)")
		}
	}

	with := asMap(jobOf(rel, "validate")["with"])
	if !strings.Contains(asStr(with[skipInput]), "needs.preflight.outputs.skip_gates") {
		findings = append(findings, ReleaseFile+" `validate` must pass `"+skipInput+": ${{ needs.preflight.outputs.skip_gates }}` to the composer (issue #1487)")
	}
	if !strings.Contains(asStr(with[carriedInput]), "needs.preflight.outputs.carried_ledger") {
		findings = append(findings, ReleaseFile+" `validate` must pass `"+carriedInput+": ${{ needs.preflight.outputs.carried_ledger }}` to the composer (issue #1487)")
	}

	if !strings.Contains(runBlob(jobOf(rel, "release")), "gate_ledger:$ledger") {
		findings = append(findings, ReleaseFile+" release-readiness.json must record the per-gate ledger as `gate_ledger` -- docker-publish.yml's tag-time reuse reads it (issue #1487)")
	}
	return findings
}

func checkPublishReuse(pub anyMap) []string {
	var findings []string
	job := jobOf(pub, reuseJob)
	if job == nil {
		return []string{PublishFile + " has no `" + reuseJob + "` job: the tag-time battery re-runs every gate release.yml already passed (issue #1487)"}
	}
	if asStr(job["continue-on-error"]) != "true" {
		findings = append(findings, PublishFile+" job `"+reuseJob+"` must be `continue-on-error: true`: it may only ever cost the reuse, never block a release (issue #1487)")
	}
	blob := runBlob(job)
	if !strings.Contains(blob, releaseplanTool+" reuse") {
		findings = append(findings, PublishFile+" job `"+reuseJob+"` must decide with `go run "+releaseplanTool+" reuse` (issue #1487)")
	}
	if !strings.Contains(blob, "merge-base --is-ancestor") {
		findings = append(findings, PublishFile+" job `"+reuseJob+"` must verify the validated commit is an ancestor of the tag before diffing (issue #1487)")
	}
	if !strings.Contains(blob, "release-readiness-") {
		findings = append(findings, PublishFile+" job `"+reuseJob+"` must read the release-readiness-<tag> artifact's ledger (issue #1487)")
	}

	gate := jobOf(pub, "release-gate")
	if !contains(needsOf(gate), reuseJob) {
		findings = append(findings, PublishFile+" `release-gate` must `needs: "+reuseJob+"` (issue #1487)")
	}
	with := asMap(gate["with"])
	if !strings.Contains(asStr(with[skipInput]), "needs."+reuseJob+".outputs.skip_gates") {
		findings = append(findings, PublishFile+" `release-gate` must pass `"+skipInput+"` from `needs."+reuseJob+".outputs.skip_gates` (issue #1487)")
	}
	if !strings.Contains(asStr(with[carriedInput]), "needs."+reuseJob+".outputs.carried") {
		findings = append(findings, PublishFile+" `release-gate` must pass `"+carriedInput+"` from `needs."+reuseJob+".outputs.carried` (issue #1487)")
	}

	cr := jobOf(pub, "create-release")
	if !contains(needsOf(cr), reuseJob) || !strings.Contains(runBlob(cr), "tag_time_battery") {
		findings = append(findings, PublishFile+" `create-release` must need `"+reuseJob+"` and record its decision as `tag_time_battery` in the attached release-readiness.json (issue #1487)")
	}
	return findings
}

func checkRetries(f ResilienceFiles) []string {
	var findings []string

	zap, errs := parseDoc(ZapFile, f.Zap)
	if errs != nil {
		return errs
	}
	if !strings.Contains(runBlob(jobOf(zap, "dast")), ZapScript) {
		findings = append(findings, ZapFile+" job `dast` must run the scan and gate through "+ZapScript+" -- that is what re-scans a blind scan once (issue #1487)")
	}
	// The script retries on a specific zapgate exit code; the two must agree or
	// the retry silently never (or always) fires.
	src := zapExitBlindRE.FindStringSubmatch(f.ZapgateSrc)
	scr := zapScriptBlindRE.FindStringSubmatch(f.ZapScript)
	switch {
	case src == nil:
		findings = append(findings, ZapgateSource+" must declare `const exitBlind` (issue #1487)")
	case scr == nil:
		findings = append(findings, ZapScript+" must declare `exit_blind=<n>` matching zapgate's exitBlind (issue #1487)")
	case src[1] != scr[1]:
		findings = append(findings, fmt.Sprintf("%s exit_blind=%s disagrees with %s exitBlind=%s: the DAST retry would never fire (issue #1487)", ZapScript, scr[1], ZapgateSource, src[1]))
	}
	if !strings.Contains(f.ZapScript, "zapgate") || !strings.Contains(f.ZapScript, `-ne "$exit_blind"`) {
		findings = append(findings, ZapScript+" must retry ONLY on zapgate's blind exit code and fail every other non-zero gate result immediately (issue #1487)")
	}

	mv, errs := parseDoc(MinVersionFile, f.MinVersion)
	if errs != nil {
		return errs
	}
	if !strings.Contains(runBlob(jobOf(mv, "go-minimum")), GoRetryScript) {
		findings = append(findings, MinVersionFile+" job `go-minimum` must run its tests through "+GoRetryScript+" (bounded failed-package retry, issue #1487)")
	}
	return findings
}
