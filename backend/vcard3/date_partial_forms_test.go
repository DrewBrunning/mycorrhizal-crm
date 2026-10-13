package vcard3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/contactmodel"
)

func ip(n int) *int { return &n }

// TestParseDatePartial_AllForms pins every accepted DATE-AND-OR-TIME spelling
// (extended, dashed, compact, reduced, timestamp) and the verbatim fallback for
// an unparseable value.
func TestParseDatePartial_AllForms(t *testing.T) {
	t.Parallel()
	partial := func(y, m, d *int) contactmodel.AnniversaryDate {
		return contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: y, Month: m, Day: d}}
	}
	ts := func(s string) contactmodel.AnniversaryDate { return contactmodel.AnniversaryDate{Timestamp: &s} }

	cases := []struct {
		in   string
		want contactmodel.AnniversaryDate
	}{
		{"1990-02-03", partial(ip(1990), ip(2), ip(3))},
		{"--02-29", partial(nil, ip(2), ip(29))},
		{"  --02-29  ", partial(nil, ip(2), ip(29))},
		{"--0229", partial(nil, ip(2), ip(29))},
		{"19900203", partial(ip(1990), ip(2), ip(3))},
		{"---05", partial(nil, nil, ip(5))},
		{"1990", partial(ip(1990), nil, nil)},
		{"1996-10-22T14:00:00-05:00", ts("1996-10-22T19:00:00Z")},
		{"not a date", ts("not a date")},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, parseDatePartial(tc.in))
		})
	}
}

// TestFormatDatePartial_AllForms pins the export spelling of every precision
// the neutral PartialDate can carry, and the timestamp truncation.
func TestFormatDatePartial_AllForms(t *testing.T) {
	t.Parallel()
	pd := func(y, m, d *int) contactmodel.AnniversaryDate {
		return contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{Year: y, Month: m, Day: d}}
	}
	long, short := "2020-05-06T07:08:09Z", "2020"
	cases := []struct {
		name string
		in   contactmodel.AnniversaryDate
		want string
	}{
		{"timestamp truncates to the date", contactmodel.AnniversaryDate{Timestamp: &long}, "2020-05-06"},
		{"short timestamp kept verbatim", contactmodel.AnniversaryDate{Timestamp: &short}, "2020"},
		{"full", pd(ip(1990), ip(2), ip(3)), "1990-02-03"},
		{"year-less", pd(nil, ip(2), ip(29)), "--02-29"},
		{"year-month", pd(ip(1990), ip(2), nil), "1990-02"},
		{"year only", pd(ip(1990), nil, nil), "1990"},
		{"month only", pd(nil, ip(2), nil), "--02"},
		{"day only", pd(nil, nil, ip(5)), "---05"},
		{"empty", contactmodel.AnniversaryDate{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, formatDatePartial(tc.in))
		})
	}
}
