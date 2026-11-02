package services

// Contact trash: snapshot-backed restore and purge (issue #2002, ADR 0037).
// Real migrated schema (trap #1), injected clock.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var trashNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// seedTrashRelations adds the hard-deleted join rows restore must bring back.
func (f cascadeFixture) seedTrashRelations(t *testing.T) (circleID, tagID, defID string) {
	t.Helper()
	db, uid := f.db, f.contact.VCardUID
	circle := models.Circle{UserID: f.userID, Name: "c"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: f.userID, MemberVCardUID: uid}).Error)
	tag := models.Tag{UserID: f.userID, Name: "t"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: tag.ID, UserID: f.userID, ContactVCardUID: uid}).Error)
	def := models.FieldDefinition{UserID: f.userID, Label: "L", Key: "k", Target: "contact", Type: "string"}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: def.ID, UserID: f.userID, EntityID: uid, Value: json.RawMessage(`"v"`)}).Error)
	return circle.ID, tag.ID, def.ID
}

func TestDeleteContact_WritesSnapshot(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	f.seedTrashRelations(t)

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))

	var snap models.ContactDeletionSnapshot
	require.NoError(t, f.db.Where("user_id = ? AND contact_id = ?", f.userID, f.contact.ID).First(&snap).Error)
	assert.Equal(t, f.contact.VCardUID, snap.ContactVCardUID)
	assert.Equal(t, models.ContactDeletionSnapshotVersion, snap.PayloadVersion)
	assert.True(t, snap.TrashedAt.Equal(trashNow))

	var payload ContactDeletionPayload
	require.NoError(t, json.Unmarshal([]byte(snap.Payload), &payload))
	assert.Len(t, payload.Hard["relationship_edges"], 1)
	assert.Len(t, payload.Hard["circle_members"], 1)
	assert.Len(t, payload.Hard["activity_contacts"], 1)
	assert.NotContains(t, payload.Hard, "reach_out_suggestions", "derived state is not snapshotted")
	assert.NotEmpty(t, payload.Soft["notes"])
	assert.True(t, payload.SelfContact)
}

func TestDeleteContact_RollsBackSnapshotWithTheDelete(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	dbtest.HideTable(t, f.db, "gifts")

	require.Error(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	assert.Zero(t, count(t, f.db, true, &models.ContactDeletionSnapshot{}, "user_id = ?", f.userID))
	assert.Zero(t, count(t, f.db, true, &models.Contact{}, "id = ? AND deleted_at IS NOT NULL", f.contact.ID))
}

func TestRestoreContact_RevivesExactlyTheDeletedSet(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	circleID, tagID, _ := f.seedTrashRelations(t)
	uid := f.contact.VCardUID

	// A note the user deleted BEFORE the contact must stay deleted.
	early := models.Note{UserID: f.userID, ContactID: &f.contact.ID, Content: "early", Date: trashNow}
	require.NoError(t, f.db.Create(&early).Error)
	require.NoError(t, f.db.Delete(&early).Error)

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	res, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, res.Skipped)

	assert.EqualValues(t, 1, count(t, f.db, false, &models.Contact{}, "id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Note{}, "contact_id = ? AND content = 'n'", f.contact.ID))
	assert.Zero(t, count(t, f.db, false, &models.Note{}, "content = 'early'"), "content deleted before the contact is not resurrected")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Gift{}, "entity_id = ?", uid))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Reminder{}, "contact_id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.RelationshipEdge{}, "source_id = ?", uid))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.CircleMember{}, "circle_id = ? AND member_vcard_uid = ?", circleID, uid))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactTag{}, "tag_id = ? AND contact_vcard_uid = ?", tagID, uid))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.FieldValue{}, "entity_id = ?", uid))
	var links int64
	require.NoError(t, f.db.Table("activity_contacts").Where("contact_id = ?", f.contact.ID).Count(&links).Error)
	assert.EqualValues(t, 1, links)
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ImportSourceLink{}, "entity_uid = ?", uid))

	// Derived / security state is deliberately not restored.
	assert.Zero(t, count(t, f.db, false, &models.DismissedDuplicatePair{}, "uid_low = ?", uid))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Feed{}, "entity_id = ? AND revoked_at IS NOT NULL", uid), "feed stays revoked")

	var user models.User
	require.NoError(t, f.db.First(&user, f.userID).Error)
	require.NotNil(t, user.SelfContactVCardUID)
	assert.Equal(t, uid, *user.SelfContactVCardUID)

	assert.Zero(t, count(t, f.db, true, &models.ContactDeletionSnapshot{}, "user_id = ?", f.userID), "snapshot consumed")

	var after models.Contact
	require.NoError(t, f.db.First(&after, f.contact.ID).Error)
	assert.Greater(t, after.Revision, f.contact.Revision, "restore bumps the revision so ETag/CardDAV clients see the change")
}

