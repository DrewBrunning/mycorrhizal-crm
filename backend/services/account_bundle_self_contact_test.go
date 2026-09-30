package services

import (
	"encoding/json"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1375: the account bundle records which contact is the account's "Me"
// and import lands it ONTO the destination's own self contact instead of
// minting a second "Me" and leaving the pointer on a bare auto-created one.

// selfDestination creates a destination user the way production does: with the
// auto-provisioned self contact (EnsureSelfContact) that every account has.
func selfDestination(t *testing.T, db *gorm.DB, username string) (models.User, models.Contact) {
	t.Helper()
	user := bundleTestUser(t, db, username)
	require.NoError(t, EnsureSelfContact(db, &user))
	require.NotNil(t, user.SelfContactVCardUID)
	var self models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, *user.SelfContactVCardUID).First(&self).Error)
	return user, self
}

func userSelfUID(t *testing.T, db *gorm.DB, userID uint) string {
	t.Helper()
	var u models.User
	require.NoError(t, db.First(&u, userID).Error)
	require.NotNil(t, u.SelfContactVCardUID)
	return *u.SelfContactVCardUID
}

// (a) A fresh destination whose only contact is its bare auto self contact,
// importing a bundle with an enriched self contact, ends with exactly one "Me"
// holding the bundle's data, still pointed to by the destination user, with the
// notes and relationships attached to it.
func TestAccountBundle_SelfContact_FreshDestinationLandsOnOwnMe(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	photoDir := useBundlePhotoDir(t)
	owner := bundleTestUser(t, srcDB, "self-src")
	seed := seedFullAccount(t, srcDB, owner, "SELF")
	dest, bare := selfDestination(t, dstDB, "self-dst")

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	ada := seed.Contacts[0]
	require.Equal(t, ada.VCardUID, exported.SelfContactUID)

	report, _, err := ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID,
		MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	require.Empty(t, report.Issues)
	assert.Equal(t, 1, report.ContactsUpdated, "Ada lands onto the destination's Me")
	assert.Equal(t, 2, report.ContactsCreated, "Bob and Cy are new")

	var contacts []models.Contact
	require.NoError(t, dstDB.Where("user_id = ?", dest.ID).Find(&contacts).Error)
	require.Len(t, contacts, 3, "no second Me: the bare auto contact + Bob + Cy only")
	for _, c := range contacts {
		assert.NotEqual(t, ada.VCardUID, c.VCardUID, "the source's self uid is never minted locally")
	}

	assert.Equal(t, bare.VCardUID, userSelfUID(t, dstDB, dest.ID), "Me still points at the destination's own row")
	var me models.Contact
	require.NoError(t, dstDB.First(&me, bare.ID).Error)
	assert.Equal(t, "AdaSELF", me.Firstname, "Me holds the bundle's data")
	assert.Equal(t, "SELF", me.Lastname)
	assert.Equal(t, "ada@work.example", me.Email)
	assert.NotEmpty(t, me.Photo, "the bundle's Me photo lands")
	assert.FileExists(t, photoDir+"/"+me.Photo)
	assert.Equal(t, "Countess", me.Nickname)

	// Card-only data with no flat home came along too.
	rec := models.RecordForContact(&me, photoDir, dstDB)
	assert.Equal(t, "Analytical Engines", rec.Card.Organizations[0].Name)

	// Notes / relationships / life events keyed to Ada now hang off Me, and
	// nothing references the source's uid.
	var notes []models.Note
	require.NoError(t, dstDB.Where("user_id = ? AND contact_id = ?", dest.ID, me.ID).Find(&notes).Error)
	require.Len(t, notes, 1, "exactly the bundle's note: a restore writes no merge-audit note")
	assert.Equal(t, "note-SELF", notes[0].Content)

	var edges []models.RelationshipEdge
	require.NoError(t, dstDB.Where("user_id = ?", dest.ID).Find(&edges).Error)
	require.Len(t, edges, 2)
	for _, e := range edges {
		assert.True(t, e.SourceID == me.VCardUID || e.TargetID == me.VCardUID, "every edge touches Me")
		assert.NotEqual(t, ada.VCardUID, e.SourceID)
		assert.NotEqual(t, ada.VCardUID, e.TargetID)
	}
	var lifeEvents []models.LifeEvent
	require.NoError(t, dstDB.Where("user_id = ? AND entity_id = ?", dest.ID, me.VCardUID).Find(&lifeEvents).Error)
	assert.Len(t, lifeEvents, 1)

	// A re-export names the destination's own Me.
	reexported, _, err := BuildAccountBundle(dstDB, dest.ID, photoDir)
	require.NoError(t, err)
	assert.Equal(t, bare.VCardUID, reexported.SelfContactUID)
}

