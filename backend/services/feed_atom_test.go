package services

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/i18n"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newFeedTestEnv(t testing.TB) (*gorm.DB, *config.Config) {
	t.Helper()
	require.NoError(t, i18n.Init())
	db := dbtest.New(t)
	return db, &config.Config{FrontendURL: "https://crm.example"}
}

// feedUserSeq keeps each seeded user's username/email unique.
var feedUserSeq int

func seedFeedUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	feedUserSeq++
	u := models.User{
		Username: fmt.Sprintf("feed-user-%d", feedUserSeq),
		Email:    fmt.Sprintf("feed-user-%d@example.com", feedUserSeq),
		Password: "password123", Language: "en",
	}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func seedContact(t *testing.T, db *gorm.DB, userID uint, first, last string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: first, Lastname: last}
	require.NoError(t, db.Create(&c).Error)
	return c
}

var feedRowSeq int

func seedFeedRow(t *testing.T, db *gorm.DB, userID uint, kind, entityID, detail string) *models.Feed {
	t.Helper()
	feedRowSeq++
	f := models.Feed{
		UserID: userID, Name: "feed", Kind: kind, EntityID: entityID, Detail: detail,
		TokenHash: fmt.Sprintf("feed-hash-%d", feedRowSeq),
	}
	require.NoError(t, db.Create(&f).Error)
	return &f
}

func parseFeed(t *testing.T, body []byte) atomFeed {
	t.Helper()
	var doc atomFeed
	require.NoError(t, xml.Unmarshal(body, &doc), "rendered Atom must be well-formed XML")
	return doc
}

func TestRenderAtomFeed_ContactFeedIsScopedAndOrdered(t *testing.T) {
	db, cfg := newFeedTestEnv(t)
	user := seedFeedUser(t, db)
	a := seedContact(t, db, user.ID, "Ada", "Lovelace")
	b := seedContact(t, db, user.ID, "Grace", "Hopper")

	now := time.Now().UTC()
	older := now.Add(-2 * time.Hour)
	newer := now.Add(-1 * time.Hour)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: "A older", Date: older}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: "A newer", Date: newer}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &b.ID, Content: "B only", Date: now.Add(-30 * time.Minute)}).Error)
	// A future-dated note must be excluded.
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: "future", Date: now.Add(time.Hour)}).Error)

	feed := seedFeedRow(t, db, user.ID, models.FeedKindContact, a.VCardUID, models.FeedDetailHeadlines)
	body, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: feed, User: &user, Contact: &a, Now: now})
	require.NoError(t, err)

	doc := parseFeed(t, body)
	require.Len(t, doc.Entries, 2, "only the contact's non-future notes")
	assert.Contains(t, doc.Title.Body, "Ada Lovelace")
	assert.NotContains(t, string(body), "B only")
	assert.NotContains(t, string(body), "future")

	// Newest first.
	require.Equal(t, atomTimestamp(newer), doc.Entries[0].Published)
	require.Equal(t, atomTimestamp(older), doc.Entries[1].Published)
}

func TestRenderAtomFeed_AggregateSpansLiveContactsAndDedupesActivity(t *testing.T) {
	db, cfg := newFeedTestEnv(t)
	user := seedFeedUser(t, db)
	a := seedContact(t, db, user.ID, "Ada", "Lovelace")
	b := seedContact(t, db, user.ID, "Grace", "Hopper")
	archived := seedContact(t, db, user.ID, "Archived", "Person")
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", archived.ID).Update("archived", true).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: "from A", Date: now.Add(-2 * time.Hour)}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &b.ID, Content: "from B", Date: now.Add(-90 * time.Minute)}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &archived.ID, Content: "from archived", Date: now.Add(-80 * time.Minute)}).Error)

	activity := models.Activity{UserID: user.ID, Title: "Shared meeting", Date: now.Add(-1 * time.Hour), Contacts: []models.Contact{a, b}}
	require.NoError(t, db.Create(&activity).Error)

	feed := seedFeedRow(t, db, user.ID, models.FeedKindAggregate, "", models.FeedDetailFull)
	body, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: feed, User: &user, Now: now})
	require.NoError(t, err)

	text := string(body)
	assert.Contains(t, text, "from A")
	assert.Contains(t, text, "from B")
	assert.NotContains(t, text, "from archived", "archived contacts must be excluded from an aggregate feed")

	doc := parseFeed(t, body)
	// Two notes + one activity = 3 entries; the multi-contact activity appears once.
	require.Len(t, doc.Entries, 3)
	var activityEntries int
	for _, e := range doc.Entries {
		if strings.Contains(e.Title.Body, "Activity") {
			activityEntries++
			assert.Contains(t, e.Title.Body, "Ada Lovelace")
			assert.Contains(t, e.Title.Body, "Grace Hopper")
		}
	}
	assert.Equal(t, 1, activityEntries, "a two-contact activity appears exactly once")
}

func TestRenderAtomFeed_DoesNotLeakOtherUsersData(t *testing.T) {
	db, cfg := newFeedTestEnv(t)
	user := seedFeedUser(t, db)
	other := seedFeedUser(t, db)
	a := seedContact(t, db, user.ID, "Ada", "Lovelace")
	otherContact := seedContact(t, db, other.ID, "Mallory", "Intruder")

	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: "own secret", Date: now}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: other.ID, ContactID: &otherContact.ID, Content: "foreign secret", Date: now}).Error)

	feed := seedFeedRow(t, db, user.ID, models.FeedKindAggregate, "", models.FeedDetailFull)
	body, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: feed, User: &user, Now: now})
	require.NoError(t, err)

	assert.Contains(t, string(body), "own secret")
	assert.NotContains(t, string(body), "foreign secret")
}

