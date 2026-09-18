package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/internal/adversarialdelta"
)

// findRepoFile walks up from the working directory until it finds rel, so the
// test works from backend/cmd/adversarialdelta (the default `go test` cwd) and
// from the repo root.
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

// TestReleaseWorkflowRunsTheDeltaGate pins issue #953. The per-release
// adversarial-delta obligation is only real if release.yml actually invokes the
// gate against the ledger; without this pin, deleting the step would leave the
// ledger/doc in place and silently unenforced.
func TestReleaseWorkflowRunsTheDeltaGate(t *testing.T) {
	body, err := os.ReadFile(findRepoFile(t, ".github/workflows/release.yml"))
	if err != nil {
		t.Fatalf("reading release.yml: %v", err)
	}
	for _, want := range []string{"cmd/adversarialdelta", "adversarial-deltas.md", "ack_adversarial_delta"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("release.yml must reference %q so the per-release adversarial delta stays a gate (issue #953)", want)
		}
	}
}

// TestLedgerExists pins the ledger the gate reads (its default -ledger path).
func TestLedgerExists(t *testing.T) {
	if _, err := os.Stat(findRepoFile(t, adversarialdelta.LedgerFile)); err != nil {
		t.Errorf("%s must exist: %v", adversarialdelta.LedgerFile, err)
	}
}
