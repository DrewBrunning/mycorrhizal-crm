package correspondence

import (
	"os"
	"strings"
	"testing"
)

// committedMatrixRel is the generated artifact, relative to this package
// (backend/correspondence -> repo docs/).
const committedMatrixRel = "../../docs/data-01-field-compatibility-matrix.md"

// isSpecial reports whether concept is governed by one of the table-grounded
// override maps (adapter-level redirects, passthrough escape hatches, and the
// documented lossy degradations) rather than the mechanical column rules.
func isSpecial(concept string) bool {
	if _, ok := v4Special[concept]; ok {
		return true
	}
	if _, ok := v3Special[concept]; ok {
		return true
	}
	if _, ok := jsSpecial[concept]; ok {
		return true
	}
	if _, ok := v4LossyReason[concept]; ok {
		return true
	}
	if _, ok := v3LossyReason[concept]; ok {
		return true
	}
	if _, ok := jsLossyReason[concept]; ok {
		return true
	}
	return false
}

// formatProp returns the correspondence-table column for a format's "home
// presence" signal (used by TestUnsupportedImpliesNoProp).
func formatProp(r Row, f Format) string {
	switch f {
	case FormatVCard4:
		return r.V4Prop
	case FormatVCard3:
		return r.V3Prop
	case FormatJSContact:
		return r.JSPtr
	}
	return ""
}

// --- (1) the committed doc is exactly what the generator produces -------------

// TestMatrixReproducesCommittedDoc is the drift test (issue #441 "How to
// verify", first bullet): a correspondence-table change that alters the matrix
// shows up as a reviewable diff, because this test fails until
// docs/data-01-field-compatibility-matrix.md is regenerated.
func TestMatrixReproducesCommittedDoc(t *testing.T) {
	got := Render()
	committed, err := os.ReadFile(committedMatrixRel)
	if err != nil {
		t.Fatalf("reading committed matrix %s: %v", committedMatrixRel, err)
	}
	if string(committed) != got {
		t.Errorf("%s is stale: it no longer matches the correspondence table — "+
			"regenerate with `cd backend && go run ./cmd/gencompatmatrix` and commit the diff",
			committedMatrixRel)
	}
}

// --- (2) every canonical field appears exactly once ---------------------------

// TestMatrixCoversEveryCorrespondenceRow asserts the matrix is a projection of
// the locked table (ADR-0002): every concept_id has exactly one matrix row with
// the table's own neutral path and transform, and no matrix row is foreign.
func TestMatrixCoversEveryCorrespondenceRow(t *testing.T) {
	table := ByConcept() // panics on a duplicate concept_id
	entries := Build()

	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Source != "correspondence" {
			continue
		}
		if seen[e.ConceptID] {
			t.Errorf("duplicate matrix row for concept_id %q", e.ConceptID)
		}
		seen[e.ConceptID] = true

		row, ok := table[e.ConceptID]
		if !ok {
			t.Errorf("matrix row %q has no correspondence-table row", e.ConceptID)
			continue
		}
		if e.NeutralPath != row.NeutralPath {
			t.Errorf("%q neutral path = %q, table says %q", e.ConceptID, e.NeutralPath, row.NeutralPath)
		}
		if e.Transform != row.Transform {
			t.Errorf("%q transform = %q, table says %q", e.ConceptID, e.Transform, row.Transform)
		}
	}
	for id := range table {
		if !seen[id] {
			t.Errorf("correspondence concept_id %q is missing from the matrix (silently absent, not unsupported)", id)
		}
	}
}

// TestNoHomeFieldsAppearAsUnsupported pins the issue #515 supplement: every
// canonical field with no correspondence row appears in the matrix, classified
// unsupported in every format — never as a missing row.
func TestNoHomeFieldsAppearAsUnsupported(t *testing.T) {
	entries := Build()
	var audit []Entry
	for _, e := range entries {
		if e.Source == "audit-515" {
			audit = append(audit, e)
		}
	}
	if len(audit) != len(noHomeFields) {
		t.Fatalf("matrix has %d audit-515 entries, want %d", len(audit), len(noHomeFields))
	}
	for _, e := range audit {
		if e.NoHome == nil {
			t.Errorf("%q: audit-515 entry has no NoHomeField", e.ConceptID)
			continue
		}
		for _, f := range []Format{FormatVCard4, FormatVCard3, FormatJSContact, FormatCardDAV} {
			if e.Cells[f].Bucket != BucketUnsupported {
				t.Errorf("%q/%s = %s, want unsupported (no home, never a missing row)",
					e.ConceptID, f, e.Cells[f].Bucket)
			}
			if e.Cells[f].Reason == "" {
				t.Errorf("%q/%s has an empty reason — DATA-02 needs the why", e.ConceptID, f)
			}
		}
	}
}

