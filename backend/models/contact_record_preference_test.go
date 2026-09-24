package models

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Preference.Level -> Card.PersonalInfo.Level projection (issue #246).
//
// Runs against database.InitDB (internal/dbtest), per CLAUDE.md trap #1: the
// hand-written migration SQL is the real schema, and AutoMigrate-based tests
// cannot see a column-name mismatch.
//
// The two vocabularies are identical (high/medium/low, the correspondence
// table's "hobby" level set), so this asserts a direct carry-through rather
// than a mapping.

func TestRecordForContact_ProjectsHobbyPreferenceLevel(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)

	user := User{Username: "preftester", Password: "password123!A", Email: "preftester@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)

	require.NoError(t, db.Create(&Preference{
		UserID: user.ID, EntityID: alice.VCardUID, Category: PreferenceCategoryHobby,
		Value: "Piano", Level: PreferenceLevelPtr(PreferenceLevelHigh),
		Sensitivity: RelationshipSensitivityNormal,
	}).Error)
	// A hobby with no level recorded: projected, but with an empty level.
	require.NoError(t, db.Create(&Preference{
		UserID: user.ID, EntityID: alice.VCardUID, Category: PreferenceCategoryHobby,
		Value: "Chess", Sensitivity: RelationshipSensitivityNormal,
	}).Error)
	// A non-hobby category never projects at all.
	require.NoError(t, db.Create(&Preference{
		UserID: user.ID, EntityID: alice.VCardUID, Category: PreferenceCategoryFood,
		Value: "Pizza", Sensitivity: RelationshipSensitivityNormal,
	}).Error)

	record := RecordForContact(&alice, "", db)

	levelsByValue := make(map[string]string, len(record.Card.PersonalInfo))
	for _, pi := range record.Card.PersonalInfo {
		assert.Equal(t, "hobby", pi.Kind)
		levelsByValue[pi.Value] = pi.Level
	}

	require.Contains(t, levelsByValue, "Piano")
	assert.Equal(t, "high", levelsByValue["Piano"], "the stored level must ride through to PersonalInfo.Level")
	require.Contains(t, levelsByValue, "Chess")
	assert.Equal(t, "", levelsByValue["Chess"], "a level-less hobby projects with an empty level")
	assert.NotContains(t, levelsByValue, "Pizza", "only hobby preferences project")
}

// TestProjectPreferences_LevelDroppedForSensitive covers the sensitivity rule
// applying to the level too: a secret hobby (however good the level) stays out
// of the default projection, and is included only under explicit opt-in.
func TestProjectPreferences_LevelDroppedForSensitive(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)

	user := User{Username: "prefsecret", Password: "password123!A", Email: "prefsecret@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)

	require.NoError(t, db.Create(&Preference{
		UserID: user.ID, EntityID: alice.VCardUID, Category: PreferenceCategoryHobby,
		Value: "Sky-diving", Level: PreferenceLevelPtr(PreferenceLevelMedium),
		Sensitivity: RelationshipSensitivitySecret,
	}).Error)

	defaultRecord := RecordForContact(&alice, "", db)
	assert.Empty(t, defaultRecord.Card.PersonalInfo, "a secret hobby must not project by default")

	all := FieldSelectionAll()
	all.IncludeSensitive = true
	optedIn := RecordForContactFiltered(&alice, "", db, all)
	require.Len(t, optedIn.Card.PersonalInfo, 1)
	assert.Equal(t, "Sky-diving", optedIn.Card.PersonalInfo[0].Value)
	assert.Equal(t, "medium", optedIn.Card.PersonalInfo[0].Level)
}

// TestProjectPreferences_LevelDoesNotDuplicateExistingEntry pins the dedupe
// behavior with a level: an imported passthrough PersonalInfo hobby with the
// same value wins, and the preference is not appended a second time.
func TestProjectPreferences_LevelDoesNotDuplicateExistingEntry(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)

	user := User{Username: "prefdedupe", Password: "password123!A", Email: "prefdedupe@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := Contact{UserID: user.ID, Firstname: "Alice"}
	// An imported hobby entry (as a CardDAV import would leave it).
	rec := RecordForContact(&alice, "", nil)
	rec.Card.PersonalInfo = []contactmodel.PersonalInfo{{Kind: "hobby", Value: "Reading"}}
	ApplyRecordToContact(&alice, rec, "")
	require.NoError(t, db.Create(&alice).Error)

	require.NoError(t, db.Create(&Preference{
		UserID: user.ID, EntityID: alice.VCardUID, Category: PreferenceCategoryHobby,
		Value: "Reading", Level: PreferenceLevelPtr(PreferenceLevelLow),
		Sensitivity: RelationshipSensitivityNormal,
	}).Error)

	record := RecordForContact(&alice, "", db)
	require.Len(t, record.Card.PersonalInfo, 1, "the existing entry must not be duplicated")
	assert.Equal(t, "Reading", record.Card.PersonalInfo[0].Value)
}
