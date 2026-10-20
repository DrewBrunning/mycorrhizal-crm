package services

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/scoring"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issues #1733 (ambient contacts) and #1736 (given gifts as interactions):
// score facets and cadence overdue read both through the real migrated schema.

var ambientTestNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func seedHousehold(t *testing.T, db *gorm.DB, userID uint, members ...models.HouseholdMember) string {
	t.Helper()
	h := models.Household{UserID: userID, Name: "Home", Type: "family_unit"}
	require.NoError(t, db.Create(&h).Error)
	for _, m := range members {
		m.HouseholdID = h.ID
		m.UserID = userID
		require.NoError(t, db.Create(&m).Error)
	}
	return h.ID
}

func seedGift(t *testing.T, db *gorm.DB, userID uint, entityID, status string, date *time.Time) models.Gift {
	t.Helper()
	g := models.Gift{UserID: userID, EntityID: entityID, Status: status, Description: "a gift", Date: date}
	require.NoError(t, db.Create(&g).Error)
	return g
}

func scoreOf(t *testing.T, db *gorm.DB, userID uint, c models.Contact) scoring.Result {
	t.Helper()
	res, err := ComputeContactScore(db, userID, &c, ambientTestNow)
	require.NoError(t, err)
	return res
}

func TestAmbient_CoResidentScoresFullRecencyAndFrequencyWithNoActivities(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "amb-household")
	self := createScoreTestContact(t, db, user.ID, "Self")
	partner := createScoreTestContact(t, db, user.ID, "Partner")
	stranger := createScoreTestContact(t, db, user.ID, "Stranger")
	setSelfContact(t, db, user.ID, self.VCardUID)
	seedHousehold(t, db, user.ID,
		models.HouseholdMember{MemberVCardUID: self.VCardUID},
		models.HouseholdMember{MemberVCardUID: partner.VCardUID})

	res := scoreOf(t, db, user.ID, partner)
	assert.True(t, res.Ambient)
	assert.Equal(t, scoring.AmbientReasonHousehold, res.AmbientReason)
	assert.Equal(t, 100.0, res.Recency.Value)
	assert.Equal(t, 100.0, res.Frequency.Value)
	assert.Equal(t, "Ambient contact (lives with you)", res.Recency.Reason)
	assert.Equal(t, "Ambient contact (lives with you)", res.Frequency.Reason)

	// A non-ambient contact is unchanged: no activity means the documented
	// "no interaction" recency default, and zero frequency.
	other := scoreOf(t, db, user.ID, stranger)
	assert.False(t, other.Ambient)
	assert.Empty(t, other.AmbientReason)
	assert.Equal(t, "No qualifying interaction recorded yet", other.Recency.Reason)
	assert.Equal(t, 0.0, other.Frequency.Value)

	// The bulk path decorates the same answer.
	all, err := ComputeAllContactScores(db, user.ID, ambientTestNow)
	require.NoError(t, err)
	assert.Equal(t, res, all[partner.ID])
	assert.False(t, all[stranger.ID].Ambient)
}

func TestAmbient_CoResidenceEndsWhenMembershipEnds(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "amb-ended")
	self := createScoreTestContact(t, db, user.ID, "Self")
	moved := createScoreTestContact(t, db, user.ID, "Moved")
	setSelfContact(t, db, user.ID, self.VCardUID)
	// The roommate moved out yesterday.
	seedHousehold(t, db, user.ID,
		models.HouseholdMember{MemberVCardUID: self.VCardUID},
		models.HouseholdMember{MemberVCardUID: moved.VCardUID, Until: "2026-10-19"})

	res := scoreOf(t, db, user.ID, moved)
	assert.False(t, res.Ambient, "a membership that ended before today no longer counts")
	assert.Equal(t, "No qualifying interaction recorded yet", res.Recency.Reason, "decays normally")

	// An until of exactly today is still current (issue: "not before today").
	require.NoError(t, db.Model(&models.HouseholdMember{}).
		Where("member_vcard_uid = ?", moved.VCardUID).Update("until", "2026-10-20").Error)
	assert.True(t, scoreOf(t, db, user.ID, moved).Ambient)

	// Deleting the membership row ends co-residence too.
	require.NoError(t, db.Where("member_vcard_uid = ?", moved.VCardUID).Delete(&models.HouseholdMember{}).Error)
	assert.False(t, scoreOf(t, db, user.ID, moved).Ambient)
}

