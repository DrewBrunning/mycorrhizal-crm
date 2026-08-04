package controllers

import (
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBulkContactOperation_RealMigratedSchema is the real-DB check for N5:
// every other bulk-operation test runs against AutoMigrate on :memory: sqlite
// (via setupRouter), which derives its schema from the same Go struct tags
// the application code uses — it cannot catch a GORM column-tag mismatch
// against the real hand-written migration SQL (this fork's own recurring bug
// class, e.g. ContactSyncLink.ETag). CircleMember.MemberVCardUID and
// ContactTag.ContactVCardUID already carry explicit gorm:"column:..." tags
// for exactly this reason, but the bulk endpoint is a new, independent read
// path over those tables, so it gets its own real-schema round trip rather
// than relying on that being proven elsewhere.
//
// Round trip: add_circle -> add_tag -> archive (retires reminders) ->
// unarchive -> delete, each verified against the real
// database.InitDB-migrated tables, not AutoMigrate's derived ones.
func TestBulkContactOperation_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bulk-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "bulk-realdb", Password: "password123!A", Email: "bulk-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&bob).Error)

	circle := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circle).Error)
	tag := models.Tag{UserID: user.ID, Name: "vip"}
	require.NoError(t, db.Create(&tag).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/bulk", middleware.ValidateJSONMiddleware(&models.BulkContactOperationInput{}), BulkContactOperation)

	// add_circle: CircleMember.MemberVCardUID must resolve against the real
	// circle_members.member_vcard_uid column, not an AutoMigrate-derived one.
	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "add_circle",
		VCardUIDs: []string{alice.VCardUID, bob.VCardUID},
		CircleID:  circle.ID,
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 2, result.Succeeded)
	var circleMemberCount int64
	require.NoError(t, db.Model(&models.CircleMember{}).Where("circle_id = ? AND member_vcard_uid = ?", circle.ID, alice.VCardUID).Count(&circleMemberCount).Error)
	assert.EqualValues(t, 1, circleMemberCount)

	// add_tag: ContactTag.ContactVCardUID against the real
	// contact_tags.contact_vcard_uid column.
	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action:    "add_tag",
		VCardUIDs: []string{alice.VCardUID},
		TagID:     tag.ID,
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)
	var contactTagCount int64
	require.NoError(t, db.Model(&models.ContactTag{}).Where("tag_id = ? AND contact_vcard_uid = ?", tag.ID, alice.VCardUID).Count(&contactTagCount).Error)
	assert.EqualValues(t, 1, contactTagCount)

	// archive: also retires reminders through the real reminders table.
	reminder := models.Reminder{UserID: user.ID, ContactID: &alice.ID, Message: "say hi", Recurrence: "once"}
	require.NoError(t, db.Create(&reminder).Error)
	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action:    "archive",
		VCardUIDs: []string{alice.VCardUID},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)
	var reloadedAlice models.Contact
	require.NoError(t, db.First(&reloadedAlice, alice.ID).Error)
	assert.True(t, reloadedAlice.Archived)
	var reminderCount int64
	require.NoError(t, db.Model(&models.Reminder{}).Where("contact_id = ?", alice.ID).Count(&reminderCount).Error)
	assert.EqualValues(t, 0, reminderCount)

	// unarchive.
	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action:    "unarchive",
		VCardUIDs: []string{alice.VCardUID},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)

	// delete: the full DeleteContact cascade against every real migrated
	// table it touches, soft-deleting the contact.
	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action:    "delete",
		VCardUIDs: []string{alice.VCardUID},
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)
	var scopedCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", alice.ID).Count(&scopedCount).Error)
	assert.EqualValues(t, 0, scopedCount, "soft-deleted contact must be excluded from scoped queries")
	var unscopedCount int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", alice.ID).Count(&unscopedCount).Error)
	assert.EqualValues(t, 1, unscopedCount, "soft delete must not hard-remove the row")

	// The tag/circle membership rows the delete cascade should have cleaned
	// up are hard-deleted per the join-row rule -- confirm they're gone too.
	require.NoError(t, db.Model(&models.CircleMember{}).Where("circle_id = ? AND member_vcard_uid = ?", circle.ID, alice.VCardUID).Count(&circleMemberCount).Error)
	assert.EqualValues(t, 0, circleMemberCount)
	require.NoError(t, db.Model(&models.ContactTag{}).Where("tag_id = ? AND contact_vcard_uid = ?", tag.ID, alice.VCardUID).Count(&contactTagCount).Error)
	assert.EqualValues(t, 0, contactTagCount)

	// Bob was never deleted -- untouched by Alice's cascade.
	var reloadedBob models.Contact
	require.NoError(t, db.First(&reloadedBob, bob.ID).Error)
	assert.False(t, reloadedBob.Archived)
}
