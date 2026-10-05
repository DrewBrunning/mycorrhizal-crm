package vcard3

import (
	"testing"

	vcard "github.com/emersion/go-vcard"

	"mycorrhizal/contactmodel"
)

// Concepts covered: adr.geo, adr.tz (the standalone GEO/TZ properties the
// adapter pairs back to their ADR). adr itself is registered by
// import_adr_test.go / export_adr_test.go.
func init() {
	registerImportCoverage("adr.geo", "adr.tz")
	registerExportCoverage("adr.geo", "adr.tz")
}

// twoAddressGeoRecord is a two-address card whose second address is the only
// one carrying coordinates and a time zone. Before issue #1442 the exporter
// emitted one ungrouped GEO/TZ and the importer reattached them to address 0
// by position.
func twoAddressGeoRecord() *contactmodel.Record {
	return &contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{
			{Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "No Geo"}}},
			{
				Components:  []contactmodel.AddressComponent{{Kind: "locality", Value: "Has Geo"}},
				Contexts:    []string{"work"},
				Coordinates: "geo:48.2,16.3",
				TimeZone:    "Europe/Vienna",
			},
		},
	}}
}

// groupOfValue returns the property group of the field carrying wantValue.
func groupOfValue(t *testing.T, fields []*vcard.Field, wantValue string) string {
	t.Helper()
	for _, f := range fields {
		if f.Value == wantValue {
			return f.Group
		}
	}
	t.Fatalf("no field with value %q among %d field(s)", wantValue, len(fields))
	return ""
}

// TestExport_AdrGeoTimeZoneGroupedWithTheirAddress pins the export-side
// association: GEO/TZ on a later address must share that address's property
// group, and the address with no companions must stay ungrouped.
func TestExport_AdrGeoTimeZoneGroupedWithTheirAddress(t *testing.T) {
	t.Parallel()
	out, _, err := (Adapter{}).Export(twoAddressGeoRecord())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	card := decodedProps(t, out)
	if len(card[PropAdr]) != 2 {
		t.Fatalf("ADR fields = %d, want 2", len(card[PropAdr]))
	}
	if card[PropAdr][0].Group != "" {
		t.Errorf("first address has no companions, want no group, got %q", card[PropAdr][0].Group)
	}
	grp := card[PropAdr][1].Group
	if grp == "" {
		t.Fatal("second address carries GEO/TZ and must be grouped")
	}
	if got := groupOfValue(t, card[PropGeo], "48.2;16.3"); got != grp {
		t.Errorf("GEO group = %q, want the second ADR's group %q", got, grp)
	}
	if got := groupOfValue(t, card[PropTz], "Europe/Vienna"); got != grp {
		t.Errorf("TZ group = %q, want the second ADR's group %q", got, grp)
	}
}

const adrGroupedGeoVCF = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"FN:Ada Lovelace\n" +
	"item1.ADR;TYPE=HOME:;;A Street\n" +
	"item2.ADR;TYPE=WORK:;;B Street\n" +
	"item2.GEO:48.2;16.3\n" +
	"item2.TZ:Europe/Vienna\n" +
	"END:VCARD\n"

