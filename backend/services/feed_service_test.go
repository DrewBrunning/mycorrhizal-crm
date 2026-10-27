package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/fireandforget"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevokeAllFeeds_RevokesOnlyCallersActiveFeeds(t *testing.T) {
	db := dbtest.New(t)
	user := seedFeedUser(t, db)
	other := seedFeedUser(t, db)

	active := &models.Feed{UserID: user.ID, Name: "active", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "revoke-active"}
	require.NoError(t, db.Create(active).Error)
	already := &models.Feed{UserID: user.ID, Name: "already", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "revoke-already"}
	require.NoError(t, db.Create(already).Error)
	require.NoError(t, db.Model(already).Update("revoked_at", time.Now()).Error)
	foreign := &models.Feed{UserID: other.ID, Name: "foreign", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "revoke-foreign"}
	require.NoError(t, db.Create(foreign).Error)

	n, err := RevokeAllFeeds(db, user.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "only the caller's active feed is counted")

	var gotActive, gotForeign models.Feed
	require.NoError(t, db.First(&gotActive, "id = ?", active.ID).Error)
	require.NoError(t, db.First(&gotForeign, "id = ?", foreign.ID).Error)
	assert.NotNil(t, gotActive.RevokedAt)
	assert.Nil(t, gotForeign.RevokedAt, "another user's feed stays active")
}

func TestTouchFeed_WritesAndThrottlesRecentAccess(t *testing.T) {
	db := dbtest.New(t)
	user := seedFeedUser(t, db)
	feed := &models.Feed{UserID: user.ID, Name: "touch", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "touch-1"}
	require.NoError(t, db.Create(feed).Error)

	// A NULL last_accessed_at schedules the async write.
	TouchFeed(db, feed)
	fireandforget.Wait() // the write is tracked, so shutdown (and this test) can drain it
	var touched models.Feed
	require.NoError(t, db.First(&touched, "id = ?", feed.ID).Error)
	require.NotNil(t, touched.LastAccessedAt, "TouchFeed must write last_accessed_at")

	// A fresh, recent row takes the throttle branch and returns synchronously.
	var recent models.Feed
	require.NoError(t, db.First(&recent, "id = ?", feed.ID).Error)
	TouchFeed(db, &recent)
}
