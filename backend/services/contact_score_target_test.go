package services

import (
	"mycorrhizal/internal/logtest"
	"testing"
	"time"

	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1796: target_closeness reaches scoring through the one bulk contact
// load, both for a single contact and for the whole account.
func TestComputeContactScore_TargetClosenessProducesTarget(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-target")
	targeted := createScoreTestContact(t, db, user.ID, "Targeted")
	plain := createScoreTestContact(t, db, user.ID, "Plain")
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", targeted.ID).Update("target_closeness", "close").Error)
	require.NoError(t, db.First(&targeted, targeted.ID).Error)

	now := time.Now()
	createScoreTestActivity(t, db, user.ID, targeted, models.InteractionTypeCall, now.AddDate(0, 0, -15))

	got, err := ComputeContactScore(db, user.ID, &targeted, now)
	require.NoError(t, err)
	require.NotNil(t, got.Target)
	assert.Equal(t, "close", got.Target.Tier)
	assert.Equal(t, 30, got.Target.IntervalDays)
	assert.Equal(t, scoring.IntervalSourceTargetTier, got.Target.IntervalSource)

	none, err := ComputeContactScore(db, user.ID, &plain, now)
	require.NoError(t, err)
	assert.Nil(t, none.Target)

	// The bulk entry point (graph) selects the column too.
	all, err := ComputeAllContactScores(db, user.ID, now)
	require.NoError(t, err)
	require.NotNil(t, all[targeted.ID].Target)
	assert.Equal(t, got.Target, all[targeted.ID].Target)
	assert.Nil(t, all[plain.ID].Target)
}

// A pending target_behind row must not lower the ReachOut facet (ADR 0023 "No
// feedback loop"), while a pending org/title/address row still does.
func TestComputeContactScore_TargetBehindSuggestionDoesNotLowerReachOut(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-target-loop")
	behind := createScoreTestContact(t, db, user.ID, "Behind")
	titled := createScoreTestContact(t, db, user.ID, "Titled")

	require.NoError(t, db.Create(&models.ReachOutSuggestion{
		UserID: user.ID, ContactVCardUID: behind.VCardUID, Kind: models.ReachOutKindTargetBehind,
		OldValue: "close", NewValue: "2030-01-01T00:00:00Z", Status: models.ReachOutStatusPending,
	}).Error)
	require.NoError(t, db.Create(&models.ReachOutSuggestion{
		UserID: user.ID, ContactVCardUID: titled.VCardUID, Kind: models.ReachOutKindTitle,
		OldValue: "a", NewValue: "b", AuditEventID: 1, Status: models.ReachOutStatusPending,
	}).Error)

	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)
	now := time.Now()
	b, err := ComputeContactScore(db, user.ID, &behind, now)
	require.NoError(t, err)
	ti, err := ComputeContactScore(db, user.ID, &titled, now)
	require.NoError(t, err)

	assert.Equal(t, 100.0, b.ReachOut.Value)
	assert.Equal(t, cfg.ReachOutPendingValue, ti.ReachOut.Value)
}
