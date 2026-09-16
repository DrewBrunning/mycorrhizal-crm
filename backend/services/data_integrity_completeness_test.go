package services

// DB-02 (issue #912) — the anti-rot gate for ADR 0012. The three lists this
// closes:
//
//   - the ADR's INV-* rows (docs/adrs/0012-canonical-database-invariants.md)
//   - the probe registry (dataIntegrityChecks() in data_integrity_service.go)
//   - the DB-03 matrix (db03MatrixCases() in data_integrity_matrix_test.go)
//
// were previously kept in sync by hand, with nothing to fail when they
// drifted: a new INV-D10 with no probe, or a new probe with no matrix row,
// was invisible (finding F6 of the #502 review). This file makes both
// directions mechanical:
//
//   - TestDB02_EveryADRInvariantIsCovered reads the ADR's own #### INV-*
//     headings and requires each to resolve to a DB-03 matrix invariant or an
//     opLevelCoverage citation.
//   - TestDB02_EveryRegisteredCheckIsCovered reads data_integrity_service.go's
//     own Check-slug literals and requires each to appear in db03MatrixCases()
//     or an elsewhereCheckCoverage citation.
//   - TestDB02_CoverageCitationsExist greps for every test name the two
//     ledgers above cite, so a rename that orphans a citation fails here
//     instead of silently weakening the gate.
//   - TestDB02_RegistryProbesAreAcknowledged calls dataIntegrityChecks()
//     directly (issue #912's point 1: nothing referenced it from a test) and
//     diffs its probe names against a restated ledger.
//
// A check or invariant that cannot be driven by a single at-rest mutation on
// the TEST-02 fixture (a code-defect-only probe like the relation-type
// registry involution, or an operation-level property like durability) is not
// a matrix gap — it is legitimately covered elsewhere, and both "elsewhere"
// ledgers below require a citation to a real, existing test rather than a
// bare exemption.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// coverageCitation names one committed "break the check" test proving an
// invariant or Check slug is actually exercised outside the DB-03 matrix.
type coverageCitation struct {
	file string // relative to backend/services
	test string // top-level `func Test...` name
}

// opLevelCoverage lists, for every ADR invariant with no at-rest probe
// (INV-D9 and every INV-A* except the cheap INV-A5 count — ADR 0012's own
// "Implemented by" scoping), the test the ADR's "Verified by" section already
// names. These are operation-level (durability, atomicity, idempotency,
// convergence) and cannot be expressed as a single mutation on the TEST-02
// fixture, so they never appear in db03MatrixCases().
var opLevelCoverage = map[string]coverageCitation{
	"INV-D9": {file: "search_consistency_service_test.go", test: "TestCheckSearchIndexConsistency_DetectsMissingFromIndex"},
	"INV-A1": {file: "../internal/propertytest/data_invariant_property_test.go", test: "TestDataInvariant_A1_CommittedStateSurvivesReopen"},
	"INV-A2": {file: "../internal/propertytest/data_invariant_property_test.go", test: "TestDataInvariant_A2_CancelledImportCommitsNothing"},
	"INV-A3": {file: "../internal/propertytest/idempotency_property_test.go", test: "TestIdempotentMutation_RetryIsFixpoint"},
	"INV-A4": {file: "../internal/propertytest/data_invariant_property_test.go", test: "TestDataInvariant_A4_ImportAccountsForEveryRecord"},
	"INV-A6": {file: "data_integrity_sync_property_test.go", test: "TestDataInvariant_A6_ContactSyncConvergesToAFixpoint"},
}