func TestRestoreContact_SkipsEdgeWhoseOtherEndpointIsGone(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	require.NoError(t, DeleteContact(f.db, f.other, f.userID, trashNow))

	res, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow)
	require.NoError(t, err)
	assert.Zero(t, count(t, f.db, false, &models.RelationshipEdge{}, "source_id = ?", f.contact.VCardUID))
	require.Len(t, res.Skipped, 1)
	assert.Equal(t, "relationship_edges", res.Skipped[0].Table)
	assert.Equal(t, 1, res.Skipped[0].Count)
}

func TestRestoreContact_UIDInUseIs409Equivalent(t *testing.T) {
	f := newCascadeFixture(t)
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	live := models.Contact{UserID: f.userID, VCardUID: f.contact.VCardUID, Firstname: "Reborn"}
	require.NoError(t, f.db.Create(&live).Error)

	_, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow)
	var inUse *ContactUIDInUseError
	require.ErrorAs(t, err, &inUse)
	assert.Equal(t, live.ID, inUse.LiveContactID)
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ? AND deleted_at IS NOT NULL", f.contact.ID), "still trashed")
	assert.EqualValues(t, 1, count(t, f.db, true, &models.ContactDeletionSnapshot{}, "contact_id = ?", f.contact.ID), "snapshot kept")
}

func TestRestoreContact_NotFoundCases(t *testing.T) {
	f := newCascadeFixture(t)
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))

	_, err := RestoreContact(f.db, f.userID+99, f.contact.ID, trashNow)
	assert.ErrorIs(t, err, ErrContactNotInTrash, "another user's trashed contact")
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ? AND deleted_at IS NOT NULL", f.contact.ID), "no side effect")

	_, err = RestoreContact(f.db, f.userID, f.other.ID, trashNow)
	assert.ErrorIs(t, err, ErrContactNotInTrash, "a live contact is not in the trash")
}

func TestRestoreContact_UnsupportedSnapshotVersion(t *testing.T) {
	f := newCascadeFixture(t)
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	require.NoError(t, f.db.Model(&models.ContactDeletionSnapshot{}).Where("contact_id = ?", f.contact.ID).
		Update("payload_version", 99).Error)

	_, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow)
	assert.ErrorIs(t, err, ErrSnapshotUnsupported)
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ? AND deleted_at IS NOT NULL", f.contact.ID))
}

func TestDeleteContact_RetainsSyncLinkAsTombstone(t *testing.T) {
	f := newCascadeFixture(t)
	sub := models.ContactSubscription{UserID: f.userID, Name: "s", URL: "https://x.example"}
	require.NoError(t, f.db.Create(&sub).Error)
	require.NoError(t, f.db.Create(&models.ContactSyncLink{SubscriptionID: sub.ID, UserID: f.userID, Href: "/a.vcf", ContactID: f.contact.ID, ContentHash: "h"}).Error)

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactSyncLink{}, "contact_id = ?", f.contact.ID),
		"the link survives so a later pull cannot re-import the deleted contact")
}

func TestListTrash_OnlySnapshotBackedOwnerScoped(t *testing.T) {
	f := newCascadeFixture(t)
	// A soft-deleted contact with no snapshot (pre-migration delete or merge loser) is not listed.
	ghost := models.Contact{UserID: f.userID, Firstname: "Ghost"}
	require.NoError(t, f.db.Create(&ghost).Error)
	require.NoError(t, f.db.Delete(&ghost).Error)
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))

	entries, total, err := ListTrash(f.db, f.userID, 0, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, f.contact.ID, entries[0].Contact.ID)
	assert.True(t, entries[0].DeletedAt.Equal(trashNow))

	other, total, err := ListTrash(f.db, f.userID+1, 0, 50)
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, other)
}

func TestPurgeContact_RemovesEverythingAndReportsFiles(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", f.contact.ID).Update("photo", "p.jpg").Error)
	require.NoError(t, f.db.Create(&models.Attachment{UserID: f.userID, ContactVCardUID: f.contact.VCardUID, StoredName: "a-uuid", OriginalName: "a", ContentType: "x/y", SizeBytes: 1}).Error)
	var contact models.Contact
	require.NoError(t, f.db.First(&contact, f.contact.ID).Error)
	require.NoError(t, DeleteContact(f.db, contact, f.userID, trashNow))

	files, err := PurgeContact(f.db, f.userID, f.contact.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"p.jpg"}, files.Photos)
	assert.Equal(t, []string{"a-uuid"}, files.Attachments)

	assert.Zero(t, count(t, f.db, true, &models.Contact{}, "id = ?", f.contact.ID))
	assert.Zero(t, count(t, f.db, true, &models.Note{}, "contact_id = ?", f.contact.ID))
	assert.Zero(t, count(t, f.db, true, &models.Gift{}, "entity_id = ?", f.contact.VCardUID))
	assert.Zero(t, count(t, f.db, true, &models.Attachment{}, "contact_vcard_uid = ?", f.contact.VCardUID))
	assert.Zero(t, count(t, f.db, true, &models.ContactDeletionSnapshot{}, "contact_id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ?", f.other.ID), "other contacts untouched")

	_, err = PurgeContact(f.db, f.userID, f.contact.ID)
	assert.ErrorIs(t, err, ErrContactNotInTrash)
}

