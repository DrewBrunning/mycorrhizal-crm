// TEST-07 (issue #435) round-trip property: for every generated contact and
// every serialized format, import(export(c)) ≈ c under the semantic
// equivalence comparison from TEST-03 (backend/internal/semanticequal).
//
// The fixture-based TestRoundTrip_CanonicalFixture proves the cases someone
// thought of; this property generalizes it over arbitrarily generated neutral
// records (backend/internal/contactgen — the shared generator, whose shape is
// pinned to the correspondence oracle that also validates the fixture), so a
// shape bug the fixture does not happen to contain still fails the build with
// a shrunk counterexample. The two-tier degradation policy is classified by
// the same classify helper as the fixture test (ADR-0002), so a generated
// record that exercises a no-home field is expected to warn, not to fail.
//
// Iteration budget: RAPID_CHECKS (rapid's native env var) is tiered by CI
// trigger — 200 on PRs, 1000 on a push to main, 8000 on the nightly schedule
// (see unit-tests.yml). This is the cheap pure-in-memory property, so it is
// the one that can afford the deep nightly search.
package roundtrip

import (
	"bytes"
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
	"mycorrhizal/internal/contactgen"
	"mycorrhizal/internal/semanticequal"

	"pgregory.net/rapid"
)

// TestRoundTrip_Property is the load-bearing TEST-07 property. A failure here
// means the neutral model expresses a contact that one of the three formats
// cannot represent losslessly (or warns about) — exactly the class a
// hand-written fixture misses.
//
// The shrinking budget makes this property's value: rapid reduces a
// 400-field failing record to a small readable counterexample, which is what
// gets committed to the TEST-02 manifest when a real bug is found (the
// "persist failing seeds" requirement).
func TestRoundTrip_Property(t *testing.T) {
	t.Parallel()
	byConcept := correspondence.ByConcept()

	for _, f := range formats {
		f := f
		t.Run(f.name, rapid.MakeCheck(func(t *rapid.T) {
			rec := contactgen.Record(t)

			out, exportDiags, err := f.exporter.Export(rec)
			if err != nil {
				t.Fatalf("export must never fail on a generated neutral record (error is reserved for invalid format instances):\n  %v\n  record: %s", err, recordJSON(rec))
			}

			roundTripped, _, err := f.importer.Import(out)
			if err != nil {
				t.Fatalf("re-import of own export must parse:\n  %v\n  record: %s", err, recordJSON(rec))
			}

			report := semanticequal.Compare(rec, roundTripped)
			for _, d := range report.Differences {
				for _, problem := range classify(d, f, byConcept[d.Concept], exportDiags) {
					t.Errorf("%s\n  %s\n  record: %s", d.String(), problem, recordJSON(rec))
				}
			}
		}))
	}
}

// TestRepeatedConversion_Property is the DATA-03 (issue #443) idempotence
// property wired into the TEST-07 generative suite: for every generated
// contact and every serialized format, f(f(x)) must equal f(x) — the
// repeated-conversion version of the single round trip above. It runs the
// same assertions as TestRepeatedConversion_SameFormatStabilizes against
// generated contacts (not only the fixture, per the issue's recommendation),
// so a non-converging shape the manifest does not happen to contain fails the
// build with a shrunk counterexample.
//
// It is a natural rapid property and shrinks well: a record that fails
// idempotence shrinks to the smallest shape that still double-escapes,
// double-wraps, or fails to settle.
func TestRepeatedConversion_Property(t *testing.T) {
	t.Parallel()
	for _, f := range formats {
		f := f
		t.Run(f.name, rapid.MakeCheck(func(t *rapid.T) {
			rec := contactgen.Record(t)

			out1, diags1, err := f.exporter.Export(rec)
			if err != nil {
				t.Fatalf("export must never fail on a generated neutral record:\n  %v\n  record: %s", err, recordJSON(rec))
			}
			rec1, _, err := f.importer.Import(out1)
			if err != nil {
				t.Fatalf("re-import of own export must parse:\n  %v\n  record: %s", err, recordJSON(rec))
			}

			out2, diags2, err := f.exporter.Export(rec1)
			if err != nil {
				t.Fatalf("export of a previously-imported record must never fail:\n  %v\n  record: %s", err, recordJSON(rec))
			}
			rec2, _, err := f.importer.Import(out2)
			if err != nil {
				t.Fatalf("re-import of pass-2 output must parse:\n  %v\n  record: %s", err, recordJSON(rec))
			}

			if !bytes.Equal(out2, out1) {
				t.Fatalf("repeated conversion changed the serialized form — f(f(x)) must equal f(x)\n  pass 1: %s\n  pass 2: %s\n  record: %s", out1, out2, recordJSON(rec))
			}

			report := semanticequal.Compare(rec1, rec2)
			if !report.Equal() {
				t.Fatalf("repeated conversion is not idempotent-after-first-conversion:\n%s\n  record: %s", report.DiffText(), recordJSON(rec))
			}

			pt1, _ := json.Marshal(rec1.Passthrough)
			pt2, _ := json.Marshal(rec2.Passthrough)
			if !bytes.Equal(pt2, pt1) {
				t.Fatalf("repeated conversion changed the passthrough payload — unknown properties must be byte-stable\n  record: %s", recordJSON(rec))
			}
			if len(pt2) > len(pt1) {
				t.Fatalf("repeated conversion grew the passthrough payload from %d to %d bytes — double-wrapping would appear exactly here\n  record: %s", len(pt1), len(pt2), recordJSON(rec))
			}

			firstWarns := warnConcepts(diags1)
			for concept := range warnConcepts(diags2) {
				if !firstWarns[concept] {
					t.Fatalf("pass 2 introduced a warn diagnostic for concept %q that pass 1 did not emit — diagnostics must stabilize after the first conversion\n  record: %s", concept, recordJSON(rec))
				}
			}
		}))
	}
}

// recordJSON renders a generated record for failure output (the shrunk
// counterexample the property presents).
func recordJSON(rec *contactmodel.Record) string {
	data, err := json.Marshal(rec)
	if err != nil {
		return "unmarshalable record"
	}
	return string(data)
}
