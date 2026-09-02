package roundtrip

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"
	"mycorrhizal/internal/rfctest"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/jscontact"
	"mycorrhizal/vcard3"

	"github.com/stretchr/testify/require"
)

// --- classification unit tests ----------------------------------------------
//
// classify encodes ADR-0002's two-tier degradation. The fixture-driven test
// (TestRoundTrip_CanonicalFixture) exercises the *accept* paths; these tests
// pin the *defect* paths that the fixture must never hit — a silent drop, a
// mapped field failing to land, a change with no warn to explain it.

func TestClassify_LossWithHomeAndNoWarn_IsDefect(t *testing.T) {
	t.Parallel()
	d := semanticequal.Difference{Concept: "email", PresentA: true}
	// email has a v4 home (v4_prop EMAIL).
	row := correspondence.Row{ConceptID: "email", V4Prop: "EMAIL"}
	problems := classify(d, formats[1], row, nil)
	if len(problems) == 0 {
		t.Fatal("a mapped field lost with no warn must be a defect")
	}
	if !strings.Contains(problems[0], "DEFECT") {
		t.Errorf("problem should be labelled a defect, got %q", problems[0])
	}
}

func TestClassify_LossWithHomeAndWarn_IsAcceptable(t *testing.T) {
	t.Parallel()
	d := semanticequal.Difference{Concept: "adr", PresentA: true}
	row := correspondence.Row{ConceptID: "adr", V3Prop: "ADR"}
	diags := []contactmodel.Diagnostic{{Severity: "warn", Concept: "adr"}}
	if problems := classify(d, formats[0], row, diags); len(problems) != 0 {
		t.Errorf("a warn documented the loss; must be acceptable, got %v", problems)
	}
}

func TestClassify_LossWithoutHomeAndNoWarn_IsSilentDropDefect(t *testing.T) {
	t.Parallel()
	d := semanticequal.Difference{Concept: "kind", PresentA: true}
	row := correspondence.Row{ConceptID: "kind", V3Prop: "-"}
	problems := classify(d, formats[0], row, nil)
	if len(problems) == 0 {
		t.Fatal("a no-home field dropped without a warn must be a silent-drop defect")
	}
	if !strings.Contains(problems[0], "silent drop") {
		t.Errorf("problem should name the silent drop, got %q", problems[0])
	}
}

func TestClassify_LossWithoutHomeAndWarn_IsAcceptable(t *testing.T) {
	t.Parallel()
	d := semanticequal.Difference{Concept: "created", PresentA: true}
	row := correspondence.Row{ConceptID: "created", V3Prop: "-"}
	diags := []contactmodel.Diagnostic{{Severity: "warn", Concept: "created"}}
	if problems := classify(d, formats[0], row, diags); len(problems) != 0 {
		t.Errorf("a no-home field with its warn must be acceptable, got %v", problems)
	}
}

func TestClassify_ChangedWithoutWarn_IsDefect(t *testing.T) {
	t.Parallel()
	d := semanticequal.Difference{Concept: "email", PresentA: true, PresentB: true}
	row := correspondence.Row{ConceptID: "email", V4Prop: "EMAIL"}
	if problems := classify(d, formats[1], row, nil); len(problems) == 0 {
		t.Fatal("a changed value with no warn must be a defect")
	}
	// the same change with a warn (e.g. v3's X-ANNIVERSARY redirect changing a
	// timestamp into a partial date) is acceptable.
	diags := []contactmodel.Diagnostic{{Severity: "warn", Concept: "email"}}
	if problems := classify(d, formats[1], row, diags); len(problems) != 0 {
		t.Errorf("a warn documented the change; must be acceptable, got %v", problems)
	}
}

func TestClassify_Gain_IsAcceptable(t *testing.T) {
	t.Parallel()
	// A value present only in the round-tripped record (vCard 3.0 synthesizing
	// the mandatory FN from components) is reported, not failed.
	d := semanticequal.Difference{Concept: "name.full", PresentB: true}
	if problems := classify(d, formats[0], correspondence.Row{ConceptID: "name.full", V3Prop: "FN"}, nil); len(problems) != 0 {
		t.Errorf("a gain must be acceptable, got %v", problems)
	}
}

