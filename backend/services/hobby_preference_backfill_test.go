package services

import (
	"mycorrhizal/internal/logtest"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1738's one-time lossless move of hobby preferences into
// Card.PersonalInfo (favorites/likes) or the new dislike_activity category
// (dislikes). These run on the real migrated schema (dbtest), because the
// data_backfills ledger and the encrypted preference/value columns are what
// the once-only gate and write path depend on.

func newHobbyBackfillFixture(t *testing.T) (*gorm.DB, models.User, models.Contact) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "hobbybf", Password: "password123!A", Email: "hobbybf@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&contact).Error)
	return db, user, contact
}

func seedHobbyPreference(t *testing.T, db *gorm.DB, userID uint, entityID, key, value, level, sensitivity string) models.Preference {
	t.Helper()
	pref := models.Preference{
		UserID:      userID,
		EntityID:    entityID,
		Category:    models.PreferenceCategoryHobby,
		Key:         key,
		Value:       value,
		Notes:       "",
		Source:      models.PreferenceSourceUser,
		Sensitivity: sensitivity,
	}
	if level != "" {
		pref.Level = models.PreferenceLevelPtr(level)
	}
	if sensitivity == "" {
		pref.Sensitivity = models.RelationshipSensitivityNormal
	}
	require.NoError(t, db.Create(&pref).Error)
	return pref
}

func reloadContact(t *testing.T, db *gorm.DB, id uint) models.Contact {
	t.Helper()
	var contact models.Contact
	require.NoError(t, db.First(&contact, id).Error)
	return contact
}

func personalInfoByValue(contact models.Contact, value string) (contactmodel.PersonalInfo, bool) {
	for _, entry := range contact.Card.PersonalInfo {
		if entry.Value == value {
			return entry, true
		}
	}
	return contactmodel.PersonalInfo{}, false
}

// TestMigrateHobbyPreferences_FavoriteBecomesPersonalInfoWithMeta covers the
// main mapping and every carried-over field, then the recoverable soft-delete
// and the once-only ledger.
func TestMigrateHobbyPreferences_FavoriteBecomesPersonalInfoWithMeta(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db, user, contact := newHobbyBackfillFixture(t)
	confirmed := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	confidence := 0.8

	favorite := seedHobbyPreference(t, db, user.ID, contact.VCardUID, "favorite", "Piano", models.PreferenceLevelHigh, models.RelationshipSensitivityPrivate)
	favorite.Notes = "plays every evening"
	favorite.Confidence = &confidence
	favorite.LastConfirmed = &confirmed
	require.NoError(t, db.Save(&favorite).Error)

	like := seedHobbyPreference(t, db, user.ID, contact.VCardUID, "like", "Chess", models.PreferenceLevelMedium, models.RelationshipSensitivityNormal)

	stats, err := MigrateHobbyPreferencesToPersonalInfo(db)
	require.NoError(t, err)
	assert.False(t, stats.AlreadyDone)
	assert.EqualValues(t, 2, stats.PreferencesScanned)
	assert.EqualValues(t, 1, stats.ContactsUpdated)
	assert.EqualValues(t, 2, stats.PersonalInfoEntriesMerged)
	assert.EqualValues(t, 2, stats.PreferencesSoftDeleted)

	stored := reloadContact(t, db, contact.ID)
	piano, ok := personalInfoByValue(stored, "Piano")
	require.True(t, ok, "favorite hobby must become a personalInfo entry")
	assert.Equal(t, "hobby", piano.Kind)
	assert.Equal(t, "high", piano.Level)
	require.NotEmpty(t, piano.ID, "the new entry needs a stable ID")

	pianoMeta, ok := stored.CRM.PersonalInfoMeta[piano.ID]
	require.True(t, ok, "metadata must be keyed by the entry ID")
	assert.Equal(t, models.RelationshipSensitivityPrivate, pianoMeta.Sensitivity)
	assert.True(t, pianoMeta.Favorite)
	assert.Equal(t, "plays every evening", pianoMeta.Notes)
	assert.Equal(t, models.PreferenceSourceUser, pianoMeta.Source)
	require.NotNil(t, pianoMeta.Confidence)
	assert.InDelta(t, 0.8, *pianoMeta.Confidence, 1e-9)
	require.NotNil(t, pianoMeta.LastConfirmed)
	assert.True(t, confirmed.Equal(*pianoMeta.LastConfirmed))

	chess, ok := personalInfoByValue(stored, "Chess")
	require.True(t, ok)
	assert.Equal(t, "medium", chess.Level)
	chessMeta := stored.CRM.PersonalInfoMeta[chess.ID]
	assert.Equal(t, models.RelationshipSensitivityNormal, chessMeta.NormalizedSensitivity())
	assert.False(t, chessMeta.Favorite, "a like is not a favorite")

	// Moved rows are recoverable, not destroyed, and the normal list endpoint
	// (soft-delete-scoped) no longer returns them.
	for _, pref := range []models.Preference{favorite, like} {
		var raw models.Preference
		require.NoError(t, db.Unscoped().First(&raw, "id = ?", pref.ID).Error)
		assert.True(t, raw.DeletedAt.Valid, "moved preference %s must be soft-deleted", pref.ID)
		assert.Equal(t, pref.Value, raw.Value)
		assert.Equal(t, pref.Sensitivity, raw.Sensitivity, "the row itself stays intact for undo")
	}
	assert.EqualValues(t, 0, countPreferences(t, db, models.PreferenceCategoryHobby))

	// The ledger makes the next boot a no-op — a hobby preference created
	// after the backfill is not swept mid-window and keeps projecting.
	after := seedHobbyPreference(t, db, user.ID, contact.VCardUID, "favorite", "Skiing", "", models.RelationshipSensitivityNormal)
	second, err := MigrateHobbyPreferencesToPersonalInfo(db)
	require.NoError(t, err)
	assert.True(t, second.AlreadyDone)
	assert.Zero(t, second.PreferencesScanned)

	var stillLive models.Preference
	require.NoError(t, db.First(&stillLive, "id = ?", after.ID).Error)
	assert.Equal(t, models.PreferenceCategoryHobby, stillLive.Category)

	record := models.RecordForContact(&stored, "", db)
	foundSkiing := false
	for _, entry := range record.Card.PersonalInfo {
		if entry.Value == "Skiing" {
			foundSkiing = true
		}
	}
	assert.True(t, foundSkiing, "post-backfill hobby preferences still work through the deprecated projection")
}

