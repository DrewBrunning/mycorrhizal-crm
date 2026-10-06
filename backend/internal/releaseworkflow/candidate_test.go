package releaseworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wf is the committed workflow set CheckCandidate reads, as raw text.
type wf struct {
	composer string
	publish  string
	gates    map[string]string
}

func loadWorkflows(t *testing.T) wf {
	t.Helper()
	dir := filepath.Join(repoRoot(t), ".github", "workflows")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		return string(b)
	}
	w := wf{composer: read(ComposerFile), publish: read(PublishFile), gates: map[string]string{}}
	for _, g := range ImageGateFiles {
		w.gates[g] = read(g)
	}
	return w
}

func (w wf) check() []string { return CheckCandidate(w.composer, w.publish, w.gates) }

// replaceOnce applies old->new exactly once and fails the test if old is not
// there, so a refactor that moves the text breaks the mutation loudly instead
// of turning the test into a no-op that "passes".
func replaceOnce(t *testing.T, text, old, repl string) string {
	t.Helper()
	require.Contains(t, text, old, "mutation target not found -- the workflow text moved; update this test")
	return strings.Replace(text, old, repl, 1)
}

func replaceAll(t *testing.T, text, old, repl string) string {
	t.Helper()
	require.Contains(t, text, old, "mutation target not found -- the workflow text moved; update this test")
	return strings.ReplaceAll(text, old, repl)
}

// replaceAfter applies the replacement only to the text after marker (a job
// header), to hit one job when a string occurs in several.
func replaceAfter(t *testing.T, text, marker, old, repl string) string {
	t.Helper()
	i := strings.Index(text, marker)
	require.GreaterOrEqual(t, i, 0, "marker %q not found", marker)
	return text[:i] + replaceOnce(t, text[i:], old, repl)
}

// TestCommittedCandidateWiring is the same check cmd/releasegatecheck runs,
// against the real workflows.
func TestCommittedCandidateWiring(t *testing.T) {
	assert.Empty(t, loadWorkflows(t).check())
}

