package contactmodel

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

type geoParseCase struct {
	Input string  `json:"input"`
	Valid bool    `json:"valid"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

type geoFormatCase struct {
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Expected string  `json:"expected"`
}

type geoFixtures struct {
	Parse  []geoParseCase  `json:"parse"`
	Format []geoFormatCase `json:"format"`
}

func loadGeoFixtures(t *testing.T) geoFixtures {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/geo-uri-fixtures.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var f geoFixtures
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if len(f.Parse) == 0 || len(f.Format) == 0 {
		t.Fatal("fixture is empty")
	}
	return f
}

// The shared geo: URI parity fixture (testdata/geo-uri-fixtures.json): Go is
// the validation authority, so its verdict defines `valid`; the web and
// Android tests read the same file and must accept every valid=true input.
func TestGeoURIFixture_Parse(t *testing.T) {
	for _, c := range loadGeoFixtures(t).Parse {
		lat, lon, ok := ParseGeoURI(c.Input)
		if ok != c.Valid {
			t.Errorf("ParseGeoURI(%q) ok=%v, fixture valid=%v", c.Input, ok, c.Valid)
			continue
		}
		if ok && (math.Abs(lat-c.Lat) > 1e-12 || math.Abs(lon-c.Lon) > 1e-12) {
			t.Errorf("ParseGeoURI(%q) = %v,%v, want %v,%v", c.Input, lat, lon, c.Lat, c.Lon)
		}
	}
}

func TestGeoURIFixture_Format(t *testing.T) {
	for _, c := range loadGeoFixtures(t).Format {
		got := FormatGeoURI(c.Lat, c.Lon)
		if got != c.Expected {
			t.Errorf("FormatGeoURI(%v,%v) = %q, want %q", c.Lat, c.Lon, got, c.Expected)
		}
		if strings.ContainsAny(strings.TrimPrefix(got, "geo:"), "eE") {
			t.Errorf("FormatGeoURI(%v,%v) = %q uses an exponent", c.Lat, c.Lon, got)
		}
		if _, _, ok := ParseGeoURI(got); !ok {
			t.Errorf("FormatGeoURI output %q does not parse", got)
		}
	}
}
