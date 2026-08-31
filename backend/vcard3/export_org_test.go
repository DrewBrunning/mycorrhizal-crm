package vcard3

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// Concepts covered: org, org.unit.
func init() {
	registerExportCoverage("org", "org.unit")
}

func TestExport_Org(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Organizations: []contactmodel.Organization{{
			Name:  "Lotus Development Corporation",
			Units: []contactmodel.OrgUnit{{Name: "Sales"}, {Name: "East Region"}},
		}},
	}}
	out, _, err := (Adapter{}).Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, PropOrg, nil, "Lotus Development Corporation;Sales;East Region")
}

// TestExport_OrgSkipsAllEmptyContent pins the DATA-03 (issue #443) fix: an
// organization with no name and no non-empty unit has no semantic content
// (importOrganizations would not reconstruct it), so emitting an "ORG:;" line
// would churn the serialized form across a repeated conversion. It must emit
// no ORG line at all, and empty units must be filtered out of a named org.
func TestExport_OrgSkipsAllEmptyContent(t *testing.T) {
	t.Run("all_empty_org_emits_no_org_line", func(t *testing.T) {
		rec := &contactmodel.Record{Card: contactmodel.Card{
			Organizations: []contactmodel.Organization{{Name: "", Units: []contactmodel.OrgUnit{{Name: ""}}}},
		}}
		out, _, err := (Adapter{}).Export(rec)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		if strings.Contains(string(out), "ORG") {
			t.Errorf("all-empty organization emitted an ORG line, want none:\n%s", out)
		}
	})
	t.Run("empty_units_filtered_out", func(t *testing.T) {
		rec := &contactmodel.Record{Card: contactmodel.Card{
			Organizations: []contactmodel.Organization{{Name: "Acme", Units: []contactmodel.OrgUnit{{Name: ""}, {Name: "R&D"}}}},
		}}
		out, _, err := (Adapter{}).Export(rec)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		rfctest.AssertVCardLine(t, out, PropOrg, nil, "Acme;R&D")
	})
}
