package releaseworkflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadResilience(t *testing.T) ResilienceFiles {
	t.Helper()
	root := repoRoot(t)
	read := func(rel ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{root}, rel...)...))
		require.NoError(t, err)
		return string(b)
	}
	return ResilienceFiles{
		Composer:   read(".github", "workflows", ComposerFile),
		Release:    read(".github", "workflows", ReleaseFile),
		Publish:    read(".github", "workflows", PublishFile),
		Zap:        read(".github", "workflows", ZapFile),
		MinVersion: read(".github", "workflows", MinVersionFile),
		ZapScript:  read(".github", "scripts", "zap-scan-gate.sh"),
		ZapgateSrc: read("backend", "cmd", "zapgate", "main.go"),
	}
}

// TestCommittedResilienceWiring is the check cmd/releasegatecheck runs, against
// the real files.
func TestCommittedResilienceWiring(t *testing.T) {
	assert.Empty(t, CheckResilience(loadResilience(t)))
}

// TestCheckResilienceCatchesEachRegression breaks one piece of the real wiring
// at a time and asserts the checker names it. Every case is a way the #1487
// flake-exposure reduction would silently stop working.
func TestCheckResilienceCatchesEachRegression(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, f *ResilienceFiles)
		want   string
	}{
		// --- the composer: skip inputs and per-gate skip conditions ---------
		{"skip_gates input removed", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "      skip_gates:\n", "      skip_gatez:\n")
		}, "must declare workflow_call input `skip_gates`"},
		{"carried_ledger input removed", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "      carried_ledger:\n", "      carried_ledgez:\n")
		}, "must declare workflow_call input `carried_ledger`"},
		{"mandatory gate loses its skip condition", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "    if: ${{ !contains(format(',{0},', inputs.skip_gates), ',unit-tests,') }}\n", "")
		}, "gate `unit-tests` must carry an `if:`"},
		{"gate skip condition names the wrong gate", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "',sast,')", "',sasx,')")
		}, "gate `sast` must carry an `if:`"},
		{"release-tier gate loses its skip condition", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "inputs.release_tier && !contains(format(',{0},', inputs.skip_gates), ',zap-dast,')", "inputs.release_tier")
		}, "gate `zap-dast` must carry an `if:`"},
		{"skip condition no longer reads the input", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "!contains(format(',{0},', inputs.skip_gates), ',schemathesis,')", "!contains(format(',{0},', inputs.skip_gatez), ',schemathesis,')")
		}, "gate `schemathesis` must carry an `if:`"},
		{"dependent gate loses !cancelled()", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "${{ !cancelled() && inputs.release_tier", "${{ inputs.release_tier")
		}, "gate `reference-clients-e2e` needs gate `android-tests`"},
		{"dependent gate stops accepting a skipped need", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, " || needs.android-tests.result == 'skipped'", "")
		}, "accept needs.android-tests.result of 'success' or 'skipped'"},
		{"candidate job made conditional", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "  build-candidate:\n    runs-on: ubuntu-latest\n", "  build-candidate:\n    if: inputs.release_tier\n    runs-on: ubuntu-latest\n")
		}, "must not be conditional"},
		{"results stops recording the ledger", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "go run ./cmd/releaseplan ledger", "go run ./cmd/releaseplan noop")
		}, "must record the gate ledger"},
		{"results stops passing skip_gates", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "          SKIP_GATES: ${{ inputs.skip_gates }}\n", "")
		}, "must pass inputs.skip_gates to the ledger step"},
		{"results stops passing carried_ledger", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "          CARRIED_LEDGER: ${{ inputs.carried_ledger }}\n", "")
		}, "must pass inputs.carried_ledger to the ledger step"},
		{"ledger not uploaded", func(t *testing.T, f *ResilienceFiles) {
			f.Composer = replaceOnce(t, f.Composer, "            release-gate-ledger.json\n", "")
		}, "must upload release-gate-ledger.json"},

		// --- release.yml: rerun_gates -------------------------------------
		{"rerun_gates input removed", func(t *testing.T, f *ResilienceFiles) {
			f.Release = replaceOnce(t, f.Release, "      rerun_gates:\n", "      rerun_gatez:\n")
		}, "must declare the `rerun_gates` dispatch input"},
		{"preflight stops planning the rerun", func(t *testing.T, f *ResilienceFiles) {
			f.Release = replaceOnce(t, f.Release, "go run ./cmd/releaseplan rerun", "go run ./cmd/releaseplan noop")
		}, "preflight needs a step `rerun`"},
		{"preflight loses the skip output", func(t *testing.T, f *ResilienceFiles) {
			f.Release = replaceOnce(t, f.Release, "      skip_gates: ${{ steps.rerun.outputs.skip_gates }}\n", "")
		}, "preflight must output `skip_gates`"},
		{"preflight loses the carried output", func(t *testing.T, f *ResilienceFiles) {
			f.Release = replaceOnce(t, f.Release, "      carried_ledger: ${{ steps.rerun.outputs.carried }}\n", "")
		}, "preflight must output `carried_ledger`"},
		{"validate stops passing skip_gates", func(t *testing.T, f *ResilienceFiles) {
			f.Release = replaceOnce(t, f.Release, "      skip_gates: ${{ needs.preflight.outputs.skip_gates }}\n", "")
		}, "`validate` must pass `skip_gates"},
		{"validate stops passing carried_ledger", func(t *testing.T, f *ResilienceFiles) {
			f.Release = replaceOnce(t, f.Release, "      carried_ledger: ${{ needs.preflight.outputs.carried_ledger }}\n", "")
		}, "`validate` must pass `carried_ledger"},
		{"readiness stops recording the ledger", func(t *testing.T, f *ResilienceFiles) {
			f.Release = replaceOnce(t, f.Release, "gate_ledger:$ledger[0], ", "")
		}, "must record the per-gate ledger as `gate_ledger`"},

		// --- docker-publish.yml: tag-time reuse ---------------------------
		{"reuse-decision job removed", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "  reuse-decision:\n    needs: validate-tag", "  reuse-decisioz:\n    needs: validate-tag")
		}, "has no `reuse-decision` job"},
		{"reuse-decision can block a release", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "    continue-on-error: true\n    permissions:\n      contents: read\n      # Read the release-readiness artifact.", "    permissions:\n      contents: read\n      # Read the release-readiness artifact.")
		}, "must be `continue-on-error: true`"},
		{"reuse-decision stops using the tool", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "go run ./cmd/releaseplan reuse", "go run ./cmd/releaseplan noop")
		}, "must decide with `go run ./cmd/releaseplan reuse`"},
		{"reuse-decision stops checking ancestry", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "merge-base --is-ancestor", "merge-base --noop")
		}, "must verify the validated commit is an ancestor"},
		{"reuse-decision stops reading the readiness artifact", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceAfter(t, f.Publish, "  reuse-decision:\n", "release-readiness-${RELEASE_TAG}", "something-${RELEASE_TAG}")
		}, "must read the release-readiness-<tag> artifact"},
		{"release-gate stops needing the decision", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "    needs: [validate-tag, reuse-decision]\n    if: ${{ inputs.override_reason == '' }}\n    uses:", "    needs: validate-tag\n    if: ${{ inputs.override_reason == '' }}\n    uses:")
		}, "`release-gate` must `needs: reuse-decision`"},
		{"release-gate stops passing the skip list", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "      skip_gates: ${{ needs.reuse-decision.outputs.skip_gates }}\n", "")
		}, "`release-gate` must pass `skip_gates`"},
		{"release-gate stops passing the carried ledger", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "      carried_ledger: ${{ needs.reuse-decision.outputs.carried }}\n", "")
		}, "`release-gate` must pass `carried_ledger`"},
		{"create-release stops recording the decision", func(t *testing.T, f *ResilienceFiles) {
			f.Publish = replaceOnce(t, f.Publish, "tag_time_battery", "tag_time_batterx")
		}, "must need `reuse-decision` and record its decision as `tag_time_battery`"},

		// --- the two bounded retries --------------------------------------
		{"zap-dast bypasses the retry wrapper", func(t *testing.T, f *ResilienceFiles) {
			f.Zap = replaceOnce(t, f.Zap, "run: bash .github/scripts/zap-scan-gate.sh", "run: go run ./cmd/zapgate")
		}, "must run the scan and gate through .github/scripts/zap-scan-gate.sh"},
		{"zapgate drops its blind exit code", func(t *testing.T, f *ResilienceFiles) {
			f.ZapgateSrc = replaceOnce(t, f.ZapgateSrc, "const exitBlind = 3", "const exitBlinx = 3")
		}, "must declare `const exitBlind`"},
		{"retry script drops its blind exit code", func(t *testing.T, f *ResilienceFiles) {
			f.ZapScript = replaceOnce(t, f.ZapScript, "exit_blind=3", "exit_blinx=3")
		}, "must declare `exit_blind=<n>`"},
		{"retry script and zapgate disagree on the code", func(t *testing.T, f *ResilienceFiles) {
			f.ZapScript = replaceOnce(t, f.ZapScript, "exit_blind=3", "exit_blind=4")
		}, "the DAST retry would never fire"},
		{"retry script retries every failure", func(t *testing.T, f *ResilienceFiles) {
			f.ZapScript = replaceOnce(t, f.ZapScript, `-ne "$exit_blind"`, `-ne 0`)
		}, "must retry ONLY on zapgate's blind exit code"},
		{"go floor leg bypasses the retry script", func(t *testing.T, f *ResilienceFiles) {
			f.MinVersion = replaceOnce(t, f.MinVersion, "bash ../.github/scripts/go-test-retry-failed.sh", "go test ./...")
		}, "job `go-minimum` must run its tests through"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := loadResilience(t)
			tc.mutate(t, &f)
			findings := CheckResilience(f)
			require.NotEmpty(t, findings, "the mutation must be caught")
			assert.Contains(t, joined(findings), tc.want)
		})
	}
}

func joined(ss []string) string {
	out := ""
	for _, s := range ss {
		out += s + "\n"
	}
	return out
}

func TestCheckResilienceParseErrors(t *testing.T) {
	good := loadResilience(t)
	for name, mutate := range map[string]func(*ResilienceFiles){
		"composer":    func(f *ResilienceFiles) { f.Composer = "jobs: [" },
		"release":     func(f *ResilienceFiles) { f.Release = "jobs: [" },
		"publish":     func(f *ResilienceFiles) { f.Publish = "jobs: [" },
		"zap":         func(f *ResilienceFiles) { f.Zap = "jobs: [" },
		"min-version": func(f *ResilienceFiles) { f.MinVersion = "jobs: [" },
	} {
		t.Run(name, func(t *testing.T) {
			f := good
			mutate(&f)
			findings := CheckResilience(f)
			require.NotEmpty(t, findings)
			assert.Contains(t, joined(findings), "does not parse")
		})
	}
}
