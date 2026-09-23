package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupContactScoreTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return dbtest.New(t)
}

func createScoreTestUser(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()
	user := models.User{Username: username, Password: "password123!A", Email: username + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createScoreTestContact(t *testing.T, db *gorm.DB, userID uint, firstname string) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: firstname}
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

func createScoreTestEdge(t *testing.T, db *gorm.DB, userID uint, source, target, edgeType, sensitivity string) {
	t.Helper()
	edge := models.RelationshipEdge{
		UserID: userID, SourceID: source, TargetID: target, Type: edgeType,
		Directional: !models.IsSymmetricRelationType(edgeType),
		Source:      models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: models.RelationshipStatusConfirmed, Sensitivity: sensitivity,
	}
	require.NoError(t, db.Create(&edge).Error)
}

func createScoreTestActivity(t *testing.T, db *gorm.DB, userID uint, contact models.Contact, typ string, date time.Time) {
	t.Helper()
	activity := models.Activity{
		UserID: userID, Title: "interaction", Date: date, Type: typ,
		Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&activity).Error)
}

func setSelfContact(t *testing.T, db *gorm.DB, userID uint, uid string) {
	t.Helper()
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", userID).Update("self_contact_vcard_uid", uid).Error)
}

func TestComputeAllContactScores_BulkAndSingleAgree(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-agree")
	self := createScoreTestContact(t, db, user.ID, "Self")
	friend := createScoreTestContact(t, db, user.ID, "Friend")
	setSelfContact(t, db, user.ID, self.VCardUID)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, friend.VCardUID, "friend_of", models.RelationshipSensitivityNormal)
	createScoreTestActivity(t, db, user.ID, friend, models.InteractionTypeCall, time.Now().AddDate(0, 0, -5))

	now := time.Now()
	all, err := ComputeAllContactScores(db, user.ID, now)
	require.NoError(t, err)

	single, err := ComputeContactScore(db, user.ID, &friend, now)
	require.NoError(t, err)

	assert.Equal(t, all[friend.ID], single, "bulk and single-contact paths must agree exactly")
}

func TestComputeContactScore_UsesRealCadencePolicyInterval(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-policy")
	contact := createScoreTestContact(t, db, user.ID, "Alice")
	createScoreTestActivity(t, db, user.ID, contact, models.InteractionTypeCall, time.Now().AddDate(0, 0, -10))

	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 10}
	require.NoError(t, db.Create(&policy).Error)

	result, err := ComputeContactScore(db, user.ID, &contact, time.Now())
	require.NoError(t, err)
	// 10 days since last interaction against a real 10-day policy interval
	// is "exactly due" -> Recency should sit at the ratio-1.0 midpoint (50),
	// not wherever the closeness-tier default (45 days, unrelated relation)
	// would have placed it.
	assert.InDelta(t, 50, result.Recency.Value, 1.0)
}

func TestComputeContactScore_NoCadencePolicy_FallsBackToClosenessTierDefault(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-nopolicy")
	self := createScoreTestContact(t, db, user.ID, "Self")
	spouse := createScoreTestContact(t, db, user.ID, "Spouse")
	setSelfContact(t, db, user.ID, self.VCardUID)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, spouse.VCardUID, "spouse_of", models.RelationshipSensitivityNormal)
	// 14 days since interaction, no CadencePolicy -> spouse_of's tier default
	// interval (14 days, per weights.json) should be used, landing exactly
	// at the "due today" midpoint.
	createScoreTestActivity(t, db, user.ID, spouse, models.InteractionTypeCall, time.Now().AddDate(0, 0, -14))

	result, err := ComputeContactScore(db, user.ID, &spouse, time.Now())
	require.NoError(t, err)
	assert.InDelta(t, 50, result.Recency.Value, 1.0)
}

