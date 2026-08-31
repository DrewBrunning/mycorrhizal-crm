package jscontact

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

func init() {
	registerExportCoverage("note", "keywords")
}

func TestExport_NoteWithAuthor(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		UID: "note-author-example",
		Notes: []contactmodel.Note{
			{
				ID:      "N1",
				Note:    "This is some note.",
				Author:  &contactmodel.Author{Name: "John Doe", URI: "mailto:john@example.com"},
				Created: &contactmodel.Timestamp{UTC: "2022-11-22T15:18:23Z"},
			},
		},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/notes/N1/note", "This is some note.")
	rfctest.AssertJSONPointer(t, out, "/notes/N1/author/name", "John Doe")
	rfctest.AssertJSONPointer(t, out, "/notes/N1/created", "2022-11-22T15:18:23Z")
}

func TestExport_Keywords(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		UID:      "keywords-example",
		Keywords: []string{"family", "work"},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/keywords/family", true)
	rfctest.AssertJSONPointer(t, out, "/keywords/work", true)
}

// TestExport_Keywords_DuplicateWarns pins the TEST-07 (#435) fix:
// JSContact's keywords is a boolean-set, so duplicate neutral keywords
// collapse on export and must emit a warn diagnostic for the keywords
// concept (ADR-0002) rather than drop silently. The round-trip property
// found the silent dedupe.
func TestExport_Keywords_DuplicateWarns(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Keywords: []string{"e\u0301tude", "e\u0301tude"},
	}}
	_, diags, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !hasDiagWarn(diags, "keywords") {
		t.Errorf("diags = %+v, want a warn for concept keywords (duplicate collapse)", diags)
	}
}
