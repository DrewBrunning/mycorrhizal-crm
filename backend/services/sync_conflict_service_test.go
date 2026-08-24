package services

import (
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// --- snapshot / diff primitives ---------------------------------------------

func TestSyncConflictFieldSnapshot_NormalizesEmptySlices(t *testing.T) {
	c := models.Contact{Firstname: "Ada", Lastname: "Lovelace"}
	snap := syncConflictFieldSnapshot(&c)

	assert.Equal(t, "Ada", snap[models.SyncConflictFieldFirstname])
	assert.Equal(t, "Lovelace", snap[models.SyncConflictFieldLastname])
	// An empty (nil) array must encode canonically as `[]`, never `null`, so
	// it compares equal to a `[]`-loaded JSON column.
	assert.Equal(t, "[]", snap[models.SyncConflictFieldEmail])
	assert.Equal(t, "[]", snap[models.SyncConflictFieldPhone])
	assert.Equal(t, "[]", snap[models.SyncConflictFieldCircles])
	assert.Equal(t, "", snap[models.SyncConflictFieldBirthday])
}

func TestDiffSyncConflictFields_DetectsOverwrittenLocalEdit(t *testing.T) {
	local := map[string]string{models.SyncConflictFieldPhone: `[{"value":"555-0100"}]`}
	synced := map[string]string{models.SyncConflictFieldPhone: "[]"}
	remote := map[string]string{models.SyncConflictFieldPhone: "[]"}

	conflicts := diffSyncConflictFields(local, synced, remote)
	require.Len(t, conflicts, 1)
	assert.Equal(t, models.SyncConflictFieldPhone, conflicts[0].Field)
	assert.Equal(t, `[{"value":"555-0100"}]`, conflicts[0].LocalValue)
	assert.Equal(t, "[]", conflicts[0].RemoteValue)
	assert.Equal(t, models.SyncConflictStatusPending, conflicts[0].Status)
}

func TestDiffSyncConflictFields_NoConflictOnPlainRemoteChange(t *testing.T) {
	// No local edit (local == synced): a remote change is applied silently.
	local := map[string]string{models.SyncConflictFieldEmail: "[]"}
	synced := map[string]string{models.SyncConflictFieldEmail: "[]"}
	remote := map[string]string{models.SyncConflictFieldEmail: `[{"value":"new@example.com"}]`}

	assert.Empty(t, diffSyncConflictFields(local, synced, remote))
}

func TestDiffSyncConflictFields_NoConflictWhenRemoteMatchesLocal(t *testing.T) {
	// The user edited a field to the same value the remote now carries: the
	// edit survives, nothing is lost, no notice needed.
	local := map[string]string{models.SyncConflictFieldPhone: `[{"value":"555-0100"}]`}
	synced := map[string]string{models.SyncConflictFieldPhone: "[]"}
	remote := map[string]string{models.SyncConflictFieldPhone: `[{"value":"555-0100"}]`}

	assert.Empty(t, diffSyncConflictFields(local, synced, remote))
}

// --- reconcileContactSync integration ----------------------------------------

// TestReconcileContactSyncRecordsConflictOnLocalEditOverwrite is the issue
// #395 end-to-end detection check: a local edit made through the array path
// (the modern editing surface), followed by a remote change to an unrelated
// field, must produce a ContactSyncConflict naming the field and both values
// instead of losing the edit silently.
func TestReconcileContactSyncRecordsConflictOnLocalEditOverwrite(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	href := "/addressbooks/test/carol.vcf"
	first := carddav.AddressObject{Path: href, ETag: "\"etag-1\"", Card: testCard(t, "carol-uid", "Carol", "Danvers", "carol@example.com")}
	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{first}, nil, false, "")
	require.NoError(t, err)

	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, href).First(&link).Error)

	// Local edit through the array path (the same shape the nested REST API
	// writes): a phone the remote vCard never carries.
	var editTarget models.Contact
	require.NoError(t, db.First(&editTarget, link.ContactID).Error)
	editTarget.Phones = []models.ContactPhone{{Value: "555-0100"}}
	editTarget.JobTitle = "Local Edit"
	require.NoError(t, db.Save(&editTarget).Error)

	// A remote change to an unrelated field (email) triggers a sync.
	second := carddav.AddressObject{Path: href, ETag: "\"etag-2\"", Card: testCard(t, "carol-uid", "Carol", "Danvers", "carol.new@example.com")}
	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{second}, nil, false, "")
	require.NoError(t, err)
	require.Equal(t, ContactSyncStats{Updated: 1}, stats)

	var conflicts []models.ContactSyncConflict
	require.NoError(t, db.Where("user_id = ? AND contact_id = ?", user.ID, link.ContactID).Order("field ASC").Find(&conflicts).Error)
	require.Len(t, conflicts, 2, "phone + job_title were both local edits that got discarded")

	fields := map[string]models.ContactSyncConflict{}
	for _, conflict := range conflicts {
		fields[conflict.Field] = conflict
	}

	phone := fields[models.SyncConflictFieldPhone]
	assert.Equal(t, `[{"type":"","value":"555-0100"}]`, phone.LocalValue)
	assert.Equal(t, "[]", phone.RemoteValue)

	jobTitle := fields[models.SyncConflictFieldJobTitle]
	assert.Equal(t, "Local Edit", jobTitle.LocalValue)
	assert.Equal(t, "", jobTitle.RemoteValue)

	assert.Equal(t, sub.ID, conflicts[0].SubscriptionID)
	assert.Equal(t, models.SyncConflictStatusPending, conflicts[0].Status)

	// A re-sync of identical remote content is a no-op skip (content hash
	// unchanged), so the conflict is not spammy.
	stats, err = reconcileContactSync(db, sub, []carddav.AddressObject{second}, nil, false, "")
	require.NoError(t, err)
	require.Equal(t, ContactSyncStats{Skipped: 1}, stats)
}

