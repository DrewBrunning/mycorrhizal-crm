package models

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnsurePersonalInfoIDs_AssignsStableIDsAndPrunesOrphanMeta pins the write
// contract from issue #1738: every personalInfo entry gets an ID exactly once,
// and a metadata key whose entry no longer exists is dropped. Saving twice
// must not churn IDs (metadata is keyed by them).
func TestEnsurePersonalInfoIDs_AssignsStableIDsAndPrunesOrphanMeta(t *testing.T) {
	db := dbtest.New(t)
	user := User{Username: "piids", Password: "password123!A", Email: "piids@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Ada"}
	contact.Card.PersonalInfo = []contactmodel.PersonalInfo{
		{ID: "kept-id", Kind: "hobby", Value: "Pottery"},
		{Kind: "interest", Value: "Astronomy"},
	}
	contact.CRM.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{
		"kept-id": {Sensitivity: RelationshipSensitivityPrivate, Favorite: true},
		"orphan":  {Sensitivity: RelationshipSensitivitySecret},
	}
	require.NoError(t, db.Create(&contact).Error)

	first := contact.Card.PersonalInfo
	require.Len(t, first, 2)
	assert.Equal(t, "kept-id", first[0].ID)
	require.NotEmpty(t, first[1].ID, "an entry without an ID must get one on write")
	assert.NotEqual(t, first[0].ID, first[1].ID)

	// The metadata for the surviving entry is intact; the orphan key is gone.
	require.Contains(t, contact.CRM.PersonalInfoMeta, "kept-id")
	assert.Equal(t, RelationshipSensitivityPrivate, contact.CRM.PersonalInfoMeta["kept-id"].Sensitivity)
	assert.NotContains(t, contact.CRM.PersonalInfoMeta, "orphan")

	// A second save keeps the same IDs.
	require.NoError(t, db.Save(&contact).Error)
	assert.Equal(t, first[0].ID, contact.Card.PersonalInfo[0].ID)
	assert.Equal(t, first[1].ID, contact.Card.PersonalInfo[1].ID)

	var stored Contact
	require.NoError(t, db.First(&stored, contact.ID).Error)
	require.Len(t, stored.Card.PersonalInfo, 2)
	assert.Equal(t, first[1].ID, stored.Card.PersonalInfo[1].ID)
	assert.Equal(t, map[string]contactmodel.PersonalInfoMeta{
		"kept-id": {Sensitivity: RelationshipSensitivityPrivate, Favorite: true},
	}, stored.CRM.PersonalInfoMeta)
}

// TestApplyRecordToContact_PreservesPersonalInfoMetaWhenAbsent pins the
// compatibility half of the 1.x window: an older client that rebuilds crm
// without personalInfoMeta (it does not know the field) must not wipe the
// hobby metadata. An explicit empty map is how a newer writer clears it.
func TestApplyRecordToContact_PreservesPersonalInfoMetaWhenAbsent(t *testing.T) {
	contact := Contact{Firstname: "Ada"}
	contact.Card.PersonalInfo = []contactmodel.PersonalInfo{{ID: "pi1", Kind: "hobby", Value: "Chess"}}
	contact.CRM.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{
		"pi1": {Sensitivity: RelationshipSensitivityPrivate, Notes: "kept"},
	}

	record := &contactmodel.Record{Card: contact.Card} // crm absent, as an old client sends
	ApplyRecordToContact(&contact, record, "")
	assert.Equal(t, map[string]contactmodel.PersonalInfoMeta{
		"pi1": {Sensitivity: RelationshipSensitivityPrivate, Notes: "kept"},
	}, contact.CRM.PersonalInfoMeta, "absent metadata must preserve the stored map")

	// An explicit empty object is a deliberate clear.
	record.Envelope.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{}
	ApplyRecordToContact(&contact, record, "")
	assert.Empty(t, contact.CRM.PersonalInfoMeta)
}

