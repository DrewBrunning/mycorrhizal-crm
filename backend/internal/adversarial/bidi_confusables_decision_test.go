package adversarial

// TestBidiZeroWidthPreservedThroughExport pins the ADR-0016 decision #6 /
// asvs-l2.md P9 acceptance (issue #945): bidi-control and zero-width
// characters in display-text fields are preserved verbatim, deliberately,
// rather than stripped. TestDeclaredTiersHold + preserveLandingChecks
// already prove these two fixtures land on the parsed Record unmodified;
// this test proves the same claim end-to-end through both vCard exporters,
// so a future change that starts silently stripping these characters (in
// either direction) fails here rather than shipping as an undocumented
// behavior change.

import (
	"testing"

	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBidiZeroWidthPreservedThroughExport(t *testing.T) {
	t.Parallel()

	cases := []struct {
		fixture string
		want    string // a substring proving the special characters survived
	}{
		{fixture: "enc-rtl-override.vcf", want: "‮"}, // RTL override
		{fixture: "enc-zero-width.vcf", want: "‍"},   // zero-width joiner
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			fx := ByName(tc.fixture)
			require.NotNil(t, fx, "fixture must be declared in the manifest")
			require.Equal(t, "preserve", fx.Tier, "this decision only holds for the preserve tier")

			raw := LoadFixture(tc.fixture)
			record, _, err := importFixture(*fx, raw)
			require.NoError(t, err)
			require.NotNil(t, record.Card.Name)
			assert.Contains(t, record.Card.Name.Full, tc.want, "the parsed Record must still carry the character")

			v3Out, _, err := vcard3.Adapter{}.Export(record)
			require.NoError(t, err)
			assert.Contains(t, string(v3Out), tc.want, "vCard 3 export must not strip the character")

			v4Out, _, err := vcard4.Adapter{}.Export(record)
			require.NoError(t, err)
			assert.Contains(t, string(v4Out), tc.want, "vCard 4 export must not strip the character")
		})
	}
}
