package differential

import (
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/require"
)

// TestVCardVObjectDifferential is the TEST-08 vCard leg: every corpus
// contact is pushed through our exporter and read back by Python vobject
// (ours -> reference) and built by vobject and read back by our importer
// (reference -> ours), and both legs must be semantically equivalent per
// TEST-03's comparator — filtered to the concepts vCard can actually carry
// (see formatName.inScope) and to the pinned reference-side divergences.
//
// The reference (vobject 0.9.9, backend/differential/reference/vobject/
// vcard_ref.py) shares no code with backend/vcard3 or backend/vcard4 — those
// adapters are built on emersion/go-vcard, so comparing against it would be
// comparing our mapping against our own low-level code. vobject is a third
// implementation of RFC 6350/2426; a disagreement between us and it is
// evidence one of us misreads the spec.
func TestVCardVObjectDifferential(t *testing.T) {
	ref, reason := NewPyRef()
	if ref.python == "" {
		t.Skip(reason)
	}
	registerVCardPins()

	corpus, err := ContactCorpus()
	require.NoError(t, err)

	legs := []struct {
		version  string
		format   formatName
		importer contactmodel.Importer
		exporter contactmodel.Exporter
	}{
		{"3.0", formatVCard3, vcard3.Adapter{}, vcard3.Adapter{}},
		{"4.0", formatVCard4, vcard4.Adapter{}, vcard4.Adapter{}},
	}

	for _, leg := range legs {
		leg := leg
		t.Run(leg.version, func(t *testing.T) {
			for _, entry := range corpus {
				entry := entry
				t.Run(entry.ID, func(t *testing.T) {
					assertVCardLeg(t, ref, leg.version, leg.format, leg.importer, leg.exporter, entry)
				})
			}
		})
	}
}

func assertVCardLeg(t *testing.T, ref pyRef, version string, format formatName, importer contactmodel.Importer, exporter contactmodel.Exporter, entry CorpusEntry) {
	t.Helper()

	// ours -> reference: our export, read by vobject.
	out, exportDiags, err := exporter.Export(entry.Record)
	require.NoError(t, err, "our exporter must never fail on corpus data")
	_ = exportDiags
	refJSON, err := ref.toNeutral(out)
	require.NoError(t, err, "vobject failed to parse our vCard %s export", version)
	refRec := &contactmodel.Record{}
	require.NoError(t, json.Unmarshal(refJSON, refRec))

	reportLeg(t, format.label, dirOursToRef, entry.ID, format, entry.Record, refRec)

	// reference -> ours: vobject's build, read by our importer.
	ourJSON, err := json.Marshal(entry.Record)
	require.NoError(t, err)
	refVCF, err := ref.toFormat(ourJSON, version)
	require.NoError(t, err, "vobject failed to build vCard %s from the corpus record", version)
	roundTripped, _, err := importer.Import(refVCF)
	require.NoError(t, err, "our vCard %s importer failed on vobject's output", version)

	reportLeg(t, format.label, dirRefToOurs, entry.ID, format, entry.Record, roundTripped)
}

// reportLeg runs the semantic comparison for one leg, filters through the
// pins, and fails the test on any unpinned disagreement or stale pin, naming
// the concept.
func reportLeg(t *testing.T, format, dir, entryID string, f formatName, original, roundTripped *contactmodel.Record) {
	t.Helper()

	report := semanticequal.Compare(original, roundTripped)
	var diffs []Difference
	for _, d := range report.Differences {
		if !f.inScope(d.Concept) {
			continue
		}
		// A gain (value present only in the round-tripped record) is
		// synthesized data, not a disagreement: vCard requires FN, so a card
		// whose seed has components but no full name gets an FN synthesized
		// by whichever side builds the card, and the other side faithfully
		// reads it. The round-trip suite treats gains the same way.
		if !d.PresentA && d.PresentB {
			continue
		}
		diffs = append(diffs, Difference{
			Concept:  d.Concept,
			ValuesA:  d.ValuesA,
			ValuesB:  d.ValuesB,
			presentA: d.PresentA,
			presentB: d.PresentB,
		})
	}

	verdict := classifyLeg(diffs, entryID, format, dir)
	for _, d := range verdict.Unexpected {
		t.Errorf("%s %s: %s", dir, entryID, DiffText(d))
	}
	for _, concept := range verdict.Stale {
		t.Errorf("%s %s: stale pin — concept %q is pinned as a reference divergence for %s but no longer disagrees (reference fixed or corpus changed); remove the pin", dir, entryID, concept, format)
	}
}
