package services

// Repair for contacts deleted by the pre-#2249 bare CardDAV delete (issue
// #2254). Real migrated schema (trap #1); the fixture reproduces the defect
// with the very statement the old DAV path ran: db.Delete(&contact).

import (
	"context"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// davDeletedFixture is a cascadeFixture whose contact was deleted with the
// pre-fix bare delete: soft-deleted, no snapshot, cascade never ran.
func davDeletedFixture(t *testing.T) cascadeFixture {
	t.Helper()
	f := newCascadeFixture(t)
	f.seed(t)
	f.seedTrashRelations(t)
	require.NoError(t, f.db.Delete(&f.contact).Error)
	return f
}

func tableCount(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table(table).Count(&n).Error)
	return n
}

// stateOf summarises every table the repair could write, so "no writes" is a
// whole-state comparison rather than a spot check.
func stateOf(t *testing.T, db *gorm.DB) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, tbl := range []string{
		"contacts", "notes", "reminders", "gifts", "relationship_edges", "circle_members", "contact_tags",
		"field_values", "activity_contacts", "contact_deletion_snapshots", "audit_events", "system_events",
		"import_source_links", "dismissed_duplicate_pairs", "notification_deliveries",
	} {
		out[tbl] = tableCount(t, db, tbl)
	}
	var live int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM notes WHERE deleted_at IS NULL").Scan(&live).Error)
	out["notes_live"] = live
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM gifts WHERE deleted_at IS NULL").Scan(&live).Error)
	out["gifts_live"] = live
	return out
}

func TestRepairDavDeleted_DryRunReportsAndWritesNothing(t *testing.T) {
	f := davDeletedFixture(t)
	before := stateOf(t, f.db)

	rep, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{})
	require.NoError(t, err)

	assert.True(t, rep.DryRun)
	require.Len(t, rep.Contacts, 1)
	c := rep.Contacts[0]
	assert.Equal(t, f.contact.ID, c.ContactID)
	assert.Equal(t, f.userID, c.UserID)
	assert.Equal(t, "cascade", c.Action)
	assert.EqualValues(t, 1, c.Soft["notes"])
	assert.EqualValues(t, 1, c.Soft["gifts"])
	assert.EqualValues(t, 1, c.Soft["reminders"])
	assert.EqualValues(t, 1, c.Hard["relationship_edges"])
	assert.EqualValues(t, 1, c.Hard["circle_members"])
	assert.EqualValues(t, 1, c.Hard["contact_tags"])
	assert.EqualValues(t, 1, c.Mutate["users"])
	assert.EqualValues(t, 1, c.Mutate["feeds"])
	assert.Positive(t, c.Leftovers())
	assert.Zero(t, rep.Cascaded)
	assert.Equal(t, before, stateOf(t, f.db), "a dry run must not write anything")
}

func TestRepairDavDeleted_ApplyCascadesAndIsIdempotent(t *testing.T) {
	f := davDeletedFixture(t)
	var deletedAt time.Time
	require.NoError(t, f.db.Unscoped().Model(&models.Contact{}).Where("id = ?", f.contact.ID).Select("deleted_at").Scan(&deletedAt).Error)
	now := time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)

	rep, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true, Now: now})
	require.NoError(t, err)
	assert.False(t, rep.DryRun)
	assert.Equal(t, 1, rep.Cascaded)
	assert.Zero(t, rep.Snapshotted)

	db := f.db
	assert.Zero(t, count(t, db, false, &models.Note{}, "contact_id = ?", f.contact.ID), "notes tombstoned")
	assert.Zero(t, count(t, db, false, &models.Gift{}, "entity_id = ?", f.contact.VCardUID))
	assert.Zero(t, count(t, db, false, &models.Reminder{}, "contact_id = ?", f.contact.ID))
	assert.Zero(t, count(t, db, true, &models.RelationshipEdge{}, "user_id = ?", f.userID), "edge hard-deleted")
	assert.Zero(t, count(t, db, true, &models.CircleMember{}, "member_vcard_uid = ?", f.contact.VCardUID))
	assert.Zero(t, count(t, db, true, &models.ContactTag{}, "contact_vcard_uid = ?", f.contact.VCardUID))
	assert.Zero(t, count(t, db, true, &models.NotificationDelivery{}, "1 = 1"))

	// Soft steps use the contact's own deleted_at, never "now".
	var note models.Note
	require.NoError(t, db.Unscoped().Where("contact_id = ?", f.contact.ID).First(&note).Error)
	assert.True(t, note.DeletedAt.Time.Equal(deletedAt), "note deleted_at %v, want contact's %v", note.DeletedAt.Time, deletedAt)
	var gift models.Gift
	require.NoError(t, db.Unscoped().Where("entity_id = ?", f.contact.VCardUID).First(&gift).Error)
	assert.True(t, gift.DeletedAt.Time.Equal(deletedAt))
	assert.True(t, note.UpdatedAt.Equal(now), "notes updated_at bumped so a sync cursor sees the tombstone")

	var user models.User
	require.NoError(t, db.First(&user, f.userID).Error)
	assert.Nil(t, user.SelfContactVCardUID)
	var feed models.Feed
	require.NoError(t, db.Where("token_hash = ?", "h1").First(&feed).Error)
	assert.NotNil(t, feed.RevokedAt)

	// The other contact's data is untouched.
	assert.EqualValues(t, 1, count(t, db, false, &models.Contact{}, "id = ?", f.other.ID))
	assert.EqualValues(t, 1, count(t, db, true, &models.ImportSourceLink{}, "entity_uid = ?", f.other.VCardUID))

	// Second run: nothing left to do, nothing written.
	after := stateOf(t, db)
	rep2, err := RepairDavDeletedContacts(context.Background(), db, DavDeleteRepairOptions{Apply: true, Now: now.Add(time.Hour)})
	require.NoError(t, err)
	assert.Zero(t, rep2.Cascaded)
	for _, c := range rep2.Contacts {
		assert.Equal(t, "none", c.Action)
		assert.Zero(t, c.Leftovers())
	}
	assert.Equal(t, after, stateOf(t, db), "a re-run is a no-op")
}

