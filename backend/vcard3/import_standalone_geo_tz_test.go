package vcard3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vCard 3.0 has no per-address GEO/TZ, so an UNGROUPED GEO/TZ can only be placed
// when exactly one ADR exists. These tests pin the three outcomes: placed,
// dropped-with-warning because unparsable/duplicated, and dropped-with-warning
// because ambiguous across several ADRs.

const standaloneHead = "BEGIN:VCARD\nVERSION:3.0\nFN:Ada Lovelace\n"

func importWarnings(t *testing.T, vcf string) (addrs []struct{ coord, tz string }, warns map[string]int) {
	t.Helper()
	rec, diags, err := (Adapter{}).Import([]byte(vcf))
	require.NoError(t, err)
	warns = map[string]int{}
	for _, d := range diags {
		if d.Severity == "warn" {
			warns[d.Concept+": "+d.Message]++
		}
	}
	for _, a := range rec.Card.Addresses {
		addrs = append(addrs, struct{ coord, tz string }{a.Coordinates, a.TimeZone})
	}
	return addrs, warns
}

func TestImport_StandaloneGeoTZ_SingleAddress(t *testing.T) {
	t.Parallel()
	t.Run("placed on the only address", func(t *testing.T) {
		addrs, warns := importWarnings(t, standaloneHead+"ADR;TYPE=HOME:;;A Street\nGEO:48.2;16.3\nTZ:Europe/Vienna\nEND:VCARD\n")
		require.Len(t, addrs, 1)
		assert.Equal(t, "geo:48.2,16.3", addrs[0].coord)
		assert.Equal(t, "Europe/Vienna", addrs[0].tz)
		assert.Empty(t, warns)
	})
	t.Run("unparsable GEO is dropped with a warning", func(t *testing.T) {
		addrs, warns := importWarnings(t, standaloneHead+"ADR;TYPE=HOME:;;A Street\nGEO:not-a-pair\nEND:VCARD\n")
		require.Len(t, addrs, 1)
		assert.Empty(t, addrs[0].coord)
		assert.Equal(t, map[string]int{"adr.geo: standalone GEO is not a lat;lon pair; dropped": 1}, warns)
	})
	t.Run("extra GEO and TZ are dropped with warnings", func(t *testing.T) {
		addrs, warns := importWarnings(t, standaloneHead+
			"ADR;TYPE=HOME:;;A Street\nGEO:48.2;16.3\nGEO:1.0;2.0\nTZ:Europe/Vienna\nTZ:Europe/Paris\nEND:VCARD\n")
		require.Len(t, addrs, 1)
		assert.Equal(t, "geo:48.2,16.3", addrs[0].coord, "the first GEO wins")
		assert.Equal(t, "Europe/Vienna", addrs[0].tz, "the first TZ wins")
		assert.Equal(t, map[string]int{
			"adr.geo: multiple standalone GEO properties for a single ADR; extra dropped": 1,
			"adr.tz: multiple standalone TZ properties for a single ADR; extra dropped":   1,
		}, warns)
	})
}

func TestImport_StandaloneGeoTZ_AmbiguousAcrossAddresses(t *testing.T) {
	t.Parallel()
	addrs, warns := importWarnings(t, standaloneHead+
		"ADR;TYPE=HOME:;;A Street\nADR;TYPE=WORK:;;B Street\nGEO:48.2;16.3\nTZ:Europe/Vienna\nEND:VCARD\n")
	require.Len(t, addrs, 2)
	for i, a := range addrs {
		assert.Emptyf(t, a.coord, "address %d must not receive the ambiguous GEO", i)
		assert.Emptyf(t, a.tz, "address %d must not receive the ambiguous TZ", i)
	}
	assert.Equal(t, map[string]int{
		"adr.geo: standalone GEO with multiple ADRs is ambiguous; dropped rather than misassigned": 1,
		"adr.tz: standalone TZ with multiple ADRs is ambiguous; dropped rather than misassigned":   1,
	}, warns)
}
