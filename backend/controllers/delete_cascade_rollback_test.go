package controllers

import (
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newDeleteCascadeRouter wires the real DeleteUser (admin) and DeleteContact
// routes against a dbtest.New(t) real-migrated-schema database, with a fixed
// caller identity injected the way AuthMiddleware/AdminMiddleware normally
// would. Kept local to this file rather than reusing setupRouter from
// admin_user_delete_test.go, which another agent is rewriting concurrently.
func newDeleteCascadeRouter(db *gorm.DB, actingUserID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", actingUserID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.DELETE("/admin/users/:id", DeleteUser)
	router.DELETE("/contacts/:id", DeleteContact)
	return router
}

func doDelete(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("DELETE", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestDeleteUserCascade_RollsBackOnLateFailure pins that deleteUserCascade
// (user_delete_cascade.go) runs as one all-or-nothing transaction: a forced
// failure on `contacts` — one of the very last tables the cascade touches
// (user_delete_cascade.go line ~327, right before the user row itself) —
// must roll back every earlier delete in the same cascade, not just leave
// the contact behind. Backend traps #6/#7 (CLAUDE.md) call the cascade
// checklist manual and easy to get wrong; this is the regression test for
// "manual cascade forgets to roll back," not just "manual cascade forgets a
// table."
func TestDeleteUserCascade_RollsBackOnLateFailure(t *testing.T) {
	db := dbtest.New(t)

	admin := models.User{Username: "admin", Password: "password123!A", Email: "admin@example.com", IsAdmin: true}
	require.NoError(t, db.Create(&admin).Error)

	target := models.User{Username: "target", Password: "password123!A", Email: "target@example.com"}
	require.NoError(t, db.Create(&target).Error)

	contact := models.Contact{UserID: target.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	attachment := models.Attachment{
		UserID: target.ID, ContactVCardUID: contact.VCardUID,
		StoredName: "stored.bin", OriginalName: "original.bin", ContentType: "application/octet-stream", SizeBytes: 10,
	}
	require.NoError(t, db.Create(&attachment).Error)

	note := models.Note{UserID: target.ID, Content: "hello", ContactID: &contact.ID}
	require.NoError(t, db.Create(&note).Error)

	pref := models.Preference{UserID: target.ID, EntityID: contact.VCardUID, Category: "food"}
	require.NoError(t, db.Create(&pref).Error)

	// Force a failure on one of the last tables deleteUserCascade touches.
	require.NoError(t, db.Exec(
		"CREATE TRIGGER block_contacts_delete BEFORE DELETE ON contacts BEGIN SELECT RAISE(ABORT, 'simulated cascade failure'); END;",
	).Error)

	router := newDeleteCascadeRouter(db, admin.ID)
	resp := doDelete(router, "/admin/users/"+strconv.Itoa(int(target.ID)))
	require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())

	// Every row the cascade deleted BEFORE reaching the faulted `contacts`
	// table must still be present — Unscoped() per CLAUDE.md trap #6, since
	// several of these are soft-deleted rather than hard-deleted, and a plain
	// Count would pass whether the row was rolled back or merely tombstoned.
	assertStillPresent(t, db, &models.User{}, target.ID)
	assertStillPresent(t, db, &models.Contact{}, contact.ID)
	assertStillPresent(t, db, &models.Attachment{}, attachment.ID)
	assertStillPresent(t, db, &models.Note{}, note.ID)

	var prefCount int64
	require.NoError(t, db.Unscoped().Model(&models.Preference{}).Where("id = ? AND deleted_at IS NULL", pref.ID).Count(&prefCount).Error)
	require.EqualValues(t, 1, prefCount, "Preference must survive the rolled-back transaction, undeleted")
}

// TestDeleteContactCascade_RollsBackOnLateFailure is the DeleteContact
// analogue of the test above: a forced failure on the final tx.Delete(&contact)
// call (contact_controller.go's DeleteContact, after deleteContactAssociations
// has already run every earlier delete in the same transaction) must roll
// back the whole cascade, not just leave the contact undeleted while its
// associations are gone.
func TestDeleteContactCascade_RollsBackOnLateFailure(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "owner", Password: "password123!A", Email: "owner@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	reminder := models.Reminder{
		UserID: user.ID, Message: "call Ada", RemindAt: time.Now().Add(24 * time.Hour),
		Recurrence: "once", ContactID: &contact.ID,
	}
	require.NoError(t, db.Create(&reminder).Error)

	note := models.Note{UserID: user.ID, Content: "hello", ContactID: &contact.ID}
	require.NoError(t, db.Create(&note).Error)

	attachment := models.Attachment{
		UserID: user.ID, ContactVCardUID: contact.VCardUID,
		StoredName: "stored.bin", OriginalName: "original.bin", ContentType: "application/octet-stream", SizeBytes: 10,
	}
	require.NoError(t, db.Create(&attachment).Error)

	// Contact is user-authored content, so DeleteContact's final tx.Delete
	// soft-deletes it (an UPDATE of deleted_at, CLAUDE.md trap #7) rather
	// than issuing a raw DELETE — the abort trigger has to match that.
	require.NoError(t, db.Exec(
		"CREATE TRIGGER block_contact_soft_delete BEFORE UPDATE OF deleted_at ON contacts "+
			"WHEN NEW.deleted_at IS NOT NULL BEGIN SELECT RAISE(ABORT, 'simulated cascade failure'); END;",
	).Error)

	router := newDeleteCascadeRouter(db, user.ID)
	resp := doDelete(router, "/contacts/"+strconv.Itoa(int(contact.ID)))
	require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())

	assertStillPresent(t, db, &models.Contact{}, contact.ID)
	assertStillPresent(t, db, &models.Reminder{}, reminder.ID)
	assertStillPresent(t, db, &models.Note{}, note.ID)
	assertStillPresent(t, db, &models.Attachment{}, attachment.ID)
}

// assertStillPresent counts a uint-PK, gorm.Model-embedding row by ID with
// Unscoped() AND an explicit deleted_at IS NULL filter — not just Unscoped()
// alone. Most of this cascade's rows are soft-deleted (an UPDATE of
// deleted_at, CLAUDE.md trap #7), so a plain Unscoped() existence count
// would pass whether the transaction rolled back or committed the soft
// delete: the row is still "there" either way. Requiring deleted_at IS NULL
// is what actually proves the earlier delete in the same transaction was
// rolled back, not merely that the row wasn't hard-deleted.
func assertStillPresent(t *testing.T, db *gorm.DB, model any, id uint) {
	t.Helper()
	var count int64
	require.NoError(t, db.Unscoped().Model(model).Where("id = ? AND deleted_at IS NULL", id).Count(&count).Error)
	require.EqualValues(t, 1, count, "row must still be present, undeleted, after a rolled-back cascade")
}
