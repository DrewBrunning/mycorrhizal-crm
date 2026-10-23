package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiftCalendarDay(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	auckland, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	cases := []struct {
		name string
		in   time.Time
		loc  *time.Location
		want time.Time
	}{
		{"a UTC-midnight calendar day is kept, whatever the zone", day(2026, 10, 22), berlin, day(2026, 10, 22)},
		{"UTC midnight in another offset's clothing is still that day", time.Date(2026, 10, 22, 2, 0, 0, 0, time.FixedZone("+02", 2*3600)), chicago, day(2026, 10, 22)},
		{"Berlin local midnight (22:00Z the day before) is the Berlin day", time.Date(2026, 10, 22, 0, 0, 0, 0, berlin), berlin, day(2026, 10, 22)},
		{"Auckland local midnight (11:00Z the day before) is the Auckland day", time.Date(2026, 10, 22, 0, 0, 0, 0, auckland), auckland, day(2026, 10, 22)},
		{"Chicago local midnight (05:00Z) is the Chicago day", time.Date(2026, 10, 22, 0, 0, 0, 0, chicago), chicago, day(2026, 10, 22)},
		{"an evening 'mark given now' west of UTC stays on that evening's day", time.Date(2026, 10, 23, 2, 30, 0, 0, time.UTC), chicago, day(2026, 10, 22)},
		{"nil loc reads the value in its own offset", time.Date(2026, 10, 22, 0, 0, 0, 0, berlin), nil, day(2026, 10, 22)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := GiftCalendarDay(tc.in, tc.loc)
			assert.True(t, got.Equal(tc.want), "got %s, want %s", got, tc.want)
			assert.Equal(t, time.UTC, got.Location(), "stored at UTC midnight")
		})
	}
}

func TestGiftCalendarDayPtr(t *testing.T) {
	t.Parallel()
	assert.Nil(t, GiftCalendarDayPtr(nil, time.UTC))
	in := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := GiftCalendarDayPtr(&in, time.UTC)
	require.NotNil(t, got)
	assert.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), *got)
}