func TestRepairDavDeleted_WriteSnapshotMakesContactRestorableAndListed(t *testing.T) {
	f := davDeletedFixture(t)
	now := time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)

	rep, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true, WriteSnapshot: true, Now: now})
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Snapshotted)
	require.Len(t, rep.Contacts, 1)
	assert.Equal(t, "cascade+snapshot", rep.Contacts[0].Action)

	entries, total, err := ListTrash(f.db, f.userID, 0, 50)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, entries, 1)
	assert.Equal(t, f.contact.ID, entries[0].Contact.ID)
	assert.True(t, entries[0].DeletedAt.Equal(entries[0].Contact.DeletedAt.Time), "trashed_at is the original deletion time")

	var payload ContactDeletionPayload
	var snap models.ContactDeletionSnapshot
	require.NoError(t, f.db.Where("contact_id = ?", f.contact.ID).First(&snap).Error)
	payload, err = decodePayload(snap.Payload)
	require.NoError(t, err)
	assert.Len(t, payload.Hard["relationship_edges"], 1, "rows the repair removes are captured")
	assert.Len(t, payload.Hard["circle_members"], 1)
	assert.Len(t, payload.Hard["contact_tags"], 1)
	assert.NotEmpty(t, payload.Soft["notes"])
	assert.True(t, payload.SelfContact)

	res, err := RestoreContact(f.db, f.userID, f.contact.ID, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, res.Skipped)
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Contact{}, "id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Note{}, "contact_id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Gift{}, "entity_id = ?", f.contact.VCardUID))
	assert.EqualValues(t, 1, count(t, f.db, true, &models.RelationshipEdge{}, "user_id = ?", f.userID))
	assert.EqualValues(t, 1, count(t, f.db, true, &models.CircleMember{}, "member_vcard_uid = ?", f.contact.VCardUID))
	assert.Zero(t, count(t, f.db, true, &models.ContactDeletionSnapshot{}, "user_id = ?", f.userID), "restore consumes the snapshot")
}

// Hard rows removed before the repair were never captured; restore must
// tolerate the snapshot being short of them (the documented loss).
func TestRepairDavDeleted_RestoreToleratesLossySnapshot(t *testing.T) {
	f := davDeletedFixture(t)
	// Someone already removed the circle membership and the edge before the
	// repair ran (e.g. a manual cleanup): the snapshot cannot hold them.
	require.NoError(t, f.db.Exec("DELETE FROM circle_members WHERE user_id = ?", f.userID).Error)
	require.NoError(t, f.db.Exec("DELETE FROM relationship_edges WHERE user_id = ?", f.userID).Error)

	_, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true, WriteSnapshot: true})
	require.NoError(t, err)

	res, err := RestoreContact(f.db, f.userID, f.contact.ID, time.Now())
	require.NoError(t, err)
	assert.Empty(t, res.Skipped)
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Contact{}, "id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Note{}, "contact_id = ?", f.contact.ID), "soft children come back")
	assert.EqualValues(t, 1, count(t, f.db, true, &models.ContactTag{}, "contact_vcard_uid = ?", f.contact.VCardUID), "captured rows come back")
	assert.Zero(t, count(t, f.db, true, &models.CircleMember{}, "member_vcard_uid = ?", f.contact.VCardUID), "pre-repair losses stay lost")
	assert.Zero(t, count(t, f.db, true, &models.RelationshipEdge{}, "user_id = ?", f.userID))
}

