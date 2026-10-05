package vcard3

import (
	"testing"

	"mycorrhizal/correspondence"
)

// importCoverage / exportCoverage are populated by each import_*_test.go /
// export_*_test.go file's init(), one entry per correspondence concept_id it
// exercises. init() (rather than inside a Test func body) so registration
// happens unconditionally whenever the test binary starts, independent of
// any -run filter — this is what makes TestCoverage_* mechanically reliable
// rather than aspirational (docs/adrs/0003-golden-fixtures-external-test-oracle.md).
var (
	importCoverage = map[string]bool{}
	exportCoverage = map[string]bool{}
)

func registerImportCoverage(conceptIDs ...string) {
	for _, id := range conceptIDs {
		importCoverage[id] = true
	}
}

func registerExportCoverage(conceptIDs ...string) {
	for _, id := range conceptIDs {
		exportCoverage[id] = true
	}
}

// TestCoverage_AllMappedConceptsHaveImportAndExportTests iterates the
// correspondence matrix and asserts that every concept with an effective
// vCard 3.0 home has been exercised by at least one import test and one export
// test in this package (docs/adrs/0003-golden-fixtures-external-test-oracle.md).
//
// The gate keys on the classified v3 cell's bucket, not on the raw v3_prop
// column: a concept whose v3_prop is "-" but which the adapter still carries
// through an adapter-level redirect (adr.geo/adr.tz via standalone GEO/TZ,
// anniversary.wedding via X-ANNIVERSARY, related via AGENT —
// correspondence.Build()'s v3 cell is the single source for that) has a real
// v3 home and must be covered too. Keying on v3_prop alone is exactly what let
// adr.geo/adr.tz ship without an import/export integration test (issue #1442).
func TestCoverage_AllMappedConceptsHaveImportAndExportTests(t *testing.T) {
	t.Parallel()
	for _, entry := range correspondence.Build() {
		cell := entry.Cells[correspondence.FormatVCard3]
		if cell.Bucket == correspondence.BucketUnsupported {
			continue
		}
		if !importCoverage[entry.ConceptID] {
			t.Errorf("concept %q (v3 bucket %q, label %q) has no registered import test coverage", entry.ConceptID, cell.Bucket, cell.Label)
		}
		if !exportCoverage[entry.ConceptID] {
			t.Errorf("concept %q (v3 bucket %q, label %q) has no registered export test coverage", entry.ConceptID, cell.Bucket, cell.Label)
		}
	}
}