func TestAmbient_SelfMembershipMustAlsoBeCurrent(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "amb-selfended")
	self := createScoreTestContact(t, db, user.ID, "Self")
	old := createScoreTestContact(t, db, user.ID, "OldFlatmate")
	setSelfContact(t, db, user.ID, self.VCardUID)
	// The user left the house; the flatmate still lives there (open-ended).
	seedHousehold(t, db, user.ID,
		models.HouseholdMember{MemberVCardUID: self.VCardUID, Until: "2026-01-01"},
		models.HouseholdMember{MemberVCardUID: old.VCardUID})

	assert.False(t, scoreOf(t, db, user.ID, old).Ambient, "no longer co-resident once the user's own membership ended")
}

func TestAmbient_PartialDatesAndFutureSince(t *testing.T) {
	assert.True(t, ambientTenureCurrent("", "", "2026-10-20"))
	assert.True(t, ambientTenureCurrent("", "2026-10", "2026-10-20"), "until month: through the end of that month")
	assert.False(t, ambientTenureCurrent("", "2026-09", "2026-10-20"))
	assert.True(t, ambientTenureCurrent("", "2026", "2026-10-20"))
	assert.False(t, ambientTenureCurrent("", "2025", "2026-10-20"))
	assert.True(t, ambientTenureCurrent("2026-10-20", "", "2026-10-20"), "since today has started")
	assert.False(t, ambientTenureCurrent("2026-11-01", "", "2026-10-20"), "a future move-in has not started")
	assert.False(t, ambientTenureCurrent("2026-11", "", "2026-10-20"))
}

func TestAmbient_NoSelfContactMeansNoCoResidents(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "amb-noself")
	a := createScoreTestContact(t, db, user.ID, "A")
	b := createScoreTestContact(t, db, user.ID, "B")
	seedHousehold(t, db, user.ID,
		models.HouseholdMember{MemberVCardUID: a.VCardUID},
		models.HouseholdMember{MemberVCardUID: b.VCardUID})

	assert.False(t, scoreOf(t, db, user.ID, b).Ambient, "documented: no self-contact, no co-resident ambient")

	// The flag still works without a self-contact.
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", b.ID).Update("ambient", true).Error)
	res := scoreOf(t, db, user.ID, b)
	assert.True(t, res.Ambient)
	assert.Equal(t, scoring.AmbientReasonFlag, res.AmbientReason)
	assert.Equal(t, "Ambient contact (marked)", res.Recency.Reason)
}

func TestAmbient_FlagOnlyContactAndOtherFacetsUnchanged(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "amb-flag")
	self := createScoreTestContact(t, db, user.ID, "Self")
	coworker := createScoreTestContact(t, db, user.ID, "Coworker")
	twin := createScoreTestContact(t, db, user.ID, "Twin")
	setSelfContact(t, db, user.ID, self.VCardUID)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, coworker.VCardUID, "coworker_of", models.RelationshipSensitivityNormal)
	createScoreTestEdge(t, db, user.ID, self.VCardUID, twin.VCardUID, "coworker_of", models.RelationshipSensitivityNormal)
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", coworker.ID).Update("ambient", true).Error)

	flagged := scoreOf(t, db, user.ID, coworker)
	plain := scoreOf(t, db, user.ID, twin)
	assert.True(t, flagged.Ambient)
	assert.Equal(t, scoring.AmbientReasonFlag, flagged.AmbientReason)
	assert.Equal(t, 100.0, flagged.Recency.Value)
	assert.Equal(t, 100.0, flagged.Frequency.Value)
	assert.Equal(t, plain.Closeness, flagged.Closeness, "closeness is not overridden")
	assert.Equal(t, plain.ReachOut, flagged.ReachOut)
	assert.Equal(t, plain.LastUpdated.Weight, flagged.LastUpdated.Weight)
	assert.Greater(t, flagged.Score, plain.Score)
}

