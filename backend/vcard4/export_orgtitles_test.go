package vcard4

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

func init() {
	registerExportCoverage("org", "org.unit", "title", "role")
}

func TestExport_OrgUnits(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Organizations: []contactmodel.Organization{{Name: "Example Corp", Units: []contactmodel.OrgUnit{{Name: "Sales"}, {Name: "East"}}}},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, "ORG", nil, "Example Corp;Sales;East")
}

// TestExport_OrganizationSkipsEmptyContent pins the DATA-03 (issue #443) fix:
// empty unit names are dropped by importOrganizations and ignored by the
// semantic comparison, so an organization with no name and no non-empty unit
// would serialize to a degenerate "ORG:;" line that never survives a
// re-import — churning the serialized form across a repeated conversion. Such
// an org must not emit an ORG line at all, and empty units must be filtered
// out of a named org.
func TestExport_OrganizationSkipsEmptyContent(t *testing.T) {
	t.Parallel()
	t.Run("all_empty_org_emits_no_org_line", func(t *testing.T) {
		rec := &contactmodel.Record{Card: contactmodel.Card{
			Organizations: []contactmodel.Organization{{Name: "", Units: []contactmodel.OrgUnit{{Name: ""}}}},
		}}
		out, _, err := Adapter{}.Export(rec)
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
		out, _, err := Adapter{}.Export(rec)
		if err != nil {
			t.Fatalf("Export: %v", err)
		}
		rfctest.AssertVCardLine(t, out, "ORG", nil, "Acme;R&D")
	})
}

func TestExport_TitleRoleOrganizationID(t *testing.T) {
	t.Parallel()
	// Reproduces golden fixture title-role.v4.vcf's GROUP-linking mechanism
	// (RFC 9555 §2.9.6): a Title with Kind=role and a matching
	// OrganizationID must share a synthetic GROUP prefix with its ORG.
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Organizations: []contactmodel.Organization{{ID: "ORG-1", Name: "ABC, Inc."}},
		Titles: []contactmodel.Title{
			{Name: "Research Scientist", Kind: "title"},
			{Name: "Project Leader", Kind: "role", OrganizationID: "ORG-1"},
		},
	}}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertVCardLine(t, out, "TITLE", nil, "Research Scientist")

	dec, err := parseVCardForTest(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	roleField := dec["ROLE"]
	orgField := dec["ORG"]
	if len(roleField) != 1 || len(orgField) != 1 {
		t.Fatalf("ROLE=%v ORG=%v", roleField, orgField)
	}
	if roleField[0].Group == "" || roleField[0].Group != orgField[0].Group {
		t.Errorf("ROLE.Group = %q, ORG.Group = %q, want equal and non-empty", roleField[0].Group, orgField[0].Group)
	}
}