// elsewhereCheckCoverage lists Check slugs data_integrity_service.go emits
// that are deliberately verified outside db03MatrixCases(), each with a real
// citation:
//
//   - relationship_type.registry_inconsistent (INV-D2) can only fire when
//     models.KnownRelationTypes()'s registry itself is a broken involution —
//     a code defect, not a state the TEST-02 fixture can be mutated into.
//   - canonical_record.unresolved_remote_photo (INV-D8) already has a
//     dedicated probe-level test pinning both the violation and its clean
//     counterpart (TestDataIntegrity_INV_D8_DownloadedRemotePhotoIsClean).
var elsewhereCheckCoverage = map[string]coverageCitation{
	"relationship_type.registry_inconsistent":  {file: "data_integrity_service_test.go", test: "TestDataIntegrity_INV_D2_RegistryIsAConsistentInvolution"},
	"canonical_record.unresolved_remote_photo": {file: "data_integrity_service_test.go", test: "TestDataIntegrity_INV_D8_UnresolvedRemotePhoto"},
}

// knownProbeNames restates dataIntegrityChecks()'s registry so a probe added
// there without updating this list fails TestDB02_RegistryProbesAreAcknowledged
// — the #912 finding that nothing referenced the registry from a test.
var knownProbeNames = map[string]bool{
	"relationship_endpoints":     true,
	"relationship_type_registry": true,
	"orphaned_contact_refs":      true,
	"dangling_external_refs":     true,
	"dangling_field_values":      true,
	"missing_files":              true,
	"vanished_audit_refs":        true,
	"required_fields":            true,
	"canonical_records":          true,
	"derived_indexes":            true,
	"derived_contact_columns":    true,
}

var adrInvariantHeadingRe = regexp.MustCompile(`(?m)^#### (INV-[DA]\d+)\b`)