func TestAmbient_HouseholdReasonWinsOverFlagAndIsUserScoped(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "amb-both")
	intruder := createScoreTestUser(t, db, "amb-other")
	self := createScoreTestContact(t, db, user.ID, "Self")
	both := createScoreTestContact(t, db, user.ID, "Both")
	setSelfContact(t, db, user.ID, self.VCardUID)
	seedHousehold(t, db, user.ID,
		models.HouseholdMember{MemberVCardUID: self.VCardUID},
		models.HouseholdMember{MemberVCardUID: both.VCardUID})
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", both.ID).Update("ambient", true).Error)
	assert.Equal(t, scoring.AmbientReasonHousehold, scoreOf(t, db, user.ID, both).AmbientReason)

	// Another user's household rows naming these UIDs never count for this user.
	other := createScoreTestContact(t, db, user.ID, "Other")
	seedHousehold(t, db, intruder.ID,
		models.HouseholdMember{MemberVCardUID: self.VCardUID},
		models.HouseholdMember{MemberVCardUID: other.VCardUID})
	assert.False(t, scoreOf(t, db, user.ID, other).Ambient)

	// A member of a household the self-contact is NOT in is not a co-resident.
	third := createScoreTestContact(t, db, user.ID, "Third")
	fourth := createScoreTestContact(t, db, user.ID, "Fourth")
	seedHousehold(t, db, user.ID,
		models.HouseholdMember{MemberVCardUID: third.VCardUID},
		models.HouseholdMember{MemberVCardUID: fourth.VCardUID})
	assert.False(t, scoreOf(t, db, user.ID, fourth).Ambient)
}

func TestAmbient_BulkLookupIsConstantQueries(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "amb-queries")
	self := createScoreTestContact(t, db, user.ID, "Self")
	setSelfContact(t, db, user.ID, self.VCardUID)
	var uids []string
	for i := 0; i < 12; i++ {
		c := createScoreTestContact(t, db, user.ID, "C")
		uids = append(uids, c.VCardUID)
	}
	counter := &queryCounter{}
	// Scan runs through the Row callback, Pluck through Query.
	require.NoError(t, db.Callback().Query().Register("test:ambient_count_q", counter.count))
	require.NoError(t, db.Callback().Row().Register("test:ambient_count_r", counter.count))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test:ambient_count_q")
		_ = db.Callback().Row().Remove("test:ambient_count_r")
	})

	_, err := ambientByEntityID(db, user.ID, uids, ambientTestNow)
	require.NoError(t, err)
	assert.Equal(t, 1, counter.n, "one bulk query, independent of the contact count")
}

type queryCounter struct{ n int }

func (q *queryCounter) count(*gorm.DB) { q.n++ }

func TestAmbientContactIsNeverOverdueInCadence(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	// 40 days since the last call on a 30-day interval: overdue for a normal contact.
	policy := seedOverduePolicy(t, db, user, contact)
	now := time.Now()

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	require.Greater(t, health.OverdueBy, 0)
	assert.False(t, health.Ambient)
	overdue, err := ListOverdueCadences(db, user.ID, now)
	require.NoError(t, err)
	require.Len(t, overdue, 1)
	emitted, err := ProcessOverdueCadences(db, config.Config{})
	require.NoError(t, err)
	assert.Equal(t, 1, emitted)

	// Flag it: never overdue, no list entry, no webhook, reason reported.
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", contact.ID).Update("ambient", true).Error)
	health, err = ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	assert.True(t, health.Ambient)
	assert.Equal(t, scoring.AmbientReasonFlag, health.AmbientReason)
	assert.Zero(t, health.OverdueBy)
	assert.True(t, health.HasQualifyingInteraction, "the truthful last interaction is still reported")
	overdue, err = ListOverdueCadences(db, user.ID, now)
	require.NoError(t, err)
	assert.Empty(t, overdue)
	require.NoError(t, db.Unscoped().Where("job_name = ?", models.JobNameCadenceOverdue).Delete(&models.JobExecution{}).Error)
	emitted, err = ProcessOverdueCadences(db, config.Config{})
	require.NoError(t, err)
	assert.Zero(t, emitted)
}

