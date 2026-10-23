package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"
)

// The Frequency facet's window is counted in calendar days. A bound
// `a.date >= ?` with a reminder-zone `since` used to compare RFC 3339 text
// across offsets ("…+02:00" vs "…Z"), so east of UTC an interaction on the
// window's first day was silently dropped, and a gift (stored at UTC midnight)
// with it (v1.5.0 review).
func TestFrequencyWindow_CountsByCalendarDayInAnyZone(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)

	for _, zone := range []string{"Europe/Berlin", "UTC", "America/Chicago", "Pacific/Auckland"} {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			require.NoError(t, err)
			db := dbtest.New(t)
			user := models.User{Username: "freq", Password: "password123!A", Email: "freq@example.com"}
			require.NoError(t, db.Create(&user).Error)
			contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
			require.NoError(t, db.Create(&contact).Error)

			// 01:00 local on 24 July: in Berlin that is still 23 July in UTC.
			now := time.Date(2026, 7, 24, 1, 0, 0, 0, loc)
			first := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -cfg.FrequencyWindowDays)
			before := first.AddDate(0, 0, -1)

			for _, d := range []time.Time{first, before} {
				act := models.Activity{UserID: user.ID, Title: "call", Type: "call", Date: d, Contacts: []models.Contact{contact}}
				require.NoError(t, db.Create(&act).Error)
			}
			seedGift(t, db, user.ID, contact.VCardUID, models.GiftStatusGiven, &first)

			scores, err := ComputeAllContactScores(db, user.ID, now)
			require.NoError(t, err)
			freq := scores[contact.ID].Frequency
			require.NotNil(t, freq.ReasonParams.Count)
			assert.Equal(t, 2, *freq.ReasonParams.Count,
				"the activity and the gift on the window's first day count; the day before does not")
		})
	}
}
