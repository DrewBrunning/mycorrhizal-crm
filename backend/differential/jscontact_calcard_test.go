package differential

import (
	"testing"

	"mycorrhizal/jscontact"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/require"
)

// TestJSContactCalcardDifferential is the TEST-08 JSContact leg. There is no
// independent pure-Go (or pure-JS) JSContact implementation to run per-PR, so
// this leg uses the pinned Rust calcard reference CLI
// (backend/differential/reference/calcard, issue #680) — an independent,
// mature RFC 9553 JSContact / RFC 9555 conversion implementation — as the
// reference:
//
//	ours -> reference: our JSContact exporter's output is parsed and
//	                   re-emitted by calcard (its independent reading of the
//	                   RFC 9553 Card), which our JSContact importer reads
//	                   back.
//	reference -> ours: our vCard 4.0 exporter's output is converted to
//	                   JSContact by calcard (its RFC 9555 vCard->JSContact
//	                   engine), which our JSContact importer reads back. This
//	                   is the only way for the reference to *emit* JSContact
//	                   from a corpus contact; the comparison is scoped to the
//	                   concepts vCard can carry (vcard4-home), so a
//	                   vCard-carriable concept must survive the conversion.
//
// Both legs assert semantic equivalence per TEST-03's comparator against the
// original corpus contact. calcard shares no code with our jscontact/vcard4
// adapters, so a disagreement is evidence one of us misreads the spec.
//
// The leg runs per-PR in CI's scheduled differential job (the reference
// binary is built there); locally it runs when the binary is present or
// $MYCORRHIZAL_CALCARD_CMD is set, and skips otherwise.
func TestJSContactCalcardDifferential(t *testing.T) {
	ref, reason := NewCalcardRef()
	if len(ref.argv) == 0 {
		t.Skip(reason)
	}
	registerJSContactPins()

	corpus, err := ContactCorpus()
	require.NoError(t, err)

	for _, entry := range corpus {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			// ours -> reference: JSContact out, calcard re-emits it, we read it.
			ourJS, _, err := (jscontact.Adapter{}).Export(entry.Record)
			require.NoError(t, err, "our JSContact exporter must never fail on corpus data")
			reemitted, err := ref.run("jscontact-reemit", ourJS)
			require.NoError(t, err, "calcard failed to parse/re-emit our JSContact export")
			roundTripped, _, err := (jscontact.Adapter{}).Import(reemitted)
			require.NoError(t, err, "our JSContact importer failed on calcard's re-emitted JSContact")

			reportLeg(t, formatJSContact.label, dirOursToRef, entry.ID, formatJSContact, entry.Record, roundTripped)

			// reference -> ours: vCard out, calcard converts to JSContact, we read it.
			ourVCF, _, err := (vcard4.Adapter{}).Export(entry.Record)
			require.NoError(t, err, "our vCard 4.0 exporter must never fail on corpus data")
			refJS, err := ref.run("vcard-to-jscontact", ourVCF)
			require.NoError(t, err, "calcard failed to convert our vCard export to JSContact")
			roundTrippedJS, _, err := (jscontact.Adapter{}).Import(refJS)
			require.NoError(t, err, "our JSContact importer failed on calcard's JSContact output")

			// This leg ends at our JSContact importer but STARTED from our
			// vCard 4.0 exporter (the reference only emits JSContact via its
			// RFC 9555 conversion), so the comparison is scoped to the
			// concepts vCard can carry: everything vCard carries must survive
			// calcard's conversion and our JSContact import. The pin key is
			// the JSContact format (this is a JSContact leg, distinct from
			// the vCard-vobject leg's own vcard4 pins).
			reportLeg(t, formatJSContact.label, dirRefToOurs, entry.ID, formatVCard4, entry.Record, roundTrippedJS)
		})
	}
}
