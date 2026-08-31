package differential

import (
	"fmt"
	"sort"
	"strings"

	"mycorrhizal/correspondence"
)

// Direction names the two legs of a differential property:
//
//	ours -> reference: our exporter produces the format bytes; the reference
//	                   parses (and where applicable re-emits) them.
//	reference -> ours: the reference produces the format bytes; our importer
//	                   parses them.
const (
	dirOursToRef = "ours->reference"
	dirRefToOurs = "reference->ours"
)

// formatName ties a serialized format to the correspondence column that
// declares which concepts have a home in it. A concept with a home must
// survive a differential leg with the reference; a concept without one is
// format-inherently lost on both sides and is out of the differential's
// scope (the two-tier degradation policy, ADR-0002, already governs it in
// the round-trip suite). Passthrough concepts (pt.vcard / pt.jscontact) are
// excluded here too: the references do not preserve our exact unknown-
// property shapes, so passthrough fidelity is asserted by the round-trip
// suite, not by this one.
type formatName struct {
	label   string
	homeCol func(correspondence.Row) bool
}

var (
	formatVCard3 = formatName{
		label: "vcard3",
		homeCol: func(row correspondence.Row) bool {
			return row.V3Prop != "-"
		},
	}
	formatVCard4 = formatName{
		label: "vcard4",
		homeCol: func(row correspondence.Row) bool {
			return row.V4Prop != "-"
		},
	}
	formatJSContact = formatName{
		label: "jscontact",
		homeCol: func(row correspondence.Row) bool {
			return row.JSPtr != "-"
		},
	}
)

// inScope reports whether a concept is part of the differential comparison
// for a format: it has a correspondence home and is not a passthrough row.
func (f formatName) inScope(concept string) bool {
	if strings.HasPrefix(concept, "pt.") {
		return false
	}
	for _, row := range correspondence.Load() {
		if row.ConceptID == concept {
			return f.homeCol(row)
		}
	}
	return false
}

// Difference is the concept-level disagreement the differential reports. It
// is what semanticequal produces, re-exported here so the format legs can
// name a concept in failure output without importing the internal package's
// type all over the place.
type Difference struct {
	Concept  string
	ValuesA  []string
	ValuesB  []string
	presentA bool
	presentB bool
}

// DivergencePin pins ONE known reference-side disagreement: for one corpus
// entry, one format, one direction, the listed concepts may differ from the
// reference, and that is documented (Reason), not a bug. It is the TEST-08
// analogue of #496's pinned Radicale divergences.
type DivergencePin struct {
	CorpusID string
	Format   string
	Dir      string
	Concepts []string
	Reason   string
}

// registry holds every accepted reference-side divergence. It is appended
// to as the suite is brought up against the pinned references; the drift
// test (TestPinsAreCurrent) keeps it honest.
var registry = []DivergencePin{}

// Per-format registration guards: each format's pin table registers exactly
// once even though several tests may run in the same binary.
var (
	vcardPinsRegistered     bool
	jscontactPinsRegistered bool
)

// checkPin registers a divergence pin. It is only callable from _test code
// (the registry is test-scoped state, never shipped).
func checkPin(pin DivergencePin) {
	registry = append(registry, pin)
}

// legResult is the outcome of running one differential leg against the
// corpus. The test harness calls reportDifferences with the raw semantic
// differences and the applicable pins, and receives the verdict.
type legResult struct {
	// Unexpected lists concept-level disagreements that no pin covers —
	// these are the failures (a concept with a home did not survive the
	// reference, or changed shape through it).
	Unexpected []Difference
	// Stale lists pins whose concept no longer disagrees — the reference
	// changed or the corpus card changed, and the pin is now a lie.
	Stale []string
}

// classifyLeg filters raw semantic differences (already restricted to
// in-scope concepts by the caller) through the pin registry for one
// (corpus entry, format, direction). It returns:
//
//   - unexpected disagreements (no matching pin) — the caller fails on these;
//   - stale pins (registered but not reproduced by any difference) — the
//     caller fails on these, so a pin that stops reproducing is surfaced
//     rather than silently trusted.
//
// Deterministic: results are sorted so failure output is stable.
func classifyLeg(diffs []Difference, entryID, format, dir string) legResult {
	var unexpected []Difference
	matched := map[string]bool{} // concept -> pinned

	for _, d := range diffs {
		if pinned(d, entryID, format, dir) {
			matched[d.Concept] = true
			continue
		}
		unexpected = append(unexpected, d)
	}
	sort.Slice(unexpected, func(i, j int) bool { return unexpected[i].Concept < unexpected[j].Concept })

	var stale []string
	for _, pin := range registry {
		if pin.CorpusID != entryID || pin.Format != format || pin.Dir != dir {
			continue
		}
		for _, concept := range pin.Concepts {
			if !matched[concept] {
				stale = append(stale, concept)
			}
		}
	}
	sort.Strings(stale)

	return legResult{Unexpected: unexpected, Stale: stale}
}

func pinned(d Difference, entryID, format, dir string) bool {
	for _, pin := range registry {
		if pin.CorpusID != entryID || pin.Format != format || pin.Dir != dir {
			continue
		}
		for _, concept := range pin.Concepts {
			if concept == d.Concept {
				return true
			}
		}
	}
	return false
}

// formatLabels lists every format the differential suite covers (for the
// pins-are-current test and for leg iteration).
var formatLabels = []struct {
	label string
	impl  formatName
}{
	{"vcard3", formatVCard3},
	{"vcard4", formatVCard4},
	{"jscontact", formatJSContact},
}

// DiffText renders an unexpected disagreement for failure output, naming the
// concept and both sides' values (never "outputs differ").
func DiffText(d Difference) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "concept %q: ", d.Concept)
	switch {
	case d.presentA && d.presentB:
		fmt.Fprintf(&sb, "values differ\n  ours/seed: %s\n  reference: %s", joinVals(d.ValuesA), joinVals(d.ValuesB))
	case d.presentA:
		fmt.Fprintf(&sb, "present in the seed but lost through the reference: %s", joinVals(d.ValuesA))
	case d.presentB:
		fmt.Fprintf(&sb, "present only after the reference round trip (reference synthesized it): %s", joinVals(d.ValuesB))
	default: // # pragma: no cover — diffConcept never emits a both-absent Difference, so this branch is structurally unreachable
		fmt.Fprintf(&sb, "unexpected (neither side present)")
	}
	return sb.String()
}

func joinVals(v []string) string { return "[" + strings.Join(v, ", ") + "]" }
