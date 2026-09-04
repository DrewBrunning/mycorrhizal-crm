package perfbench

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// Artifacts renders the two committed files this suite produces: the baseline
// JSON (the regression gate) and the markdown report (adds indicative
// wall-clock time).
func (s Suite) Artifacts() (baselineJSON []byte, reportMD []byte, err error) {
	b, err := s.Baseline().Marshal()
	if err != nil {
		return nil, nil, err // # pragma: no cover — the baseline struct always marshals
	}
	return b, []byte(s.Report()), nil
}

// ArtifactPaths resolves the on-disk locations of the two committed files,
// given the repo root.
func ArtifactPaths(repoRoot string) (baselinePath, reportPath string) {
	return filepath.Join(repoRoot, "backend", "internal", "perfbench", BaselineFile),
		filepath.Join(repoRoot, ReportFile)
}

// WriteArtifacts writes both files under repoRoot.
func WriteArtifacts(repoRoot string, baselineJSON, reportMD []byte) error {
	baselinePath, reportPath := ArtifactPaths(repoRoot)
	if err := os.WriteFile(baselinePath, baselineJSON, 0o600); err != nil {
		return err
	}
	return os.WriteFile(reportPath, reportMD, 0o600)
}

// CheckBaseline reports whether the committed baseline.json differs from the
// freshly rendered bytes (a missing or unreadable file counts as stale). Only
// the baseline is drift-gated: it is deterministic. The markdown report
// carries per-run wall-clock medians and a generation date, so it is
// rewritten on every run and never compared.
func CheckBaseline(repoRoot string, baselineJSON []byte) (stalePath string, stale bool) {
	baselinePath, _ := ArtifactPaths(repoRoot)
	got, err := os.ReadFile(baselinePath) // #nosec G304 -- repo-relative build path, not request input
	if err != nil || !bytes.Equal(got, baselineJSON) {
		return baselinePath, true
	}
	return baselinePath, false
}

// WallClockTrendPath resolves the on-disk location of the committed rolling
// wall-clock record, given the repo root.
func WallClockTrendPath(repoRoot string) string {
	return filepath.Join(repoRoot, "backend", "internal", "perfbench", WallClockTrendFile)
}

// WriteWallClockTrend writes the rolling wall-clock record under repoRoot. It
// is not drift-gated (like the markdown reports) — every run rewrites it.
func WriteWallClockTrend(repoRoot string, trendJSON []byte) error {
	return os.WriteFile(WallClockTrendPath(repoRoot), trendJSON, 0o600)
}

// FindRepoRoot walks up from startDir (or the working directory when empty)
// until it finds the directory containing backend/go.mod.
func FindRepoRoot(startDir string) (string, error) {
	dir := startDir
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil { // # pragma: no cover — Getwd fails only if the cwd was deleted
			return "", err
		}
		dir = wd
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repo root (a dir containing backend/go.mod) not found above %s", startDir)
		}
		dir = parent
	}
}