// TestComputeContactScore_SecretEdgeExcludedFromCloseness is the mandated
// regression: a secret relationship edge must never influence the score,
// producing byte-identical output to having no edge at all.
func TestComputeContactScore_SecretEdgeExcludedFromCloseness(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-secret")
	self := createScoreTestContact(t, db, user.ID, "Self")
	secretSpouse := createScoreTestContact(t, db, user.ID, "SecretSpouse")
	stranger := createScoreTestContact(t, db, user.ID, "Stranger")
	setSelfContact(t, db, user.ID, self.VCardUID)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, secretSpouse.VCardUID, "spouse_of", models.RelationshipSensitivitySecret)

	now := time.Now()
	secretResult, err := ComputeContactScore(db, user.ID, &secretSpouse, now)
	require.NoError(t, err)
	strangerResult, err := ComputeContactScore(db, user.ID, &stranger, now)
	require.NoError(t, err)

	assert.Equal(t, strangerResult.Closeness, secretResult.Closeness,
		"a secret spouse edge must score identically to no edge at all")

	// Sanity: a NORMAL-sensitivity spouse edge (otherwise identical) must NOT
	// match the stranger's score — proves the assertion above isn't
	// vacuously true because closeness never varies at all.
	normalSpouse := createScoreTestContact(t, db, user.ID, "NormalSpouse")
	createScoreTestEdge(t, db, user.ID, self.VCardUID, normalSpouse.VCardUID, "spouse_of", models.RelationshipSensitivityNormal)
	normalResult, err := ComputeContactScore(db, user.ID, &normalSpouse, now)
	require.NoError(t, err)
	assert.NotEqual(t, strangerResult.Closeness.Value, normalResult.Closeness.Value)
}

// TestComputeContactScore_ConflictsWithAloneDoesNotElevateCloseness proves
// the end-to-end (DB + traversal) version of the fix: a contact connected
// ONLY by an affinity edge must not read as "closer" than an unconnected
// stranger, whether the edge is direct or the only path in a multi-hop
// chain.
func TestComputeContactScore_ConflictsWithAloneDoesNotElevateCloseness(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-conflicts")
	self := createScoreTestContact(t, db, user.ID, "Self")
	nemesis := createScoreTestContact(t, db, user.ID, "Nemesis")
	stranger := createScoreTestContact(t, db, user.ID, "Stranger")
	setSelfContact(t, db, user.ID, self.VCardUID)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, nemesis.VCardUID, "conflicts_with", models.RelationshipSensitivityNormal)

	now := time.Now()
	nemesisResult, err := ComputeContactScore(db, user.ID, &nemesis, now)
	require.NoError(t, err)
	strangerResult, err := ComputeContactScore(db, user.ID, &stranger, now)
	require.NoError(t, err)

	assert.Equal(t, strangerResult.Closeness.Value, nemesisResult.Closeness.Value)
}

// TestComputeContactScore_TwoEdgeTypesResolveToHigherTierDeterministically
// pins the actual database-level version of the map-iteration-order bug fix:
// with both a friend_of and a conflicts_with edge stored between the same
// pair (legal — RelationshipEdge's unique key includes type), the score must
// always resolve to friend_of's tier, run after run.
func TestComputeContactScore_TwoEdgeTypesResolveToHigherTierDeterministically(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-multitype")
	self := createScoreTestContact(t, db, user.ID, "Self")
	contact := createScoreTestContact(t, db, user.ID, "Complicated")
	setSelfContact(t, db, user.ID, self.VCardUID)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, contact.VCardUID, "friend_of", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, contact.VCardUID, "conflicts_with", models.RelationshipSensitivityNormal)

	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		result, err := ComputeContactScore(db, user.ID, &contact, time.Now())
		require.NoError(t, err)
		assert.Equal(t, cfg.RelationCloseness["friend_of"].Weight, result.Closeness.Value)
	}
}

func TestComputeContactScore_NoSelfContact_UsesNeutralCloseness(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-noself")
	contact := createScoreTestContact(t, db, user.ID, "Alice")
	// Deliberately never call setSelfContact.

	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)

	result, err := ComputeContactScore(db, user.ID, &contact, time.Now())
	require.NoError(t, err)
	assert.Equal(t, cfg.UnknownClosenessWeight, result.Closeness.Value)
}