func TestRestoreContact_AfterPurgeIsNotFoundAndTheUIDIsFree(t *testing.T) {
	f := newCascadeFixture(t)
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	_, err := PurgeContact(f.db, f.userID, f.contact.ID)
	require.NoError(t, err)

	// A purged contact has no snapshot, so restore cannot resurrect it.
	_, err = RestoreContact(f.db, f.userID, f.contact.ID, trashNow)
	assert.ErrorIs(t, err, ErrContactNotInTrash)
	assert.Zero(t, count(t, f.db, true, &models.Contact{}, "id = ?", f.contact.ID))

	// The UID is free after a purge; reusing it must not make the old row restorable.
	require.NoError(t, f.db.Create(&models.Contact{UserID: f.userID, VCardUID: f.contact.VCardUID, Firstname: "Reused"}).Error)
	_, err = RestoreContact(f.db, f.userID, f.contact.ID, trashNow)
	assert.ErrorIs(t, err, ErrContactNotInTrash)
}

func TestPurgeContact_CrossUserIsNotFoundWithNoSideEffect(t *testing.T) {
	f := newCascadeFixture(t)
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	_, err := PurgeContact(f.db, f.userID+99, f.contact.ID)
	assert.ErrorIs(t, err, ErrContactNotInTrash)
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, f.db, true, &models.ContactDeletionSnapshot{}, "contact_id = ?", f.contact.ID))
}

func TestEmptyTrash_PurgesOnlyTheCallersTrash(t *testing.T) {
	f := newCascadeFixture(t)
	otherUser := models.User{Username: "u2", Email: "u2@example.com", Password: "x"}
	require.NoError(t, f.db.Create(&otherUser).Error)
	theirs := models.Contact{UserID: otherUser.ID, Firstname: "Theirs"}
	require.NoError(t, f.db.Create(&theirs).Error)
	require.NoError(t, DeleteContact(f.db, theirs, otherUser.ID, trashNow))
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	require.NoError(t, DeleteContact(f.db, f.other, f.userID, trashNow))

	files, n, err := EmptyTrash(f.db, f.userID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.NotNil(t, files)
	assert.Zero(t, count(t, f.db, true, &models.Contact{}, "user_id = ?", f.userID))
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ?", theirs.ID))
	assert.EqualValues(t, 1, count(t, f.db, true, &models.ContactDeletionSnapshot{}, "user_id = ?", otherUser.ID))
}

func TestContactDeletionPayload_StepsAreSnapshottedOrSkippedWithReason(t *testing.T) {
	for _, s := range ContactCascadeRegistry() {
		if s.Mode != CascadeHard {
			continue
		}
		_, restorable := snapshotRestorers[s.Table]
		if restorable {
			assert.Empty(t, s.SnapshotSkip, "%s is restored, so it must not also be marked SnapshotSkip", s.Table)
			continue
		}
		assert.NotEmpty(t, s.SnapshotSkip, "%s is a hard step with no restorer: restore it or record why not (ADR 0037 decision 9)", s.Table)
	}
}

var _ = errors.Is
var _ *gorm.DB

// syncedTrashFixture links one CardDAV object to a contact via the real pull.
func syncedTrashFixture(t *testing.T) (db *gorm.DB, user models.User, sub *models.ContactSubscription, href string, contact models.Contact) {
	t.Helper()
	db = setupContactSyncTestDB(t)
	user = createContactSyncTestUser(t, db)
	sub = newContactTestSubscription(t, db, contactSyncTestConfig(), user.ID, "https://example.com/addressbooks/test/", "", "")
	href = "/addressbooks/test/zed.vcf"
	obj := carddav.AddressObject{Path: href, ETag: "\"e1\"", Card: testCard(t, "zed-uid", "Zed", "Trash", "zed@example.com")}
	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{obj}, nil, false, "")
	require.NoError(t, err)
	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, href).First(&link).Error)
	require.NoError(t, db.First(&contact, link.ContactID).Error)
	return
}

func changedZed(t *testing.T, href, email string) []carddav.AddressObject {
	return []carddav.AddressObject{{Path: href, ETag: "\"e-" + email + "\"", Card: testCard(t, "zed-uid", "Zed", "Trash", email)}}
}