// TestMigrateHobbyPreferences_MergesCaseInsensitiveDuplicatePerRules pins the
// merge table from the issue: level only when empty, stricter sensitivity,
// favorite if either, notes concatenated, existing provenance kept.
func TestMigrateHobbyPreferences_MergesCaseInsensitiveDuplicatePerRules(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db, user, contact := newHobbyBackfillFixture(t)
	oldSource := models.PreferenceSourceExternal
	oldConfidence := 0.4

	// An existing CardDAV/import entry and its metadata.
	contact.Card.PersonalInfo = []contactmodel.PersonalInfo{
		{ID: "existing-piano", Kind: "hobby", Value: "PIANO", Level: ""},
	}
	contact.CRM.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{
		"existing-piano": {
			Sensitivity: models.RelationshipSensitivityNormal,
			Favorite:    true,
			Notes:       "learned as a child",
			Source:      oldSource,
			Confidence:  &oldConfidence,
		},
	}
	require.NoError(t, db.Save(&contact).Error)

	pref := seedHobbyPreference(t, db, user.ID, contact.VCardUID, "like", "piano", models.PreferenceLevelMedium, models.RelationshipSensitivitySecret)
	pref.Notes = "practising Für Elise"
	pref.Source = models.PreferenceSourceUser
	require.NoError(t, db.Save(&pref).Error)

	stats, err := MigrateHobbyPreferencesToPersonalInfo(db)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.PersonalInfoEntriesMerged)

	stored := reloadContact(t, db, contact.ID)
	require.Len(t, stored.Card.PersonalInfo, 1, "the case-insensitive duplicate must merge, not duplicate")
	entry := stored.Card.PersonalInfo[0]
	assert.Equal(t, "existing-piano", entry.ID)
	assert.Equal(t, "PIANO", entry.Value, "the existing entry's value wins")
	assert.Equal(t, "medium", entry.Level, "level fills only when the existing entry has none")

	meta := stored.CRM.PersonalInfoMeta["existing-piano"]
	assert.Equal(t, models.RelationshipSensitivitySecret, meta.Sensitivity, "the stricter sensitivity wins")
	assert.True(t, meta.Favorite, "favorite is OR-ed")
	assert.Equal(t, "learned as a child\n\npractising Für Elise", meta.Notes, "non-empty notes are concatenated with a blank line")
	assert.Equal(t, oldSource, meta.Source, "existing provenance is kept unless empty")
	require.NotNil(t, meta.Confidence)
	assert.InDelta(t, 0.4, *meta.Confidence, 1e-9, "existing confidence is kept unless empty")

	// A second hobby with the same value and a different case in the same run
	// also merges into the same entry.
	seedHobbyPreference(t, db, user.ID, contact.VCardUID, "", "PiAnO", "low", models.RelationshipSensitivityPrivate)
	require.NoError(t, db.Exec("DELETE FROM data_backfills").Error)
	_, err = MigrateHobbyPreferencesToPersonalInfo(db)
	require.NoError(t, err)
	stored = reloadContact(t, db, contact.ID)
	require.Len(t, stored.Card.PersonalInfo, 1)
	assert.Equal(t, "medium", stored.Card.PersonalInfo[0].Level, "an existing level is never overwritten")
	assert.Equal(t, models.RelationshipSensitivitySecret, stored.CRM.PersonalInfoMeta["existing-piano"].Sensitivity,
		"secret stays stricter than private")
}

