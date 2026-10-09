package carddav

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
)

// FuzzNormalizeGeoCoordinate fuzzes the CardDAV geo-coordinate reader
// (issue #1626). normalizeGeoCoordinate canonicalizes a coordinate read off
// a card that a client PUT (vcard_geo.go) — untrusted input — and its
// contract is exactly two-sided: it returns the trimmed, comma-unescaped
// input when that is a valid geo: URI, and "" when it is not, so a hostile
// or non-coordinate GEO property is dropped rather than stored.
//
// Property: a non-empty result is always a parseable, in-range geo: URI; the
// accept/reject decision matches contactmodel.ParseGeoURI on the
// trimmed/unescaped input; and the function is idempotent.
func FuzzNormalizeGeoCoordinate(f *testing.F) {
	seeds := []string{
		"geo:48.2010,16.3695",
		"GEO:48.2,16.3",
		"geo:48.2,16.3,183",
		`geo:48.2\,16.3`, // go-vcard's comma escape
		`geo:48.2,16.3\,1`,
		"geo:90,180",
		"geo:-90,-180",
		"geo:91,0",
		"geo:0,181",
		"geo:NaN,0",
		"geo:1e999,0",
		"geo:",
		"48.2,16.3",
		"not a geo uri",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, v string) {
		got := normalizeGeoCoordinate(v)

		// The accept/reject decision must be exactly ParseGeoURI's, applied
		// to the trimmed, comma-unescaped input.
		unescaped := strings.ReplaceAll(strings.TrimSpace(v), `\,`, ",")
		_, _, parseOK := contactmodel.ParseGeoURI(unescaped)
		if parseOK != (got != "") {
			t.Fatalf("normalizeGeoCoordinate(%q) = %q; ParseGeoURI(%q)=%v", v, got, unescaped, parseOK)
		}
		if got == "" {
			return
		}

		lat, lon, ok := contactmodel.ParseGeoURI(got)
		if !ok || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			t.Fatalf("normalizeGeoCoordinate(%q) = %q is not a valid in-range geo URI", v, got)
		}
		if again := normalizeGeoCoordinate(got); again != got {
			t.Fatalf("normalizeGeoCoordinate not idempotent: %q -> %q -> %q", v, got, again)
		}
	})
}
