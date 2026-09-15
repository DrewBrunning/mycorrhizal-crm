package vcard4

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Issue #966: a day-only reduced-precision date ("---DD", RFC 6350 §4.3)
// was corrupted into a month on import, because parsePartialDate's "--"
// branch also matched the "---" prefix and read the first two digits as
// the month.

func TestImport_AnniversaryBirthDayOnly(t *testing.T) {
	t.Parallel()
	raw := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:day-only\r\nFN:Test\r\nBDAY:---05\r\nEND:VCARD\r\n")
	rec, _, err := Adapter{}.Import(raw)
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
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Anniversaries: []contactmodel.Anniversary{{
			Kind: "birth",
			Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Day: intPtr(5)}},
		}},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, "BDAY", nil, "---05")
}

func TestRoundTrip_AnniversaryBirthDayOnly(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Anniversaries: []contactmodel.Anniversary{{
			Kind: "birth",
			Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Day: intPtr(28)}},
		}},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	roundTripped, _, err := Adapter{}.Import(out)
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

// Issue #969: a vCard DATE-AND-OR-TIME timestamp value was stored verbatim
// on import instead of being normalized to RFC3339 UTC, so an offset form
// later reached JSContact's `utc` field unconverted — not RFC 9553's
// UTCDateTime, and read by a conformant consumer as if the offset were
// zero (a silent shift).

func TestImport_AnniversaryBirthTimestampWithOffset(t *testing.T) {
	t.Parallel()
	raw := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:ts-offset\r\nFN:Test\r\nBDAY:1985-04-12T10:00:00-05:00\r\nEND:VCARD\r\n")
	rec, _, err := Adapter{}.Import(raw)
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

func TestImport_AnniversaryBirthTimestampCompactWithOffset(t *testing.T) {
	t.Parallel()
	// The compact TIMESTAMP form with a short (hour-only) offset, exactly
	// as docs/specs/rfc6350-baseline.md's own example: "19961022T140000-05".
	raw := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:ts-compact-offset\r\nFN:Test\r\nBDAY:19850412T100000-05\r\nEND:VCARD\r\n")
	rec, _, err := Adapter{}.Import(raw)
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
		t.Errorf("Date.Timestamp = %q, want %q", *date.Timestamp, want)
	}
}

func TestImport_AnniversaryBirthTimestampCompactUTC(t *testing.T) {
	t.Parallel()
	raw := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:ts-compact-z\r\nFN:Test\r\nBDAY:19850412T150000Z\r\nEND:VCARD\r\n")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	date := rec.Card.Anniversaries[0].Date
	if date.Timestamp == nil || *date.Timestamp != "1985-04-12T15:00:00Z" {
		t.Errorf("Date.Timestamp = %v, want 1985-04-12T15:00:00Z", date.Timestamp)
	}
}

// TestImport_ZoneLessTimestampsStayVerbatim pins docs/adrs/0015-temporal-semantics.md
// Category 5 (and the `sem-timestamp-no-tz` adversarial fixture): a
// DATE-AND-OR-TIME value that carries no zone at all must never be assumed
// UTC by this fix — only a value that already states its own offset/Z is
// converted. This guards against the natural but wrong fix of adding a
// zone-less layout to vcardDateTimeLayouts.
func TestImport_ZoneLessTimestampsStayVerbatim(t *testing.T) {
	t.Parallel()
	raw := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:zone-less\r\nFN:Test\r\nREV:20260801T120000\r\nBDAY:1985-04-12T10:00:00\r\nEND:VCARD\r\n")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if rec.Card.Updated == nil || rec.Card.Updated.UTC != "20260801T120000" {
		t.Errorf("Updated = %+v, want the zone-less REV preserved verbatim", rec.Card.Updated)
	}
	if len(rec.Card.Anniversaries) != 1 || rec.Card.Anniversaries[0].Date.Timestamp == nil ||
		*rec.Card.Anniversaries[0].Date.Timestamp != "1985-04-12T10:00:00" {
		t.Errorf("Anniversaries = %+v, want the zone-less BDAY timestamp preserved verbatim", rec.Card.Anniversaries)
	}
}

func TestImport_UpdatedWithOffset(t *testing.T) {
	t.Parallel()
	raw := []byte("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:rev-offset\r\nFN:Test\r\nREV:19961022T090000-05\r\nEND:VCARD\r\n")
	rec, _, err := Adapter{}.Import(raw)
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
