package models

import (
	"reflect"
	"sort"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
)

// TestEnvelopeLossConceptsAlignWithMatrix pins the runtime loss-report
// concepts to the DATA-01 matrix (issue #441): a DATA-02 loss report for a
// vCard/JSContact file export names a field via the diagnostic concept, and
// that concept must be exactly the matrix's issue #515 envelope field with a
// corresponding loss report — otherwise the report⇄matrix correspondence the
// milestone demands breaks at runtime.
func TestEnvelopeLossConceptsAlignWithMatrix(t *testing.T) {
	t.Parallel()
	// Derive the diagnostic concepts from the code itself (not a hand-kept
	// mirror): every populated envelope field that EnvelopeExportLossDiagnostics
	// names.
	rec := &contactmodel.Record{
		Envelope: contactmodel.CRMEnvelope{
			Gender:             "x",
			Circles:            []string{"x"},
			HowWeMet:           "x",
			WorkInformation:    "x",
			ContactInformation: "x",
		},
	}
	var diagConcepts []string
	for _, d := range EnvelopeExportLossDiagnostics(rec) {
		diagConcepts = append(diagConcepts, d.Concept)
	}
	sort.Strings(diagConcepts)

	// The matrix's envelope no-home fields (the ones with a LossReport).
	var matrixConcepts []string
	for _, e := range correspondence.Build() {
		if e.NoHome != nil && e.NoHome.LossReport {
			matrixConcepts = append(matrixConcepts, e.ConceptID)
		}
	}
	sort.Strings(matrixConcepts)

	if !reflect.DeepEqual(diagConcepts, matrixConcepts) {
		t.Errorf("EnvelopeExportLossDiagnostics concepts %v != DATA-01 matrix envelope fields %v — "+
			"a runtime loss report would name a field the matrix does not classify, or vice versa",
			diagConcepts, matrixConcepts)
	}
}