func TestHasWarn_MatchesConceptAndSeverity(t *testing.T) {
	t.Parallel()
	diags := []contactmodel.Diagnostic{
		{Severity: "info", Concept: "kind"},
		{Severity: "warn", Concept: "created"},
	}
	if hasWarn(diags, "kind") {
		t.Error("info diagnostics are not warns")
	}
	if !hasWarn(diags, "created") {
		t.Error("warn diagnostic must be found")
	}
	if hasWarn(diags, "email") {
		t.Error("concept must match")
	}
	if hasWarn(nil, "created") {
		t.Error("nil diagnostics must not match")
	}
}

// --- passthrough round trips (ADR-0002: preserve, don't reject) -------------

// TestRoundTrip_UnknownVCardPropertySurvivesViaPassthrough is the "an unknown
// X- property survives import -> export unchanged via Passthrough" check from
// the issue's How-to-verify list: an unrecognized vCard property is captured
// into Record.Passthrough.VCard on import, re-emitted on export, and compares
// equal across the full round trip.
func TestRoundTrip_UnknownVCardPropertySurvivesViaPassthrough(t *testing.T) {
	t.Parallel()
	for _, f := range []format{formats[0], formats[1]} {
		f := f
		t.Run(f.name, func(t *testing.T) {
			raw := []byte("BEGIN:VCARD\nVERSION:" + vcardVersion(f.name) + "\nFN:Test\nUID:test-1\nX-CUSTOM;TYPE=CUSTOM:keep-me\nEND:VCARD\n")
			rec, _, err := f.importer.Import(raw)
			require.NoError(t, err)
			require.Len(t, rec.Passthrough.VCard, 1, "unknown property must be captured into Passthrough.VCard")

			out, _, err := f.exporter.Export(rec)
			require.NoError(t, err)
			rfctest.AssertVCardLine(t, out, "X-CUSTOM", map[string]string{"TYPE": "CUSTOM"}, "keep-me")

			rec2, _, err := f.importer.Import(out)
			require.NoError(t, err)
			report := semanticequal.Compare(rec, rec2)
			if !report.Equal() {
				t.Errorf("unknown property did not round-trip unchanged:\n%s", report.DiffText())
			}
		})
	}
}

func vcardVersion(name string) string {
	if name == "vcard3" {
		return "3.0"
	}
	return "4.0"
}

// TestRoundTrip_UnknownJSContactPropertySurvivesViaPassthrough is the JSContact
// half of the same check: an unknown top-level JSContact property is captured
// into Record.Passthrough.JSContact and re-spliced on export.
func TestRoundTrip_UnknownJSContactPropertySurvivesViaPassthrough(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"@type":"Card","version":"1.0","uid":"test-1","kind":"individual","x-vendor-prop":"verbatim"}`)
	rec, _, err := jscontact.Adapter{}.Import(raw)
	require.NoError(t, err)
	require.NotEmpty(t, rec.Passthrough.JSContact, "unknown property must be captured into Passthrough.JSContact")

	out, _, err := jscontact.Adapter{}.Export(rec)
	require.NoError(t, err)
	rfctest.AssertJSONPointer(t, out, "/x-vendor-prop", "verbatim")

	rec2, _, err := jscontact.Adapter{}.Import(out)
	require.NoError(t, err)
	report := semanticequal.Compare(rec, rec2)
	if !report.Equal() {
		t.Errorf("unknown JSContact property did not round-trip unchanged:\n%s", report.DiffText())
	}
}

// TestRoundTrip_NoHomeFieldWarnsInsteadOfSilentDrop pins the "a field with no
// target-format home produces a warn diagnostic rather than a silent drop or a
// hard failure" check for a representative vCard 3.0 case (kind has no 3.0
// home): the export succeeds, emits the warn, and omits the property.
func TestRoundTrip_NoHomeFieldWarnsInsteadOfSilentDrop(t *testing.T) {
	t.Parallel()
	rec := &contactmodel.Record{Card: contactmodel.Card{Kind: "group", UID: "u1", Name: &contactmodel.Name{Full: "Test"}}}
	out, diags, err := vcard3.Adapter{}.Export(rec)
	require.NoError(t, err, "no-home fields must never hard-fail export")
	if !hasWarn(diags, "kind") {
		t.Errorf("diags = %+v, want a warn for concept kind", diags)
	}
	for _, line := range splitLines(string(out)) {
		if len(line) > 3 && line[:3] == "KIND" {
			t.Errorf("KIND must not appear in vCard 3.0 output:\n%s", out)
		}
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