// TestMigrateHobbyPreferences_DislikeRecategorizedInPlace pins the lossless
// second half: a dislike is not a hobby, keeps every column, gains the new
// category, clears the key, and is not soft-deleted.
// A hobby preference whose value matches an entry of a DIFFERENT kind
// (expertise "Chess") is a different fact: it becomes its own hobby entry, and
// the expertise entry and its metadata are left exactly as they were. Merging
// by value alone used to fold the hobby into the expertise entry.
func TestMigrateHobbyPreferences_SameValueOtherKindIsNotMerged(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db, user, contact := newHobbyBackfillFixture(t)
	contact.Card.PersonalInfo = []contactmodel.PersonalInfo{
		{ID: "existing-chess", Kind: "expertise", Value: "Chess", Level: "high"},
	}
	contact.CRM.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{
		"existing-chess": {Notes: "club champion"},
	}
	require.NoError(t, db.Save(&contact).Error)
	seedHobbyPreference(t, db, user.ID, contact.VCardUID, "favorite", " chess ", "", models.RelationshipSensitivityNormal)

	stats, err := MigrateHobbyPreferencesToPersonalInfo(db)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.PersonalInfoEntriesMerged)

	stored := reloadContact(t, db, contact.ID)
	require.Len(t, stored.Card.PersonalInfo, 2, "a same-value entry of another kind must not absorb the hobby")
	byKind := map[string]contactmodel.PersonalInfo{}
	for _, e := range stored.Card.PersonalInfo {
		byKind[e.Kind] = e
	}
	assert.Equal(t, "existing-chess", byKind["expertise"].ID)
	assert.Equal(t, "high", byKind["expertise"].Level)
	assert.Equal(t, "club champion", stored.CRM.PersonalInfoMeta["existing-chess"].Notes)
	assert.False(t, stored.CRM.PersonalInfoMeta["existing-chess"].Favorite, "the expertise entry's metadata is untouched")

	hobby, ok := byKind["hobby"]
	require.True(t, ok, "the preference becomes its own hobby entry")
	assert.Equal(t, "chess", hobby.Value, "the moved value is trimmed")
	assert.True(t, stored.CRM.PersonalInfoMeta[hobby.ID].Favorite)
}

func TestMigrateHobbyPreferences_DislikeRecategorizedInPlace(t *testing.T) {
	logtest.Guard(t) // happy-path tests fail on unexpected warn/error logs (issue #1474)
	db, user, contact := newHobbyBackfillFixture(t)
	pref := seedHobbyPreference(t, db, user.ID, contact.VCardUID, models.PreferenceKeyDislike, "Crowded parties", models.PreferenceLevelLow, models.RelationshipSensitivityPrivate)
	pref.Notes = "leaves early"
	pref.Source = models.PreferenceSourceConversationNote
	confidence := 0.65
	pref.Confidence = &confidence
	require.NoError(t, db.Save(&pref).Error)

	stats, err := MigrateHobbyPreferencesToPersonalInfo(db)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.DislikesRecategorized)
	assert.Zero(t, stats.PreferencesSoftDeleted)

	var stored models.Preference
	require.NoError(t, db.First(&stored, "id = ?", pref.ID).Error)
	assert.Equal(t, models.PreferenceCategoryDislikeActivity, stored.Category)
	assert.Empty(t, stored.Key)
	assert.Equal(t, "Crowded parties", stored.Value)
	require.NotNil(t, stored.Level, "every other column is kept (level included)")
	assert.Equal(t, models.PreferenceLevelLow, *stored.Level)
	assert.Equal(t, "leaves early", stored.Notes)
	assert.Equal(t, models.PreferenceSourceConversationNote, stored.Source)
	require.NotNil(t, stored.Confidence)
	assert.InDelta(t, 0.65, *stored.Confidence, 1e-9)
	assert.Equal(t, models.RelationshipSensitivityPrivate, stored.Sensitivity)
	assert.False(t, stored.DeletedAt.Valid, "a recategorized dislike is still live")

	live := reloadContact(t, db, contact.ID)
	assert.Empty(t, live.Card.PersonalInfo, "a dislike never projects to a Card")
}

