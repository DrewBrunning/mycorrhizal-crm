package vcard3

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Issue #966: a day-only reduced-precision date ("---DD", RFC 6350 §4.3)
// fell through vcard3's parseDatePartial default case and was mangled into
// a bogus Timestamp; formatDatePartial had no export case for it either.

func TestImport_AnniversaryBirthDayOnly(t *testing.T) {
	t.Parallel()
	rec, _, err := (Adapter{}).Import([]byte("BEGIN:VCARD\nVERSION:3.0\nFN:Day Only\nBDAY:---05\nEND:VCARD\n"))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Anniversaries) != 1 {
		t.Fatalf("Anniversaries = %+v, want exactly one", rec.Card.Anniversaries)
	}
	pd := rec.Card.Anniversaries[0].Date.Partial
	if pd == nil || pd.Year != nil || pd.Month != nil || pd.Day == nil || *pd.Day != 5 {
		t.Errorf("Date.Partial = %+v, want {Day: 5} with no Year/Month", pd)
	}
}

func TestExport_AnniversaryBirthDayOnly(t *testing.T) {
	t.Parallel()
	day := 5
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Anniversaries: []contactmodel.Anniversary{{
			Kind: "birth",
			Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Day: &day}},
		}},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropBday, nil, "---05")
}

func TestRoundTrip_AnniversaryBirthDayOnly(t *testing.T) {
	t.Parallel()
	day := 28
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Anniversaries: []contactmodel.Anniversary{{
			Kind: "birth",
			Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Day: &day}},
		}},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	roundTripped, _, err := (Adapter{}).Import(out)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(roundTripped.Card.Anniversaries) != 1 {
		t.Fatalf("Anniversaries = %+v, want exactly one", roundTripped.Card.Anniversaries)
	}
	pd := roundTripped.Card.Anniversaries[0].Date.Partial
	if pd == nil || pd.Year != nil || pd.Month != nil || pd.Day == nil || *pd.Day != 28 {
		t.Errorf("round-tripped Date.Partial = %+v, want {Day: 28} with no Year/Month", pd)
	}
}

// Issue #969: a vCard DATE-AND-OR-TIME timestamp value (BDAY given as a
// full timestamp rather than a reduced-precision date) was stored verbatim
// instead of being normalized to RFC3339 UTC.

func TestImport_AnniversaryBirthTimestampWithOffset(t *testing.T) {
	t.Parallel()
	rec, _, err := (Adapter{}).Import([]byte("BEGIN:VCARD\nVERSION:3.0\nFN:Offset\nBDAY:1985-04-12T10:00:00-05:00\nEND:VCARD\n"))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Anniversaries) != 1 {
		t.Fatalf("Anniversaries = %+v, want exactly one", rec.Card.Anniversaries)
	}
	date := rec.Card.Anniversaries[0].Date
	if date.Timestamp == nil {
		t.Fatalf("Date.Timestamp = nil, want a normalized UTC timestamp")
	}
	if want := "1985-04-12T15:00:00Z"; *date.Timestamp != want {
		t.Errorf("Date.Timestamp = %q, want %q (offset -05:00 converted to UTC)", *date.Timestamp, want)
	}
}

func TestImport_RevWithOffset(t *testing.T) {
	t.Parallel()
	rec, _, err := (Adapter{}).Import([]byte("BEGIN:VCARD\nVERSION:3.0\nFN:Rev Offset\nREV:19961022T090000-05\nEND:VCARD\n"))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if rec.Card.Updated == nil {
		t.Fatalf("Updated = nil")
	}
	if want := "1996-10-22T14:00:00Z"; rec.Card.Updated.UTC != want {
		t.Errorf("Updated.UTC = %q, want %q", rec.Card.Updated.UTC, want)
	}
}