// --- (3) the classification obeys the table columns ---------------------------

// TestBucketInvariants checks the mechanical rules that keep the matrix a
// projection of the table rather than an independent invention:
//
//   - exact      -> the row's transform is identity
//   - unsupported -> the format's property column is "-" (no home)
//   - extended    -> an X- property, or one of the table-grounded escape
//     hatches / redirects (issue #514)
//   - lossy       -> a documented reduced-fidelity degradation
//   - transformed -> a declared value transform (or a noted v3 redirect)
//
// Each check runs only on cells the mechanical rule governs; the special cases
// are asserted separately by TestSpecialCasesAreTableGrounded.
func TestBucketInvariants(t *testing.T) {
	for _, e := range Build() {
		if e.Source != "correspondence" {
			continue
		}
		for _, f := range []Format{FormatVCard4, FormatVCard3, FormatJSContact} {
			c := e.Cells[f]
			row := ByConcept()[e.ConceptID]
			prop := formatProp(row, f)
			mechanical := !isSpecial(e.ConceptID) &&
				!(f == FormatVCard3 && e.ConceptID == "related")

			switch c.Bucket {
			case BucketExact:
				if !isSpecial(e.ConceptID) && e.Transform != "identity" {
					t.Errorf("%q/%s is exact but transform is %q", e.ConceptID, f, e.Transform)
				}
			case BucketUnsupported:
				if e.NoHome == nil && prop != "-" {
					t.Errorf("%q/%s is unsupported but the column is %q (not a no-home field)", e.ConceptID, f, prop)
				}
			case BucketExtended:
				if mechanical && !strings.HasPrefix(prop, "X-") {
					t.Errorf("%q/%s is extended but column is %q", e.ConceptID, f, prop)
				}
			case BucketLossy:
				if _, ok := v4LossyReason[e.ConceptID]; ok {
					continue
				}
				if _, ok := v3LossyReason[e.ConceptID]; ok {
					continue
				}
				if _, ok := jsLossyReason[e.ConceptID]; ok {
					continue
				}
				if e.ConceptID == "related" {
					continue // v3 AGENT redirect: v3Special, still lossy
				}
				t.Errorf("%q/%s is lossy but has no documented degradation", e.ConceptID, f)
			case BucketTransformed:
				if e.Transform == "identity" && e.ConceptID != "adr.tz" {
					t.Errorf("%q/%s is transformed but transform is identity", e.ConceptID, f)
				}
			default:
				t.Errorf("%q/%s has unknown bucket %q", e.ConceptID, f, c.Bucket)
			}
		}
	}
}

// TestEveryCellHasABucket guards against an empty Bucket leaking into the doc.
func TestEveryCellHasABucket(t *testing.T) {
	for _, e := range Build() {
		for f, c := range e.Cells {
			if c.Bucket == "" {
				t.Errorf("%q/%s has no bucket", e.ConceptID, f)
			}
		}
	}
}

// --- (4) the CardDAV column tracks version negotiation ------------------------

// TestCardDAVColumnTracksNegotiation pins the "CardDAV constrains which vCard
// version is negotiated" claim: the column repeats the vCard 4.0 classification
// (the server default) and annotates the vCard 3.0 classification wherever it
// differs.
func TestCardDAVColumnTracksNegotiation(t *testing.T) {
	for _, e := range Build() {
		v4 := e.Cells[FormatVCard4]
		v3 := e.Cells[FormatVCard3]
		cd := e.Cells[FormatCardDAV]

		if cd.Bucket != v4.Bucket {
			t.Errorf("%q carddav = %s, want the vCard 4.0 default %s", e.ConceptID, cd.Bucket, v4.Bucket)
		}
		if v3.Bucket != v4.Bucket && !strings.Contains(cd.Reason, "v3:") {
			t.Errorf("%q carddav does not annotate the negotiated v3 divergence (%s)", e.ConceptID, v3.Bucket)
		}
		if v3.Bucket == v4.Bucket && strings.Contains(cd.Reason, "v3:") {
			t.Errorf("%q carddav annotates a v3 divergence but the buckets agree", e.ConceptID)
		}
	}
}

