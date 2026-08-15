package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// N5 bulk-operation tests (N5).
// These pin the contract the frontend relies on: partial-success semantics,
// ownership scoping (a foreign contact is a reported failure, never silently
// skipped), and duplicate-add tolerance (an already-present membership is
// success, not the 409 the per-contact endpoints return).

func bulkTestRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, router := setupRouter()
	// The production route uses ValidateJSONMiddleware (not withValidated), so
	// the tests exercise the same validation (oneof action, uuid4 uids, etc.).
	router.POST("/contacts/bulk", middleware.ValidateJSONMiddleware(&models.BulkContactOperationInput{}), BulkContactOperation)
	return db, router
}

func doBulk(t *testing.T, router *gin.Engine, input models.BulkContactOperationInput) (int, models.BulkOperationResult) {
	t.Helper()
	jsonValue, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", "/contacts/bulk", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var result models.BulkOperationResult
	if len(w.Body.Bytes()) > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &result)
	}
	return w.Code, result
}

func createBulkTestUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	// setupRouter seeds one user and authenticates requests as it, so the
	// tests must operate on that same user.
	var user models.User
	require.NoError(t, db.First(&user).Error)
	return user
}

// createBulkForeignUser creates a genuinely different user whose contacts the
// authenticated user must never touch (ownership-scoping tests).
func createBulkForeignUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	user := models.User{Username: "bulk-other", Password: "password123!A", Email: "bulk-other@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createBulkContact(t *testing.T, db *gorm.DB, userID uint, firstname string) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: firstname}
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

func TestBulkAddCircleCreatesMemberships(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	bob := createBulkContact(t, db, user.ID, "Bob")
	circle := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circle).Error)

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "add_circle",
		VCardUIDs: []string{alice.VCardUID, bob.VCardUID},
		CircleID:  circle.ID,
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 2, result.Succeeded)
	assert.Equal(t, 0, result.Failed)
	assert.Len(t, result.Failures, 0)

	var count int64
	db.Model(&models.CircleMember{}).Where("circle_id = ?", circle.ID).Count(&count)
	assert.EqualValues(t, 2, count)
}

func TestBulkAddCircleDuplicateIsSuccess(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	circle := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: alice.VCardUID}).Error)

	// Already a member — the bulk add must not 409; it reports success.
	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "add_circle",
		VCardUIDs: []string{alice.VCardUID},
		CircleID:  circle.ID,
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, 0, result.Failed)

	var count int64
	db.Model(&models.CircleMember{}).Where("circle_id = ?", circle.ID).Count(&count)
	assert.EqualValues(t, 1, count) // no duplicate row
}

func TestBulkOwnershipScopingPartialSuccess(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	otherUser := createBulkForeignUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	bob := createBulkContact(t, db, user.ID, "Bob")
	foreign := createBulkContact(t, db, otherUser.ID, "Not Yours")
	circle := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circle).Error)

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "add_circle",
		VCardUIDs: []string{alice.VCardUID, foreign.VCardUID, bob.VCardUID},
		CircleID:  circle.ID,
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 2, result.Succeeded)
	assert.Equal(t, 1, result.Failed)
	require.Len(t, result.Failures, 1)
	assert.Equal(t, foreign.VCardUID, result.Failures[0].VCardUID)

	// Only the two owned contacts got memberships; the foreign one was not
	// silently affected.
	var count int64
	db.Model(&models.CircleMember{}).Where("circle_id = ?", circle.ID).Count(&count)
	assert.EqualValues(t, 2, count)

	// The foreign contact itself is untouched.
	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, foreign.ID).Error)
	assert.Equal(t, otherUser.ID, reloaded.UserID)
}

func TestBulkRemoveCircle(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	circle := models.Circle{UserID: user.ID, Name: "Friends"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: alice.VCardUID}).Error)

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "remove_circle",
		VCardUIDs: []string{alice.VCardUID},
		CircleID:  circle.ID,
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)

	var count int64
	db.Model(&models.CircleMember{}).Where("circle_id = ?", circle.ID).Count(&count)
	assert.EqualValues(t, 0, count)
}

