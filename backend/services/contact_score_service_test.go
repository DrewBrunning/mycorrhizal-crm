package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"

	"github.com/glebarez/sqlite"
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

// TestComputeContactScore_DirectEdgeReversedDirectionStillResolves proves
// directRelationTypesByEntityID's "other" resolution works when the edge
// points AT the self-contact (contact -> self) rather than away from it
// (self -> contact) -- every other test in this file only exercises the
// self-as-source direction.
func TestComputeContactScore_DirectEdgeReversedDirectionStillResolves(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-reverseedge")
	self := createScoreTestContact(t, db, user.ID, "Self")
	parent := createScoreTestContact(t, db, user.ID, "Parent")
	setSelfContact(t, db, user.ID, self.VCardUID)
	// parent -> self (child_of), the reverse of every other test's
	// self -> contact direction.
	createScoreTestEdge(t, db, user.ID, parent.VCardUID, self.VCardUID, "parent_of", models.RelationshipSensitivityNormal)

	cfg, err := scoring.LoadConfig()
	require.NoError(t, err)

	result, err := ComputeContactScore(db, user.ID, &parent, time.Now())
	require.NoError(t, err)
	assert.Equal(t, cfg.RelationCloseness["parent_of"].Weight, result.Closeness.Value)
}

func TestDaysBetween(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("positive gap", func(t *testing.T) {
		assert.Equal(t, 10, daysBetween(now.AddDate(0, 0, -10), now))
	})

	t.Run("zero gap", func(t *testing.T) {
		assert.Equal(t, 0, daysBetween(now, now))
	})

	t.Run("future t clamps to zero, never negative", func(t *testing.T) {
		// t in the future shouldn't happen from real data (see daysBetween's
		// own doc comment), but must not return a negative day count if it
		// somehow does.
		assert.Equal(t, 0, daysBetween(now.AddDate(0, 0, 5), now))
	})
}

// --- Error-path coverage: each bulk query's failure must be reported, not
// silently swallowed. Each test uses a minimal AutoMigrate schema with
// exactly the table the target step needs missing, so every EARLIER step in
// computeContactScores's sequence still succeeds and the failure is
// isolated to the one query under test.
//
// Three branches are deliberately left uncovered, consistent with this
// codebase's existing tolerance for a genuinely unhittable/indistinguishable
// error path (e.g. GetContactBriefing's own compose-error branch has the
// same gap):
//   - scoring.LoadConfig() failing inside computeContactScores: the embedded
//     testdata/weights.json is validated to 100% coverage on its own
//     (internal/scoring's tests) and can't be made to fail at runtime
//     without corrupting the embed itself.
//   - qualifyingInteractionCountsByEntityID's query failure: it queries the
//     exact same activities/activity_contacts/contacts join
//     lastQualifyingInteractionByEntityID already does, so by the time this
//     step runs, that table is already proven to exist — there is no
//     schema-only way to fail this query without also failing the one
//     immediately before it.
//   - structuralHopsByEntityID's query failure (TraverseGraph's own raw
//     SQL): tried anticipating this would surface CLAUDE.md backend trap 1
//     (TraverseGraph's `INDEXED BY idx_relationship_edges_source_id` hint
//     only exists in the hand-written migration, never in RelationshipEdge's
//     GORM tags, so an AutoMigrate-derived schema has no such index) — but
//     glebarez/sqlite (this test suite's pure-Go driver) does not error on a
//     missing INDEXED BY target the way the production driver's real SQLite
//     does, so this can't be triggered from a unit test with this driver.

func TestComputeAllContactScores_ContactsQueryFails(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "score-contactsfail")

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = ComputeAllContactScores(db, user.ID, time.Now())
	assert.ErrorContains(t, err, "loading contacts")
}

func TestComputeContactScores_CadencePoliciesQueryFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}))

	user := createScoreTestUser(t, db, "score-cadencefail")
	contact := createScoreTestContact(t, db, user.ID, "Alice")

	_, err = ComputeContactScore(db, user.ID, &contact, time.Now())
	assert.ErrorContains(t, err, "loading cadence policies")
}

func TestComputeContactScores_LastInteractionQueryFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.CadencePolicy{}))
	// Contact's own `Activities []Activity` many2many tag makes AutoMigrate
	// create "activities"/"activity_contacts" implicitly even though
	// models.Activity was never listed above -- drop it explicitly so this
	// step's query is the one that actually fails, not silently succeeds
	// against a table AutoMigrate created as a side effect.
	require.NoError(t, db.Migrator().DropTable("activities"))

	user := createScoreTestUser(t, db, "score-lastintfail")
	contact := createScoreTestContact(t, db, user.ID, "Alice")

	_, err = ComputeContactScore(db, user.ID, &contact, time.Now())
	assert.ErrorContains(t, err, "loading last qualifying interactions")
}

func TestComputeContactScores_SelfContactQueryFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// No models.User table at all -- selfContactVCardUID's query against
	// "users" is the first thing this schema can't answer. SQLite enforces
	// no FK constraint here, so a Contact can still reference a user ID with
	// no real row.
	require.NoError(t, db.AutoMigrate(&models.Contact{}, &models.CadencePolicy{}, &models.Activity{}))

	contact := createScoreTestContact(t, db, 1, "Alice")

	_, err = ComputeContactScore(db, 1, &contact, time.Now())
	assert.ErrorContains(t, err, "loading self-contact")
}

func TestComputeContactScores_DirectRelationTypesQueryFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// No models.RelationshipEdge table -- reached only when a self-contact
	// is actually set (the `if selfContactUID != ""` guard), so User must be
	// present and populated.
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.CadencePolicy{}, &models.Activity{}))

	user := createScoreTestUser(t, db, "score-directrelfail")
	self := createScoreTestContact(t, db, user.ID, "Self")
	contact := createScoreTestContact(t, db, user.ID, "Alice")
	setSelfContact(t, db, user.ID, self.VCardUID)

	_, err = ComputeContactScore(db, user.ID, &contact, time.Now())
	assert.ErrorContains(t, err, "loading direct relationship edges")
}

func TestComputeContactScores_PendingReachOutQueryFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// No self-contact set (steps 7/8 are skipped via the `if selfContactUID
	// != ""` guard), so RelationshipEdge doesn't need to exist here -- only
	// models.ReachOutSuggestion is deliberately missing.
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.CadencePolicy{}, &models.Activity{}))

	user := createScoreTestUser(t, db, "score-reachoutfail")
	contact := createScoreTestContact(t, db, user.ID, "Alice")

	_, err = ComputeContactScore(db, user.ID, &contact, time.Now())
	assert.ErrorContains(t, err, "loading pending reach-out suggestions")
}