// TestReconcileContactSyncNoConflictOnPlainRemoteChange pins that a sync
// touching only fields the user never edited surfaces no conflict.
func TestReconcileContactSyncNoConflictOnPlainRemoteChange(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	href := "/addressbooks/test/dave.vcf"
	first := carddav.AddressObject{Path: href, ETag: "\"etag-1\"", Card: testCard(t, "dave-uid", "Dave", "Grohl", "dave@example.com")}
	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{first}, nil, false, "")
	require.NoError(t, err)

	second := carddav.AddressObject{Path: href, ETag: "\"etag-2\"", Card: testCard(t, "dave-uid", "Dave", "Grohl", "dave.new@example.com")}
	_, err = reconcileContactSync(db, sub, []carddav.AddressObject{second}, nil, false, "")
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&models.ContactSyncConflict{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Zero(t, count, "a remote-only change must not be reported as a conflict")
}

// TestReconcileContactSyncNoBaselineNoConflicts covers links that predate
// migration 000032 (SyncedValues empty): the first sync establishes the
// baseline without retroactively reporting conflicts, because there is no
// way to distinguish a local edit from prior sync state.
func TestReconcileContactSyncNoBaselineNoConflicts(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	href := "/addressbooks/test/erin.vcf"
	first := carddav.AddressObject{Path: href, ETag: "\"etag-1\"", Card: testCard(t, "erin-uid", "Erin", "Esurance", "erin@example.com")}
	_, err := reconcileContactSync(db, sub, []carddav.AddressObject{first}, nil, false, "")
	require.NoError(t, err)

	var link models.ContactSyncLink
	require.NoError(t, db.Where("subscription_id = ? AND href = ?", sub.ID, href).First(&link).Error)
	assert.NotEmpty(t, link.SyncedValues, "a freshly-created link gets a baseline on its first sync")

	// Simulate a pre-migration link with no baseline.
	require.NoError(t, db.Model(&link).Update("synced_values", "").Error)

	var editTarget models.Contact
	require.NoError(t, db.First(&editTarget, link.ContactID).Error)
	editTarget.Phones = []models.ContactPhone{{Value: "555-0100"}}
	require.NoError(t, db.Save(&editTarget).Error)

	second := carddav.AddressObject{Path: href, ETag: "\"etag-2\"", Card: testCard(t, "erin-uid", "Erin", "Esurance", "erin.new@example.com")}
	_, err = reconcileContactSync(db, sub, []carddav.AddressObject{second}, nil, false, "")
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&models.ContactSyncConflict{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Zero(t, count, "a missing baseline must not produce retroactive conflicts")

	require.NoError(t, db.First(&link, link.ID).Error)
	assert.NotEmpty(t, link.SyncedValues, "the sync must re-establish the baseline")
}

// TestContactSyncLinkSyncedValuesSavesAgainstRealMigratedSchema guards the
// new column + table against GORM/ migration drift the AutoMigrate-based test
// DBs cannot see (CLAUDE.md backend trap 1).
func TestContactSyncLinkSyncedValuesSavesAgainstRealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "contact-sync-conflicts.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := createContactSyncTestUser(t, db)
	cfg := contactSyncTestConfig()
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	contact := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&contact).Error)

	link := models.ContactSyncLink{
		SubscriptionID: sub.ID,
		UserID:         user.ID,
		Href:           "/addressbooks/test/bob.vcf",
		ContactID:      contact.ID,
		ContentHash:    "hash-1",
		SyncedValues:   `{"phone":"[]"}`,
	}
	require.NoError(t, db.Create(&link).Error)

	var reloaded models.ContactSyncLink
	require.NoError(t, db.First(&reloaded, link.ID).Error)
	assert.Equal(t, `{"phone":"[]"}`, reloaded.SyncedValues)

	// And a conflict row round-trips against the real schema too.
	conflict := models.ContactSyncConflict{
		UserID: user.ID, SubscriptionID: sub.ID, ContactID: contact.ID,
		Field: models.SyncConflictFieldPhone, LocalValue: `[{"value":"555-0100"}]`,
		RemoteValue: "[]", Status: models.SyncConflictStatusPending,
	}
	require.NoError(t, db.Create(&conflict).Error)
	require.NotEmpty(t, conflict.ID)

	var reloadedConflict models.ContactSyncConflict
	require.NoError(t, db.First(&reloadedConflict, "id = ?", conflict.ID).Error)
	assert.Equal(t, `[{"value":"555-0100"}]`, reloadedConflict.LocalValue)
}

