package contactmodel

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// FuzzGeoURI fuzzes the RFC 5870 geo: URI boundary (issue #1626):
// ParseGeoURI ingests a coordinate string that ultimately traces back to
// client-supplied vCard data, and FormatGeoURI emits one for export/sync.
//
// Property: FormatGeoURI then ParseGeoURI round-trips within the formatter's
// six-decimal rounding, an accepted point is always inside the WGS-84 bounds
// (lat ±90, lon ±180), and anything out of bounds or non-finite is rejected
// rather than silently clamped. Seeds come from the shared geo: URI parity
// fixture (testdata/geo-uri-fixtures.json), the same external oracle
// geo_fixture_test.go reads.
func FuzzGeoURI(f *testing.F) {
	raw, err := os.ReadFile("../../testdata/geo-uri-fixtures.json")
	if err != nil {
		f.Fatalf("read geo fixture: %v", err)
	}
	var fixtures geoFixtures
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		f.Fatalf("decode geo fixture: %v", err)
	}
	for _, c := range fixtures.Parse {
		f.Add(c.Input, 0.0, 0.0)
	}
	for _, c := range fixtures.Format {
		f.Add(c.Expected, c.Lat, c.Lon)
	}
	f.Add("geo:0,0", math.NaN(), 0.0)
	f.Add("geo:0,0", math.Inf(1), math.Inf(-1))
	f.Add("geo:90,180", -90.0, -180.0)

	f.Fuzz(func(t *testing.T, uri string, lat, lon float64) {
		// Every accepted parse must be inside the valid coordinate ranges,
		// and must survive a format+reparse within the rounding tolerance.
		if plat, plon, ok := ParseGeoURI(uri); ok {
			if plat < -90 || plat > 90 || plon < -180 || plon > 180 {
				t.Fatalf("ParseGeoURI(%q) accepted out-of-range (%v, %v)", uri, plat, plon)
			}
			back := FormatGeoURI(plat, plon)
			blat, blon, bok := ParseGeoURI(back)
			if !bok || math.Abs(blat-plat) > 1e-6 || math.Abs(blon-plon) > 1e-6 {
				t.Fatalf("ParseGeoURI(%q) -> FormatGeoURI -> ParseGeoURI(%q) = (%v, %v), %v; want back within 1e-6", uri, back, blat, blon, bok)
			}
		}

		// Format->Parse is the other direction: a finite point whose
		// six-decimal-rounded form is in range round-trips; a non-finite
		// or out-of-range one is rejected by the parser (never silently
		// clamped). The range test is on the ROUNDED value, because
		// FormatGeoURI rounds first and 90.0000004 legitimately becomes
		// the in-range 90.
		uri = FormatGeoURI(lat, lon)
		glat, glon, ok := ParseGeoURI(uri)
		rl := math.Round(lat*1e6) / 1e6
		rn := math.Round(lon*1e6) / 1e6
		inRange := !math.IsNaN(lat) && !math.IsInf(lat, 0) &&
			!math.IsNaN(lon) && !math.IsInf(lon, 0) &&
			rl >= -90 && rl <= 90 && rn >= -180 && rn <= 180
		if inRange {
			if !ok || math.Abs(glat-lat) > 1e-6 || math.Abs(glon-lon) > 1e-6 {
				t.Fatalf("ParseGeoURI(FormatGeoURI(%v, %v)=%q) = (%v, %v), %v; want within 1e-6", lat, lon, uri, glat, glon, ok)
			}
		} else if ok {
			t.Fatalf("ParseGeoURI(FormatGeoURI(%v, %v)=%q) accepted an out-of-range/non-finite point", lat, lon, uri)
		}
	})
}
