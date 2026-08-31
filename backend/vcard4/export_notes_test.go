package vcard4

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

func init() {
	registerExportCoverage("note", "keywords")
}

func TestExport_NoteAuthor(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Notes: []contactmodel.Note{{
			Note:    "This is some note.",
			Author:  &contactmodel.Author{Name: "John Doe", URI: "mailto:john@example.com"},
			Created: &contactmodel.Timestamp{UTC: "2022-11-22T15:18:23Z"},
		}},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, "NOTE", map[string]string{
		"AUTHOR": "mailto:john@example.com", "AUTHOR-NAME": "John Doe", "CREATED": "20221122T151823Z",
	}, "This is some note.")
}

func TestExport_Keywords(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{Keywords: []string{"family", "work"}}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, "CATEGORIES", nil, "family,work")
}

// TestExport_KeywordsSkipsEmpty pins the DATA-03 (issue #443) fix: empty
// keyword entries are dropped by every importer and ignored by the semantic
// comparison, so emitting them would produce a degenerate "CATEGORIES:" line
// that never survives a re-import — churning the serialized form across a
// repeated conversion. An all-empty keyword list must emit no CATEGORIES line
// at all, and empty entries must be filtered out of a mixed list.
func TestExport_KeywordsSkipsEmpty(t *testing.T) {
	t.Run("all_empty_keywords_emit_no_categories", func(t *testing.T) {
		out, _, err := Adapter{}.Export(&contactmodel.Record{Card: contactmodel.Card{Keywords: []string{"", ""}}})
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		if strings.Contains(string(out), "CATEGORIES") {
			t.Errorf("all-empty keywords emitted a CATEGORIES line, want none:\n%s", out)
		}
	})
	t.Run("empty_entries_filtered_out", func(t *testing.T) {
		out, _, err := Adapter{}.Export(&contactmodel.Record{Card: contactmodel.Card{Keywords: []string{"", "family", ""}}})
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		rfctest.AssertVCardLine(t, out, "CATEGORIES", nil, "family")
	})
}
