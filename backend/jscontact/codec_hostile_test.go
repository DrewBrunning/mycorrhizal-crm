package jscontact

import (
	"strings"
	"testing"
)

// TestUnmarshal_UTF8BOM pins the TEST-04 js-bom fix: a UTF-8 BOM prefix must
// be stripped, not reject the card.
func TestUnmarshal_UTF8BOM(t *testing.T) {
	raw := "\xef\xbb\xbf{\"@type\":\"Card\",\"version\":\"1.0\",\"uid\":\"bom\",\"name\":{\"full\":\"Ada Lovelace\"}}"
	c, err := Unmarshal([]byte(raw))
	if err != nil {
		t.Fatalf("Unmarshal with BOM: %v", err)
	}
	if c.Name == nil || c.Name.Full != "Ada Lovelace" {
		t.Errorf("c.Name = %+v, want full Ada Lovelace", c.Name)
	}
}

// TestUnmarshal_NullRejected pins the TEST-04 js-null-card fix: top-level
// null is not a Card instance and must error rather than silently decode to
// an empty card.
func TestUnmarshal_NullRejected(t *testing.T) {
	for _, raw := range []string{"null", "  null  ", "\nnull\n"} {
		_, err := Unmarshal([]byte(raw))
		if err == nil || !strings.Contains(err.Error(), "not a JSContact Card") {
			t.Errorf("Unmarshal(%q) err = %v, want not-a-Card error", raw, err)
		}
	}
}

// TestUnmarshal_EmptyRejected: an empty document is not a Card either.
func TestUnmarshal_EmptyRejected(t *testing.T) {
	for _, raw := range []string{"", "   ", "\n"} {
		if _, err := Unmarshal([]byte(raw)); err == nil {
			t.Errorf("Unmarshal(%q) succeeded, want error", raw)
		}
	}
}

// TestAdapterImport_UTF8BOM pins the strip at the adapter entry point too:
// importUnknownTopLevel re-parses the raw bytes, so the strip must happen
// before both passes.
func TestAdapterImport_UTF8BOM(t *testing.T) {
	raw := "\xef\xbb\xbf{\"@type\":\"Card\",\"version\":\"1.0\",\"uid\":\"bom\",\"name\":{\"full\":\"Ada Lovelace\"},\"xCustom\":1}"
	rec, _, err := Adapter{}.Import([]byte(raw))
	if err != nil {
		t.Fatalf("adapter Import with BOM: %v", err)
	}
	if rec.Card.Name == nil || rec.Card.Name.Full != "Ada Lovelace" {
		t.Errorf("rec.Card.Name = %+v, want full Ada Lovelace", rec.Card.Name)
	}
	if _, ok := rec.Passthrough.JSContact["/xCustom"]; !ok {
		t.Errorf("Passthrough.JSContact = %+v, want /xCustom", rec.Passthrough.JSContact)
	}
}
