package releaseworkflow

import (
	"regexp"
	"strings"
)

// Issue #1704: when docker-publish.yml reuses the pre-tag battery, the image it
// publishes must be the candidate that battery validated -- not a fresh build
// of the tag commit. A tag-commit rebuild can never reproduce the validated
// digest (the build stamp carries the built commit and its committer date), so
// before #1704 the v1.4.2 release shipped an image whose only runtime check was
// deploy-smoke, while the 17 carried gates had exercised a different digest.
//
// The wiring that fixes it is a chain of hand-offs, and any one of them rotting
// turns the release back into "tested one image, shipped another" without a
// failing job: reuse-decision verifies and exports the digest, release-gate
// hands it to the composer, build-candidate adopts instead of building, and
// verify-release-assets asserts published == validated.
const (
	adoptInput     = "adopt_candidate_digest"
	validatedEnv   = "VALIDATED_DIGEST"
	verifyCmd      = "candidate-image.sh verify-candidate"
	candidateFlag  = "-candidate-digest"
	reuseDigestOut = "candidate_digest"
)

// errorMentionsValidatedRE matches an ::error:: line interpolating the
// validated digest: the failure branch of published == validated.
var errorMentionsValidatedRE = regexp.MustCompile(`::error::[^\n]*\$\{?VALIDATED_DIGEST`)

// CheckReusePublishesValidated reports every way the reuse-publishes-the-
// validated-digest wiring (issue #1704) is broken.
func CheckReusePublishesValidated(composer, publish string) []string {
	comp, errs := parseDoc(ComposerFile, composer)
	if errs != nil {
		return errs
	}
	pub, errs := parseDoc(PublishFile, publish)
	if errs != nil {
		return errs
	}
	var f []string

	// --- composer: build-candidate adopts instead of building.
	in := asMap(asMap(asMap(comp["on"])["workflow_call"])["inputs"])
	if a := asMap(in[adoptInput]); a == nil {
		f = append(f, ComposerFile+" must declare a workflow_call `"+adoptInput+"` input: without it a reused tag-time battery cannot publish the validated image (issue #1704)")
	} else if asStr(a["default"]) != "" {
		f = append(f, ComposerFile+" `"+adoptInput+"` must default to empty so release.yml and a non-reuse run build the candidate")
	}
	if job := jobOf(comp, candidateJob); job != nil {
		var adopt anyMap
		for _, s := range stepsOf(job) {
			if asStr(s["id"]) == "adopt" {
				adopt = s
			}
		}
		if adopt == nil {
			f = append(f, ComposerFile+" job `"+candidateJob+"` has no step `adopt`: a reused battery would build a tag-commit candidate whose digest differs from the validated one (issue #1704)")
		} else {
			if !strings.Contains(asStr(adopt["if"]), "inputs."+adoptInput+" != ''") {
				f = append(f, ComposerFile+" step `adopt` must run only when inputs."+adoptInput+" is set")
			}
			if !strings.Contains(asStr(adopt["run"]), "candidate-image.sh check-digest") {
				f = append(f, ComposerFile+" step `adopt` must shape-check the adopted digest with candidate-image.sh check-digest")
			}
			if !strings.Contains(asStr(asMap(job["outputs"])["digest"]), "steps.adopt.outputs.digest") {
				f = append(f, ComposerFile+" job `"+candidateJob+"` output `digest` must fall back to steps.adopt.outputs.digest, or the adopted digest never reaches the gates and build-and-push")
			}
		}
		if build := stepByUses(job, buildPushUsing); build != nil && !strings.Contains(asStr(build["if"]), "inputs."+adoptInput+" == ''") {
			f = append(f, ComposerFile+" job `"+candidateJob+"`: the candidate build step must be skipped when inputs."+adoptInput+" is set (`if: inputs."+adoptInput+" == ''`), or it rebuilds anyway")
		}
	}

	// --- docker-publish: reuse-decision verifies and exports the digest.
	rd := jobOf(pub, reuseJob)
	if rd == nil {
		return append(f, PublishFile+" has no `"+reuseJob+"` job")
	}
	if !strings.Contains(asStr(asMap(rd["outputs"])[reuseDigestOut]), "steps.decide.outputs."+reuseDigestOut) {
		f = append(f, PublishFile+" job `"+reuseJob+"` must export output `"+reuseDigestOut+"` from the decide step")
	}
	if asStr(asMap(rd["permissions"])["packages"]) != "read" {
		f = append(f, PublishFile+" job `"+reuseJob+"` must grant `packages: read` to verify the validated candidate in GHCR")
	}
	run := runBlob(rd)
	if !strings.Contains(run, verifyCmd) {
		f = append(f, PublishFile+" job `"+reuseJob+"` must verify the validated candidate with "+verifyCmd+" (exists in GHCR, revision = validated commit) before adopting it")
	}
	if !strings.Contains(run, "releaseplan reuse") || !strings.Contains(run, candidateFlag) {
		f = append(f, PublishFile+" job `"+reuseJob+"` must pass the verified digest to `releaseplan reuse "+candidateFlag+"`, which refuses reuse without one")
	}

	// --- docker-publish: release-gate hands it to the composer, only on reuse.
	rg := asStr(asMap(jobOf(pub, "release-gate")["with"])[adoptInput])
	if !strings.Contains(rg, "needs."+reuseJob+".outputs."+reuseDigestOut) || !strings.Contains(rg, "needs."+reuseJob+".outputs.reuse") {
		f = append(f, PublishFile+" job `release-gate` must pass `"+adoptInput+"` from needs."+reuseJob+".outputs."+reuseDigestOut+", gated on needs."+reuseJob+".outputs.reuse == 'true'")
	}

	// --- docker-publish: verify-release-assets asserts published == validated.
	vra := jobOf(pub, "verify-release-assets")
	if !contains(needsOf(vra), reuseJob) {
		f = append(f, PublishFile+" job `verify-release-assets` must `needs: "+reuseJob+"` to read the validated digest")
	}
	asserts, envOK := false, false
	for _, s := range stepsOf(vra) {
		if errorMentionsValidatedRE.MatchString(asStr(s["run"])) {
			asserts = true
		}
		if strings.Contains(asStr(asMap(s["env"])[validatedEnv]), "needs."+reuseJob+".outputs."+reuseDigestOut) {
			envOK = true
		}
	}
	if !envOK {
		f = append(f, PublishFile+" job `verify-release-assets` must set env "+validatedEnv+" from needs."+reuseJob+".outputs."+reuseDigestOut)
	}
	if !asserts {
		f = append(f, PublishFile+" job `verify-release-assets` must fail (::error::) when a reused release's published digest differs from the "+validatedEnv+" the pre-tag battery validated (issue #1704)")
	}
	return f
}