// (b) An attach-style destination whose Me already has its own data: merged
// (the flat merge rules, destination-only data kept), not duplicated.
func TestAccountBundle_SelfContact_MergesIntoPopulatedMe(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "selfm-src")
	seed := seedFullAccount(t, srcDB, owner, "MERGEME")
	dest, me := selfDestination(t, dstDB, "selfm-dst")

	me.Emails = []models.ContactEmail{{Type: "home", Value: "dest-only@dest.example"}}
	require.NoError(t, dstDB.Save(&me).Error)
	// The destination's Me already carries its own details.
	require.NoError(t, dstDB.Model(&me).Updates(map[string]interface{}{
		"job_title": "Dest Title", "how_we_met": "dest-how",
	}).Error)
	require.NoError(t, dstDB.Create(&models.Note{UserID: dest.ID, ContactID: &me.ID, Content: "dest-own-note"}).Error)

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	useBundlePhotoDir(t)
	_, _, err = ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID,
		MapAccountBundle(exported), nil, nil)
	require.NoError(t, err)

	var count int64
	require.NoError(t, dstDB.Model(&models.Contact{}).Where("user_id = ?", dest.ID).Count(&count).Error)
	assert.EqualValues(t, 3, count, "Me + Bob + Cy: merged, not duplicated")
	assert.Equal(t, me.VCardUID, userSelfUID(t, dstDB, dest.ID))

	var got models.Contact
	require.NoError(t, dstDB.First(&got, me.ID).Error)
	assert.Equal(t, "AdaMERGEME", got.Firstname, "incoming non-empty scalars win, like every source merge")
	assert.Equal(t, "conference MERGEME", got.HowWeMet, "incoming non-empty scalars win")
	var addrs []string
	for _, e := range got.Emails {
		addrs = append(addrs, e.Value)
	}
	assert.Contains(t, addrs, "dest-only@dest.example", "the destination's own email survives (additive merge)")
	assert.Contains(t, addrs, "ada@work.example")
	var notes []models.Note
	require.NoError(t, dstDB.Where("user_id = ? AND contact_id = ?", dest.ID, me.ID).Find(&notes).Error)
	assert.Len(t, notes, 2, "the destination's own note plus the bundle's")
}

// A destination Me with its own photo keeps it.
func TestAccountBundle_SelfContact_KeepsDestinationPhoto(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "selfp-src")
	seed := seedFullAccount(t, srcDB, owner, "KEEPPHOTO")
	dest, me := selfDestination(t, dstDB, "selfp-dst")
	require.NoError(t, dstDB.Model(&me).Updates(map[string]interface{}{"photo": "mine.jpg", "photo_thumbnail": "mine_t.jpg"}).Error)

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	useBundlePhotoDir(t)
	_, _, err = ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(exported), nil, nil)
	require.NoError(t, err)

	var got models.Contact
	require.NoError(t, dstDB.First(&got, me.ID).Error)
	assert.Equal(t, "mine.jpg", got.Photo)
}

// No destination self contact: the bundle's Me is created and becomes the Me.
func TestAccountBundle_SelfContact_CreatedWhenDestinationHasNone(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "selfn-src")
	seed := seedFullAccount(t, srcDB, owner, "NONE")
	dest := bundleTestUser(t, dstDB, "selfn-dst") // no EnsureSelfContact

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	useBundlePhotoDir(t)
	report, _, err := ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(exported), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, report.ContactsCreated)
	assert.Equal(t, seed.Contacts[0].VCardUID, userSelfUID(t, dstDB, dest.ID))
}

// A dangling pointer (Me soft-deleted) is treated as "no self contact".
func TestAccountBundle_SelfContact_DanglingPointerCreatesFresh(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "selfd-src")
	seed := seedFullAccount(t, srcDB, owner, "DANGLE")
	dest, me := selfDestination(t, dstDB, "selfd-dst")
	require.NoError(t, dstDB.Delete(&me).Error)

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	useBundlePhotoDir(t)
	_, _, err = ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(exported), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, seed.Contacts[0].VCardUID, userSelfUID(t, dstDB, dest.ID))
}