// A contact whose cascade already ran (a REST delete from before ADR 0037, or a
// partial manual fix) still lacks a snapshot: it reports clean, and
// --write-snapshot alone makes it restorable.
func TestRepairDavDeleted_CleanSnapshotlessContact(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return DeleteContactInTx(tx, f.contact, f.userID, time.Now())
	}))

	rep, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true})
	require.NoError(t, err)
	require.Len(t, rep.Contacts, 1)
	assert.Equal(t, "none", rep.Contacts[0].Action)
	assert.Zero(t, rep.Contacts[0].Leftovers())
	assert.Zero(t, tableCount(t, f.db, "contact_deletion_snapshots"))

	rep, err = RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true, WriteSnapshot: true})
	require.NoError(t, err)
	require.Len(t, rep.Contacts, 1)
	assert.Equal(t, "snapshot", rep.Contacts[0].Action)
	assert.Zero(t, rep.Cascaded)
	assert.Equal(t, 1, rep.Snapshotted)
	_, err = RestoreContact(f.db, f.userID, f.contact.ID, time.Now())
	require.NoError(t, err)
}

func TestRepairDavDeleted_NeverTouchesSnapshotBackedOrLiveContacts(t *testing.T) {
	f := newCascadeFixture(t)
	f.seed(t)
	f.seedTrashRelations(t)
	// A normally deleted contact (snapshot-backed) and the live `other`.
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	// Give the trashed contact a stray live dependant: even so it is not a
	// candidate, because it is snapshot-backed.
	require.NoError(t, f.db.Create(&models.Gift{UserID: f.userID, EntityID: f.contact.VCardUID, Description: "stray"}).Error)
	require.NoError(t, f.db.Create(&models.Gift{UserID: f.userID, EntityID: f.other.VCardUID, Description: "live"}).Error)
	before := stateOf(t, f.db)

	rep, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true, WriteSnapshot: true})
	require.NoError(t, err)
	assert.Empty(t, rep.Contacts)
	assert.Equal(t, before, stateOf(t, f.db))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Gift{}, "entity_id = ?", f.contact.VCardUID), "stray row on a snapshot-backed contact untouched")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Gift{}, "entity_id = ?", f.other.VCardUID))
}

func TestRepairDavDeleted_ScopedPerUser(t *testing.T) {
	f := davDeletedFixture(t)
	// A second user with an identical shape of data: one affected contact and
	// one live contact carrying the same kinds of rows.
	stranger := models.User{Username: "stranger", Email: "s@example.com", Password: "x"}
	require.NoError(t, f.db.Create(&stranger).Error)
	theirs := models.Contact{UserID: stranger.ID, Firstname: "Their", Lastname: "Contact"}
	require.NoError(t, f.db.Create(&theirs).Error)
	theirLive := models.Contact{UserID: stranger.ID, Firstname: "Live", Lastname: "One"}
	require.NoError(t, f.db.Create(&theirLive).Error)
	require.NoError(t, f.db.Create(&models.Gift{UserID: stranger.ID, EntityID: theirs.VCardUID, Description: "tg"}).Error)
	require.NoError(t, f.db.Create(&models.Gift{UserID: stranger.ID, EntityID: theirLive.VCardUID, Description: "tl"}).Error)
	require.NoError(t, f.db.Delete(&theirs).Error)

	rep, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true, WriteSnapshot: true})
	require.NoError(t, err)
	require.Len(t, rep.Contacts, 2)
	assert.Equal(t, f.userID, rep.Contacts[0].UserID)
	assert.Equal(t, stranger.ID, rep.Contacts[1].UserID)

	// Each user's trash lists only their own contact.
	mine, _, err := ListTrash(f.db, f.userID, 0, 10)
	require.NoError(t, err)
	require.Len(t, mine, 1)
	assert.Equal(t, f.contact.ID, mine[0].Contact.ID)
	theirsTrash, _, err := ListTrash(f.db, stranger.ID, 0, 10)
	require.NoError(t, err)
	require.Len(t, theirsTrash, 1)
	assert.Equal(t, theirs.ID, theirsTrash[0].Contact.ID)
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Gift{}, "entity_id = ?", theirLive.VCardUID), "live contact's data untouched")

	// One operational event per user, attributed to that user.
	var evs []models.SystemEvent
	require.NoError(t, f.db.Where("component = ?", "contact_dav_delete_repair").Order("user_id").Find(&evs).Error)
	require.Len(t, evs, 2)
	for _, e := range evs {
		assert.Equal(t, models.SysEventJobCompleted, e.EventType)
		require.NotNil(t, e.UserID)
		assert.Contains(t, e.Detail, "contacts=1")
	}
}