// --- (5) DATA-02 loss-report correspondence -----------------------------------

// TestLossReportCorrespondence asserts the bidirectional DATA-02 agreement
// (issue #441, "How to verify" fourth bullet): every unsupported/lossy cell
// across the three serialized formats has exactly one loss report, and every
// loss report is an unsupported/lossy cell — with the deliberate policy
// exclusions (CRM-local flags and relational tables, issue #515) excluded on
// both sides.
func TestLossReportCorrespondence(t *testing.T) {
	reports := LossReports()
	reportSet := make(map[string]bool, len(reports))
	for _, lr := range reports {
		key := lr.Concept + "|" + string(lr.Format)
		if reportSet[key] {
			t.Errorf("duplicate loss report for %s", key)
		}
		reportSet[key] = true
	}

	count := 0
	for _, e := range Build() {
		if e.NoHome != nil && !e.NoHome.LossReport {
			continue
		}
		for _, f := range serializedFormats {
			c := e.Cells[f]
			key := e.ConceptID + "|" + string(f)
			reportable := c.Bucket == BucketUnsupported || c.Bucket == BucketLossy
			if reportable {
				count++
				if !reportSet[key] {
					t.Errorf("%s is %s but has no DATA-02 loss report", key, c.Bucket)
				}
			} else if reportSet[key] {
				t.Errorf("loss report for %s but the cell is %s", key, c.Bucket)
			}
		}
	}
	if len(reports) != count {
		t.Errorf("LossReports() = %d, want %d (one per unsupported/lossy serialized cell)", len(reports), count)
	}
}

// TestPolicyExclusionsProduceNoLossReport pins that CRM-local flags and
// relational timeline tables (issue #515 policy exclusions) never surface as
// fidelity loss in DATA-02 — they are deliberate decisions, not losses.
func TestPolicyExclusionsProduceNoLossReport(t *testing.T) {
	for _, lr := range LossReports() {
		switch lr.Concept {
		case "crm.archived", "crm.is_favorite", "crm.notes", "crm.activities", "crm.reminders":
			t.Errorf("policy exclusion %s leaked into the loss reports", lr.Concept)
		}
	}
}

// TestClassificationForCorrespondence pins the runtime half of the DATA-02
// correspondence (issue #442): ClassificationFor — the lookup a runtime loss
// report uses — is exactly LossReports() in both directions. Every reportable
// (concept, format) resolves to the same bucket/reason as the registry, and
// nothing that is not in the registry resolves. This is what makes "every
// report must correspond to a matrix entry" enforceable at runtime, not just
// on the generated doc.
func TestClassificationForCorrespondence(t *testing.T) {
	// Matrix -> lookup: every registry entry resolves identically.
	for _, lr := range LossReports() {
		got, ok := ClassificationFor(lr.Concept, lr.Format)
		if !ok {
			t.Errorf("ClassificationFor(%s, %s) failed to resolve a registry entry", lr.Concept, lr.Format)
			continue
		}
		if got.Concept != lr.Concept || got.Format != lr.Format || got.Bucket != lr.Bucket || got.Reason != lr.Reason {
			t.Errorf("ClassificationFor(%s, %s) = %+v, registry says %+v", lr.Concept, lr.Format, got, lr)
		}
	}

	// Lookup -> matrix: nothing outside the registry resolves. Scan the full
	// matrix so a concept that is present-but-not-reportable (exact/transformed/
	// extended, or a policy-excluded no-home field) also does not resolve.
	for _, e := range Build() {
		for _, f := range serializedFormats {
			_, ok := ClassificationFor(e.ConceptID, f)
			want := e.Cells[f].Bucket == BucketUnsupported || e.Cells[f].Bucket == BucketLossy
			if e.NoHome != nil && !e.NoHome.LossReport {
				want = false
			}
			if ok != want {
				t.Errorf("ClassificationFor(%s, %s) ok=%v, matrix bucket %s (want %v)",
					e.ConceptID, f, ok, e.Cells[f].Bucket, want)
			}
		}
	}

	// A foreign concept never resolves.
	if _, ok := ClassificationFor("not.a.concept", FormatVCard4); ok {
		t.Errorf("ClassificationFor resolved a foreign concept")
	}
}