// --- list / restore / dismiss -------------------------------------------------

func seedSyncConflict(t *testing.T, db *gorm.DB, userID, contactID, subID uint, field, local, remote string) models.ContactSyncConflict {
	t.Helper()
	conflict := models.ContactSyncConflict{
		UserID: userID, SubscriptionID: subID, ContactID: contactID,
		Field: field, LocalValue: local, RemoteValue: remote, Status: models.SyncConflictStatusPending,
	}
	require.NoError(t, db.Create(&conflict).Error)
	return conflict
}

func TestListContactSyncConflicts_EnrichesAndFilters(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	contact := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&contact).Error)
	archived := models.Contact{UserID: user.ID, Firstname: "Archived", Archived: true}
	require.NoError(t, db.Create(&archived).Error)

	pending := seedSyncConflict(t, db, user.ID, contact.ID, sub.ID, models.SyncConflictFieldJobTitle, "Local", "Remote")
	dismissed := seedSyncConflict(t, db, user.ID, contact.ID, sub.ID, models.SyncConflictFieldPhone, "X", "Y")
	require.NoError(t, db.Model(&dismissed).Update("status", models.SyncConflictStatusDismissed).Error)
	seedSyncConflict(t, db, user.ID, archived.ID, sub.ID, models.SyncConflictFieldPhone, "X", "Y")

	conflicts, err := ListContactSyncConflicts(db, user.ID)
	require.NoError(t, err)
	require.Len(t, conflicts, 1, "dismissed + archived-contact conflicts must not surface")
	assert.Equal(t, pending.ID, conflicts[0].ID)
	assert.Equal(t, contact.ID, conflicts[0].ContactID)
	assert.Equal(t, contact.VCardUID, conflicts[0].ContactVCardUID)
	assert.Equal(t, "Ada Lovelace", conflicts[0].ContactName)
	assert.Equal(t, sub.Name, conflicts[0].SubscriptionName)
}