// TestRecordForContactFiltered_PersonalInfoSensitivity pins the outward/owner
// split for per-entry hobby metadata (issue #1738). The owner's own read
// (nil FieldSelection) keeps everything; every outward projection (CardDAV's
// RecordForContactForSync and an export/share FieldSelection) withholds
// private/secret entries and their metadata.
func TestRecordForContactFiltered_PersonalInfoSensitivity(t *testing.T) {
	db := dbtest.New(t)
	user := User{Username: "pifilter", Password: "password123!A", Email: "pifilter@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Ada"}
	contact.Card.PersonalInfo = []contactmodel.PersonalInfo{
		{ID: "normal", Kind: "hobby", Value: "Chess"},
		{ID: "private", Kind: "hobby", Value: "Birdwatching"},
		{ID: "secret", Kind: "hobby", Value: "Lockpicking"},
	}
	contact.CRM.PersonalInfoMeta = map[string]contactmodel.PersonalInfoMeta{
		"normal":  {Favorite: true, Notes: "plays weekly"},
		"private": {Sensitivity: RelationshipSensitivityPrivate, Notes: "secret spot"},
		"secret":  {Sensitivity: RelationshipSensitivitySecret},
	}
	require.NoError(t, db.Create(&contact).Error)

	values := func(rec *contactmodel.Record) []string {
		out := make([]string, len(rec.Card.PersonalInfo))
		for i, entry := range rec.Card.PersonalInfo {
			out[i] = entry.Value
		}
		return out
	}

	owner := RecordForContact(&contact, "", db)
	assert.ElementsMatch(t, []string{"Chess", "Birdwatching", "Lockpicking"}, values(owner),
		"the owner's own read keeps every entry")
	assert.Contains(t, owner.Envelope.PersonalInfoMeta, "private")
	assert.Contains(t, owner.Envelope.PersonalInfoMeta, "secret")

	syncView := RecordForContactForSync(&contact, "", db)
	assert.Equal(t, []string{"Chess"}, values(syncView),
		"CardDAV serves only normal personalInfo entries")
	assert.Equal(t, map[string]contactmodel.PersonalInfoMeta{
		"normal": {Favorite: true, Notes: "plays weekly"},
	}, syncView.Envelope.PersonalInfoMeta,
		"metadata for withheld entries must not travel either")

	// A plain export/share selection with no opt-in behaves the same.
	outward := RecordForContactFiltered(&contact, "", db, FieldSelectionAll())
	assert.Equal(t, []string{"Chess"}, values(outward))

	// The explicit include_sensitive override restores the entries.
	all := FieldSelectionAll()
	all.IncludeSensitive = true
	optedIn := RecordForContactFiltered(&contact, "", db, all)
	assert.ElementsMatch(t, []string{"Chess", "Birdwatching", "Lockpicking"}, values(optedIn))
	assert.Contains(t, optedIn.Envelope.PersonalInfoMeta, "secret")
}

