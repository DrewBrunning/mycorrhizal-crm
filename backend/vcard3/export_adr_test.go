package vcard3

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Concept covered: adr.
func init() {
	registerExportCoverage("adr")
}

func TestExport_Adr(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{{
			Components: []contactmodel.AddressComponent{
				{Kind: "name", Value: "6544 Battleford Drive"},
				{Kind: "locality", Value: "Raleigh"},
				{Kind: "region", Value: "NC"},
				{Kind: "postcode", Value: "27613-3502"},
				{Kind: "country", Value: "U.S.A."},
			},
			Contexts: []string{"work"},
		}},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropAdr, map[string]string{"TYPE": "WORK"},
		";;6544 Battleford Drive;Raleigh;NC;27613-3502;U.S.A.")
}

func TestExport_AdrLabel(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{{
			Components: []contactmodel.AddressComponent{{Kind: "name", Value: "123 Main Street"}},
			Contexts:   []string{"private"},
			Full:       "123 Main Street, Any Town",
		}},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropLabel, map[string]string{"TYPE": "HOME"}, "123 Main Street, Any Town")
}

// TestExport_AdrCountryCodeWarns pins the ADR-0002 degradation for a field
// with no 3.0 home: RFC 2426 ADR has no CC parameter, so a non-empty
// CountryCode must produce a warn diagnostic (concept "adr") rather than drop
// silently (issue #431's "a field with no target-format home produces a warn
// diagnostic rather than a silent drop").
func TestExport_AdrCountryCodeWarns(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{{
			Components:  []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
			CountryCode: "GB",
		}},
	}}
	out, diags, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !hasWarn(diags, "adr") {
		t.Errorf("diags = %+v, want a warn for concept adr (country code dropped)", diags)
	}
	if hasProp(t, out, PropAdr) == false {
		t.Error("the ADR property itself must still be emitted")
	}
}

// TestExport_AdrSkipsDegenerateExtraKinds pins the DATA-03 (issue #443) fix:
// an address whose components are all extra kinds (no vCard 3.0 ADR position,
// e.g. "room") with no Full/Coordinates/TimeZone to carry would serialize to
// a degenerate "ADR:;;;;;;" line that importAddresses reads back as an empty
// address — churning the serialized form across a repeated conversion. The
// line must not be emitted, but the extra-kind warn must still fire so the
// drop is documented, never silent.
func TestExport_AdrSkipsDegenerateExtraKinds(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{{
			Components: []contactmodel.AddressComponent{{Kind: "room", Value: "A"}},
		}},
	}}
	out, diags, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if hasProp(t, out, PropAdr) {
		t.Errorf("extra-kind-only address emitted an ADR line, want none:\n%s", out)
	}
	if !hasWarn(diags, "adr") {
		t.Errorf("diags = %+v, want a warn for concept adr (extra component kinds dropped)", diags)
	}
}

// TestExport_AdrPostOfficeBox covers the postOfficeBox ADR slot (the only
// vCard 3.0 ADR component kind the round-trip property does not generate).
func TestExport_AdrPostOfficeBox(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Addresses: []contactmodel.Address{{
			Components: []contactmodel.AddressComponent{
				{Kind: "postOfficeBox", Value: "P.O. Box 123"},
				{Kind: "locality", Value: "Springfield"},
			},
		}},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropAdr, nil, "P.O. Box 123;;;Springfield;;;")
}
