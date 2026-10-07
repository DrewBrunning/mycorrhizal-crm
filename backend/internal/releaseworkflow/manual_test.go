package releaseworkflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type manualFiles struct{ release, dryRun string }

func loadManual(t *testing.T) manualFiles {
	t.Helper()
	root := repoRoot(t)
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		require.NoError(t, err)
		return string(b)
	}
	return manualFiles{release: read(ReleaseFile), dryRun: read(DryRunFile)}
}

func TestCommittedManualGateWiring(t *testing.T) {
	m := loadManual(t)
	assert.Empty(t, CheckManualGates(m.release, m.dryRun))
}

// Each case breaks one piece of the real wiring; every one is a way the manual
// gate would silently stop being enforced (issue #1486).
func TestCheckManualGatesCatchesEachRegression(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, m *manualFiles)
		want   string
	}{
		{"input removed", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "      attest_manual_gates:\n", "      attest_manual_gatez:\n")
		}, "must declare the `attest_manual_gates` dispatch input"},
		{"input made optional", func(t *testing.T, m *manualFiles) {
			m.release = replaceAfter(t, m.release, "      attest_manual_gates:\n", "        required: true\n", "        required: false\n")
		}, "must be `required: true`"},
		{"preflight step removed", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "        id: manual\n", "        id: manuel\n")
		}, "preflight needs a step `manual`"},
		{"step stops running the checker", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "go run ./cmd/manualgatecheck check", "go run ./cmd/manualgatecheck noop")
		}, "must enforce with `go run ./cmd/manualgatecheck check`"},
		{"step stops computing git facts", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "bash .github/scripts/manual-gate-facts.sh ", "echo ")
		}, "must compute the git facts"},
		{"step stops passing the facts", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, `            -facts "${RUNNER_TEMP}/manual-gate-facts.json" -out`, `            -out`)
		}, "must pass the git facts and the release kind"},
		{"step stops passing the kind", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, `-kind "$kind" `, ``)
		}, "must pass the git facts and the release kind"},
		{"input interpolated into the script", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, `-attest "$ATTEST_MANUAL_GATES"`, `-attest "${{ inputs.attest_manual_gates }}"`)
		}, "interpolates inputs.attest_manual_gates into the script"},
		{"env stops reading the input", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "          ATTEST_MANUAL_GATES: ${{ inputs.attest_manual_gates }}\n", "          ATTEST_MANUAL_GATES: ''\n")
		}, "must take ATTEST_MANUAL_GATES from `inputs.attest_manual_gates`"},
		{"step made conditional", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "        id: manual\n", "        id: manual\n        if: ${{ !inputs.dry_run }}\n")
		}, "must be unconditional"},
		{"step made non-blocking", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "        id: manual\n", "        id: manual\n        continue-on-error: true\n")
		}, "must not be `continue-on-error`"},
		{"preflight output dropped", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "      manual_gates: ${{ steps.manual.outputs.manual_gates }}\n", "")
		}, "preflight must output `manual_gates`"},
		{"readiness stops recording the decisions", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "manual_gates:$manual_gates, ", "")
		}, "must record the decisions as `manual_gates`"},
		{"readiness reads the wrong output", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "          MANUAL_GATES: ${{ needs.preflight.outputs.manual_gates }}\n          RUN_URL", "          MANUAL_GATES: ''\n          RUN_URL")
		}, "step must read MANUAL_GATES from `needs.preflight.outputs.manual_gates`"},
		{"dry-run rehearsal stops passing the input", func(t *testing.T, m *manualFiles) {
			m.dryRun = replaceOnce(t, m.dryRun, `            -f "attest_manual_gates=${attest}"`, ``)
		}, "release-dry-run.yml must dispatch release.yml with `attest_manual_gates=...`"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := loadManual(t)
			c.mutate(t, &m)
			assert.Contains(t, joined(CheckManualGates(m.release, m.dryRun)), c.want)
		})
	}
}

func TestCheckManualGatesParseErrors(t *testing.T) {
	m := loadManual(t)
	assert.Contains(t, joined(CheckManualGates("jobs: [", m.dryRun)), "release.yml does not parse")
	assert.Contains(t, joined(CheckManualGates(m.release, "jobs: [")), "release-dry-run.yml does not parse")
}

func TestCheckManualGatesOnEmptyDocuments(t *testing.T) {
	assert.NotEmpty(t, CheckManualGates("", ""))
}