func TestRepairDavDeleted_DoesNotWriteAuditTrail(t *testing.T) {
	f := davDeletedFixture(t)
	auditBefore := tableCount(t, f.db, "audit_events")
	eventsBefore := tableCount(t, f.db, "system_events")

	_, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true, WriteSnapshot: true})
	require.NoError(t, err)
	assert.Equal(t, auditBefore, tableCount(t, f.db, "audit_events"), "the append-only audit trail is not touched")
	assert.Equal(t, eventsBefore+1, tableCount(t, f.db, "system_events"), "the run is an operational event")
}

func TestRepairDavDeleted_WriteSnapshotRequiresApply(t *testing.T) {
	f := davDeletedFixture(t)
	_, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{WriteSnapshot: true})
	require.Error(t, err)
}

func TestRepairDavDeleted_ScanFailureAbortsTheRun(t *testing.T) {
	f := davDeletedFixture(t)
	dbtest.HideTable(t, f.db, "gifts")
	_, err := RepairDavDeletedContacts(context.Background(), f.db, DavDeleteRepairOptions{Apply: true})
	require.Error(t, err)
}

func TestRepairDavDeleted_PerContactFailureIsIsolated(t *testing.T) {
	f := davDeletedFixture(t)
	db := f.db
	// Make the cascade (not the scan) fail: a trigger aborting the gifts
	// tombstone update leaves the scan intact.
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_gift_tombstone BEFORE UPDATE OF deleted_at ON gifts
		BEGIN SELECT RAISE(ABORT, 'boom'); END`).Error)

	rep, err := RepairDavDeletedContacts(context.Background(), db, DavDeleteRepairOptions{Apply: true, WriteSnapshot: true})
	require.NoError(t, err)
	assert.Equal(t, 1, rep.Failed)
	require.Len(t, rep.Contacts, 1)
	assert.Equal(t, "failed", rep.Contacts[0].Action)
	assert.Contains(t, rep.Contacts[0].Error, "boom")
	assert.EqualValues(t, 1, count(t, db, false, &models.Note{}, "contact_id = ?", f.contact.ID), "the failed contact rolled back whole")
	assert.EqualValues(t, 1, count(t, db, true, &models.RelationshipEdge{}, "user_id = ?", f.userID))
	assert.Zero(t, tableCount(t, db, "contact_deletion_snapshots"))
	var ev models.SystemEvent
	require.NoError(t, db.Where("event_type = ?", models.SysEventJobFailed).First(&ev).Error)
	assert.Equal(t, "contact_dav_delete_repair", ev.Component)
}

func TestRepairDavDeleted_SkipsContactRestoredOrSnapshottedSinceScan(t *testing.T) {
	f := davDeletedFixture(t)
	rc := DavDeleteRepairContact{UserID: f.userID, ContactID: f.contact.ID}

	// Snapshotted since the scan.
	require.NoError(t, f.db.Create(&models.ContactDeletionSnapshot{
		UserID: f.userID, ContactID: f.contact.ID, ContactVCardUID: f.contact.VCardUID,
		TrashedAt: time.Now(), PayloadVersion: models.ContactDeletionSnapshotVersion, Payload: "{}",
	}).Error)
	before := stateOf(t, f.db)
	require.NoError(t, repairOneDavDeletedContact(f.db, rc, true, time.Now()))
	assert.Equal(t, before, stateOf(t, f.db))

	// Purged since the scan.
	require.NoError(t, f.db.Exec("DELETE FROM contact_deletion_snapshots").Error)
	require.NoError(t, f.db.Unscoped().Delete(&models.Contact{}, f.contact.ID).Error)
	before = stateOf(t, f.db)
	require.NoError(t, repairOneDavDeletedContact(f.db, rc, true, time.Now()))
	assert.Equal(t, before, stateOf(t, f.db))
}

func TestFindTrashedContactsWithoutSnapshot_ScanFailureSurfaces(t *testing.T) {
	f := davDeletedFixture(t)
	dbtest.HideTable(t, f.db, "contact_deletion_snapshots")
	_, err := FindTrashedContactsWithoutSnapshot(f.db)
	require.Error(t, err)
}