// TestMergePersonalInfoFromRemote pins the inbound-sync half of issue #1738:
// a remote full-overwrite card never saw private/secret entries, so their
// absence is not a delete; metadata for entries the remote still has is
// carried forward; metadata for entries that are truly gone is dropped.
func TestMergePersonalInfoFromRemote(t *testing.T) {
	local := []contactmodel.PersonalInfo{
		{ID: "normal", Kind: "hobby", Value: "Chess", Level: "high"},
		{ID: "private", Kind: "hobby", Value: "Birdwatching", Level: "low"},
		{ID: "deleted", Kind: "hobby", Value: "Old Hobby"},
	}
	meta := map[string]contactmodel.PersonalInfoMeta{
		"normal":  {Favorite: true, Notes: "weekly"},
		"private": {Sensitivity: RelationshipSensitivityPrivate, Notes: "hidden"},
		"deleted": {Sensitivity: RelationshipSensitivityNormal, Notes: "gone"},
	}

	t.Run("sensitive entry absent remotely is re-attached, normal absence is a delete", func(t *testing.T) {
		incoming := []contactmodel.PersonalInfo{{ID: "normal", Kind: "hobby", Value: "Chess", Level: "medium"}}
		got, gotMeta := MergePersonalInfoFromRemote(local, meta, incoming)

		require.Len(t, got, 2)
		assert.Equal(t, "Chess", got[0].Value)
		assert.Equal(t, "medium", got[0].Level, "remote content wins for entries it can see")
		assert.Equal(t, "Birdwatching", got[1].Value, "private entry is re-attached")
		assert.Equal(t, "low", got[1].Level)

		assert.Equal(t, map[string]contactmodel.PersonalInfoMeta{
			"normal":  {Favorite: true, Notes: "weekly"},
			"private": {Sensitivity: RelationshipSensitivityPrivate, Notes: "hidden"},
		}, gotMeta, "metadata follows its entry; the deleted entry's metadata is gone")
	})

	t.Run("content match without an ID inherits the local ID and metadata", func(t *testing.T) {
		incoming := []contactmodel.PersonalInfo{
			{Kind: "hobby", Value: "chess"},
			{Kind: "hobby", Value: "birdwatching"},
		}
		got, gotMeta := MergePersonalInfoFromRemote(local, meta, incoming)

		require.Len(t, got, 2, "case-insensitive matching must not duplicate entries")
		assert.Equal(t, "normal", got[0].ID, "the matching local ID keeps the metadata key stable")
		assert.Equal(t, "private", got[1].ID)
		assert.Equal(t, map[string]contactmodel.PersonalInfoMeta{
			"normal":  {Favorite: true, Notes: "weekly"},
			"private": {Sensitivity: RelationshipSensitivityPrivate, Notes: "hidden"},
		}, gotMeta)
	})

	t.Run("no metadata is a pass-through", func(t *testing.T) {
		incoming := []contactmodel.PersonalInfo{{Kind: "hobby", Value: "Skiing"}}
		got, gotMeta := MergePersonalInfoFromRemote(local, nil, incoming)
		assert.Equal(t, incoming, got)
		assert.Nil(t, gotMeta)
	})

	// A local entry with no value has no content key to match on. It must not
	// fall through to "the first entry with the same (empty) value" — that
	// would overwrite an unrelated incoming entry's ID and drop the local,
	// metadata-bearing entry.
	t.Run("an entry with no value never content-matches", func(t *testing.T) {
		blankLocal := []contactmodel.PersonalInfo{{ID: "blank", Kind: "hobby"}}
		blankMeta := map[string]contactmodel.PersonalInfoMeta{
			"blank": {Sensitivity: RelationshipSensitivityPrivate},
		}
		incoming := []contactmodel.PersonalInfo{
			{ID: "a", Kind: "hobby", Value: "Chess"},
			{ID: "b", Kind: "hobby", Value: "Go"},
		}

		got, gotMeta := MergePersonalInfoFromRemote(blankLocal, blankMeta, incoming)

		require.Len(t, got, 3, "the value-less local entry is re-attached, not matched")
		assert.Equal(t, "a", got[0].ID)
		assert.Equal(t, "b", got[1].ID)
		assert.Equal(t, "blank", got[2].ID)
		assert.Equal(t, map[string]contactmodel.PersonalInfoMeta{
			"blank": {Sensitivity: RelationshipSensitivityPrivate},
		}, gotMeta)
	})
}

// TestProjectPreferences_SkipsDislike is the projectPreferences half of the
// issue #1738 bug fix: a dislike carried in the deprecated hobby category must
// never be exported as a HOBBY, even before/without the startup backfill.
func TestProjectPreferences_SkipsDislike(t *testing.T) {
	db := dbtest.New(t)
	user := User{Username: "pidislike", Password: "password123!A", Email: "pidislike@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	require.NoError(t, db.Create(&Preference{
		UserID: user.ID, EntityID: contact.VCardUID, Category: PreferenceCategoryHobby,
		Key: PreferenceKeyDislike, Value: "Crowded parties",
		Sensitivity: RelationshipSensitivityNormal,
	}).Error)
	require.NoError(t, db.Create(&Preference{
		UserID: user.ID, EntityID: contact.VCardUID, Category: PreferenceCategoryHobby,
		Key: "favorite", Value: "Chess",
		Sensitivity: RelationshipSensitivityNormal,
	}).Error)

	record := RecordForContact(&contact, "", db)
	require.Len(t, record.Card.PersonalInfo, 1)
	assert.Equal(t, "Chess", record.Card.PersonalInfo[0].Value,
		"only the real hobby projects; the dislike does not")
}