// (c) A bundle without the field keeps the old behaviour: every contact is an
// ordinary new one, the destination's Me is untouched, nothing crashes. Also a
// bundle naming a uid the file does not carry is ignored.
func TestAccountBundle_SelfContact_AbsentFieldOldBehaviour(t *testing.T) {
	for name, mutate := range map[string]func(*models.AccountBundle){
		"absent":   func(b *models.AccountBundle) { b.SelfContactUID = "" },
		"dangling": func(b *models.AccountBundle) { b.SelfContactUID = "not-in-the-bundle" },
	} {
		t.Run(name, func(t *testing.T) {
			srcDB := dbtest.New(t)
			dstDB := dbtest.New(t)
			owner := bundleTestUser(t, srcDB, "selfa-src")
			seed := seedFullAccount(t, srcDB, owner, "OLD")
			dest, me := selfDestination(t, dstDB, "selfa-dst")

			exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
			require.NoError(t, err)
			mutate(exported)
			raw, err := json.Marshal(exported)
			require.NoError(t, err)
			var reparsed models.AccountBundle
			require.NoError(t, json.Unmarshal(raw, &reparsed))

			useBundlePhotoDir(t)
			report, _, err := ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(&reparsed), nil, nil)
			require.NoError(t, err)
			assert.Equal(t, 3, report.ContactsCreated)
			assert.Equal(t, 0, report.ContactsUpdated)
			assert.Equal(t, me.VCardUID, userSelfUID(t, dstDB, dest.ID), "Me is untouched")
			var count int64
			require.NoError(t, dstDB.Model(&models.Contact{}).Where("user_id = ?", dest.ID).Count(&count).Error)
			assert.EqualValues(t, 4, count)
		})
	}
}

// (d) Re-importing the same bundle is a no-op (import_source_links ledger),
// including the self row.
func TestAccountBundle_SelfContact_ReimportIsNoOp(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "selfr-src")
	seed := seedFullAccount(t, srcDB, owner, "REIMP")
	dest, me := selfDestination(t, dstDB, "selfr-dst")
	photoDir := useBundlePhotoDir(t)

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	_, _, err = ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(exported), nil, nil)
	require.NoError(t, err)
	first, _, err := BuildAccountBundle(dstDB, dest.ID, photoDir)
	require.NoError(t, err)

	report, _, err := ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(exported), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, report.ContactsCreated)
	assert.Equal(t, 0, report.ContactsUpdated)
	second, _, err := BuildAccountBundle(dstDB, dest.ID, photoDir)
	require.NoError(t, err)
	require.JSONEq(t, bundlePlanJSON(t, first), bundlePlanJSON(t, second))
	assert.Equal(t, me.VCardUID, userSelfUID(t, dstDB, dest.ID))
}

// An explicit "skip" on the self row is honoured (the user unticked it); the
// Me pointer and row are untouched.
func TestAccountBundle_SelfContact_ExplicitSkipHonoured(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "selfs-src")
	seed := seedFullAccount(t, srcDB, owner, "SKIP")
	dest, me := selfDestination(t, dstDB, "selfs-dst")

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
	require.NoError(t, err)
	actions := map[string]SourceContactAction{
		"contact/" + seed.Contacts[0].VCardUID: {Action: SourceActionSkip},
	}
	useBundlePhotoDir(t)
	_, _, err = ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(exported), actions, nil)
	require.NoError(t, err)
	var got models.Contact
	require.NoError(t, dstDB.First(&got, me.ID).Error)
	assert.Equal(t, dest.Username, got.Firstname)
}

