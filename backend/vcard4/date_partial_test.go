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

// intPtrEqual reports whether two *int point to equal values (or are both
// nil); used by the table-driven parsePartialDate assertions below.
func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func derefInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// TestParsePartialDate exercises every reduced/partial date form RFC 6350
// §4.3 permits (day-only "---DD", month "--MM", month-day "--MMDD"/"--MM-DD",
// year "YYYY", year-month "YYYYMM"/"YYYY-MM", year-month-day
// "YYYYMMDD"/"YYYY-MM-DD"), plus the malformed-digit and wrong-length
// branches that silently drop a field (via a swallowed strconv.Atoi error)
// or the whole value (the `default: return nil` branch) rather than
// panicking. Issue #966 shipped a real bug in the "---" vs "--" prefix
// disambiguation, so this class of parsing logic is a repeat-risk, not a
// hypothetical.
func TestParsePartialDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		in                  string
		wantNil             bool
		wantY, wantM, wantD *int
	}{
		{name: "empty_string", in: "", wantNil: true},
		{name: "day_only_compact", in: "---05", wantD: intPtr(5)},
		{name: "day_only_invalid_digits", in: "---xy"},
		{name: "day_only_empty_core", in: "---"},
		{name: "double_dash_empty_core", in: "--"},
		{name: "month_only", in: "--05", wantM: intPtr(5)},
		{name: "month_only_invalid_digits", in: "--ab"},
		{name: "month_day_compact", in: "--0522", wantM: intPtr(5), wantD: intPtr(22)},
		{name: "month_day_dashed", in: "--05-22", wantM: intPtr(5), wantD: intPtr(22)},
		{name: "month_day_invalid_day_digits", in: "--05ab", wantM: intPtr(5)},
		{name: "year_only", in: "2025", wantY: intPtr(2025)},
		{name: "year_only_invalid_digits", in: "abcd"},
		{name: "year_month_compact", in: "202505", wantY: intPtr(2025), wantM: intPtr(5)},
		{name: "year_month_dashed", in: "2025-05", wantY: intPtr(2025), wantM: intPtr(5)},
		{name: "year_month_invalid_month_digits", in: "2025ab", wantY: intPtr(2025)},
		{name: "year_month_day_compact", in: "20250522", wantY: intPtr(2025), wantM: intPtr(5), wantD: intPtr(22)},
		{name: "year_month_day_dashed", in: "2025-05-22", wantY: intPtr(2025), wantM: intPtr(5), wantD: intPtr(22)},
		{name: "year_month_day_invalid_day_digits", in: "202505ab", wantY: intPtr(2025), wantM: intPtr(5)},
		{name: "invalid_length_3", in: "202", wantNil: true},
		{name: "invalid_length_5", in: "20250", wantNil: true},
		{name: "invalid_length_7", in: "2025052", wantNil: true},
		{name: "invalid_length_9", in: "202505220", wantNil: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parsePartialDate(tt.in)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("parsePartialDate(%q) = %+v, want nil", tt.in, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parsePartialDate(%q) = nil, want non-nil {Year:%v Month:%v Day:%v}",
					tt.in, derefInt(tt.wantY), derefInt(tt.wantM), derefInt(tt.wantD))
			}
			if !intPtrEqual(got.Year, tt.wantY) || !intPtrEqual(got.Month, tt.wantM) || !intPtrEqual(got.Day, tt.wantD) {
				t.Errorf("parsePartialDate(%q) = {Year:%v Month:%v Day:%v}, want {Year:%v Month:%v Day:%v}",
					tt.in, derefInt(got.Year), derefInt(got.Month), derefInt(got.Day),
					derefInt(tt.wantY), derefInt(tt.wantM), derefInt(tt.wantD))
			}
		})
	}
}

// TestFormatPartialDate exercises every non-empty PartialDate combination
// parsePartialDate can actually produce, plus the nil and all-fields-nil
// cases.
func TestFormatPartialDate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   *contactmodel.PartialDate
		want string
	}{
		{name: "nil_pointer", in: nil, want: ""},
		{name: "all_fields_nil", in: &contactmodel.PartialDate{}, want: ""},
		{name: "year_month_day", in: &contactmodel.PartialDate{Year: intPtr(1985), Month: intPtr(4), Day: intPtr(12)}, want: "1985-04-12"},
		{name: "year_month", in: &contactmodel.PartialDate{Year: intPtr(2025), Month: intPtr(5)}, want: "2025-05"},
		{name: "year_only", in: &contactmodel.PartialDate{Year: intPtr(2025)}, want: "2025"},
		{name: "month_day", in: &contactmodel.PartialDate{Month: intPtr(5), Day: intPtr(22)}, want: "--05-22"},
		{name: "month_only", in: &contactmodel.PartialDate{Month: intPtr(5)}, want: "--05"},
		{name: "day_only", in: &contactmodel.PartialDate{Day: intPtr(5)}, want: "---05"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatPartialDate(tt.in); got != tt.want {
				t.Errorf("formatPartialDate(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestPartialDateRoundTrip pins parse(format(x)) == x for every partial-date
// shape, using formatPartialDate's own canonical (dashed extended) output as
// the wire value fed back into parsePartialDate.
func TestPartialDateRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		pd   *contactmodel.PartialDate
	}{
		{"year_month_day", &contactmodel.PartialDate{Year: intPtr(1985), Month: intPtr(4), Day: intPtr(12)}},
		{"year_month", &contactmodel.PartialDate{Year: intPtr(2025), Month: intPtr(5)}},
		{"year_only", &contactmodel.PartialDate{Year: intPtr(2025)}},
		{"month_day", &contactmodel.PartialDate{Month: intPtr(5), Day: intPtr(22)}},
		{"month_only", &contactmodel.PartialDate{Month: intPtr(5)}},
		{"day_only", &contactmodel.PartialDate{Day: intPtr(28)}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wire := formatPartialDate(tt.pd)
			got := parsePartialDate(wire)
			if got == nil {
				t.Fatalf("parsePartialDate(formatPartialDate(%+v)) = %q -> nil", tt.pd, wire)
			}
			if !intPtrEqual(got.Year, tt.pd.Year) || !intPtrEqual(got.Month, tt.pd.Month) || !intPtrEqual(got.Day, tt.pd.Day) {
				t.Errorf("round trip via %q = {Year:%v Month:%v Day:%v}, want {Year:%v Month:%v Day:%v}",
					wire, derefInt(got.Year), derefInt(got.Month), derefInt(got.Day),
					derefInt(tt.pd.Year), derefInt(tt.pd.Month), derefInt(tt.pd.Day))
			}
		})
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
