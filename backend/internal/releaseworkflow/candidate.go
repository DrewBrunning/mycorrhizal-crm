package releaseworkflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// Issue #1484: the release image is built ONCE (release-validate.yml's
// `build-candidate` job), every image-running gate tests that exact digest, and
// docker-publish.yml re-tags the digest instead of rebuilding it. The wiring
// that makes that true is spread over six workflows and one script family, and
// every piece fails *quietly* if it rots: a consumer that stops honouring
// `image_digest` silently goes back to testing a from-source build; a
// `build-and-push` that loses its retag step silently rebuilds. CheckCandidate
// pins the wiring so such a regression is a build failure, not a release that
// ships an image no gate saw.

// ComposerFile, PublishFile and ImageGateFiles name the workflows
// CheckCandidate reads (basenames under .github/workflows).
const (
	ComposerFile = "release-validate.yml"
	PublishFile  = "docker-publish.yml"
)

// ImageGateFiles are the gate workflows that run the all-in-one image and so
// must consume the candidate. chaos-tests.yml is deliberately absent: it runs
// Go tests, never the container image.
var ImageGateFiles = []string{
	"e2e-tests.yml",
	"container-hardening.yml",
	"zap-dast.yml",
	"deploy-smoke.yml",
}

// composeBased is the one image gate that boots the image through Compose
// rather than a locally-tagged `docker build` result; it must use the
// candidate override file, not a build-push step.
const composeBased = "deploy-smoke.yml"

const (
	pullScript     = "candidate-image.sh pull"
	stampScript    = "release-image-stamp.sh"
	candidateJob   = "build-candidate"
	digestOutput   = "needs.build-candidate.outputs.digest"
	localImageTag  = "mycorrhizal-crm-test:latest"
	candidateComp  = "docker-compose.candidate.yml"
	buildPushUsing = "docker/build-push-action"
)

// errorMentionsDigestRE matches an ::error:: annotation line that interpolates
// the tested candidate digest -- i.e. the failure branch of the published ==
// tested comparison, as opposed to any other ::error:: in the same step.
var errorMentionsDigestRE = regexp.MustCompile(`::error::[^\n]*\$\{?CANDIDATE_DIGEST`)

type anyMap = map[string]any

func asMap(v any) anyMap {
	m, _ := v.(anyMap)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return fmt.Sprintf("%t", t)
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func parseDoc(name, text string) (anyMap, []string) {
	var doc anyMap
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, []string{fmt.Sprintf("%s does not parse: %v", name, err)}
	}
	if doc == nil {
		doc = anyMap{}
	}
	return doc, nil
}

func jobOf(doc anyMap, name string) anyMap { return asMap(asMap(doc["jobs"])[name]) }

