// Package roundtrip holds the semantic round-trip suite (TEST-03, issue
// #431): for every format (vCard 3.0, vCard 4.0, JSContact) and every contact
// in the canonical pathological fixture (TEST-02, issue #430), the test
// performs Record -> serialize -> parse -> Record and compares the two records
// semantically, driven by the correspondence oracle
// (docs/adrs/0002-correspondence-table-locked-oracle.md). It is the consumer
// of the reusable comparison (backend/internal/semanticequal) — TEST-07
// (#435) and MIG-03 (#438) consume that same helper, they do not reimplement
// it.
package roundtrip

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/jscontact"
	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/require"
)

// format ties one serialized format's adapter pair to the correspondence
// table column that declares which concepts have a home in it. A concept with
// a home must land; a concept without one is expected to drop with a warn
// diagnostic (ADR-0002 degradation policy).
type format struct {
	name     string
	importer contactmodel.Importer
	exporter contactmodel.Exporter
	hasHome  func(correspondence.Row) bool
}

var formats = []format{
	{
		name:     "vcard3",
		importer: vcard3.Adapter{},
		exporter: vcard3.Adapter{},
		hasHome: func(row correspondence.Row) bool {
			return row.V3Prop != "-" && row.V3Prop != "*verbatim*"
		},
	},
	{
		name:     "vcard4",
		importer: vcard4.Adapter{},
		exporter: vcard4.Adapter{},
		hasHome: func(row correspondence.Row) bool {
			return row.V4Prop != "-" && row.V4Prop != "*verbatim*"
		},
	},
	{
		name:     "jscontact",
		importer: jscontact.Adapter{},
		exporter: jscontact.Adapter{},
		hasHome: func(row correspondence.Row) bool {
			return row.JSPtr != "-" && row.JSPtr != "*verbatim*"
		},
	},
}

// TestRoundTrip_CanonicalFixture is the load-bearing TEST-03 test: every
// fixture contact round-trips semantically through all three serialized
// formats. "Semantically" is defined by semanticequal (driven by the
// correspondence table), not byte equality: property order, line folding and
// parameter case legitimately vary without a change in meaning.
//
// The two-tier degradation is honored in the assertions (ADR-0002):
//   - a concept with a home in the format that fails to land is a DEFECT and
//     fails the test, unless the exporter itself emitted a warn diagnostic for
//     that concept documenting the degradation (e.g. vCard 3.0 folding the
//     extra 9554 address components into the `adr` warn);
//   - a concept with no home in the format is expected to drop, and the test
//     asserts the warn diagnostic the exporter must emit for it rather than
//     demanding its survival;
//   - unknown input properties round-trip through Passthrough and are compared
//     for equality like any other mapped concept (the pt.vcard/pt.jscontact
//     rows).
func TestRoundTrip_CanonicalFixture(t *testing.T) {
	m, err := canonicalfixture.Read()
	require.NoError(t, err)
	byConcept := correspondence.ByConcept()

	for _, f := range formats {
		f := f
		for _, entry := range m.Contacts {
			entry := entry
			t.Run(f.name+"/"+entry.Name, func(t *testing.T) {
				rec := entry.Record()
				if entry.RecreatesVCardUIDOf != "" {
					rec.Card.UID = inheritedUID(m, entry.RecreatesVCardUIDOf)
				}

				out, exportDiags, err := f.exporter.Export(rec)
				require.NoError(t, err, "export must never fail on mappable/unmappable data (error is reserved for invalid format instances)")

				roundTripped, _, err := f.importer.Import(out)
				require.NoError(t, err, "re-import of own export must parse")

				report := semanticequal.Compare(rec, roundTripped)
				for _, d := range report.Differences {
					for _, problem := range classify(d, f, byConcept[d.Concept], exportDiags) {
						t.Errorf("%s\n  %s", d.String(), problem)
					}
				}
			})
		}
	}
}

// inheritedUID resolves a RecreatesVCardUIDOf reference for the round-trip
// driver (which does not populate a database like the canonicalfixture loader
// does): the named earlier contact's declared card uid.
func inheritedUID(m *canonicalfixture.Manifest, name string) string {
	for _, c := range m.Contacts {
		if c.Name == name {
			return c.Card.UID
		}
	}
	return ""
}

// classify applies the ADR-0002 degradation classification to one concept
// difference for one format. It returns the problems to fail the test on, or
// nil when the difference is an expected/acceptable degradation.
func classify(d semanticequal.Difference, f format, row correspondence.Row, exportDiags []contactmodel.Diagnostic) []string {
	warned := hasWarn(exportDiags, d.Concept)

	// A gain (value present only in the round-tripped record) is not data
	// loss — the only source today is vCard 3.0 synthesizing the mandatory FN
	// from components — so it is reported through the diff output but does not
	// fail the round trip.
	if !d.PresentA && d.PresentB {
		return nil
	}

	switch {
	case d.PresentA && !d.PresentB:
		// Loss: mappable data that fails to land is a defect; a no-home field
		// must at least have been named by a warn diagnostic (never a silent
		// drop).
		if f.hasHome(row) && !warned {
			return []string{"concept has a home in this format but did not survive the round trip, and no warn diagnostic explains the loss (mappable data failed to land — DEFECT)"}
		}
		if !f.hasHome(row) && !warned {
			return []string{"concept has no home in this format and was dropped WITHOUT the required warn diagnostic (silent drop — DEFECT)"}
		}
		return nil
	default:
		// Present on both sides but changed: a format home or a warn must
		// account for the change (e.g. vCard 3.0's X-ANNIVERSARY/AGENT
		// redirects change the shape of the data and warn about it).
		if !warned {
			return []string{"value changed across the round trip and no warn diagnostic explains it (mappable data failed to land — DEFECT)"}
		}
		return nil
	}
}

func hasWarn(diags []contactmodel.Diagnostic, concept string) bool {
	for _, d := range diags {
		if d.Concept == concept && strings.EqualFold(d.Severity, "warn") {
			return true
		}
	}
	return false
}
