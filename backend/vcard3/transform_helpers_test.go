package vcard3

import (
	"testing"
)

func TestIsAllDigits(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty", in: "", want: false},
		{name: "digits", in: "12345678", want: true},
		{name: "single digit", in: "7", want: true},
		{name: "leading zero", in: "0199", want: true},
		{name: "letter present", in: "1234a", want: false},
		{name: "hyphen present", in: "12-34", want: false},
		{name: "space present", in: "12 34", want: false},
		{name: "decimal point", in: "12.5", want: false},
		{name: "unicode digits", in: "١٢٣٤", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAllDigits(tc.in); got != tc.want {
				t.Errorf("isAllDigits(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestGeoURIToLatLon(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{name: "valid simple", in: "geo:37.386013,-122.082932", want: "37.386013;-122.082932", wantOK: true},
		{name: "valid with negative coords", in: "geo:-33.8688,151.2093", want: "-33.8688;151.2093", wantOK: true},
		// RFC 5870 allows extra semicolon-separated parameters (crs, u, uncertainty);
		// only the lat;lon pair is the vCard GEO value.
		{name: "valid with crs param", in: "geo:37.386013,-122.082932;crs=wgs84", want: "37.386013;-122.082932", wantOK: true},
		{name: "uppercase scheme", in: "GEO:37.386013,-122.082932", want: "37.386013;-122.082932", wantOK: true},
		{name: "not a geo uri", in: "http://example.com/geo:1,2", want: "", wantOK: false},
		{name: "empty", in: "", want: "", wantOK: false},
		{name: "geo scheme only", in: "geo:", want: "", wantOK: false},
		{name: "missing comma", in: "geo:37.386013", want: "", wantOK: false},
		{name: "empty latitude", in: "geo:,-122.082932", want: "", wantOK: false},
		{name: "empty longitude", in: "geo:37.386013,", want: "", wantOK: false},
		// Extra comma-separated fields are tolerated: SplitN keeps only the
		// first two fields and the rest becomes part of longitude. A real GEO
		// value is always exactly "lat;lon", so this never matters in
		// practice — the tolerance mirrors latLonToGeoURI's permissive split.
		{name: "extra comma field tolerated", in: "geo:1,2,3", want: "1;2,3", wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := geoURIToLatLon(tc.in)
			if ok != tc.wantOK {
				t.Errorf("geoURIToLatLon(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("geoURIToLatLon(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestLatLonToGeoURI(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{name: "valid simple", in: "37.386013;-122.082932", want: "geo:37.386013,-122.082932", wantOK: true},
		{name: "valid negative pair", in: "-33.8688;151.2093", want: "geo:-33.8688,151.2093", wantOK: true},
		{name: "empty", in: "", want: "", wantOK: false},
		{name: "no semicolon", in: "37.386013,-122.082932", want: "", wantOK: false},
		{name: "empty latitude", in: ";151.2093", want: "", wantOK: false},
		{name: "empty longitude", in: "-33.8688;", want: "", wantOK: false},
		// Extra semicolon-separated fields are tolerated: SplitN keeps only
		// the first two fields and the rest becomes part of longitude. This
		// mirrors geoURIToLatLon's export (which strips trailing ";..." params
		// before splitting), and a real vCard GEO value is always exactly
		// "lat;lon" anyway.
		{name: "extra field tolerated", in: "1;2;3", want: "geo:1,2;3", wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := latLonToGeoURI(tc.in)
			if ok != tc.wantOK {
				t.Errorf("latLonToGeoURI(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("latLonToGeoURI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