func stepsOf(job anyMap) []anyMap {
	var out []anyMap
	for _, s := range asList(job["steps"]) {
		if m := asMap(s); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// needsOf returns a job's `needs`, which YAML allows as a string or a list.
func needsOf(job anyMap) []string {
	switch n := job["needs"].(type) {
	case string:
		return []string{n}
	case []any:
		var out []string
		for _, x := range n {
			out = append(out, asStr(x))
		}
		return out
	}
	return nil
}

func contains(list []string, want string) bool {
	for _, x := range list {
		if x == want {
			return true
		}
	}
	return false
}

func stepUses(s anyMap, action string) bool { return strings.HasPrefix(asStr(s["uses"]), action) }

func withOf(s anyMap) anyMap { return asMap(s["with"]) }

// runBlob concatenates every `run:` body of a job, for "does this job invoke X".
func runBlob(job anyMap) string {
	var b strings.Builder
	for _, s := range stepsOf(job) {
		b.WriteString(asStr(s["run"]))
		b.WriteString("\n")
	}
	return b.String()
}

// callerJobFor returns the composer job whose `uses:` is the given workflow file.
func callerJobFor(doc anyMap, workflowFile string) (string, anyMap) {
	names := make([]string, 0)
	for n := range asMap(doc["jobs"]) {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		j := jobOf(doc, n)
		if strings.HasSuffix(asStr(j["uses"]), "/"+workflowFile) {
			return n, j
		}
	}
	return "", nil
}

// stepByUses finds the first step using the given action (prefix match).
func stepByUses(job anyMap, action string) anyMap {
	for _, s := range stepsOf(job) {
		if stepUses(s, action) {
			return s
		}
	}
	return nil
}

// CheckCandidate reports every way the build-once wiring is broken. composer
// is release-validate.yml, publish is docker-publish.yml, and gates maps each
// ImageGateFiles basename to its text. A missing gate text is itself a finding.
func CheckCandidate(composer, publish string, gates map[string]string) []string {
	var findings []string

	comp, errs := parseDoc(ComposerFile, composer)
	if errs != nil {
		return errs
	}
	pub, errs := parseDoc(PublishFile, publish)
	if errs != nil {
		return errs
	}

	findings = append(findings, checkComposerCandidate(comp)...)
	for _, f := range ImageGateFiles {
		findings = append(findings, checkImageGate(comp, f, gates)...)
	}
	findings = append(findings, checkPublishRetag(pub)...)
	findings = append(findings, checkCandidateParity(comp, pub)...)
	return findings
}

func checkComposerCandidate(comp anyMap) []string {
	var findings []string
	job := jobOf(comp, candidateJob)
	if job == nil {
		return []string{ComposerFile + " has no `" + candidateJob + "` job: the release image is no longer built once for every gate to test (issue #1484)"}
	}

	// The reusable workflow must export the digest the caller re-tags.
	outs := asMap(asMap(asMap(comp["on"])["workflow_call"])["outputs"])
	cd := asMap(outs["candidate_digest"])
	if !strings.Contains(asStr(cd["value"]), "jobs."+candidateJob+".outputs.digest") {
		findings = append(findings, ComposerFile+" must export workflow_call output `candidate_digest` from jobs."+candidateJob+".outputs.digest -- docker-publish.yml re-tags that digest (issue #1484)")
	}
	if asStr(asMap(asMap(job["outputs"]))["digest"]) == "" {
		findings = append(findings, ComposerFile+" job `"+candidateJob+"` must output `digest`")
	}

	build := stepByUses(job, buildPushUsing)
	if build == nil {
		findings = append(findings, ComposerFile+" job `"+candidateJob+"` has no docker/build-push-action step")
	} else {
		w := withOf(build)
		if asStr(w["push"]) != "true" {
			findings = append(findings, ComposerFile+" job `"+candidateJob+"` must push the candidate (push: true) -- gates pull it by digest")
		}
		platforms := asStr(w["platforms"])
		if !strings.Contains(platforms, "linux/amd64") || !strings.Contains(platforms, "linux/arm64") {
			findings = append(findings, ComposerFile+" job `"+candidateJob+"` must build linux/amd64,linux/arm64 -- the release image is multi-arch and is re-tagged verbatim")
		}
		if asStr(w["provenance"]) != "true" || asStr(w["sbom"]) != "true" {
			findings = append(findings, ComposerFile+" job `"+candidateJob+"` must set provenance: true and sbom: true -- the release re-tags this index, so the attestations must already be in it")
		}
	}
	if !strings.Contains(runBlob(job), "candidate-image.sh check-digest") {
		findings = append(findings, ComposerFile+" job `"+candidateJob+"` must validate its digest with candidate-image.sh check-digest before exporting it")
	}
	hasStamp := false
	for _, s := range stepsOf(job) {
		if strings.Contains(asStr(s["run"]), stampScript) {
			hasStamp = true
		}
	}
	if !hasStamp {
		findings = append(findings, ComposerFile+" job `"+candidateJob+"` must compute its build stamp with "+stampScript+" (shared with docker-publish.yml)")
	}

	results := jobOf(comp, "results")
	if !contains(needsOf(results), candidateJob) {
		findings = append(findings, ComposerFile+" `results` job must `needs:` "+candidateJob+" so the digest reaches release-readiness.json")
	}
	if !strings.Contains(runBlob(results), "release-candidate.json") {
		findings = append(findings, ComposerFile+" `results` job must write release-candidate.json (the digest record folded into release-readiness.json)")
	}
	return findings
}

// checkImageGate verifies one gate workflow and the composer's call of it.
func checkImageGate(comp anyMap, file string, gates map[string]string) []string {
	var findings []string

	callName, call := callerJobFor(comp, file)
	if call == nil {
		return []string{ComposerFile + " does not call " + file + ", which runs the release image and must test the candidate (issue #1484)"}
	}
	if !contains(needsOf(call), candidateJob) {
		findings = append(findings, fmt.Sprintf("%s job `%s` (%s) must `needs: %s`", ComposerFile, callName, file, candidateJob))
	}
	if !strings.Contains(asStr(asMap(call["with"])["image_digest"]), digestOutput) {
		findings = append(findings, fmt.Sprintf("%s job `%s` (%s) must pass `image_digest: ${{ %s }}`", ComposerFile, callName, file, digestOutput))
	}
	if p := asStr(asMap(call["permissions"])["packages"]); p != "read" && p != "write" {
		findings = append(findings, fmt.Sprintf("%s job `%s` (%s) must grant `packages: read` so the gate can pull the candidate from GHCR", ComposerFile, callName, file))
	}

	text, ok := gates[file]
	if !ok {
		return append(findings, file+" was not supplied to the candidate check")
	}
	doc, errs := parseDoc(file, text)
	if errs != nil {
		return append(findings, errs...)
	}

	in := asMap(asMap(asMap(doc["on"])["workflow_call"])["inputs"])
	if in == nil || asMap(in["image_digest"]) == nil {
		findings = append(findings, file+" must declare a workflow_call `image_digest` input (issue #1484)")
	} else if d := asStr(asMap(in["image_digest"])["default"]); d != "" {
		findings = append(findings, file+" `image_digest` must default to empty so non-release runs keep building from source")
	}

	jobs := asMap(doc["jobs"])
	names := make([]string, 0, len(jobs))
	for n := range jobs {
		names = append(names, n)
	}
	sort.Strings(names)

	pulls := 0
	for _, n := range names {
		j := jobOf(doc, n)
		blob := runBlob(j)
		jobPulls := strings.Contains(blob, pullScript)
		if jobPulls {
			pulls++
		}
		if file == composeBased {
			continue
		}
		// Every from-source build of the all-in-one test image must be skipped
		// when a candidate is supplied; a job that does so must also pull.
		for _, s := range stepsOf(j) {
			if !stepUses(s, buildPushUsing) || asStr(withOf(s)["tags"]) != localImageTag {
				continue
			}
			if !strings.Contains(asStr(s["if"]), "inputs.image_digest == ''") {
				findings = append(findings, fmt.Sprintf("%s job `%s`: step %q builds %s from source without `if: inputs.image_digest == ''` -- with a candidate supplied it would still test a rebuild", file, n, asStr(s["name"]), localImageTag))
			}
			if !jobPulls {
				findings = append(findings, fmt.Sprintf("%s job `%s` builds %s but has no `%s` step for the candidate case", file, n, localImageTag, pullScript))
			}
		}
	}
	if pulls == 0 {
		findings = append(findings, file+" never runs `"+pullScript+"`: it cannot be testing the candidate image")
	}

	if file == composeBased {
		if !strings.Contains(text, candidateComp) {
			findings = append(findings, file+" must boot the candidate through "+candidateComp+" (COMPOSE_FILE), or Compose rebuilds from source")
		}
		smoke := jobOf(doc, "smoke")
		if !strings.Contains(asStr(smoke["if"]), "inputs.image_digest != ''") {
			findings = append(findings, file+" `smoke` job must run whenever image_digest is set (composed into the release battery there is no diff to gate on)")
		}
	}
	return findings
}

// checkPublishRetag verifies docker-publish.yml re-tags the tested digest and
// asserts published == tested.
func checkPublishRetag(pub anyMap) []string {
	var findings []string

	gate := jobOf(pub, "release-gate")
	w := asMap(gate["with"])
	if asStr(w["release_tag"]) == "" || asStr(w["source_ref"]) == "" {
		findings = append(findings, PublishFile+" job `release-gate` must pass release_tag and source_ref to "+ComposerFile+" so the candidate is built from, and stamped as, the tag being released")
	}
	if asStr(asMap(gate["permissions"])["packages"]) != "write" {
		findings = append(findings, PublishFile+" job `release-gate` must grant `packages: write` (build-candidate pushes the candidate tag)")
	}

	bp := jobOf(pub, "build-and-push")
	if !contains(needsOf(bp), "release-gate") {
		findings = append(findings, PublishFile+" job `build-and-push` must `needs: release-gate` to read the tested candidate digest")
	}
	if !strings.Contains(asStr(asMap(bp["env"])["CANDIDATE_DIGEST"]), "needs.release-gate.outputs.candidate_digest") {
		findings = append(findings, PublishFile+" job `build-and-push` must set env CANDIDATE_DIGEST from needs.release-gate.outputs.candidate_digest")
	}

	var retag, source anyMap
	for _, s := range stepsOf(bp) {
		if strings.Contains(asStr(s["run"]), "imagetools create") && strings.Contains(asStr(s["run"]), "CANDIDATE") || strings.Contains(asStr(s["run"]), `"$src"`) && strings.Contains(asStr(s["run"]), "imagetools create") {
			retag = s
		}
		if stepUses(s, buildPushUsing) {
			source = s
		}
	}
	if retag == nil {
		findings = append(findings, PublishFile+" job `build-and-push` has no step that re-tags the candidate digest with `docker buildx imagetools create` -- the all-in-one image would be rebuilt, not the tested one")
	} else {
		run := asStr(retag["run"])
		if !strings.Contains(run, "assert-match") {
			findings = append(findings, PublishFile+" re-tag step must assert every resulting tag resolves to the tested digest (candidate-image.sh assert-match)")
		}
		if !strings.Contains(asStr(retag["if"]), "CANDIDATE_DIGEST != ''") {
			findings = append(findings, PublishFile+" re-tag step must only run when a tested candidate digest exists (`env.CANDIDATE_DIGEST != ''`)")
		}
	}
	if source == nil {
		findings = append(findings, PublishFile+" job `build-and-push` lost its from-source build step, the fallback for the backend/frontend images and an overridden gate")
	} else if !strings.Contains(asStr(source["if"]), "CANDIDATE_DIGEST != ''") {
		findings = append(findings, PublishFile+" job `build-and-push`: the from-source `Build and push` step must be skipped for the all-in-one leg when CANDIDATE_DIGEST is set (`if: !(matrix.suffix == '' && env.CANDIDATE_DIGEST != '')`), or it rebuilds anyway")
	} else if !strings.Contains(asStr(source["if"]), "matrix.suffix == ''") {
		findings = append(findings, PublishFile+" job `build-and-push`: the from-source build skip must be limited to the all-in-one leg (matrix.suffix == ''); the backend/frontend images have no candidate")
	}

	// The cosign/attest steps must run against the digest that was published,
	// whichever path made it.
	published := false
	for _, s := range stepsOf(bp) {
		if asStr(s["id"]) == "published" {
			published = true
		}
	}
	if !published {
		findings = append(findings, PublishFile+" job `build-and-push` must resolve the published digest (step id `published`) for signing/attestation from either the build or the re-tag")
	}

	vra := jobOf(pub, "verify-release-assets")
	if !contains(needsOf(vra), "release-gate") {
		findings = append(findings, PublishFile+" job `verify-release-assets` must `needs: release-gate` to compare the published digest with the tested one")
	}
	assertsEqual := false
	for _, s := range stepsOf(vra) {
		run := asStr(s["run"])
		if errorMentionsDigestRE.MatchString(run) {
			assertsEqual = true
		}
	}
	if !assertsEqual {
		findings = append(findings, PublishFile+" job `verify-release-assets` must fail (::error::) when the published image digest differs from the tested CANDIDATE_DIGEST (issue #1484)")
	}
	return findings
}

// normalizeTag lets the composer's `inputs.release_tag` compare equal to
// docker-publish.yml's `env.RELEASE_TAG` (the same value, two spellings).
func normalizeTag(s string) string {
	return strings.ReplaceAll(s, "inputs.release_tag", "env.RELEASE_TAG")
}

// checkCandidateParity pins that the candidate is built exactly as the release
// image would be: same build args, same labels, same Dockerfile/context, and
// the shared stamp script on both sides. If they drift, "the tested image is
// the shipped image" is a statement about two different images.
func checkCandidateParity(comp, pub anyMap) []string {
	cand := stepByUses(jobOf(comp, candidateJob), buildPushUsing)
	if cand == nil {
		return nil // reported by checkComposerCandidate
	}
	bp := jobOf(pub, "build-and-push")
	var rel anyMap
	for _, s := range stepsOf(bp) {
		if stepUses(s, buildPushUsing) {
			rel = s
		}
	}
	if rel == nil {
		return nil // reported by checkPublishRetag
	}

	var findings []string
	cw, rw := withOf(cand), withOf(rel)
	if strings.TrimSpace(asStr(cw["build-args"])) != strings.TrimSpace(asStr(rw["build-args"])) {
		findings = append(findings, "build-candidate and build-and-push must use identical build-args (APP_VERSION/APP_COMMIT/APP_BUILD_DATE), or the tested image is not the shipped one")
	}

	ccMeta, rcMeta := metaLabels(jobOf(comp, candidateJob)), metaLabels(bp)
	if ccMeta == "" || rcMeta == "" || normalizeTag(ccMeta) != normalizeTag(rcMeta) {
		findings = append(findings, "build-candidate and build-and-push must pin identical OCI identity labels (revision/version/created)")
	}

	// The all-in-one leg of the release matrix must build the same Dockerfile
	// and context as the candidate.
	var okMatrix bool
	for _, e := range asList(asMap(asMap(bp["strategy"])["matrix"])["include"]) {
		m := asMap(e)
		if asStr(m["suffix"]) == "" {
			okMatrix = asStr(m["context"]) == cleanPath(asStr(cw["context"])) && cleanPath(asStr(m["file"])) == cleanPath(asStr(cw["file"]))
		}
	}
	if !okMatrix {
		findings = append(findings, "build-candidate must build the same context and Dockerfile as build-and-push's all-in-one matrix leg (suffix \"\")")
	}

	if !strings.Contains(runBlobOfStep(bp, "stamp"), stampScript) || !strings.Contains(runBlobOfStep(jobOf(comp, candidateJob), "stamp"), stampScript) {
		findings = append(findings, "build-candidate and build-and-push must both compute the build stamp through "+stampScript)
	}
	return findings
}

func cleanPath(p string) string { return strings.TrimPrefix(p, "./") }

func runBlobOfStep(job anyMap, id string) string {
	for _, s := range stepsOf(job) {
		if asStr(s["id"]) == id {
			return asStr(s["run"])
		}
	}
	return ""
}

// metaLabels returns the `labels` input of the docker/metadata-action step.
func metaLabels(job anyMap) string {
	s := stepByUses(job, "docker/metadata-action")
	if s == nil {
		return ""
	}
	return strings.TrimSpace(asStr(withOf(s)["labels"]))
}