// TestCheckCandidateCatchesEachRegression breaks one piece of the real wiring
// at a time and asserts the checker names it. Every case is a way the
// "tested image == shipped image" property would silently stop holding.
func TestCheckCandidateCatchesEachRegression(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, w *wf)
		want   string
	}{
		// --- the composer ---------------------------------------------------
		{"candidate job removed", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "  build-candidate:\n", "  build-candidatx:\n")
		}, "has no `build-candidate` job"},
		{"digest no longer exported", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "value: ${{ jobs.build-candidate.outputs.digest }}", "value: ''")
		}, "must export workflow_call output `candidate_digest`"},
		{"job output digest dropped", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "      digest: ${{ steps.build.outputs.digest }}\n", "")
		}, "must output `digest`"},
		{"build step removed", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "uses: docker/build-push-action@", "uses: docker/xbuild-push-action@")
		}, "no docker/build-push-action step"},
		{"candidate not pushed", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "          push: true", "          push: false")
		}, "must push the candidate"},
		{"candidate single-arch", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "platforms: linux/amd64,linux/arm64", "platforms: linux/amd64")
		}, "must build linux/amd64,linux/arm64"},
		{"candidate without provenance", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "          provenance: true", "          provenance: false")
		}, "must set provenance: true and sbom: true"},
		{"candidate without sbom", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "          sbom: true", "          sbom: false")
		}, "must set provenance: true and sbom: true"},
		{"digest not validated", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "candidate-image.sh check-digest", "candidate-image.sh no-op")
		}, "must validate its digest"},
		{"candidate stamp not shared", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "bash .github/scripts/release-image-stamp.sh", "echo")
		}, "must compute its build stamp with release-image-stamp.sh"},
		{"results lose the candidate", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "      - build-candidate\n      - unit-tests", "      - unit-tests")
		}, "`results` job must `needs:` build-candidate"},
		{"results stop recording the digest", func(t *testing.T, w *wf) {
			w.composer = replaceAll(t, w.composer, "release-candidate.json", "release-cand.json")
		}, "must write release-candidate.json"},

		// --- how the composer calls the image gates ------------------------
		{"e2e call loses needs", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "    needs: build-candidate\n    uses: ./.github/workflows/e2e-tests.yml", "    uses: ./.github/workflows/e2e-tests.yml")
		}, "(e2e-tests.yml) must `needs: build-candidate`"},
		{"e2e call stops passing the digest", func(t *testing.T, w *wf) {
			w.composer = replaceAfter(t, w.composer, "  e2e-tests:\n", "      image_digest: ${{ needs.build-candidate.outputs.digest }}\n", "")
		}, "(e2e-tests.yml) must pass `image_digest"},
		{"hardening call cannot pull from GHCR", func(t *testing.T, w *wf) {
			w.composer = replaceAfter(t, w.composer, "  container-hardening:\n", "      packages: read\n", "")
		}, "(container-hardening.yml) must grant `packages: read`"},
		{"deploy-smoke dropped from the composer", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "uses: ./.github/workflows/deploy-smoke.yml", "uses: ./.github/workflows/chaos-tests.yml")
		}, "does not call deploy-smoke.yml"},
		{"zap-dast call stops passing the digest", func(t *testing.T, w *wf) {
			w.composer = replaceAfter(t, w.composer, "  zap-dast:\n", "      image_digest: ${{ needs.build-candidate.outputs.digest }}\n", "")
		}, "(zap-dast.yml) must pass `image_digest"},

		// --- the gate workflows --------------------------------------------
		{"e2e loses its image_digest input", func(t *testing.T, w *wf) {
			w.gates["e2e-tests.yml"] = replaceOnce(t, w.gates["e2e-tests.yml"], "      image_digest:\n        description", "      image_digxst:\n        description")
		}, "e2e-tests.yml must declare a workflow_call `image_digest` input"},
		{"e2e image_digest defaults to something", func(t *testing.T, w *wf) {
			w.gates["e2e-tests.yml"] = replaceOnce(t, w.gates["e2e-tests.yml"], "        default: ''", "        default: 'sha256:x'")
		}, "must default to empty"},
		{"e2e rebuilds even with a candidate", func(t *testing.T, w *wf) {
			w.gates["e2e-tests.yml"] = replaceAll(t, w.gates["e2e-tests.yml"], "        if: ${{ inputs.image_digest == '' }}\n", "")
		}, "builds mycorrhizal-crm-test:latest from source without `if: inputs.image_digest == ''`"},
		{"e2e never pulls the candidate", func(t *testing.T, w *wf) {
			w.gates["e2e-tests.yml"] = replaceAll(t, w.gates["e2e-tests.yml"], "candidate-image.sh pull", "echo pull")
		}, "never runs `candidate-image.sh pull`"},
		{"one e2e job builds but does not pull", func(t *testing.T, w *wf) {
			w.gates["e2e-tests.yml"] = replaceAfter(t, w.gates["e2e-tests.yml"], "  e2e-webkit-smoke:\n", "candidate-image.sh pull", "echo pull")
		}, "job `e2e-webkit-smoke` builds mycorrhizal-crm-test:latest but has no `candidate-image.sh pull` step"},
		{"hardening never pulls the candidate", func(t *testing.T, w *wf) {
			w.gates["container-hardening.yml"] = replaceAll(t, w.gates["container-hardening.yml"], "candidate-image.sh pull", "echo pull")
		}, "container-hardening.yml never runs"},
		{"zap rebuilds even with a candidate", func(t *testing.T, w *wf) {
			w.gates["zap-dast.yml"] = replaceAll(t, w.gates["zap-dast.yml"], "        if: ${{ inputs.image_digest == '' }}\n", "")
		}, "zap-dast.yml job `dast`"},
		{"deploy-smoke loses the candidate override", func(t *testing.T, w *wf) {
			w.gates["deploy-smoke.yml"] = replaceAll(t, w.gates["deploy-smoke.yml"], "docker-compose.candidate.yml", "docker-compose.yml")
		}, "must boot the candidate through docker-compose.candidate.yml"},
		{"deploy-smoke skips when composed", func(t *testing.T, w *wf) {
			w.gates["deploy-smoke.yml"] = replaceOnce(t, w.gates["deploy-smoke.yml"], "inputs.image_digest != '' || github.event_name == 'schedule'", "github.event_name == 'schedule'")
		}, "`smoke` job must run whenever image_digest is set"},
		{"deploy-smoke never pulls", func(t *testing.T, w *wf) {
			w.gates["deploy-smoke.yml"] = replaceAll(t, w.gates["deploy-smoke.yml"], "candidate-image.sh pull", "echo pull")
		}, "deploy-smoke.yml never runs"},
		{"a gate text is not supplied", func(t *testing.T, w *wf) {
			delete(w.gates, "zap-dast.yml")
		}, "zap-dast.yml was not supplied"},
		{"a gate does not parse", func(t *testing.T, w *wf) {
			w.gates["zap-dast.yml"] = ":\tnot yaml"
		}, "zap-dast.yml does not parse"},

		// --- docker-publish.yml --------------------------------------------
		{"release-gate stops passing the tag", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "      release_tag: ${{ inputs.tag || github.ref_name }}\n", "")
		}, "must pass release_tag and source_ref"},
		{"release-gate cannot push the candidate", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "      # build-candidate pushes the candidate tag to GHCR.\n      packages: write\n", "")
		}, "`release-gate` must grant `packages: write`"},
		{"build-and-push stops needing the gate", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "needs: [schema-fixture-gate, build-android-apk, release-gate]", "needs: [schema-fixture-gate, build-android-apk]")
		}, "`build-and-push` must `needs: release-gate`"},
		{"build-and-push loses the digest env", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "needs.release-gate.outputs.candidate_digest }}\n    permissions:", "'' }}\n    permissions:")
		}, "must set env CANDIDATE_DIGEST"},
		{"no re-tag step", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "docker buildx imagetools create", "docker buildx imagetools inspect")
		}, "has no step that re-tags the candidate digest"},
		{"re-tag does not verify the digest", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, `bash .github/scripts/candidate-image.sh assert-match "$DIGEST" "$got" "$t"`, "true")
		}, "must assert every resulting tag resolves to the tested digest"},
		{"re-tag runs without a candidate", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "if: ${{ matrix.suffix == '' && env.CANDIDATE_DIGEST != '' }}", "if: ${{ matrix.suffix == '' }}")
		}, "re-tag step must only run when a tested candidate digest exists"},
		{"source build no longer skipped", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "if: ${{ !(matrix.suffix == '' && env.CANDIDATE_DIGEST != '') }}", "if: ${{ true }}")
		}, "from-source `Build and push` step must be skipped"},
		{"source build skip not limited to all-in-one", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "if: ${{ !(matrix.suffix == '' && env.CANDIDATE_DIGEST != '') }}", "if: ${{ env.CANDIDATE_DIGEST == '' }}")
			// the replacement still has to mention CANDIDATE_DIGEST != '' to reach the suffix check
			w.publish = replaceOnce(t, w.publish, "if: ${{ env.CANDIDATE_DIGEST == '' }}", "if: ${{ env.CANDIDATE_DIGEST != '' }}")
		}, "must be limited to the all-in-one leg"},
		{"source build step deleted", func(t *testing.T, w *wf) {
			w.publish = replaceAfter(t, w.publish, "  build-and-push:\n", "uses: docker/build-push-action@", "uses: docker/xbuild-push-action@")
		}, "lost its from-source build step"},
		{"signing no longer keyed to the published digest", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "        id: published\n", "        id: publishd\n")
		}, "must resolve the published digest"},
		{"verify-release-assets stops needing the gate", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, "needs: [create-release, build-and-push, build-android-apk, apk-provenance, scan, release-gate]", "needs: [create-release, build-and-push, build-android-apk, apk-provenance, scan]")
		}, "`verify-release-assets` must `needs: release-gate`"},
		{"verify-release-assets stops comparing digests", func(t *testing.T, w *wf) {
			w.publish = replaceAfter(t, w.publish, "  verify-release-assets:\n", "has digest ${published}, but the release gates tested ${CANDIDATE_DIGEST}", "has digest ${published}")
		}, "must fail (::error::) when the published image digest differs"},

		// --- candidate / release parity ------------------------------------
		{"build args drift", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "            APP_VERSION=${{ steps.stamp.outputs.display_version }}", "            APP_VERSION=${{ inputs.release_tag }}")
		}, "must use identical build-args"},
		{"labels drift", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "org.opencontainers.image.created=${{ steps.stamp.outputs.created }}", "org.opencontainers.image.created=now")
		}, "must pin identical OCI identity labels"},
		{"labels missing", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "uses: docker/metadata-action@", "uses: docker/xmetadata-action@")
		}, "must pin identical OCI identity labels"},
		{"dockerfile drift", func(t *testing.T, w *wf) {
			w.composer = replaceOnce(t, w.composer, "          file: ./Dockerfile\n", "          file: ./Dockerfile.other\n")
		}, "same context and Dockerfile"},
		{"release stamp no longer shared", func(t *testing.T, w *wf) {
			w.publish = replaceOnce(t, w.publish, `run: bash .github/scripts/release-image-stamp.sh "$RELEASE_TAG"`, `run: echo stamp`)
		}, "must both compute the build stamp through release-image-stamp.sh"},

		// --- unparseable ----------------------------------------------------
		{"composer does not parse", func(t *testing.T, w *wf) { w.composer = ":\tnot yaml" }, "release-validate.yml does not parse"},
		{"publish does not parse", func(t *testing.T, w *wf) { w.publish = ":\tnot yaml" }, "docker-publish.yml does not parse"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := loadWorkflows(t)
			tc.mutate(t, &w)
			found := false
			got := w.check()
			for _, f := range got {
				if strings.Contains(f, tc.want) {
					found = true
				}
			}
			assert.True(t, found, "want a finding containing %q, got %q", tc.want, got)
		})
	}
}