func TestRenderAtomFeed_DetailLevelsAndExternalPayload(t *testing.T) {
	db, cfg := newFeedTestEnv(t)
	user := seedFeedUser(t, db)
	a := seedContact(t, db, user.ID, "Ada", "Lovelace")
	now := time.Now().UTC()

	const noteSentinel = "NOTE_BODY_SENTINEL"
	const activitySentinel = "ACTIVITY_TITLE_SENTINEL"
	const payloadSentinel = "PAYLOAD_SECRET_SENTINEL"

	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: noteSentinel, Date: now.Add(-3 * time.Hour)}).Error)
	require.NoError(t, db.Create(&models.Activity{UserID: user.ID, Title: activitySentinel, Description: "desc", Location: "loc", Date: now.Add(-2 * time.Hour), Contacts: []models.Contact{a}}).Error)
	require.NoError(t, db.Create(&models.ExternalActivity{
		UserID: user.ID, EntityID: a.VCardUID, SourceSystem: "immich", ExternalID: "ext-1",
		Type: "photo-appearance", OccurredAt: now.Add(-time.Hour),
		Payload: map[string]interface{}{"internal_url": payloadSentinel},
	}).Error)

	headlines := seedFeedRow(t, db, user.ID, models.FeedKindContact, a.VCardUID, models.FeedDetailHeadlines)
	hBody, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: headlines, User: &user, Contact: &a, Now: now})
	require.NoError(t, err)
	assert.NotContains(t, string(hBody), noteSentinel)
	assert.NotContains(t, string(hBody), activitySentinel)
	assert.NotContains(t, string(hBody), payloadSentinel)

	full := seedFeedRow(t, db, user.ID, models.FeedKindContact, a.VCardUID, models.FeedDetailFull)
	fBody, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: full, User: &user, Contact: &a, Now: now})
	require.NoError(t, err)
	assert.Contains(t, string(fBody), noteSentinel)
	assert.Contains(t, string(fBody), activitySentinel)
	assert.NotContains(t, string(fBody), payloadSentinel, "external_activity payload is never emitted, even at full detail")

	// The external entry still appears (label + source system), just without
	// content.
	doc := parseFeed(t, fBody)
	var found bool
	for _, e := range doc.Entries {
		if strings.Contains(e.Title.Body, "immich") {
			found = true
			assert.Nil(t, e.Content, "external_activity never gets content")
		}
	}
	assert.True(t, found, "external_activity entry should still be listed")
}

func TestRenderAtomFeed_EscapingRoundTripsAndTextTypes(t *testing.T) {
	db, cfg := newFeedTestEnv(t)
	user := seedFeedUser(t, db)
	a := seedContact(t, db, user.ID, "Ada", "Lovelace")
	now := time.Now().UTC()

	raw := `<script>alert("x")</script> & ]]>` + " <b>bold</b>"
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: raw, Date: now}).Error)

	feed := seedFeedRow(t, db, user.ID, models.FeedKindContact, a.VCardUID, models.FeedDetailFull)
	body, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: feed, User: &user, Contact: &a, Now: now})
	require.NoError(t, err)

	doc := parseFeed(t, body)
	require.Len(t, doc.Entries, 1)
	require.NotNil(t, doc.Entries[0].Content)
	assert.Equal(t, raw, doc.Entries[0].Content.Body, "the note text round-trips through encoding/xml unchanged")

	// Every text construct is type="text" (never html/xhtml).
	assert.Equal(t, "text", doc.Title.Type)
	for _, e := range doc.Entries {
		assert.Equal(t, "text", e.Title.Type)
		if e.Content != nil {
			assert.Equal(t, "text", e.Content.Type)
		}
	}
	assert.Contains(t, string(body), `type="text"`)
}

func TestRenderAtomFeed_IsDeterministic(t *testing.T) {
	db, cfg := newFeedTestEnv(t)
	user := seedFeedUser(t, db)
	a := seedContact(t, db, user.ID, "Ada", "Lovelace")
	base := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, ContactID: &a.ID, Content: "stable", Date: base.Add(-time.Hour)}).Error)

	feed := seedFeedRow(t, db, user.ID, models.FeedKindContact, a.VCardUID, models.FeedDetailFull)
	first, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: feed, User: &user, Contact: &a, Now: base})
	require.NoError(t, err)
	second, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: feed, User: &user, Contact: &a, Now: base.Add(3 * time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "rendering must not depend on the request time")

	// The request time must not appear anywhere in the output.
	assert.NotContains(t, string(first), base.Format(time.RFC3339))
	assert.NotContains(t, string(first), base.Add(3*time.Hour).Format(time.RFC3339))
}

func TestRenderAtomFeed_CapsAtFiftyEntries(t *testing.T) {
	db, cfg := newFeedTestEnv(t)
	user := seedFeedUser(t, db)
	a := seedContact(t, db, user.ID, "Ada", "Lovelace")
	now := time.Now().UTC()
	for i := 0; i < 60; i++ {
		require.NoError(t, db.Create(&models.Note{
			UserID: user.ID, ContactID: &a.ID, Content: "n", Date: now.Add(-time.Duration(i) * time.Minute),
		}).Error)
	}

	feed := seedFeedRow(t, db, user.ID, models.FeedKindContact, a.VCardUID, models.FeedDetailHeadlines)
	body, err := RenderAtomFeed(db, cfg, AtomRenderInput{Feed: feed, User: &user, Contact: &a, Now: now})
	require.NoError(t, err)
	assert.Len(t, parseFeed(t, body).Entries, 50)
}
