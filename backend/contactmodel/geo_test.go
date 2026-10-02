package contactmodel

import (
	"math"
	"testing"
)

func TestParseGeoURI(t *testing.T) {
	valid := []struct {
		in       string
		lat, lon float64
	}{
		{"geo:48.2010,16.3695", 48.2010, 16.3695},
		{"geo:0,0", 0, 0},
		{"geo:-90,-180", -90, -180},
		{"geo:90,180", 90, 180},
		{"GEO:51.5,-0.12", 51.5, -0.12},              // scheme is case-insensitive (RFC 3986)
		{"geo:48.2,16.3,183", 48.2, 16.3},            // altitude ignored
		{"geo:48.2,16.3;crs=wgs84;u=40", 48.2, 16.3}, // parameters ignored
		{"geo:48.2,16.3,-10;u=3", 48.2, 16.3},        // both
		{"geo:+12.5,-0", 12.5, 0},                    // signs
		{"geo:1e1,2e1", 10, 20},                      // exponent form
	}
	for _, tc := range valid {
		lat, lon, ok := ParseGeoURI(tc.in)
		if !ok || lat != tc.lat || lon != tc.lon {
			t.Errorf("ParseGeoURI(%q) = %v, %v, %v; want %v, %v, true", tc.in, lat, lon, ok, tc.lat, tc.lon)
		}
	}

	invalid := []string{
		"", "geo:", "geo:1", "geo:1,", "geo:,2", "geo:1,2,3,4", "geo:a,b", "geo:1,b", "geo:a,2",
		"geo:1,2,x",     // altitude must be numeric when present
		"geo:90.0001,0", // latitude out of range
		"geo:-90.0001,0",
		"geo:0,180.0001", // longitude out of range
		"geo:0,-180.0001",
		"geo:NaN,0", "geo:0,NaN", "geo:Inf,0", "geo:0,-Inf",
		"http://example.com/48.2,16.3", "48.2,16.3", "geo48.2,16.3", "ge:1,2",
		"geo: 1,2", "geo:1 ,2",
	}
	for _, in := range invalid {
		if lat, lon, ok := ParseGeoURI(in); ok {
			t.Errorf("ParseGeoURI(%q) = %v, %v, true; want rejection", in, lat, lon)
		}
	}
}

func TestFormatGeoURI(t *testing.T) {
	cases := []struct {
		lat, lon float64
		want     string
	}{
		{48.201, 16.3695, "geo:48.201,16.3695"},
		{51.5007292, -0.1246254, "geo:51.500729,-0.124625"}, // rounded to 6 places
		{0, 0, "geo:0,0"},
		{-33.8688, 151.2093, "geo:-33.8688,151.2093"},
		{10.0000004, 20, "geo:10,20"}, // rounding can land exactly on an integer
	}
	for _, tc := range cases {
		if got := FormatGeoURI(tc.lat, tc.lon); got != tc.want {
			t.Errorf("FormatGeoURI(%v, %v) = %q, want %q", tc.lat, tc.lon, got, tc.want)
		}
	}
}

// Whatever FormatGeoURI emits for an in-range point must be accepted by
// ParseGeoURI and read back within the 6-decimal rounding.
func TestGeoURIRoundTrip(t *testing.T) {
	for _, p := range [][2]float64{{0, 0}, {89.999999, 179.999999}, {-89.123457, -179.5}, {12.3456785, 98.7654325}} {
		uri := FormatGeoURI(p[0], p[1])
		lat, lon, ok := ParseGeoURI(uri)
		if !ok {
			t.Fatalf("ParseGeoURI(FormatGeoURI(%v)) = %q rejected", p, uri)
		}
		if math.Abs(lat-p[0]) > 1e-6 || math.Abs(lon-p[1]) > 1e-6 {
			t.Errorf("round trip of %v via %q = %v, %v", p, uri, lat, lon)
		}
	}
}
