package roundtrip

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/semanticequal"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/require"
)

// TestRoundTrip_SerializedVarianceDoesNotFail pins the issue's "Reordering
// properties, refolding lines, or changing parameter case in a serialized card
// does not fail the comparison" check. The semantic comparer (and the whole
// round-trip suite) must judge by meaning, not by bytes: a card whose
// properties arrive in a different order, whose parameter names use a
// different case, or whose long values are folded differently is the SAME
// contact.
func TestRoundTrip_SerializedVarianceDoesNotFail(t *testing.T) {
	rec := &contactmodel.Record{Card: contactmodel.Card{
		UID:  "u1",
		Name: &contactmodel.Name{Full: "Ada Lovelace"},
		Emails: []contactmodel.Email{
			{Address: "ada@example.com", Contexts: []string{"work"}},
			{Address: "ada.lovelace@example.com", Contexts: []string{"private"}},
		},
		Phones: []contactmodel.Phone{{Number: "+44 20 7946 0958"}, {Number: "+44 20 7946 0000"}},
		Notes: []contactmodel.Note{{
			Note: "A deliberately long note, long enough that the vCard encoder folds it onto multiple continuation lines — the fold point is purely presentational and must not be part of the semantic comparison. " + strings.Repeat("padding ", 30),
		}},
		Addresses: []contactmodel.Address{{
			Components:  []contactmodel.AddressComponent{{Kind: "locality", Value: "London"}},
			CountryCode: "GB",
		}},
	}}

	for _, f := range formats {
		f := f
		if f.name == "jscontact" {
			continue // JSContact JSON has no line folding or parameter case to vary; covered by the driver test
		}
		t.Run(f.name, func(t *testing.T) {
			out, _, err := f.exporter.Export(rec)
			require.NoError(t, err)

			// The reference parse of the canonical export.
			reference, _, err := f.importer.Import(out)
			require.NoError(t, err)

			// Reversed property order + parameter-name case variance.
			permuted := permuteVCard(string(out))
			permuted = strings.ReplaceAll(permuted, "TYPE=WORK", "type=work")
			permuted = strings.ReplaceAll(permuted, "TYPE=HOME", "type=home")
			require.NotEqual(t, string(out), permuted, "the permuted card must differ byte-wise to prove the comparison is semantic")

			variant, _, err := f.importer.Import([]byte(permuted))
			require.NoError(t, err)

			report := semanticequal.Compare(reference, variant)
			if !report.Equal() {
				t.Errorf("byte-varied serialization must compare equal semantically:\n%s", report.DiffText())
			}
		})
	}
}

// permuteVCard reverses the order of all body properties while keeping the
// BEGIN/VERSION/UID preamble and the END trailer in place — a legitimate
// serialization variance (vCard property order is not significant).
func permuteVCard(raw string) string {
	lines := strings.Split(raw, "\r\n")
	var head, body []string
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "BEGIN:") || strings.HasPrefix(l, "VERSION:") || strings.HasPrefix(l, "UID:"):
			head = append(head, l)
		case strings.HasPrefix(l, "END:"):
			// appended last
		default:
			if l != "" {
				body = append(body, l)
			}
		}
	}
	out := make([]string, 0, len(head)+len(body)+1)
	out = append(out, head...)
	for i := len(body) - 1; i >= 0; i-- {
		out = append(out, body[i])
	}
	out = append(out, "END:VCARD")
	return strings.Join(out, "\r\n")
}

// TestRoundTrip_LineFoldingVarianceDoesNotFail covers the folding half of the
// same check explicitly: a value that is valid after being folded at a
// different point than the encoder chose must parse to the same contact.
func TestRoundTrip_LineFoldingVarianceDoesNotFail(t *testing.T) {
	// A NOTE long enough to fold, exported once, then re-folded at a different
	// width before re-import.
	rec := &contactmodel.Record{Card: contactmodel.Card{
		UID:   "u1",
		Name:  &contactmodel.Name{Full: "Test"},
		Notes: []contactmodel.Note{{Note: strings.Repeat("abcdefghij ", 20)}},
	}}
	out, _, err := vcard4.Adapter{}.Export(rec)
	require.NoError(t, err)

	reference, _, err := vcard4.Adapter{}.Import(out)
	require.NoError(t, err)

	refolded := refoldVCard(string(out), 25)
	require.NotEqual(t, string(out), refolded, "refolded output must differ byte-wise")

	variant, _, err := vcard4.Adapter{}.Import([]byte(refolded))
	require.NoError(t, err)

	report := semanticequal.Compare(reference, variant)
	if !report.Equal() {
		t.Errorf("differently folded serialization must compare equal semantically:\n%s", report.DiffText())
	}
}

// refoldVCard re-wraps every line at width characters with a CRLF + single
// space continuation (RFC 6350 §3.2), folding long lines and leaving short
// ones alone. It is a presentation-only change.
func refoldVCard(raw string, width int) string {
	var out []string
	for _, l := range strings.Split(raw, "\r\n") {
		if len(l) <= width {
			out = append(out, l)
			continue
		}
		for len(l) > width {
			out = append(out, l[:width])
			l = " " + l[width:]
		}
		out = append(out, l)
	}
	return strings.Join(out, "\r\n")
}