// TestMetaFromHobbyPreference_NoteMergeRules pins every arm of the notes
// merge: a missing existing note takes the preference's, an absent or
// identical incoming note leaves the existing one untouched (so a re-run
// cannot duplicate it), and two different notes are joined with a blank line.
func TestMetaFromHobbyPreference_NoteMergeRules(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		incoming string
		want     string
	}{
		{name: "existing empty takes incoming", existing: "", incoming: "plays weekly", want: "plays weekly"},
		{name: "incoming empty keeps existing", existing: "learned young", incoming: "", want: "learned young"},
		{name: "identical notes are not duplicated", existing: "plays weekly", incoming: "plays weekly", want: "plays weekly"},
		{name: "different notes are joined", existing: "learned young", incoming: "plays weekly", want: "learned young\n\nplays weekly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pref := &models.Preference{Notes: tc.incoming}
			existing := contactmodel.PersonalInfoMeta{Notes: tc.existing}
			got := metaFromHobbyPreference(pref, existing)
			assert.Equal(t, tc.want, got.Notes)
		})
	}
}

// TestMigrateHobbyPreferences_EdgeBranches covers the failure and no-op
// branches: nil db, a pre-000052 schema, a dangling preference, and a
// preference key the mapping cannot represent without losing data.
func TestMigrateHobbyPreferences_EdgeBranches(t *testing.T) {
	t.Run("nil db is a no-op", func(t *testing.T) {
		logtest.Guard(t)
		stats, err := MigrateHobbyPreferencesToPersonalInfo(nil)
		require.NoError(t, err)
		assert.NotNil(t, stats)
	})

	t.Run("missing ledger fails closed", func(t *testing.T) {
		logtest.Guard(t)
		db, _, _ := newHobbyBackfillFixture(t)
		require.NoError(t, db.Exec("DROP TABLE data_backfills").Error)
		_, err := MigrateHobbyPreferencesToPersonalInfo(db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "data_backfills")
	})

	t.Run("dangling preference is left untouched", func(t *testing.T) {
		rec := logtest.Capture(t)
		db, user, _ := newHobbyBackfillFixture(t)
		pref := seedHobbyPreference(t, db, user.ID, "00000000-0000-4000-8000-000000000000", "favorite", "Orphan", "", models.RelationshipSensitivityNormal)
		// The skip is deliberate and must be visible in the log -- and be the
		// only warning.
		defer rec.AssertNoWarnings(t, "hobby preference backfill: contact not found, leaving preferences untouched")
		defer func() { assert.Len(t, rec.Warnings(), 1, "the dangling preference must be logged once") }()

		stats, err := MigrateHobbyPreferencesToPersonalInfo(db)
		require.NoError(t, err)
		assert.EqualValues(t, 1, stats.PreferencesScanned)
		assert.Zero(t, stats.PreferencesSoftDeleted)

		var raw models.Preference
		require.NoError(t, db.First(&raw, "id = ?", pref.ID).Error)
		assert.Equal(t, models.PreferenceCategoryHobby, raw.Category)
	})

	t.Run("a failed ledger write is reported", func(t *testing.T) {
		logtest.Guard(t)
		db, user, contact := newHobbyBackfillFixture(t)
		seedHobbyPreference(t, db, user.ID, contact.VCardUID, "favorite", "Chess", "", models.RelationshipSensitivityNormal)
		require.NoError(t, db.Exec(`CREATE TRIGGER fail_ledger BEFORE INSERT ON data_backfills BEGIN SELECT RAISE(ABORT, 'boom'); END`).Error)

		_, err := MigrateHobbyPreferencesToPersonalInfo(db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "record completion")
	})

	t.Run("unrecognized key is left untouched rather than losing it", func(t *testing.T) {
		rec := logtest.Capture(t)
		db, user, contact := newHobbyBackfillFixture(t)
		pref := seedHobbyPreference(t, db, user.ID, contact.VCardUID, "allergy", "Pollen", "", models.RelationshipSensitivityNormal)
		defer rec.AssertNoWarnings(t, "hobby preference backfill: unrecognized key, leaving preference untouched")
		defer func() { assert.Len(t, rec.Warnings(), 1, "the unrecognized key must be logged once") }()

		stats, err := MigrateHobbyPreferencesToPersonalInfo(db)
		require.NoError(t, err)
		assert.Zero(t, stats.PreferencesSoftDeleted)

		var raw models.Preference
		require.NoError(t, db.First(&raw, "id = ?", pref.ID).Error)
		assert.Equal(t, models.PreferenceCategoryHobby, raw.Category)
		assert.Equal(t, "allergy", raw.Key)
	})
}

func countPreferences(t *testing.T, db *gorm.DB, category string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.Preference{}).Where("category = ?", category).Count(&n).Error)
	return n
}
