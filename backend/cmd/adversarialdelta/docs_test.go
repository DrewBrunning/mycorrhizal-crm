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

// TestReleaseWorkflowRunsTheDeltaGate pins issues #953 and #1195. The
// per-release adversarial-delta obligation is only real if a release path
// actually invokes the gate against the ledger; without this pin, deleting the
// wiring would leave the ledger/doc in place and silently unenforced. The gate
// command lives once in the shared release-obligations.sh (issue #1195), which
// release.yml's final path and promote-rc.yml's promotion both call, so all
// three files must keep referencing it.
func TestReleaseWorkflowRunsTheDeltaGate(t *testing.T) {
	cases := []struct {
		path string
		want []string
	}{
		{
			// The one definition of the gate command.
			".github/scripts/release-obligations.sh",
			[]string{"cmd/adversarialdelta", "adversarial-deltas.md", "-ack"},
		},
		{
			// release.yml's final-release path.
			".github/workflows/release.yml",
			[]string{"release-obligations.sh adversarial", "ack_adversarial_delta"},
		},
		{
			// The RC-promotion path release.yml defers to (issue #1195).
			".github/workflows/promote-rc.yml",
			[]string{"release-obligations.sh adversarial", "ack_adversarial_delta"},
		},
	}
	for _, tc := range cases {
		body, err := os.ReadFile(findRepoFile(t, tc.path))
		if err != nil {
			t.Fatalf("reading %s: %v", tc.path, err)
		}
		for _, want := range tc.want {
			if !bytes.Contains(body, []byte(want)) {
				t.Errorf("%s must reference %q so the per-release adversarial delta stays a gate (issues #953, #1195)", tc.path, want)
			}
		}
	}
}

// TestLedgerExists pins the ledger the gate reads (its default -ledger path).
func TestLedgerExists(t *testing.T) {
	if _, err := os.Stat(findRepoFile(t, adversarialdelta.LedgerFile)); err != nil {
		t.Errorf("%s must exist: %v", adversarialdelta.LedgerFile, err)
	}
}
