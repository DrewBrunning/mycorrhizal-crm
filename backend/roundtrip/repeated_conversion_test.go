// TEST-03's repeated-conversion companion (DATA-03, issue #443). The semantic
// round-trip suite (roundtrip_test.go) proves a SINGLE conversion is faithful;
// this file proves conversions COMPOSE. CardDAV sync is a repeated-conversion
// machine by construction — every remote change re-serializes and re-parses,
// and reconcileContactSync is full-overwrite by design — so a per-conversion
// loss of 1% would compound silently across a sync history.
//
// The formal property under test is idempotence after the first conversion:
// f(f(x)) must equal f(x) under the ADR-0002-derived semantic comparison
// (semanticequal), even where f(x) != x because of an expected, declared
// transform. "Stabilizes after the first pass" is asserted three ways:
//
//   - serialized-form stability: passes 2..N are byte-identical to pass 1;
//   - successive-difference emptiness: the semantic diff between CONSECUTIVE
//     passes is empty from pass 2 onward — a test that only compares pass 1
//     with pass N can miss an oscillation that happens to land back on pass
//     1's shape;
//   - diagnostics stability: pass 2 onward must not accumulate new warn
//     diagnostics, or DATA-02 (issue #442)'s loss report becomes noise.
//
// Cross-format chains (vCard 4 -> vCard 3 -> vCard 4, the realistic mixed-
// client path, plus the reverse and a JSContact round-trip) are asserted to
// converge rather than degrade. Passthrough gets explicit attention: the
// unknown-property payload must be byte-stable across passes and never grow —
// double-wrapping, double-escaping, or unbounded growth would appear exactly
// there (ADR-0002 preserves unknown properties and re-emits them).
package roundtrip

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/semanticequal"

	"github.com/stretchr/testify/require"
)

// repeatedConversionPasses is the number of conversions run per format per
// fixture contact. The issue calls for "10 or more, not 2" — 12 is
// deliberately uncomfortable: a compounding loss that survives two passes has
// twelve chances to surface here.
const repeatedConversionPasses = 12

// crossFormatCycles is how many full traversals of each cross-format chain
// are run. Each cycle through the chains below is 3-4 conversions, so 5
// cycles is 15+ conversions — well past the "uncomfortable" bar.
const crossFormatCycles = 5

// warnConcepts returns the set of concept_ids that carry a warn diagnostic.
func warnConcepts(diags []contactmodel.Diagnostic) map[string]bool {
	set := make(map[string]bool)
	for _, d := range diags {
		if strings.EqualFold(d.Severity, "warn") {
			set[d.Concept] = true
		}
	}
	return set
}

// assertNoNewWarns fails the test if passDiags warns about a concept that
// firstDiags (the first conversion's diagnostics) did not — pass 2 onward
// must not accumulate new warns.
func assertNoNewWarns(t *testing.T, pass int, passDiags, firstDiags []contactmodel.Diagnostic) {
	t.Helper()
	first := warnConcepts(firstDiags)
	for concept := range warnConcepts(passDiags) {
		if !first[concept] {
			t.Errorf("pass %d introduced a warn diagnostic for concept %q that pass 1 did not emit — diagnostics must stabilize after the first conversion", pass, concept)
		}
	}
}

// passthroughBytes renders a record's Passthrough to canonical JSON bytes for
// the byte-stability / no-growth assertions.
func passthroughBytes(t *testing.T, rec *contactmodel.Record) []byte {
	t.Helper()
	data, err := json.Marshal(rec.Passthrough)
	require.NoError(t, err)
	return data
}

// TestRepeatedConversion_SameFormatStabilizes runs each format's conversion
// repeatedConversionPasses times over every TEST-02 fixture contact and
// asserts the result stabilizes after the first pass. A failure here means
// the format's exporter+importer is not idempotent-after-first-conversion:
// repeated CardDAV sync of that contact would keep mutating it.
func TestRepeatedConversion_SameFormatStabilizes(t *testing.T) {
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	for _, f := range formats {
		f := f
		for _, entry := range m.Contacts {
			entry := entry
			t.Run(f.name+"/"+entry.Name, func(t *testing.T) {
				rec := entry.Record()
				if entry.RecreatesVCardUIDOf != "" {
					rec.Card.UID = inheritedUID(m, entry.RecreatesVCardUIDOf)
				}

				out1, diags1, err := f.exporter.Export(rec)
				require.NoError(t, err, "export must never fail on mappable/unmappable data (error is reserved for invalid format instances)")

				rec1, _, err := f.importer.Import(out1)
				require.NoError(t, err, "re-import of own export must parse")

				pt1 := passthroughBytes(t, rec1)
				prev := rec1

				for pass := 2; pass <= repeatedConversionPasses; pass++ {
					outN, diagsN, err := f.exporter.Export(prev)
					require.NoError(t, err, "export of a previously-imported record must never fail")

					if !bytes.Equal(outN, out1) {
						t.Errorf("pass %d serialized output differs from pass 1 — passes 2..N must be byte-identical to pass 1 once the first conversion has settled\n  pass 1: %s\n  pass %d: %s", pass, out1, pass, outN)
					}

					recN, _, err := f.importer.Import(outN)
					require.NoError(t, err, "re-import of pass %d output must parse", pass)

					report := semanticequal.Compare(prev, recN)
					if !report.Equal() {
						t.Errorf("semantic diff between pass %d and pass %d must be empty from pass 2 onward (convergence, not just equality at N):\n%s", pass-1, pass, report.DiffText())
					}

					assertNoNewWarns(t, pass, diagsN, diags1)

					ptN := passthroughBytes(t, recN)
					if !bytes.Equal(ptN, pt1) {
						t.Errorf("pass %d changed the passthrough payload from pass 1 — unknown properties must be byte-stable across passes\n  pass 1: %s\n  pass %d: %s", pass, pt1, pass, ptN)
					}
					if len(ptN) > len(pt1) {
						t.Errorf("pass %d grew the passthrough payload from %d to %d bytes — double-wrapping would appear exactly here", pass, len(pt1), len(ptN))
					}

					prev = recN
				}
			})
		}
	}
}

