package apibaseline

import (
	"os"
	"testing"
)

// readFileBytes is a thin wrapper so the drift test's intent stays readable.
func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// mustMarshal serializes a baseline for round-tripping in tests.
func mustMarshal(t *testing.T, b *Baseline) []byte {
	t.Helper()
	data, err := b.Marshal()
	requireNoError(t, err)
	return data
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
