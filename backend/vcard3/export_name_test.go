package vcard3

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Concepts covered: name.full, name.surname, name.given, name.given2,
// name.title, name.credential.
func init() {
	registerExportCoverage(
		"name.full", "name.surname", "name.given", "name.given2",
		"name.title", "name.credential",
	)
}

func nameRecord() *contactmodel.Record {
	return &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{
			Full: "Mr. John Q. Public, Esq.",
			Components: []contactmodel.NameComponent{
				{Kind: "surname", Value: "Public"},
				{Kind: "given", Value: "John"},
				{Kind: "given2", Value: "Quinlan"},
				{Kind: "title", Value: "Mr."},
				{Kind: "credential", Value: "Esq."},
			},
		},
	}}
}

func TestExport_NameFull(t *testing.T) {
	out, _, err := (Adapter{}).Export(nameRecord())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropFN, nil, "Mr. John Q. Public, Esq.")
}

func TestExport_NameComponents(t *testing.T) {
	out, _, err := (Adapter{}).Export(nameRecord())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropN, nil, `Public;John;Quinlan;Mr.;Esq.`)
}

func TestExport_NameFull_DerivedWhenEmpty(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{
			Components: []contactmodel.NameComponent{
				{Kind: "given", Value: "John"},
				{Kind: "surname", Value: "Public"},
			},
		},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropFN, nil, "John Public")
}

// TestExport_NameSkipsAllEmptyComponents pins the DATA-03 (issue #443) fix: a
// name whose components are all extra kinds (surname2/generation have no
// vCard 3.0 N slot) would serialize to a degenerate "N:;;;;" line that
// importName reads back with no components — churning the serialized form
// across a repeated conversion. The N line must not be emitted; the per-kind
// warn documents the loss.
func TestExport_NameSkipsAllEmptyComponents(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{
			Components: []contactmodel.NameComponent{{Kind: "surname2", Value: "von Trapp"}},
		},
	}}
	out, diags, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if hasProp(t, out, PropN) {
		t.Errorf("surname2-only name emitted an N line, want none:\n%s", out)
	}
	var warned bool
	for _, d := range diags {
		if d.Concept == "name.surname2" && d.Severity == "warn" {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warn Diagnostic for the dropped surname2 component; diags = %+v", diags)
	}
}
