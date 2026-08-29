// Package semanticequal implements the semantic-equivalence comparison for
// round-trip testing (TEST-03, issue #431): a function over two
// contactmodel.Record values that answers "do these describe the same contact?"
// without demanding byte equality.
//
// The comparison is DRIVEN BY the correspondence oracle
// (docs/adrs/0002-correspondence-table-locked-oracle.md, materialized as
// backend/correspondence/testdata/correspondence.tsv and loaded by
// backend/correspondence). There is deliberately no parallel notion of "the
// same" authored here:
//
//   - the unit of comparison is the concept_id row: one row per concept,
//     with its neutral_path (where to read the value on a Record) and its
//     transform (how to normalize it) — that is exactly the granularity the
//     table declares, not the comparer's taste;
//   - repeatable properties (email, phone, nickname, ...) are compared as
//     unordered multisets, so property order never matters;
//   - normalization (timestamp -> UTC instant, partial date -> canonical
//     component tuple, enum_lower -> lowercase, ...) comes from the table's
//     transform column, applied on both sides identically.
//
// The comparison is format-agnostic: it compares the neutral surface only
// (Card + Passthrough). CRMEnvelope fields are deliberately out of scope —
// the envelope is never serialized by any format adapter ("Format adapters
// MUST ignore it entirely"), so a round-trip comparison cannot expect it to
// survive. The two-tier degradation policy is applied by the round-trip
// harness (backend/roundtrip, which classifies each reported difference
// against the format's home column and the exporter's warn diagnostics), not
// here: this package only reports what differs, per concept.
//
// Table discipline: every correspondence row MUST have a registered
// per-concept extractor (init() verifies this and panics on drift — a row
// the comparer cannot handle is the ADR's "escalate, never invent" signal),
// and every registered extractor MUST be backed by a row (no invented
// concepts). ID fields are not compared: element IDs (PROP-ID / JSContact
// map keys) are identity links, not content — a round trip that synthesizes
// or drops them must not fail a semantic comparison.
package semanticequal

import (
	"fmt"
	"sort"
	"strings"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
)

// Difference is one concept-level mismatch between two records.
type Difference struct {
	// Concept is the correspondence concept_id the mismatch is about.
	Concept string
	// PresentA / PresentB report which side carries a value for the concept.
	// PresentA && PresentB means both sides have values but they differ
	// (mismatched multiset); PresentA only means the value was lost in B
	// ("the first record has it, the second does not"); PresentB only means
	// B gained a value A never had (e.g. a format-synthesized FN).
	PresentA bool
	PresentB bool
	// ValuesA / ValuesB are the normalized extracted values on each side,
	// sorted for deterministic display.
	ValuesA []string
	ValuesB []string
}

// String renders a Difference for test failure output.
func (d Difference) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "concept %q: ", d.Concept)
	switch {
	case d.PresentA && d.PresentB:
		fmt.Fprintf(&sb, "values differ\n  a: %s\n  b: %s", join(d.ValuesA), join(d.ValuesB))
	case d.PresentA:
		fmt.Fprintf(&sb, "present only in the original record: %s", join(d.ValuesA))
	case d.PresentB:
		fmt.Fprintf(&sb, "present only in the round-tripped record: %s", join(d.ValuesB))
	default: // # pragma: no cover — diffConcept never emits a both-absent Difference
		sb.WriteString("unexpected (neither side present)")
	}
	return sb.String()
}

func join(v []string) string { return "[" + strings.Join(v, ", ") + "]" }

// Report is the full result of Compare: every concept-level difference found.
// An empty (nil) Differences slice means the two records are semantically
// equal per the correspondence table.
type Report struct {
	Differences []Difference
}

// Equal reports whether the two compared records were semantically equal.
func (r Report) Equal() bool { return len(r.Differences) == 0 }

// DiffText returns a human-readable rendering of every difference, or the
// empty string when the records are equal. It is meant for t.Errorf output.
func (r Report) DiffText() string {
	if r.Equal() {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("semantic round-trip differences:\n")
	for _, d := range r.Differences {
		sb.WriteString("  - " + d.String() + "\n")
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// Compare compares two neutral records semantically, concept by concept, per
// the correspondence table. Order-insensitive for repeatable properties;
// normalization per the table's transform column. It returns a Report naming
// every concept-level difference; Equal() reports whether the two records
// describe the same contact. Nil records are treated as empty records (the
// comparison then reports everything the non-nil side carries as a loss or
// gain).
func Compare(a, b *contactmodel.Record) Report {
	na, nb := a, b
	if na == nil {
		na = &contactmodel.Record{}
	}
	if nb == nil {
		nb = &contactmodel.Record{}
	}
	var rep Report
	for _, row := range correspondence.Load() {
		extract, ok := byConcept[row.ConceptID]
		if !ok { // # pragma: no cover — init() guarantees every table row has an extractor; a drift fails at package load, not here
			// Unreachable in practice — init() verifies full coverage and
			// panics at package load time; guarded defensively per the ADR's
			// "escalate, never invent" protocol rather than silently skipping.
			panic(fmt.Sprintf("semanticequal: no extractor registered for correspondence concept %q (correspondence table changed?)", row.ConceptID))
		}
		av, bv := extract(na), extract(nb)
		if d := diffConcept(row.ConceptID, av, bv); d != nil {
			rep.Differences = append(rep.Differences, *d)
		}
	}
	return rep
}

// diffConcept classifies one concept's normalized value multisets into a
// Difference, or returns nil when they are semantically equal.
func diffConcept(concept string, a, b []string) *Difference {
	ea, eb := multisetEmpty(a), multisetEmpty(b)
	switch {
	case ea && eb:
		return nil
	case !ea && eb:
		return &Difference{Concept: concept, PresentA: true, ValuesA: sortedCopy(a)}
	case ea && !eb:
		return &Difference{Concept: concept, PresentB: true, ValuesB: sortedCopy(b)}
	case !multisetEqual(a, b):
		return &Difference{Concept: concept, PresentA: true, PresentB: true, ValuesA: sortedCopy(a), ValuesB: sortedCopy(b)}
	}
	return nil
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// multisetEmpty reports whether the normalized value list holds no values.
func multisetEmpty(v []string) bool { return len(v) == 0 }

// multisetEqual compares two []string as unordered multisets (order-insensitive).
func multisetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}

// joinKeys is a helper for extractors that build a single comparable key
// from several fields: non-empty fields are joined with "|". Two absent
// fields yield the empty key, which callers drop (an all-empty element
// contributes nothing).
func joinKeys(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "|")
}

// boolPtrVal dereferences a *bool, reporting false for nil.
func boolPtrVal(p *bool) bool { return p != nil && *p }

// intPtrVal dereferences a *int, reporting 0 for nil.
func intPtrVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