func TestCoResidentIsNeverOverdueInCadence(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	self := models.Contact{UserID: user.ID, Firstname: "Self"}
	require.NoError(t, db.Create(&self).Error)
	setSelfContact(t, db, user.ID, self.VCardUID)
	policy := seedOverduePolicy(t, db, user, contact)
	seedHousehold(t, db, user.ID,
		models.HouseholdMember{MemberVCardUID: self.VCardUID},
		models.HouseholdMember{MemberVCardUID: contact.VCardUID})
	now := time.Now()

	health, err := ComputeCadenceHealth(db, user.ID, &policy, now)
	require.NoError(t, err)
	assert.True(t, health.Ambient)
	assert.Equal(t, scoring.AmbientReasonHousehold, health.AmbientReason)
	assert.Zero(t, health.OverdueBy)
	overdue, err := ListOverdueCadences(db, user.ID, now)
	require.NoError(t, err)
	assert.Empty(t, overdue)

	// Once the membership ends the contact is overdue again.
	require.NoError(t, db.Model(&models.HouseholdMember{}).
		Where("member_vcard_uid = ?", contact.VCardUID).
		Update("until", now.AddDate(0, 0, -1).Format("2006-01-02")).Error)
	overdue, err = ListOverdueCadences(db, user.ID, now)
	require.NoError(t, err)
	assert.Len(t, overdue, 1)
}

// ---- #1736: given gifts ----

func TestGivenGift_ResetsRecencyLikeAnActivity(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "gift-recency")
	viaGift := createScoreTestContact(t, db, user.ID, "ViaGift")
	viaActivity := createScoreTestContact(t, db, user.ID, "ViaActivity")
	three := ambientTestNow.AddDate(0, 0, -3)
	seedGift(t, db, user.ID, viaGift.VCardUID, models.GiftStatusGiven, &three)
	createScoreTestActivity(t, db, user.ID, viaActivity, models.InteractionTypeCall, three)

	g := scoreOf(t, db, user.ID, viaGift)
	a := scoreOf(t, db, user.ID, viaActivity)
	assert.Equal(t, a.Recency, g.Recency, "a 3-day-old given gift resets recency like a 3-day-old activity")
	assert.Equal(t, a.Frequency, g.Frequency, "and counts toward frequency")
	assert.Contains(t, g.Recency.Reason, "3 day(s) ago")
	assert.Greater(t, g.Frequency.Value, 0.0)
}

func TestGift_OnlyGivenWithDateCountsAndOnlyLiveOwnedOnes(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "gift-only-given")
	other := createScoreTestUser(t, db, "gift-other-user")
	c := createScoreTestContact(t, db, user.ID, "C")
	when := ambientTestNow.AddDate(0, 0, -2)

	for _, status := range []string{models.GiftStatusReceived, models.GiftStatusIdea, models.GiftStatusPurchased} {
		seedGift(t, db, user.ID, c.VCardUID, status, &when)
	}
	seedGift(t, db, user.ID, c.VCardUID, models.GiftStatusGiven, nil) // given, but undated
	deleted := seedGift(t, db, user.ID, c.VCardUID, models.GiftStatusGiven, &when)
	require.NoError(t, db.Delete(&deleted).Error)
	seedGift(t, db, other.ID, c.VCardUID, models.GiftStatusGiven, &when) // another user's row naming this contact

	res := scoreOf(t, db, user.ID, c)
	assert.Equal(t, "No qualifying interaction recorded yet", res.Recency.Reason,
		"received/idea/purchased, undated, soft-deleted and foreign-user gifts must not count")
	assert.Equal(t, 0.0, res.Frequency.Value)

	// The one that should count.
	seedGift(t, db, user.ID, c.VCardUID, models.GiftStatusGiven, &when)
	res = scoreOf(t, db, user.ID, c)
	assert.Contains(t, res.Recency.Reason, "2 day(s) ago")
}

func TestGift_LinkedActivityIsNotCountedTwice(t *testing.T) {
	db := setupContactScoreTestDB(t)
	user := createScoreTestUser(t, db, "gift-dedupe")
	c := createScoreTestContact(t, db, user.ID, "C")
	when := ambientTestNow.AddDate(0, 0, -1)
	act := models.Activity{UserID: user.ID, Title: "gave it", Date: when, Type: "gift", Contacts: []models.Contact{c}}
	require.NoError(t, db.Create(&act).Error)
	g := seedGift(t, db, user.ID, c.VCardUID, models.GiftStatusGiven, &when)
	require.NoError(t, db.Model(&g).Update("activity_id", act.ID).Error)

	counts, err := qualifyingInteractionCountsByEntityID(db, user.ID, []string{c.VCardUID}, nil, ambientTestNow.AddDate(0, 0, -30))
	require.NoError(t, err)
	assert.Equal(t, 1, counts[c.VCardUID], "the gift and the activity it is linked to are one handover")
}

