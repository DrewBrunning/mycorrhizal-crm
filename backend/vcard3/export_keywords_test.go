package vcard3

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Concept covered: keywords.
func init() {
	registerExportCoverage("keywords")
}

func TestExport_Keywords(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{Keywords: []string{"Family", "Friends"}}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropCategories, nil, "Family,Friends")
}

// TestExport_KeywordsSkipsEmpty pins the DATA-03 (issue #443) fix: empty
// keyword entries are dropped by every importer and ignored by the semantic
// comparison, so emitting them would produce a degenerate "CATEGORIES:" line
// that never survives a re-import — churning the serialized form across a
// repeated conversion.
func TestExport_KeywordsSkipsEmpty(t *testing.T) {
	t.Parallel()
	out, _, err := (Adapter{}).Export(&contactmodel.Record{Card: contactmodel.Card{Keywords: []string{"", ""}}})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if strings.Contains(string(out), "CATEGORIES") {
		t.Errorf("all-empty keywords emitted a CATEGORIES line, want none:\n%s", out)
	}
}