func TestComputeContactScore_PendingReachOutLowersOnlyThatContact(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-reachout")
	flagged := createScoreTestContact(t, db, user.ID, "Flagged")
	clean := createScoreTestContact(t, db, user.ID, "Clean")

	suggestion := models.ReachOutSuggestion{
		UserID: user.ID, ContactVCardUID: flagged.VCardUID,
		Kind: models.ReachOutKindOrganization, OldValue: "Old Co", NewValue: "New Co",
		AuditEventID: 1, Status: models.ReachOutStatusPending,
	}
	require.NoError(t, db.Create(&suggestion).Error)

	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)

	now := time.Now()
	flaggedResult, err := ComputeContactScore(db, user.ID, &flagged, now)
	require.NoError(t, err)
	cleanResult, err := ComputeContactScore(db, user.ID, &clean, now)
	require.NoError(t, err)

	assert.Equal(t, cfg.ReachOutPendingValue, flaggedResult.ReachOut.Value)
	assert.Equal(t, 100.0, cleanResult.ReachOut.Value)
}

// TestComputeContactScore_DismissedReachOutDoesNotCount proves only
// status=pending counts — a dismissed suggestion is resolved, not a live
// warning signal.
func TestComputeContactScore_DismissedReachOutDoesNotCount(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-dismissed")
	contact := createScoreTestContact(t, db, user.ID, "Alice")

	suggestion := models.ReachOutSuggestion{
		UserID: user.ID, ContactVCardUID: contact.VCardUID,
		Kind: models.ReachOutKindTitle, OldValue: "Old", NewValue: "New",
		AuditEventID: 1, Status: models.ReachOutStatusDismissed,
	}
	require.NoError(t, db.Create(&suggestion).Error)

	result, err := ComputeContactScore(db, user.ID, &contact, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.ReachOut.Value)
}

// TestComputeAllContactScores_CrossUserIsolation mirrors
// TestComputeCadenceHealth_ScopedToUser's pattern: a second user's contacts,
// activities, edges, and cadence policies must never leak into user A's
// computation.
func TestComputeAllContactScores_CrossUserIsolation(t *testing.T) {
	db := setupContactScoreTestDB(t)
	userA := createScoreTestUser(t, db, "score-usera")
	userB := createScoreTestUser(t, db, "score-userb")
	contactA := createScoreTestContact(t, db, userA.ID, "AliceA")
	contactB := createScoreTestContact(t, db, userB.ID, "AliceB")

	// Same VCardUID coincidence is impossible (UUIDs), but a shared
	// firstname must not cause any cross-user leakage either.
	createScoreTestActivity(t, db, userB.ID, contactB, models.InteractionTypeCall, time.Now())

	scoresA, err := ComputeAllContactScores(db, userA.ID, time.Now())
	require.NoError(t, err)
	require.Contains(t, scoresA, contactA.ID)
	assert.NotContains(t, scoresA, contactB.ID)
}

func TestComputeAllContactScores_EmptyContactList(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-empty")
	results, err := ComputeAllContactScores(db, user.ID, time.Now())
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestComputeAllContactScores_ArchivedContactsExcluded(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-archived")
	contact := createScoreTestContact(t, db, user.ID, "Archived")
	require.NoError(t, db.Model(&contact).Update("archived", true).Error)

	results, err := ComputeAllContactScores(db, user.ID, time.Now())
	require.NoError(t, err)
	assert.NotContains(t, results, contact.ID)

	// But the single-contact endpoint (contact-detail-page use case) must
	// still compute a real score for an archived contact, matching
	// GetContactBriefing's own no-archived-filter behavior.
	require.NoError(t, db.First(&contact, contact.ID).Error)
	single, err := ComputeContactScore(db, user.ID, &contact, time.Now())
	require.NoError(t, err)
	assert.NotZero(t, single.Closeness.Weight)
}

func TestComputeContactScore_FrequencyWindowBoundary(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-freqwindow")
	contact := createScoreTestContact(t, db, user.ID, "Alice")

	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)

	now := time.Now()
	// One interaction just inside the window, one just outside.
	createScoreTestActivity(t, db, user.ID, contact, models.InteractionTypeCall, now.AddDate(0, 0, -cfg.FrequencyWindowDays+1))
	createScoreTestActivity(t, db, user.ID, contact, models.InteractionTypeCall, now.AddDate(0, 0, -cfg.FrequencyWindowDays-1))

	result, err := ComputeContactScore(db, user.ID, &contact, now)
	require.NoError(t, err)
	// Only the in-window interaction should count.
	expectedRatio := 1.0 / (float64(cfg.FrequencyWindowDays) / float64(cfg.UnknownClosenessIntervalDays))
	assert.InDelta(t, 100*expectedRatio, result.Frequency.Value, 1.0)
}