// --- (6) special cases trace to the table / adapters --------------------------

// TestSpecialCasesAreTableGrounded verifies every override key resolves to a
// correspondence row, so a removed concept can't leave an orphaned override
// silently misclassifying something else.
func TestSpecialCasesAreTableGrounded(t *testing.T) {
	table := ByConcept()
	for _, m := range []map[string]Cell{v4Special, v3Special, jsSpecial} {
		for concept := range m {
			if _, ok := table[concept]; !ok {
				t.Errorf("special-case override %q has no correspondence row", concept)
			}
		}
	}
	for _, m := range []map[string]string{v4LossyReason, v3LossyReason, jsLossyReason} {
		for concept := range m {
			if _, ok := table[concept]; !ok {
				t.Errorf("lossy override %q has no correspondence row", concept)
			}
		}
	}
}

// --- (7) the classification functions are total ---------------------------------

// TestClassifyBranches exercises the classification branches that no current
// correspondence row reaches (the locked table has a v4_prop/js_ptr for every
// row and no v4 X- property), so a future table change that reintroduces them
// doesn't silently misclassify. Each branch is the documented interpretation of
// the table's columns: "-" means no home, "X-" / a non-pointer js_ptr means an
// extension, a non-identity transform means transformed.
func TestClassifyBranches(t *testing.T) {
	// vCard 4.0: "-" -> unsupported.
	if c := classifyV4(Row{ConceptID: "synthetic.v4dash", V4Prop: "-", Transform: "identity"}); c.Bucket != BucketUnsupported {
		t.Errorf("classifyV4(\"-\") = %s, want unsupported", c.Bucket)
	}
	// vCard 4.0: "X-" -> extended.
	if c := classifyV4(Row{ConceptID: "synthetic.v4x", V4Prop: "X-FOO", Transform: "identity"}); c.Bucket != BucketExtended {
		t.Errorf("classifyV4(X-) = %s, want extended", c.Bucket)
	}
	// JSContact: "-" -> unsupported.
	if c := classifyJS(Row{ConceptID: "synthetic.jsdash", JSPtr: "-", Transform: "identity"}); c.Bucket != BucketUnsupported {
		t.Errorf("classifyJS(\"-\") = %s, want unsupported", c.Bucket)
	}
	// JSContact: a non-pointer (extension) js_ptr -> extended.
	if c := classifyJS(Row{ConceptID: "synthetic.jsesc", JSPtr: "(pointer keys)", Transform: "identity"}); c.Bucket != BucketExtended {
		t.Errorf("classifyJS(non-pointer) = %s, want extended", c.Bucket)
	}
}

// TestFormatName covers the CardDAV case and the default fallthrough.
func TestFormatName(t *testing.T) {
	for f, want := range map[Format]string{
		FormatVCard4:    "vCard 4.0",
		FormatVCard3:    "vCard 3.0",
		FormatJSContact: "JSContact",
		FormatCardDAV:   "CardDAV",
	} {
		if got := formatName(f); got != want {
			t.Errorf("formatName(%s) = %q, want %q", f, got, want)
		}
	}
	if got := formatName("bogus"); got != "bogus" {
		t.Errorf("formatName(bogus) = %q, want %q", got, "bogus")
	}
}

// --- (8) the doc contains every required section ------------------------------

// TestRenderedDocHasRequiredSections guards the committed artifact's shape:
// the matrix, the no-home supplement, and the DATA-02 input must all be
// present, and the no-home table must be non-empty.
func TestRenderedDocHasRequiredSections(t *testing.T) {
	doc := Render()
	for _, want := range []string{
		"## Bucket legend",
		"## CardDAV-on-the-wire",
		"## Matrix — correspondence concepts",
		"## Canonical fields with no neutral-model home (issue #515)",
		"## Loss reports (DATA-02 input)",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("rendered matrix is missing section %q", want)
		}
	}
	if !strings.Contains(doc, "`crm.gender`") {
		t.Errorf("rendered matrix is missing the issue #515 Gender canary")
	}
}
