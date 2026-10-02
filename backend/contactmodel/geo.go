package contactmodel

import (
	"math"
	"strconv"
	"strings"
)

// ParseGeoURI parses an RFC 5870 geo: URI ("geo:48.2010,16.3695" or
// "geo:48.2,16.3,183;crs=wgs84;u=40") — the form Address.Coordinates carries
// (vCard GEO) — and returns its WGS-84 latitude and longitude. ok is false
// when the scheme is not geo:, the coordinate pair is missing or non-numeric,
// or the values are out of range (latitude outside -90..90, longitude outside
// -180..180). The optional altitude and the URI parameters are ignored: the
// contact map plots a point, nothing else.
func ParseGeoURI(uri string) (lat, lon float64, ok bool) {
	rest, found := cutPrefixFold(uri, "geo:")
	if !found {
		return 0, 0, false
	}
	if i := strings.IndexByte(rest, ';'); i >= 0 {
		rest = rest[:i]
	}
	parts := strings.Split(rest, ",")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, false
	}
	var err error
	if lat, err = strconv.ParseFloat(parts[0], 64); err != nil {
		return 0, 0, false
	}
	if lon, err = strconv.ParseFloat(parts[1], 64); err != nil {
		return 0, 0, false
	}
	if len(parts) == 3 {
		if _, err = strconv.ParseFloat(parts[2], 64); err != nil {
			return 0, 0, false
		}
	}
	// ParseFloat accepts "NaN"/"Inf"; the range check below rejects both
	// (every comparison with NaN is false), so spell the finite test out.
	if math.IsNaN(lat) || math.IsNaN(lon) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}
	return lat, lon, true
}

// FormatGeoURI renders a latitude/longitude as a geo: URI, rounded to six
// decimal places (about 0.1 m) with trailing zeros trimmed.
func FormatGeoURI(lat, lon float64) string {
	return "geo:" + formatCoord(lat) + "," + formatCoord(lon)
}

func formatCoord(v float64) string {
	return strconv.FormatFloat(math.Round(v*1e6)/1e6, 'f', -1, 64)
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return s, false
	}
	return s[len(prefix):], true
}