// crossFormatChain declares one conversion path through multiple formats.
// The first and last formats are the same so a full traversal returns the
// record to its original form and consecutive traversals are directly
// comparable.
type crossFormatChain struct {
	name   string
	format []string
}

// crossFormatChains are the realistic mixed-client conversion paths: the
// issue's headline case (vCard 4 -> vCard 3 -> vCard 4 — a user with v4 and
// v3 clients), the reverse order, and a chain that passes through JSContact.
var crossFormatChains = []crossFormatChain{
	{name: "vcard4-vcard3-vcard4", format: []string{"vcard4", "vcard3", "vcard4"}},
	{name: "vcard3-vcard4-vcard3", format: []string{"vcard3", "vcard4", "vcard3"}},
	{name: "jscontact-vcard4-vcard3-jscontact", format: []string{"jscontact", "vcard4", "vcard3", "jscontact"}},
}

func formatByName(name string) format {
	for _, f := range formats {
		if f.name == name {
			return f
		}
	}
	panic("roundtrip: no format named " + name)
}

// TestRepeatedConversion_CrossFormatChainsConverge runs each cross-format
// chain over every TEST-02 fixture contact and asserts the chain converges
// rather than degrading: the record after traversal N+1 is semantically equal
// to the record after traversal N (idempotence-after-first-conversion applied
// to the composite chain transform), the passthrough payload is byte-stable
// and never grows, and no new warn diagnostics appear after the first
// traversal. Cross-format is where cardinality and structure loss compound.
func TestRepeatedConversion_CrossFormatChainsConverge(t *testing.T) {
	m, err := canonicalfixture.Read()
	require.NoError(t, err)

	for _, chain := range crossFormatChains {
		chain := chain
		steps := make([]format, len(chain.format))
		for i, name := range chain.format {
			steps[i] = formatByName(name)
		}

		for _, entry := range m.Contacts {
			entry := entry
			t.Run(chain.name+"/"+entry.Name, func(t *testing.T) {
				rec := entry.Record()
				if entry.RecreatesVCardUIDOf != "" {
					rec.Card.UID = inheritedUID(m, entry.RecreatesVCardUIDOf)
				}

				// recAfterCycle[c] is the record after c+1 full traversals;
				// warnByCycle[c] is the union of warn concepts emitted during
				// traversal c.
				recAfterCycle := make([]*contactmodel.Record, 0, crossFormatCycles)
				warnByCycle := make([]map[string]bool, 0, crossFormatCycles)

				for cycle := 0; cycle < crossFormatCycles; cycle++ {
					cur := rec
					cycleWarns := map[string]bool{}
					for _, s := range steps {
						out, diags, err := s.exporter.Export(cur)
						require.NoError(t, err, "chain export at cycle %d through %s must never fail", cycle, s.name)
						for c := range warnConcepts(diags) {
							cycleWarns[c] = true
						}
						cur, _, err = s.importer.Import(out)
						require.NoError(t, err, "chain re-import at cycle %d through %s must parse", cycle, s.name)
					}
					recAfterCycle = append(recAfterCycle, cur)
					warnByCycle = append(warnByCycle, cycleWarns)
					rec = cur
				}

				pt0 := passthroughBytes(t, recAfterCycle[0])

				for cycle := 1; cycle < crossFormatCycles; cycle++ {
					report := semanticequal.Compare(recAfterCycle[cycle-1], recAfterCycle[cycle])
					if !report.Equal() {
						t.Errorf("cycle %d changed the record produced by cycle %d — the chain must converge, not degrade:\n%s", cycle, cycle-1, report.DiffText())
					}

					ptC := passthroughBytes(t, recAfterCycle[cycle])
					if !bytes.Equal(ptC, pt0) {
						t.Errorf("cycle %d changed the passthrough payload from the first traversal — unknown properties must be stable across a cross-format chain", cycle)
					}
					if len(ptC) > len(pt0) {
						t.Errorf("cycle %d grew the passthrough payload from %d to %d bytes — double-wrapping would appear exactly here", cycle, len(pt0), len(ptC))
					}

					for concept := range warnByCycle[cycle] {
						if !warnByCycle[0][concept] {
							t.Errorf("cycle %d introduced a warn on concept %q that the first traversal did not — diagnostics must stabilize after the first traversal", cycle, concept)
						}
					}
				}
			})
		}
	}
}