// TestImport_AdrGeoTimeZoneGroupedToLaterAddress proves the importer honors
// the group rather than position: the GEO/TZ land on address 1, never address 0.
func TestImport_AdrGeoTimeZoneGroupedToLaterAddress(t *testing.T) {
	t.Parallel()
	rec, _, err := (Adapter{}).Import([]byte(adrGroupedGeoVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Addresses) != 2 {
		t.Fatalf("Addresses = %d, want 2", len(rec.Card.Addresses))
	}
	if got := rec.Card.Addresses[0].Coordinates; got != "" {
		t.Errorf("address 0 Coordinates = %q, want empty (the GEO belongs to address 1)", got)
	}
	if got := rec.Card.Addresses[0].TimeZone; got != "" {
		t.Errorf("address 0 TimeZone = %q, want empty (the TZ belongs to address 1)", got)
	}
	if got := rec.Card.Addresses[1].Coordinates; got != "geo:48.2,16.3" {
		t.Errorf("address 1 Coordinates = %q, want geo:48.2,16.3", got)
	}
	if got := rec.Card.Addresses[1].TimeZone; got != "Europe/Vienna" {
		t.Errorf("address 1 TimeZone = %q, want Europe/Vienna", got)
	}
}

// TestRoundTrip_AdrGeoTimeZoneOnLaterAddress is the acceptance case: a
// 2-address card with GEO+TZ only on address 1 round-trips back onto address 1.
func TestRoundTrip_AdrGeoTimeZoneOnLaterAddress(t *testing.T) {
	t.Parallel()
	out, _, err := (Adapter{}).Export(twoAddressGeoRecord())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	got, _, err := (Adapter{}).Import(out)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(got.Card.Addresses) != 2 {
		t.Fatalf("round-tripped Addresses = %d, want 2", len(got.Card.Addresses))
	}
	if s := got.Card.Addresses[0]; s.Coordinates != "" || s.TimeZone != "" {
		t.Errorf("address 0 = %+v, want no coordinates/time zone", s)
	}
	if s := got.Card.Addresses[1]; s.Coordinates != "geo:48.2,16.3" || s.TimeZone != "Europe/Vienna" {
		t.Errorf("address 1 coordinates/time zone = %q/%q, want geo:48.2,16.3 and Europe/Vienna", s.Coordinates, s.TimeZone)
	}
}

// twoSameTypeLabelRecord is a 2-address card whose ADRs share TYPE=WORK and
// each carry their own formatted FULL (LABEL). The old TYPE-matching importer
// attached one LABEL to both addresses.
func twoSameTypeLabelRecord() *contactmodel.Record {
	return &contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{
			{
				Components: []contactmodel.AddressComponent{{Kind: "name", Value: "A Street"}},
				Contexts:   []string{"work"},
				Full:       "Alpha address",
			},
			{
				Components: []contactmodel.AddressComponent{{Kind: "name", Value: "B Street"}},
				Contexts:   []string{"work"},
				Full:       "Beta address",
			},
		},
	}}
}

// TestExport_AdrLabelsSameTypeGroupedSeparately pins that two same-TYPE ADRs
// each with their own LABEL are grouped separately (so TYPE cannot be the
// pairing key).
func TestExport_AdrLabelsSameTypeGroupedSeparately(t *testing.T) {
	t.Parallel()
	out, _, err := (Adapter{}).Export(twoSameTypeLabelRecord())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	card := decodedProps(t, out)
	if len(card[PropAdr]) != 2 || len(card[PropLabel]) != 2 {
		t.Fatalf("ADR/LABEL fields = %d/%d, want 2/2", len(card[PropAdr]), len(card[PropLabel]))
	}
	g0, g1 := card[PropAdr][0].Group, card[PropAdr][1].Group
	if g0 == "" || g1 == "" {
		t.Fatalf("both ADRs carry a LABEL and must be grouped, got %q/%q", g0, g1)
	}
	if g0 == g1 {
		t.Fatalf("the two same-TYPE ADRs share group %q; their LABELs cannot be told apart", g0)
	}
	if got := groupOfValue(t, card[PropLabel], "Alpha address"); got != g0 {
		t.Errorf("Alpha label group = %q, want address 0's group %q", got, g0)
	}
	if got := groupOfValue(t, card[PropLabel], "Beta address"); got != g1 {
		t.Errorf("Beta label group = %q, want address 1's group %q", got, g1)
	}
}

const adrGroupedLabelsVCF = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"FN:Ada Lovelace\n" +
	"item1.ADR;TYPE=WORK:;;A Street\n" +
	"item1.LABEL;TYPE=WORK:Alpha address\n" +
	"item2.ADR;TYPE=WORK:;;B Street\n" +
	"item2.LABEL;TYPE=WORK:Beta address\n" +
	"END:VCARD\n"

