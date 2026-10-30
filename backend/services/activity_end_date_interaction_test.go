package services

import (
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2006 interaction rule: the START date stays the interaction date for
// cadence (and so reminders/occasions that read it). An end date never creates
// a second qualifying interaction and never moves the last interaction or the
// next-due day.
func TestActivityEndDateDoesNotChangeInteractionDate(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	trip := models.Activity{
		UserID: user.ID, Title: "Long trip", Type: models.InteractionTypeVisit,
		Date: start, EndDate: &end, Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&trip).Error)

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	require.NoError(t, db.Create(&policy).Error)

	// Jan 31 is start + 30. If the end counted, next due would be Feb 19.
	now := time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC)
	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.True(t, health.HasQualifyingInteraction)
	require.NotNil(t, health.LastInteraction)
	assert.Equal(t, start, health.LastInteraction.UTC())
	require.NotNil(t, health.NextDue)
	assert.Equal(t, "2026-01-31", health.NextDue.Format("2006-01-02"))
	assert.Equal(t, 5, health.OverdueBy, "overdue is measured from start + interval, not end + interval")

	// The batch path (overdue list) agrees.
	overdue, err := ListOverdueCadences(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, overdue, 1)
	assert.Equal(t, 5, overdue[0].Health.OverdueBy)
	assert.Equal(t, start, overdue[0].Health.LastInteraction.UTC())

	// And the timeline shows ONE entry, dated at the start.
	items, _, err := ComposeTimeline(db, user.ID, TimelineQuery{ContactID: &contact.ID, Limit: 10, Desc: true, Types: []string{models.TimelineTypeActivity}, Now: now})
	require.NoError(t, err)
	require.Len(t, items, 1, "a multi-day activity is a single interaction, not one per day")
	assert.Equal(t, start, items[0].Date.UTC())
}