// TestCheckCandidateOnEmptyDocuments covers the degenerate inputs: an empty
// YAML document parses to nothing and every requirement is reported rather
// than panicking on the nil maps.
func TestCheckCandidateOnEmptyDocuments(t *testing.T) {
	got := CheckCandidate("", "", map[string]string{})
	require.NotEmpty(t, got)
	assert.Contains(t, got[0], "no `build-candidate` job")
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "true", asStr(true))
	assert.Equal(t, "5", asStr(5))
	assert.Equal(t, "", asStr(nil))
	assert.Equal(t, "x", asStr("x"))
	assert.Nil(t, asMap("not a map"))
	assert.Nil(t, asList("not a list"))
	assert.Equal(t, []string{"a"}, needsOf(anyMap{"needs": "a"}))
	assert.Equal(t, []string{"a", "b"}, needsOf(anyMap{"needs": []any{"a", "b"}}))
	assert.Nil(t, needsOf(anyMap{}))
	assert.Nil(t, stepsOf(anyMap{"steps": []any{"not-a-step"}}))
	assert.Equal(t, "env.RELEASE_TAG", normalizeTag("inputs.release_tag"))
	assert.Equal(t, "", runBlobOfStep(anyMap{}, "stamp"))
	assert.Equal(t, "", metaLabels(anyMap{}))
	assert.Equal(t, "Dockerfile", cleanPath("./Dockerfile"))

	name, job := callerJobFor(anyMap{"jobs": anyMap{"a": anyMap{"uses": "./.github/workflows/x.yml"}}}, "x.yml")
	assert.Equal(t, "a", name)
	assert.NotNil(t, job)
	name, job = callerJobFor(anyMap{}, "x.yml")
	assert.Empty(t, name)
	assert.Nil(t, job)
}
