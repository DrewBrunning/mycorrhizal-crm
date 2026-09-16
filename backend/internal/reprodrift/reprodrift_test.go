// Package reprodrift is the structural coupling behind issue #947 (and its
// follow-up, the "Go server binary is byte-reproducible" required-check
// starvation bug fixed alongside this test): the reproducibility docs
// (docs/security/release-verification.md, docs/security/reproducible-builds.md,
// docs/security/threat-model.md) claimed the Go server binary is
// "byte-reproducible and gated every PR", when .github/workflows/reproducibility.yml's
// go-binary job only proves *path*-independence -- it double-builds on one
// runner, which does not rule out a compromised toolchain or a
// non-deterministic dependency that happens to be stable across two paths on
// the same box. A true independent rebuild (a different runner entirely) was
// added as go-binary-independent-rebuild, gated to the weekly schedule /
// manual dispatch rather than every PR.
//
// go-binary is also a *required* status check (docs/development/repo-governance.md).
// It used to gate the actual double build with a `paths:` filter on the
// workflow's own `pull_request` trigger -- which meant a PR outside that
// filter never triggered the workflow at all, so no status was ever posted
// under that check name, and GitHub showed it as permanently stuck
// "Expected -- waiting for status", blocking merge indefinitely. The fix:
// the trigger now fires on every PR unconditionally, a `changes` job
// resolves whether the double-build inputs actually changed, and go-binary's
// build step is gated on that job's output instead of the trigger itself --
// so the required check always runs and reports, and only the expensive
// build is still conditional.
//
// This test ties the docs to that structure the same way internal/compatci
// ties supported-runtime-matrix.md to its CI jobs: read the actual workflow,
// and fail if either (a) the workflow's build step stops being gated or
// drops the independent-rebuild job without the docs changing, or (b) the
// docs regress to the flat "gated every PR" overclaim -- which would now be
// doubly wrong, since the *check* runs every PR but the *double build* it
// performs still does not. Kept entirely in a _test.go file -- there is no
// runtime code here for anything to import.
package reprodrift

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Paths are relative to this package (backend/internal/reprodrift -> repo
// root is three levels up).
const (
	workflowRel            = "../../../.github/workflows/reproducibility.yml"
	releaseVerificationRel = "../../../docs/security/release-verification.md"
	reproducibleBuildsRel  = "../../../docs/security/reproducible-builds.md"
	threatModelRel         = "../../../docs/security/threat-model.md"
)

// workflowStep is the minimal shape of a job step this test reasons about.
type workflowStep struct {
	Name string `yaml:"name"`
	If   string `yaml:"if"`
}

// workflowFile is the minimal shape of reproducibility.yml this test reasons
// about.
type workflowFile struct {
	On struct {
		PullRequest *struct {
			Paths []string `yaml:"paths"`
		} `yaml:"pull_request"`
	} `yaml:"on"`
	Jobs map[string]struct {
		If       string            `yaml:"if"`
		Needs    any               `yaml:"needs"`
		Outputs  map[string]string `yaml:"outputs"`
		Strategy *struct {
			Matrix map[string]any `yaml:"matrix"`
		} `yaml:"strategy"`
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadWorkflow(t *testing.T) workflowFile {
	t.Helper()
	data, err := os.ReadFile(workflowRel)
	if err != nil {
		t.Fatalf("reading %s: %v", workflowRel, err)
	}
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parsing %s: %v", workflowRel, err)
	}
	return wf
}

func readDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// needsContains reports whether a job's `needs:` (a bare string, or a list
// of strings) names the given job id.
func needsContains(needs any, id string) bool {
	switch v := needs.(type) {
	case string:
		return v == id
	case []any:
		for _, n := range v {
			if s, ok := n.(string); ok && s == id {
				return true
			}
		}
	}
	return false
}

// matrixValues returns the string entries of matrix[key], or nil if the key
// is absent or not a list.
func matrixValues(matrix map[string]any, key string) []string {
	raw, ok := matrix[key]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// TestGoBinaryCheckAlwaysRunsButBuildStaysGated pins the two facts the docs
// must not contradict: (1) go-binary is a required status check, so its
// *trigger* must not carry a paths filter -- that was the #947-adjacent bug
// (a required check stranded "Expected -- waiting for status" forever on any
// PR outside the filter, since the workflow never even ran); and (2) the
// actual double build inside go-binary must still be conditional on a
// `changes`-style job, not unconditional -- an unfiltered double build on
// every PR would make the "path-independence, not every PR" doc qualifier
// (checked by TestDocsDoNotOverclaimUnconditionalGating) false in the other
// direction.
func TestGoBinaryCheckAlwaysRunsButBuildStaysGated(t *testing.T) {
	wf := loadWorkflow(t)
	if wf.On.PullRequest == nil {
		t.Fatal("reproducibility.yml has no pull_request trigger -- update this test and the docs together")
	}
	if len(wf.On.PullRequest.Paths) != 0 {
		t.Fatal("reproducibility.yml's pull_request trigger has a paths filter again -- the required check " +
			"\"Go server binary is byte-reproducible\" would be stranded (no status posted at all, so branch " +
			"protection waits forever) on any PR outside it. Gate the expensive build steps instead, via a " +
			"changes job + step-level `if:`, the way go-binary already does.")
	}

	changes, ok := wf.Jobs["changes"]
	if !ok {
		t.Fatal("reproducibility.yml has no `changes` job -- go-binary's double build has nothing to gate on")
	}
	if _, ok := changes.Outputs["repro"]; !ok {
		t.Fatalf("changes job's outputs (%v) has no \"repro\" key -- go-binary's build step has nothing to gate on", changes.Outputs)
	}

	goBinary, ok := wf.Jobs["go-binary"]
	if !ok {
		t.Fatal("reproducibility.yml has no go-binary job")
	}
	if !needsContains(goBinary.Needs, "changes") {
		t.Errorf("go-binary's `needs:` (%v) does not include `changes`", goBinary.Needs)
	}

	// Deliberately targets the expensive step by name, not "any step
	// mentions needs.changes.outputs.repro" -- the "Skip (...)" step
	// mentions the same output (with a `!=` check) and would make a loose
	// substring match here pass even if the actual build step's gate were
	// dropped.
	const buildStepName = "Build the server twice from different paths"
	var buildStep *workflowStep
	for i := range goBinary.Steps {
		if goBinary.Steps[i].Name == buildStepName {
			buildStep = &goBinary.Steps[i]
			break
		}
	}
	if buildStep == nil {
		t.Fatalf("go-binary has no step named %q -- update this test to match the renamed step", buildStepName)
	}
	if !strings.Contains(buildStep.If, "needs.changes.outputs.repro") || !strings.Contains(buildStep.If, "== 'true'") {
		t.Errorf("go-binary's %q step has `if:` %q -- it must be gated on needs.changes.outputs.repro == 'true', "+
			"or the double build becomes unconditional, making every PR pay for it and falsifying the docs' "+
			"\"proves path-independence ... not per-PR\" wording", buildStepName, buildStep.If)
	}
}

// TestIndependentRebuildJobExists is the structural half of issue #947's
// fix: a true independent rebuild (two different runner images, not two
// paths on one runner) must exist, gated to the weekly schedule / manual
// dispatch rather than every PR.
func TestIndependentRebuildJobExists(t *testing.T) {
	wf := loadWorkflow(t)

	build, ok := wf.Jobs["go-binary-independent-rebuild"]
	if !ok {
		t.Fatal("reproducibility.yml has no go-binary-independent-rebuild job -- issue #947's fix " +
			"requires a true independent rebuild (different runner) to substantiate the " +
			"\"byte-reproducible\" claim beyond same-runner path-independence")
	}
	if build.Strategy == nil {
		t.Fatal("go-binary-independent-rebuild has no strategy.matrix -- it must build on more than one runner")
	}
	runners := matrixValues(build.Strategy.Matrix, "runner")
	distinct := map[string]bool{}
	for _, r := range runners {
		distinct[r] = true
	}
	if len(distinct) < 2 {
		t.Fatalf("go-binary-independent-rebuild's matrix.runner has %d distinct runner(s) (%v) -- "+
			"needs at least 2 different runner images for the rebuild to be independent", len(distinct), runners)
	}

	for _, trigger := range []string{"schedule", "workflow_dispatch"} {
		if !strings.Contains(build.If, trigger) {
			t.Errorf("go-binary-independent-rebuild's `if:` (%q) does not mention %q -- "+
				"it must be restricted to the weekly schedule / manual dispatch, not run on every PR", build.If, trigger)
		}
	}
	if strings.Contains(build.If, "pull_request") {
		t.Errorf("go-binary-independent-rebuild's `if:` (%q) mentions pull_request -- "+
			"the independent rebuild is meant to be weekly/manual only, not a per-PR gate", build.If)
	}

	compare, ok := wf.Jobs["go-binary-independent-rebuild-compare"]
	if !ok {
		t.Fatal("reproducibility.yml has no go-binary-independent-rebuild-compare job -- " +
			"go-binary-independent-rebuild's per-runner hashes are never actually compared")
	}
	if !needsContains(compare.Needs, "go-binary-independent-rebuild") {
		t.Errorf("go-binary-independent-rebuild-compare's `needs:` (%v) does not include "+
			"go-binary-independent-rebuild", compare.Needs)
	}
}

// TestDocsDoNotOverclaimUnconditionalGating is the doc-accuracy half of
// #947: while go-binary's actual double build is gated on the `changes` job
// (asserted by TestGoBinaryCheckAlwaysRunsButBuildStaysGated), the security
// docs must not repeat the flat "gated every PR" claim that shipped before
// the #947 fix -- it reads as an unconditional guarantee the path filter
// doesn't provide, and stays wrong even now that the *check itself* runs
// every PR, because the *double build* still does not.
func TestDocsDoNotOverclaimUnconditionalGating(t *testing.T) {
	const overclaim = "gated every PR"
	for _, path := range []string{releaseVerificationRel, reproducibleBuildsRel} {
		doc := readDoc(t, path)
		if strings.Contains(doc, overclaim) {
			t.Errorf("%s still contains %q -- go-binary's actual double build is gated on the `changes` "+
				"job's repro output, not every PR, so this reads as a false unconditional claim. Reword to "+
				"say the required check runs on every PR but the double build itself only runs when the PR "+
				"touches the backend/build inputs, and describe the same-runner double build as proving "+
				"path-independence, not full reproduction.", path, overclaim)
		}
	}
}

// TestDocsDescribeIndependentRebuild makes sure the two reproducibility
// docs actually mention the go-binary-independent-rebuild job's weekly
// cross-runner check, not just the per-PR same-runner one -- otherwise a
// reader has no way to learn the stronger claim is substantiated at all.
func TestDocsDescribeIndependentRebuild(t *testing.T) {
	for _, path := range []string{releaseVerificationRel, reproducibleBuildsRel} {
		doc := strings.ToLower(readDoc(t, path))
		if !strings.Contains(doc, "independent rebuild") {
			t.Errorf("%s does not mention \"independent rebuild\" -- it should describe the "+
				"go-binary-independent-rebuild job (weekly, two different runners) that substantiates "+
				"the byte-reproducible claim beyond the per-PR path-independence check", path)
		}
		if !strings.Contains(doc, "path-independen") { // matches path-independent/path-independence
			t.Errorf("%s does not mention path-independence -- it should describe what the per-PR "+
				"same-runner double build actually proves", path)
		}
	}
}