// TestImport_AdrLabelsSameTypeGroupedSeparately proves the importer keeps two
// same-TYPE LABELs with their own ADR rather than broadcasting by TYPE.
func TestImport_AdrLabelsSameTypeGroupedSeparately(t *testing.T) {
	t.Parallel()
	rec, _, err := (Adapter{}).Import([]byte(adrGroupedLabelsVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Addresses) != 2 {
		t.Fatalf("Addresses = %d, want 2", len(rec.Card.Addresses))
	}
	if got := rec.Card.Addresses[0].Full; got != "Alpha address" {
		t.Errorf("address 0 Full = %q, want Alpha address", got)
	}
	if got := rec.Card.Addresses[1].Full; got != "Beta address" {
		t.Errorf("address 1 Full = %q, want Beta address", got)
	}
}

// TestRoundTrip_AdrLabelsSameType is the acceptance case: two same-TYPE ADRs
// each with their own LABEL round-trip separately.
func TestRoundTrip_AdrLabelsSameType(t *testing.T) {
	t.Parallel()
	out, _, err := (Adapter{}).Export(twoSameTypeLabelRecord())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	got, _, err := (Adapter{}).Import(out)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(got.Card.Addresses) != 2 {
		t.Fatalf("round-tripped Addresses = %d, want 2", len(got.Card.Addresses))
	}
	if got.Card.Addresses[0].Full != "Alpha address" || got.Card.Addresses[1].Full != "Beta address" {
		t.Errorf("round-tripped Full = %q/%q, want Alpha address / Beta address", got.Card.Addresses[0].Full, got.Card.Addresses[1].Full)
	}
}

const adrAmbiguousGeoVCF = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"FN:Ada Lovelace\n" +
	"ADR;TYPE=HOME:;;A Street\n" +
	"ADR;TYPE=HOME:;;B Street\n" +
	"GEO:48.2;16.3\n" +
	"END:VCARD\n"

// TestImport_AdrAmbiguousGeoDropped pins the "never misassign" rule: a
// standalone, ungrouped GEO on a multi-address card is dropped with a warn
// rather than attached to an arbitrary address.
func TestImport_AdrAmbiguousGeoDropped(t *testing.T) {
	t.Parallel()
	rec, diags, err := (Adapter{}).Import([]byte(adrAmbiguousGeoVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !hasWarn(diags, "adr.geo") {
		t.Errorf("diags = %+v, want a warn for ambiguous adr.geo", diags)
	}
	for i, a := range rec.Card.Addresses {
		if a.Coordinates != "" {
			t.Errorf("address %d Coordinates = %q, want empty (ambiguous GEO must not be misassigned)", i, a.Coordinates)
		}
	}
}

const adrAmbiguousLabelsVCF = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"FN:Ada Lovelace\n" +
	"ADR;TYPE=HOME:;;A Street\n" +
	"ADR;TYPE=HOME:;;B Street\n" +
	"LABEL;TYPE=HOME:Alpha\n" +
	"LABEL;TYPE=HOME:Beta\n" +
	"END:VCARD\n"

// TestImport_AdrAmbiguousLabelsDropped pins that two ungrouped same-TYPE ADRs
// with two same-TYPE LABELs are ambiguous by TYPE and the labels are dropped
// with a warn instead of being attached to both addresses.
func TestImport_AdrAmbiguousLabelsDropped(t *testing.T) {
	t.Parallel()
	rec, diags, err := (Adapter{}).Import([]byte(adrAmbiguousLabelsVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !hasWarn(diags, "adr.full") {
		t.Errorf("diags = %+v, want a warn for ambiguous adr.full", diags)
	}
	for i, a := range rec.Card.Addresses {
		if a.Full != "" {
			t.Errorf("address %d Full = %q, want empty (ambiguous LABEL must not be misassigned)", i, a.Full)
		}
	}
}