func TestListContactSyncConflicts_ScopedToOwner(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	other := models.User{Username: "other", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	contact := models.Contact{UserID: user.ID, Firstname: "Mine"}
	require.NoError(t, db.Create(&contact).Error)
	seedSyncConflict(t, db, user.ID, contact.ID, sub.ID, models.SyncConflictFieldPhone, "X", "Y")

	otherConflicts, err := ListContactSyncConflicts(db, other.ID)
	require.NoError(t, err)
	assert.Nil(t, otherConflicts, "another user must see nothing")

	myConflicts, err := ListContactSyncConflicts(db, user.ID)
	require.NoError(t, err)
	require.Len(t, myConflicts, 1)
}

func TestRestoreContactSyncConflict_RestoresScalarAndArrayFields(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	contact := models.Contact{UserID: user.ID, Firstname: "Carol", Lastname: "Danvers"}
	require.NoError(t, db.Create(&contact).Error)

	jobTitle := seedSyncConflict(t, db, user.ID, contact.ID, sub.ID, models.SyncConflictFieldJobTitle, "Local Title", "Remote Title")
	phone := seedSyncConflict(t, db, user.ID, contact.ID, sub.ID, models.SyncConflictFieldPhone, `[{"type":"","value":"555-0100"}]`, "[]")

	require.NoError(t, RestoreContactSyncConflict(db, user.ID, jobTitle.ID))
	require.NoError(t, RestoreContactSyncConflict(db, user.ID, phone.ID))

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Equal(t, "Local Title", reloaded.JobTitle)
	require.Len(t, reloaded.Phones, 1)
	assert.Equal(t, "555-0100", reloaded.Phones[0].Value)
	assert.Equal(t, "555-0100", reloaded.Phone, "the denormalized scalar follows the restored array via BeforeSave")

	for _, id := range []string{jobTitle.ID, phone.ID} {
		var conflict models.ContactSyncConflict
		require.NoError(t, db.First(&conflict, "id = ?", id).Error)
		assert.Equal(t, models.SyncConflictStatusDismissed, conflict.Status)
	}
}

func TestRestoreContactSyncConflict_AlreadyDismissedIsConflict(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	contact := models.Contact{UserID: user.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&contact).Error)
	conflict := seedSyncConflict(t, db, user.ID, contact.ID, sub.ID, models.SyncConflictFieldJobTitle, "A", "B")
	require.NoError(t, db.Model(&conflict).Update("status", models.SyncConflictStatusDismissed).Error)

	err := RestoreContactSyncConflict(db, user.ID, conflict.ID)
	require.Error(t, err)
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok)
	assert.Equal(t, apperrors.ErrCodeConflict, appErr.Code)
}

func TestRestoreContactSyncConflict_UnknownOrForeignIs404(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	other := models.User{Username: "other", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	contact := models.Contact{UserID: user.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&contact).Error)
	foreign := seedSyncConflict(t, db, other.ID, contact.ID, sub.ID, models.SyncConflictFieldPhone, "A", "B")

	err := RestoreContactSyncConflict(db, user.ID, foreign.ID)
	require.Error(t, err)
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	err = RestoreContactSyncConflict(db, user.ID, "does-not-exist")
	require.Error(t, err)
	appErr, ok = err.(*apperrors.AppError)
	require.True(t, ok)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
}

func TestDismissContactSyncConflict_IdempotentAndScoped(t *testing.T) {
	db := setupContactSyncTestDB(t)
	cfg := contactSyncTestConfig()
	user := createContactSyncTestUser(t, db)
	other := models.User{Username: "other", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/test/", "", "")

	contact := models.Contact{UserID: user.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&contact).Error)
	conflict := seedSyncConflict(t, db, user.ID, contact.ID, sub.ID, models.SyncConflictFieldPhone, "A", "B")

	require.NoError(t, DismissContactSyncConflict(db, user.ID, conflict.ID))
	require.NoError(t, DismissContactSyncConflict(db, user.ID, conflict.ID), "dismiss is idempotent")

	var reloaded models.ContactSyncConflict
	require.NoError(t, db.First(&reloaded, "id = ?", conflict.ID).Error)
	assert.Equal(t, models.SyncConflictStatusDismissed, reloaded.Status)

	err := DismissContactSyncConflict(db, other.ID, conflict.ID)
	require.Error(t, err)
	appErr, ok := err.(*apperrors.AppError)
	require.True(t, ok)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)

	err = DismissContactSyncConflict(db, user.ID, "does-not-exist")
	require.Error(t, err)
	appErr, ok = err.(*apperrors.AppError)
	require.True(t, ok)
	assert.Equal(t, apperrors.ErrCodeNotFound, appErr.Code)
}
