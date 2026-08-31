package vcard3

import (
	"encoding/json"
	"testing"
)

// Concept covered: pt.vcard.
func init() {
	registerImportCoverage("pt.vcard")
}

const passthroughImportVCF = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"FN:Frank Dawson\n" +
	"X-CUSTOM-PROP:hello\n" +
	"END:VCARD\n"

func TestImport_PassthroughVCard(t *testing.T) {
	rec, _, err := (Adapter{}).Import([]byte(passthroughImportVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var found bool
	for _, jp := range rec.Passthrough.VCard {
		if jp.Name == "x-custom-prop" {
			found = true
			var s string
			if uerr := json.Unmarshal(jp.Value, &s); uerr == nil && s != "hello" {
				t.Errorf("passthrough value = %q, want %q", s, "hello")
			}
		}
	}
	if !found {
		t.Errorf("Passthrough.VCard = %+v, want an x-custom-prop entry", rec.Passthrough.VCard)
	}
}

// TestImport_PassthroughOrderDeterministic pins the DATA-03 (issue #443) fix
// to importPassthrough: the input vCard is a map, so iterating it directly
// put Passthrough.VCard in a different order on every import — making the
// passthrough payload non-byte-stable and churning the serialized form across
// a repeated conversion. Property names must be captured in sorted order,
// exactly like vcard4.
func TestImport_PassthroughOrderDeterministic(t *testing.T) {
	raw := "BEGIN:VCARD\nVERSION:3.0\nFN:Test\nX-ZEBRA:a\nX-ALPHA:b\nEND:VCARD\n"
	rec1, _, err := (Adapter{}).Import([]byte(raw))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	rec2, _, err := (Adapter{}).Import([]byte(raw))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec1.Passthrough.VCard) != 2 {
		t.Fatalf("want 2 passthrough entries, got %d", len(rec1.Passthrough.VCard))
	}
	for i := range rec1.Passthrough.VCard {
		if rec1.Passthrough.VCard[i].Name != rec2.Passthrough.VCard[i].Name {
			t.Fatalf("passthrough order is nondeterministic: run1=%v run2=%v", rec1.Passthrough.VCard, rec2.Passthrough.VCard)
		}
	}
	if rec1.Passthrough.VCard[0].Name != "x-alpha" || rec1.Passthrough.VCard[1].Name != "x-zebra" {
		t.Errorf("passthrough order = [%s, %s], want sorted [x-alpha, x-zebra]", rec1.Passthrough.VCard[0].Name, rec1.Passthrough.VCard[1].Name)
	}
}
