package perfbench

import (
	"bytes"
	"os"
	"path/filepath"
)

// Artifacts renders the two committed files a PERF-03 run produces: the
// data-movement baseline JSON (the regression gate) and its markdown report
// (adds the indicative byte counts and durations).
func (s DataMovementSuite) Artifacts() (baselineJSON []byte, reportMD []byte, err error) {
	b, err := s.Baseline().Marshal()
	if err != nil {
		return nil, nil, err // # pragma: no cover — the baseline struct always marshals
	}
	return b, []byte(s.Report()), nil
}

// DataMovementArtifactPaths resolves the on-disk locations of the two
// committed PERF-03 files, given the repo root.
func DataMovementArtifactPaths(repoRoot string) (baselinePath, reportPath string) {
	return filepath.Join(repoRoot, "backend", "internal", "perfbench", DataMovementBaselineFile),
		filepath.Join(repoRoot, DataMovementReportFile)
}

// WriteDataMovementArtifacts writes both PERF-03 files under repoRoot.
func WriteDataMovementArtifacts(repoRoot string, baselineJSON, reportMD []byte) error {
	baselinePath, reportPath := DataMovementArtifactPaths(repoRoot)
	if err := os.WriteFile(baselinePath, baselineJSON, 0o600); err != nil {
		return err
	}
	return os.WriteFile(reportPath, reportMD, 0o600)
}

// CheckDataMovementBaseline reports whether the committed
// datamovement-baseline.json differs from the freshly rendered bytes (a
// missing or unreadable file counts as stale). Only the baseline is
// drift-gated — the report carries per-run byte counts and a date.
func CheckDataMovementBaseline(repoRoot string, baselineJSON []byte) (stalePath string, stale bool) {
	baselinePath, _ := DataMovementArtifactPaths(repoRoot)
	got, err := os.ReadFile(baselinePath) // #nosec G304 -- repo-relative build path, not request input
	if err != nil || !bytes.Equal(got, baselineJSON) {
		return baselinePath, true
	}
	return baselinePath, false
}
