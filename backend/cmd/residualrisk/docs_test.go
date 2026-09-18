package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// findRepoFile walks up from the working directory until it finds rel, so the
// test works from backend/cmd/residualrisk (the default `go test` cwd) and from
// the repo root.
func findRepoFile(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("%s not found from the working directory upward", rel)
		}
		dir = parent
	}
}

// TestReleaseWorkflowEmitsResidualRisk pins issue #953. The residual-risk
// statement is only part of the release artifact if release.yml runs the
// command and merges its JSON under the documented key; without this pin,
// deleting either half would leave the command orphaned and the artifact
// silently missing what it promises.
func TestReleaseWorkflowEmitsResidualRisk(t *testing.T) {
	body, err := os.ReadFile(findRepoFile(t, ".github/workflows/release.yml"))
	if err != nil {
		t.Fatalf("reading release.yml: %v", err)
	}
	for _, want := range []string{"cmd/residualrisk", "residual_risk"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("release.yml must reference %q so release-metadata.json carries the residual-risk statement (issue #953)", want)
		}
	}
	// TestMainExitEndToEnd also reads every declared source from the real repo,
	// so a renamed or removed accept list fails there as a clear test failure
	// rather than only at release-dispatch time.
}