// adrInvariantIDs reads the ADR's own #### INV-* headings — the authoritative
// list of what the ADR currently claims — so an added/renamed/removed row is
// picked up with no separate list to keep in sync by hand.
func adrInvariantIDs(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "adrs", "0012-canonical-database-invariants.md")
	body, err := os.ReadFile(path)
	require.NoError(t, err, "reading %s", path)

	seen := map[string]bool{}
	var ids []string
	for _, m := range adrInvariantHeadingRe.FindAllStringSubmatch(string(body), -1) {
		id := m[1]
		if seen[id] {
			t.Fatalf("%s has more than one #### heading for %s", path, id)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	require.NotEmpty(t, ids, "found no '#### INV-*' headings in %s — did the heading format change?", path)
	sort.Strings(ids)
	return ids
}

var checkSlugLiteralRe = regexp.MustCompile(`\b(?:Check|missCheck|softCheck):\s*"([^"]+)"`)

// declaredCheckSlugs reads data_integrity_service.go's own source for every
// Check/missCheck/softCheck string literal — the full set of Check slugs the
// registry can currently emit — so the reverse-direction gate is checked
// against the real registry, not a hand-copied restatement of it.
func declaredCheckSlugs(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile("data_integrity_service.go")
	require.NoError(t, err)

	out := map[string]bool{}
	for _, m := range checkSlugLiteralRe.FindAllStringSubmatch(string(body), -1) {
		out[m[1]] = true
	}
	require.NotEmpty(t, out, "found no Check/missCheck/softCheck literals in data_integrity_service.go — did the field names change?")
	return out
}

// TestDB02_EveryADRInvariantIsCovered is the forward direction: every
// invariant the ADR currently declares must map to a DB-03 matrix row or an
// opLevelCoverage citation. An INV-D10 added to the ADR with neither fails
// here.
func TestDB02_EveryADRInvariantIsCovered(t *testing.T) {
	matrixInvariants := map[string]bool{}
	for _, c := range db03MatrixCases() {
		matrixInvariants[c.invariant] = true
	}

	for _, id := range adrInvariantIDs(t) {
		if matrixInvariants[id] {
			continue
		}
		if _, ok := opLevelCoverage[id]; ok {
			continue
		}
		t.Errorf("ADR 0012 declares %s but it has no db03MatrixCases() row (data_integrity_matrix_test.go) "+
			"and no opLevelCoverage citation (data_integrity_completeness_test.go) — "+
			"add a probe + matrix row (DB-01/#460, DB-03/#494), or an opLevelCoverage entry if it is "+
			"genuinely operation-level and not checkable against a database at rest", id)
	}

	// Reverse: an opLevelCoverage entry for an invariant the ADR no longer
	// declares would silently stop meaning anything.
	adrIDs := map[string]bool{}
	for _, id := range adrInvariantIDs(t) {
		adrIDs[id] = true
	}
	for id := range opLevelCoverage {
		if !adrIDs[id] {
			t.Errorf("opLevelCoverage cites %s, which ADR 0012 no longer declares", id)
		}
	}
}

// TestDB02_EveryRegisteredCheckIsCovered is the reverse direction the issue
// asks for explicitly: every Check slug the registry
// (data_integrity_service.go) can emit must appear in db03MatrixCases() or an
// elsewhereCheckCoverage citation. A new probe, or a new finding class added
// to an existing probe, with neither fails here.
func TestDB02_EveryRegisteredCheckIsCovered(t *testing.T) {
	declared := declaredCheckSlugs(t)

	inMatrix := map[string]bool{}
	for _, c := range db03MatrixCases() {
		inMatrix[c.wantCheck] = true
	}

	for slug := range declared {
		if inMatrix[slug] {
			continue
		}
		if _, ok := elsewhereCheckCoverage[slug]; ok {
			continue
		}
		t.Errorf("Check slug %q appears in data_integrity_service.go but has no db03MatrixCases() row "+
			"(data_integrity_matrix_test.go) and no elsewhereCheckCoverage citation "+
			"(data_integrity_completeness_test.go) — a probe finding with no test coverage is invisible", slug)
	}

	// Reverse: an elsewhereCheckCoverage entry for a slug the registry no
	// longer emits is a stale citation.
	for slug := range elsewhereCheckCoverage {
		if !declared[slug] {
			t.Errorf("elsewhereCheckCoverage cites Check slug %q, which data_integrity_service.go no longer emits", slug)
		}
	}
}

// TestDB02_RegistryProbesAreAcknowledged calls dataIntegrityChecks() directly
// — issue #912's point 1 was that nothing did — and diffs its probe names
// against knownProbeNames, so an added or renamed probe is not silently
// invisible to this file's other two tests (both of which key off Check
// slugs, not probe names, and would otherwise never notice a probe that
// happens to reuse an existing slug).
func TestDB02_RegistryProbesAreAcknowledged(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range dataIntegrityChecks() {
		registered[c.name] = true
		if !knownProbeNames[c.name] {
			t.Errorf("dataIntegrityChecks() registers probe %q, which is not in knownProbeNames "+
				"(data_integrity_completeness_test.go)", c.name)
		}
	}
	for name := range knownProbeNames {
		if !registered[name] {
			t.Errorf("knownProbeNames cites probe %q, which dataIntegrityChecks() no longer registers", name)
		}
	}
}

// TestDB02_CoverageCitationsExist greps backend/services and
// backend/internal/propertytest for every test name the two "elsewhere"
// ledgers cite, so a rename that orphans a citation fails here rather than
// silently weakening the gate (the same shape as int02Coverage's
// TestListedFailureTestsExist).
func TestDB02_CoverageCitationsExist(t *testing.T) {
	funcDecl := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	definedIn := func(t *testing.T, file string) map[string]bool {
		t.Helper()
		body, err := os.ReadFile(file)
		require.NoError(t, err, "reading %s", file)
		out := map[string]bool{}
		for _, m := range funcDecl.FindAllStringSubmatch(string(body), -1) {
			out[m[1]] = true
		}
		return out
	}

	check := func(label, id string, cit coverageCitation) {
		if !strings.HasSuffix(cit.file, "_test.go") {
			t.Errorf("%s[%s] cites %q, which is not a _test.go file", label, id, cit.file)
			return
		}
		defined := definedIn(t, cit.file)
		if !defined[cit.test] {
			t.Errorf("%s[%s] cites %q as defined in %s, but no such top-level test function was found there",
				label, id, cit.test, cit.file)
		}
	}

	for id, cit := range opLevelCoverage {
		check("opLevelCoverage", id, cit)
	}
	for slug, cit := range elsewhereCheckCoverage {
		check("elsewhereCheckCoverage", slug, cit)
	}
}