func TestSyncPull_TrashedContactIsNotReimported(t *testing.T) {
	db, user, sub, href, contact := syncedTrashFixture(t)
	require.NoError(t, DeleteContact(db, contact, user.ID, trashNow))

	stats, err := reconcileContactSync(db, sub, changedZed(t, href, "zed2@example.com"), nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{Skipped: 1}, stats)
	assert.EqualValues(t, 1, count(t, db, true, &models.Contact{}, "user_id = ?", user.ID), "no duplicate was imported")
	assert.EqualValues(t, 1, count(t, db, false, &models.ContactSyncLink{}, "href = ?", href), "link kept as the tombstone")

	// Restore re-enters sync as the same linked object: the next change updates it.
	_, err = RestoreContact(db, user.ID, contact.ID, trashNow)
	require.NoError(t, err)
	stats, err = reconcileContactSync(db, sub, changedZed(t, href, "zed3@example.com"), nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{Updated: 1}, stats)
	assert.EqualValues(t, 1, count(t, db, false, &models.Contact{}, "user_id = ?", user.ID))
}

func TestSyncPull_PurgedContactIsNotReimported(t *testing.T) {
	db, user, sub, href, contact := syncedTrashFixture(t)
	require.NoError(t, DeleteContact(db, contact, user.ID, trashNow))
	_, err := PurgeContact(db, user.ID, contact.ID)
	require.NoError(t, err)

	stats, err := reconcileContactSync(db, sub, changedZed(t, href, "zed4@example.com"), nil, false, "")
	require.NoError(t, err)
	assert.Equal(t, ContactSyncStats{Skipped: 1}, stats)
	assert.Zero(t, count(t, db, true, &models.Contact{}, "user_id = ?", user.ID), "purged contact stays purged")
	assert.EqualValues(t, 1, count(t, db, false, &models.ContactSyncLink{}, "href = ?", href), "tombstone survives the purge")
}

func TestScheduledPurge_KeepsSyncLinkRemovesSnapshotAndFiles(t *testing.T) {
	db, user, _, href, contact := syncedTrashFixture(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.jpg"), []byte("x"), 0o600))
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", contact.ID).Update("photo", "p.jpg").Error)
	require.NoError(t, db.First(&contact, contact.ID).Error)
	require.NoError(t, DeleteContact(db, contact, user.ID, trashNow))
	softDeleteAt(t, db, &models.Contact{}, contact.ID, time.Now().AddDate(0, 0, -60))

	require.NoError(t, PurgeSoftDeletedRows(db, config.Config{DeleteRetentionDays: 30, ProfilePhotoDir: dir}))

	assert.Zero(t, count(t, db, true, &models.Contact{}, "id = ?", contact.ID))
	assert.Zero(t, count(t, db, true, &models.ContactDeletionSnapshot{}, "contact_id = ?", contact.ID))
	assert.EqualValues(t, 1, count(t, db, false, &models.ContactSyncLink{}, "href = ?", href))
	_, err := os.Stat(filepath.Join(dir, "p.jpg"))
	assert.True(t, os.IsNotExist(err), "photo file removed after the purge committed")
}

func TestContactUIDInUseError_Message(t *testing.T) {
	err := &ContactUIDInUseError{LiveContactID: 42}
	assert.Equal(t, "a live contact (id 42) already uses this contact's UID", err.Error())
}

func TestNormalizeRow_BytesBecomeStrings(t *testing.T) {
	row := normalizeRow(map[string]any{"blob": []byte("abc"), "text": "x", "n": 1})
	assert.Equal(t, "abc", row["blob"])
	assert.Equal(t, "x", row["text"])
	assert.Equal(t, 1, row["n"])
}

func TestDecodePayload_RejectsMalformedJSON(t *testing.T) {
	_, err := decodePayload("{not json")
	assert.Error(t, err)
}

func TestListTrash_SurfacesQueryFailure(t *testing.T) {
	db, userID := newPurgeDB(t)
	require.NoError(t, db.Exec("DROP TABLE contact_deletion_snapshots").Error)

	_, _, err := ListTrash(db, userID, 0, 10)
	assert.Error(t, err)
}

func TestPurgedFiles_MergeCarriesImageFiles(t *testing.T) {
	var files PurgedFiles
	files.merge(PurgedFiles{Images: []ImageFile{{UserID: 1, Name: "a.jpg"}}})
	files.Images = appendImageFiles(files.Images, 2, "b.jpg", "", "b_t.jpg")
	require.Len(t, files.Images, 3)
	assert.Equal(t, ImageFile{UserID: 2, Name: "b_t.jpg"}, files.Images[2])
}