// The Android restore flow forwards every preview row's suggested action
// unchanged. Through the real session manager: the self row must preview as an
// "update" onto the destination's Me, and confirming the suggested actions must
// leave exactly one Me. Even a client that forces "add" on that row must not
// create a second one.
func TestMycorrhizalImportManager_SelfContactSuggestedActions(t *testing.T) {
	for _, force := range []string{"", "add"} {
		t.Run("force="+force, func(t *testing.T) {
			srcDB := dbtest.New(t)
			dstDB := dbtest.New(t)
			owner := bundleTestUser(t, srcDB, "selfmgr-src")
			seed := seedFullAccount(t, srcDB, owner, "MGRSELF")
			dest, me := selfDestination(t, dstDB, "selfmgr-dst")
			useBundlePhotoDir(t)

			exported, _, err := BuildAccountBundle(srcDB, owner.ID, seed.PhotoDir)
			require.NoError(t, err)
			data, err := json.Marshal(exported)
			require.NoError(t, err)

			mgr := NewMycorrhizalImportManager()
			log := zerolog.Nop()
			up, appErr := mgr.Upload(dest.ID, bundleMultipartHeader(t, data))
			require.Nil(t, appErr)
			require.Nil(t, mgr.StartFetch(dstDB, dest.ID, models.MycorrhizalFetchRequest{SessionID: up.SessionID}, &log))
			waitForMycorrhizalPhase(t, mgr, dest.ID, up.SessionID, models.SourceImportPhaseReady)
			preview, appErr := mgr.Preview(dest.ID, up.SessionID)
			require.Nil(t, appErr)

			actions := make([]models.RowImportAction, 0, len(preview.Rows))
			sawSelf := false
			for i, row := range preview.Rows {
				action := row.SuggestedAction
				if exported.Plan.Contacts[i].UID == exported.SelfContactUID {
					sawSelf = true
					assert.Equal(t, "update", row.SuggestedAction, "the self row merges into Me, never 'add'")
					require.NotNil(t, row.DuplicateMatch)
					assert.Equal(t, me.ID, row.DuplicateMatch.ExistingContactID)
					require.NotNil(t, row.MergeDiff)
					if force != "" {
						action = force
					}
				}
				actions = append(actions, models.RowImportAction{RowIndex: row.RowIndex, Action: action})
			}
			require.True(t, sawSelf)
			require.Nil(t, mgr.Confirm(dstDB, dest.ID, models.SourceImportConfirmRequest{SessionID: up.SessionID, Actions: actions}, &log))
			waitForMycorrhizalPhase(t, mgr, dest.ID, up.SessionID, models.SourceImportPhaseDone)

			var contacts []models.Contact
			require.NoError(t, dstDB.Where("user_id = ?", dest.ID).Find(&contacts).Error)
			assert.Len(t, contacts, 3, "Me + Bob + Cy")
			assert.Equal(t, me.VCardUID, userSelfUID(t, dstDB, dest.ID))
			var got models.Contact
			require.NoError(t, dstDB.First(&got, me.ID).Error)
			assert.Equal(t, "AdaMGRSELF", got.Firstname)
		})
	}
}

// With no destination Me the self row previews as an ordinary "add".
func TestMycorrhizalImportManager_SelfContactPreviewWithoutDestinationMe(t *testing.T) {
	srcDB := dbtest.New(t)
	dstDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "selfnp-src")
	seedFullAccount(t, srcDB, owner, "NOPREV")
	dest := bundleTestUser(t, dstDB, "selfnp-dst")

	exported, _, err := BuildAccountBundle(srcDB, owner.ID, "")
	require.NoError(t, err)
	previews := buildSourceImportPreview(dstDB, dest.ID, MapAccountBundle(exported))
	for _, p := range previews {
		assert.Equal(t, "add", p.SuggestedAction)
		assert.Nil(t, p.DuplicateMatch)
	}
}

// A self row that fails validation stays "skip" rather than becoming an update.
func TestMarkSelfContactRow_InvalidRowStaysSkip(t *testing.T) {
	db := dbtest.New(t)
	dest, _ := selfDestination(t, db, "selfv-dst")
	row := models.ImportRowPreview{SuggestedAction: "skip", ValidationErrors: []string{"bad"}}
	markSelfContactRow(db, dest.ID, &models.Contact{}, &row)
	assert.Equal(t, "skip", row.SuggestedAction)
	assert.Nil(t, row.DuplicateMatch)
}

// Export: a user with no self contact (or a pointer at a contact that is gone)
// produces a bundle without the field.
func TestBuildAccountBundle_SelfContactUID(t *testing.T) {
	db := dbtest.New(t)
	none := bundleTestUser(t, db, "selfx-none")
	require.NoError(t, db.Create(&models.Contact{UserID: none.ID, Firstname: "Solo"}).Error)
	b, _, err := BuildAccountBundle(db, none.ID, "")
	require.NoError(t, err)
	assert.Empty(t, b.SelfContactUID)
	raw, err := json.Marshal(b)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "self_contact_uid", "absent, not empty, when there is no Me")

	withMe, me := selfDestination(t, db, "selfx-me")
	b, _, err = BuildAccountBundle(db, withMe.ID, "")
	require.NoError(t, err)
	assert.Equal(t, me.VCardUID, b.SelfContactUID)

	require.NoError(t, db.Delete(&me).Error)
	b, _, err = BuildAccountBundle(db, withMe.ID, "")
	require.NoError(t, err)
	assert.Empty(t, b.SelfContactUID, "a soft-deleted Me is not named")
}