func TestBulkAddAndRemoveTag(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	tag := models.Tag{UserID: user.ID, Name: "vip"}
	require.NoError(t, db.Create(&tag).Error)

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "add_tag",
		VCardUIDs: []string{alice.VCardUID},
		TagID:     tag.ID,
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)

	var tagged int64
	db.Model(&models.ContactTag{}).Where("tag_id = ?", tag.ID).Count(&tagged)
	assert.EqualValues(t, 1, tagged)

	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action:    "remove_tag",
		VCardUIDs: []string{alice.VCardUID},
		TagID:     tag.ID,
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)

	db.Model(&models.ContactTag{}).Where("tag_id = ?", tag.ID).Count(&tagged)
	assert.EqualValues(t, 0, tagged)
}

func TestBulkArchiveAndUnarchive(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	reminder := models.Reminder{UserID: user.ID, ContactID: &alice.ID, Message: "say hi", RemindAt: time.Now(), Recurrence: "once"}
	require.NoError(t, db.Create(&reminder).Error)

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "archive",
		VCardUIDs: []string{alice.VCardUID},
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, alice.ID).Error)
	assert.True(t, reloaded.Archived)
	// Archive retires reminders like the single-contact path.
	var reminderCount int64
	db.Model(&models.Reminder{}).Where("contact_id = ?", alice.ID).Count(&reminderCount)
	assert.EqualValues(t, 0, reminderCount)

	code, result = doBulk(t, router, models.BulkContactOperationInput{
		Action:    "unarchive",
		VCardUIDs: []string{alice.VCardUID},
	})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)

	require.NoError(t, db.First(&reloaded, alice.ID).Error)
	assert.False(t, reloaded.Archived)
}

func TestBulkDeleteRemovesContactsWithPartialSuccess(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	otherUser := createBulkForeignUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	foreign := createBulkContact(t, db, otherUser.ID, "Not Yours")

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "delete",
		VCardUIDs: []string{alice.VCardUID, foreign.VCardUID},
	})

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, 1, result.Failed)
	assert.Equal(t, foreign.VCardUID, result.Failures[0].VCardUID)

	// Alice is soft-deleted (hidden from scoped queries); the foreign contact
	// is untouched.
	var count int64
	db.Model(&models.Contact{}).Where("id = ?", alice.ID).Count(&count)
	assert.EqualValues(t, 0, count)
	var foreignCount int64
	db.Model(&models.Contact{}).Where("id = ?", foreign.ID).Count(&foreignCount)
	assert.EqualValues(t, 1, foreignCount)
}

func TestBulkForeignCircleRejected(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	otherUser := createBulkForeignUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	foreignCircle := models.Circle{UserID: otherUser.ID, Name: "Theirs"}
	require.NoError(t, db.Create(&foreignCircle).Error)

	// A circle the user does not own makes the whole request malformed — 404,
	// not a partial success.
	code, _ := doBulk(t, router, models.BulkContactOperationInput{
		Action:    "add_circle",
		VCardUIDs: []string{alice.VCardUID},
		CircleID:  foreignCircle.ID,
	})
	assert.Equal(t, http.StatusNotFound, code)
}

func TestBulkValidationErrors(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")

	t.Run("unknown action", func(t *testing.T) {
		code, _ := doBulk(t, router, models.BulkContactOperationInput{
			Action:    "explode",
			VCardUIDs: []string{alice.VCardUID},
		})
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("missing circle_id for circle action", func(t *testing.T) {
		code, _ := doBulk(t, router, models.BulkContactOperationInput{
			Action:    "add_circle",
			VCardUIDs: []string{alice.VCardUID},
		})
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("empty uid list", func(t *testing.T) {
		code, _ := doBulk(t, router, models.BulkContactOperationInput{
			Action:    "archive",
			VCardUIDs: []string{},
		})
		assert.Equal(t, http.StatusBadRequest, code)
	})
}
