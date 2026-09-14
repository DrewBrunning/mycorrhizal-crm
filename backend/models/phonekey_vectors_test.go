package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// phoneKeyVectors mirrors testdata/phonekey-vectors/vectors.json.
type phoneKeyVectors struct {
	Vectors []struct {
		Input  string `json:"input"`
		Digits string `json:"digits"`
		Key    string `json:"key"`
		Why    string `json:"why"`
	} `json:"vectors"`
	SameKeyGroups []struct {
		Why    string   `json:"why"`
		Inputs []string `json:"inputs"`
	} `json:"same_key_groups"`
}

// TestPhoneKeySharedVectors pins the Go implementation against the shared
// cross-implementation vector table (issue #963) that Android's PhoneKeyTest
// reads from the same file. The Android port (core/data PhoneKey) must agree
// with models.PhoneKey byte-for-byte, or a number normalized one way on-device
// and matched another way server-side silently stops finding itself — a shared
// table is what makes a one-sided change fail CI instead of shipping as a
// matching bug.
func TestPhoneKeySharedVectors(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "phonekey-vectors", "vectors.json"))
	if err != nil {
		t.Fatalf("read shared phonekey vectors: %v", err)
	}
	var table phoneKeyVectors
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("parse shared phonekey vectors: %v", err)
	}
	if len(table.Vectors) == 0 {
		t.Fatal("shared phonekey vectors are empty")
	}

	for _, v := range table.Vectors {
		if got := NormalizePhoneDigits(v.Input); got != v.Digits {
			t.Errorf("NormalizePhoneDigits(%q) = %q, want %q (%s)", v.Input, got, v.Digits, v.Why)
		}
		if got := PhoneKey(v.Input); got != v.Key {
			t.Errorf("PhoneKey(%q) = %q, want %q (%s)", v.Input, got, v.Key, v.Why)
		}
	}

	for _, group := range table.SameKeyGroups {
		if len(group.Inputs) < 2 {
			t.Fatalf("same_key_groups entry %q needs at least two inputs", group.Why)
		}
		want := PhoneKey(group.Inputs[0])
		if want == "" {
			t.Fatalf("same_key_groups entry %q first input keys to empty", group.Why)
		}
		for _, in := range group.Inputs[1:] {
			if got := PhoneKey(in); got != want {
				t.Errorf("PhoneKey(%q) = %q, want %q (same as %q; %s)", in, got, want, group.Inputs[0], group.Why)
			}
		}
	}
}
