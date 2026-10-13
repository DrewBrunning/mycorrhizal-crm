package releaseworkflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommittedDryRunPaperwork(t *testing.T) {
	m := loadManual(t)
	assert.Empty(t, CheckDryRunPaperwork(m.release, m.dryRun))
}

// Each case breaks one half of the dry-run/real split: either the dry run stops
// reporting, or (worse) a real release stops blocking.
func TestCheckDryRunPaperworkCatchesEachRegression(t *testing.T) {
	asvsReal := "            bash .github/scripts/release-obligations.sh asvs --ack \"${ACK_ASVS_CURRENT:-}\"\n"
	asvsDry := "            bash .github/scripts/release-obligations.sh asvs --ack \"${ACK_ASVS_CURRENT:-}\" --dry-run\n"
	cases := []struct {
		name   string
		mutate func(t *testing.T, m *manualFiles)
		want   string
	}{
		{"real branch also dry-runs", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, asvsReal, asvsDry)
		}, "exactly once without it"},
		{"branches swapped", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, asvsDry+"          else\n"+asvsReal, asvsReal+"          else\n"+asvsDry)
		}, "a swap would downgrade a real release"},
		{"dry-run branch dropped", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, asvsDry, "            true\n")
		}, "exactly once with --dry-run"},
		{"adversarial real branch softened", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, `adversarial --release "$VERSION" --ack "${ACK_ADVERSARIAL_DELTA:-}"`+"\n          fi", `adversarial --release "$VERSION" --ack "${ACK_ADVERSARIAL_DELTA:-}" --dry-run`+"\n          fi")
		}, "Gate - per-release adversarial delta"},
		{"step skipped in dry run", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "      - name: Gate - ASVS/MASVS re-verification obligation\n        if: ${{ env.IS_RC != 'true' }}",
				"      - name: Gate - ASVS/MASVS re-verification obligation\n        if: ${{ env.IS_RC != 'true' && !inputs.dry_run }}")
		}, "must not be skipped via"},
		{"continue-on-error", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, "      - name: Gate - ASVS/MASVS re-verification obligation\n",
				"      - name: Gate - ASVS/MASVS re-verification obligation\n        continue-on-error: true\n")
		}, "must not be continue-on-error"},
		{"summary section removed", func(t *testing.T, m *manualFiles) {
			m.release = replaceOnce(t, m.release, `echo "### Pending release paperwork"`, `echo "### nothing"`)
		}, "Pending release paperwork"},
		{"dry-run workflow passes an ack", func(t *testing.T, m *manualFiles) {
			m.dryRun = replaceOnce(t, m.dryRun, `-f "attest_manual_gates=${attest}"`, `-f "attest_manual_gates=${attest}" -f "ack_asvs_current=dry run"`)
		}, "must not pass `ack_asvs_current`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := loadManual(t)
			tc.mutate(t, &m)
			f := CheckDryRunPaperwork(m.release, m.dryRun)
			require.NotEmpty(t, f)
			assert.Contains(t, f[0], tc.want)
		})
	}
}