func TestGift_ResetsCadenceOverdueAndRespectsQualifyingTypes(t *testing.T) {
	db, user, contact := setupCadenceServiceTestDB(t)
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	// Only an old call (Jan 1): 30-day interval -> overdue.
	seedActivity(t, db, user, contact, models.InteractionTypeCall, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	gifted := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	seedGift(t, db, user.ID, contact.VCardUID, models.GiftStatusGiven, &gifted)
	seedGift(t, db, user.ID, contact.VCardUID, models.GiftStatusReceived, &gifted)

	cases := []struct {
		name    string
		types   []string
		overdue bool
		last    time.Time
	}{
		{"empty list counts gifts", nil, false, gifted},
		{"call-only ignores gifts", []string{"call"}, true, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)},
		{"gift listed counts", []string{"gift"}, false, gifted},
		{"call and gift", []string{"call", "gift"}, false, gifted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("entity_id = ?", contact.VCardUID).Delete(&models.CadencePolicy{}).Error)
			policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30, QualifyingTypes: tc.types}
			require.NoError(t, db.Create(&policy).Error)

			single, err := ComputeCadenceHealth(db, user.ID, &policy, now)
			require.NoError(t, err)
			assert.Equal(t, tc.overdue, single.OverdueBy > 0)
			require.NotNil(t, single.LastInteraction)
			assert.True(t, tc.last.Equal(*single.LastInteraction))

			overdue, err := ListOverdueCadences(db, user.ID, now)
			require.NoError(t, err)
			assert.Equal(t, tc.overdue, len(overdue) == 1, "the batch path must agree with the single-policy path")
		})
	}
}

// Issue #1733: the account bundle carries crm.ambient and the import restores
// it; a bundle that predates the flag (no crm.ambient) imports as non-ambient.
func TestAccountBundle_CarriesAmbientFlag(t *testing.T) {
	ownerDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "bundle-amb-owner")
	target := bundleTestUser(t, targetDB, "bundle-amb-target")
	seed := seedFullAccount(t, ownerDB, owner, "AMB")
	useBundlePhotoDir(t)
	bob := seed.Contacts[1]
	require.NoError(t, ownerDB.Model(&models.Contact{}).Where("id = ?", bob.ID).Update("ambient", true).Error)

	exported, _, err := BuildAccountBundle(ownerDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	flagged := 0
	for _, c := range exported.Plan.Contacts {
		if c.CRM.Ambient != nil && *c.CRM.Ambient {
			flagged++
			assert.Equal(t, bob.VCardUID, c.UID)
		}
	}
	require.Equal(t, 1, flagged, "exactly the ambient contact carries crm.ambient=true")

	report, _, err := ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	require.Empty(t, report.Issues)

	var imported []models.Contact
	require.NoError(t, targetDB.Where("user_id = ?", target.ID).Find(&imported).Error)
	require.Len(t, imported, 3)
	for _, c := range imported {
		assert.Equal(t, c.VCardUID == bob.VCardUID, c.Ambient, "ambient must round-trip for %s", c.Firstname)
	}

	// A pre-flag bundle: crm.ambient absent everywhere -> everything non-ambient.
	for i := range exported.Plan.Contacts {
		exported.Plan.Contacts[i].CRM.Ambient = nil
	}
	target2DB := dbtest.New(t)
	target2 := bundleTestUser(t, target2DB, "bundle-amb-target2")
	_, _, err = ExecuteSourceImportWithActions(t.Context(), target2DB, target2.ID, MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	var plain []models.Contact
	require.NoError(t, target2DB.Where("user_id = ?", target2.ID).Find(&plain).Error)
	require.Len(t, plain, 3)
	for _, c := range plain {
		assert.False(t, c.Ambient)
	}
}
