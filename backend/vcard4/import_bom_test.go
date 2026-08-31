package vcard4

import (
	"strings"
	"testing"
)

// TestImport_UTF8BOM pins the TEST-04 str-bom fix: a UTF-8 BOM prefix
// (Windows exporters emit one) must be stripped, not reject the card.
func TestImport_UTF8BOM(t *testing.T) {
	raw := "\xef\xbb\xbfBEGIN:VCARD\r\nVERSION:4.0\r\nUID:bom-test\r\nFN:Ada Lovelace\r\nEND:VCARD\r\n"
	rec, _, err := Adapter{}.Import([]byte(raw))
	if err != nil {
		t.Fatalf("Import with BOM: %v", err)
	}
	if rec.Card.Name == nil || rec.Card.Name.Full != "Ada Lovelace" {
		t.Errorf("rec.Card.Name = %+v, want FN Ada Lovelace", rec.Card.Name)
	}
}

// TestImport_UTF8BOMNonVCardStillErrors: the strip must not turn a
// BOM-prefixed non-vCard into a silently-empty success.
func TestImport_UTF8BOMNonVCardStillErrors(t *testing.T) {
	raw := "\xef\xbb\xbfthis is not a vcard"
	_, _, err := Adapter{}.Import([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("err = %v, want a malformed-vCard error", err)
	}
}
