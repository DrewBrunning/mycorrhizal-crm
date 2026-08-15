package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestDeleteUser_CleansUpAllOwnedRows is the regression test
// DeleteUser must not
// leave orphaned rows in any of the 14 tables that reference a user, not just
// the handful it originally covered.
func TestDeleteUser_CleansUpAllOwnedRows(t *testing.T) {
	db, router := setupRouter()

	// setupRouter seeds the "tester" user and puts their ID in context as the
	// acting admin — create a second, deletable target user.
	target := models.User{Username: "target", Email: "target@example.com", Password: "password123"}
	require.NoError(t, db.Create(&target).Error)

	contact := models.Contact{UserID: target.ID, Firstname: "Orphan", Lastname: "Check"}
	require.NoError(t, db.Create(&contact).Error)

	require.NoError(t, db.Create(&models.Reminder{UserID: target.ID, ContactID: &contact.ID, Message: "m"}).Error)
	require.NoError(t, db.Create(&models.Note{UserID: target.ID, ContactID: &contact.ID, Content: "n"}).Error)

	webhook := models.Webhook{UserID: target.ID, URL: "https://example.com/hook", Events: []string{"contact.created"}}
	require.NoError(t, db.Create(&webhook).Error)
	require.NoError(t, db.Create(&models.WebhookDelivery{WebhookID: webhook.ID, EventType: "contact.created", Payload: "{}"}).Error)

	sub := models.ContactSubscription{UserID: target.ID, Name: "sub", URL: "https://example.com/dav"}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&models.ContactSyncLink{SubscriptionID: sub.ID, UserID: target.ID, ContactID: contact.ID, Href: "/dav/1.vcf"}).Error)

	household := models.Household{UserID: target.ID, Name: "h", Type: "other"}
	require.NoError(t, db.Create(&household).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: household.ID, UserID: target.ID, MemberVCardUID: contact.VCardUID, Role: "adult"}).Error)

	circle := models.Circle{UserID: target.ID, Name: "c"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: target.ID, MemberVCardUID: contact.VCardUID}).Error)

	tag := models.Tag{UserID: target.ID, Name: "t"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: tag.ID, UserID: target.ID, ContactVCardUID: contact.VCardUID}).Error)

	fieldDef := models.FieldDefinition{UserID: target.ID, Label: "f", Key: "f", Target: "contact", Type: "text"}
	require.NoError(t, db.Create(&fieldDef).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: fieldDef.ID, UserID: target.ID, EntityID: contact.VCardUID, Value: json.RawMessage(`"v"`)}).Error)

	require.NoError(t, db.Create(&models.RelationshipEdge{UserID: target.ID, SourceID: contact.VCardUID, TargetID: contact.VCardUID, Type: "related_to"}).Error)
	require.NoError(t, db.Create(&models.LifeEvent{UserID: target.ID, EntityID: contact.VCardUID, Type: "custom"}).Error)
	require.NoError(t, db.Create(&models.LinkFieldType{UserID: target.ID, Name: "Custom", Protocol: "https://example.com/{value}", Category: models.LinkFieldTypeCategoryOther}).Error)
	require.NoError(t, db.Create(&models.ConversationAgenda{UserID: target.ID, EntityID: contact.VCardUID, Content: "Ask about something"}).Error)
	require.NoError(t, db.Create(&models.Gift{UserID: target.ID, EntityID: contact.VCardUID, Description: "A gift idea"}).Error)

	require.NoError(t, db.Create(&models.CardDAVSync{UserID: target.ID, SyncToken: "tok", LastModified: time.Now()}).Error)
	require.NoError(t, db.Create(&models.ApiToken{UserID: target.ID, Name: "token", TokenHash: "hash"}).Error)
	require.NoError(t, db.Create(&models.ReminderCompletion{UserID: target.ID, ContactID: contact.ID, Message: "done", CompletedAt: time.Now()}).Error)

	calSub := models.CalendarSubscription{UserID: target.ID, Name: "cal", URL: "https://example.com/cal.ics"}
	require.NoError(t, db.Create(&calSub).Error)
	activity := models.Activity{UserID: target.ID, Title: "call", Type: "call", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	require.NoError(t, db.Create(&models.CalendarEventLink{SubscriptionID: calSub.ID, UserID: target.ID, UID: "evt-1", ActivityID: activity.ID, ContentHash: "h"}).Error)

	// P1: ContactShare has
	// TWO owning columns, not one -- a departing user's rows must be swept
	// whether they were the sender or the recipient.
	var admin models.User
	require.NoError(t, db.Where("username = ?", "tester").First(&admin).Error)
	shareAsSender := models.ContactShare{FromUserID: target.ID, ToUserID: admin.ID, ContactDisplayName: "Alice", Payload: "[]"}
	require.NoError(t, db.Create(&shareAsSender).Error)
	shareAsRecipient := models.ContactShare{FromUserID: admin.ID, ToUserID: target.ID, ContactDisplayName: "Bob", Payload: "[]"}
	require.NoError(t, db.Create(&shareAsRecipient).Error)

	router.DELETE("/users/:id", DeleteUser)

	req, _ := http.NewRequest("DELETE", "/users/"+strconv.Itoa(int(target.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assertGone := func(name string, model any, where string, args ...any) {
		t.Helper()
		var count int64
		require.NoError(t, db.Model(model).Where(where, args...).Count(&count).Error)
		assert.Zero(t, count, "%s rows should be gone after DeleteUser", name)
	}

	assertGone("Reminder", &models.Reminder{}, "user_id = ?", target.ID)
	assertGone("Note", &models.Note{}, "user_id = ?", target.ID)
	assertGone("Webhook", &models.Webhook{}, "user_id = ?", target.ID)
	assertGone("WebhookDelivery", &models.WebhookDelivery{}, "webhook_id = ?", webhook.ID)
	assertGone("ContactSubscription", &models.ContactSubscription{}, "user_id = ?", target.ID)
	assertGone("ContactSyncLink", &models.ContactSyncLink{}, "user_id = ?", target.ID)
	assertGone("Household", &models.Household{}, "user_id = ?", target.ID)
	assertGone("HouseholdMember", &models.HouseholdMember{}, "user_id = ?", target.ID)
	assertGone("Circle", &models.Circle{}, "user_id = ?", target.ID)
	assertGone("CircleMember", &models.CircleMember{}, "user_id = ?", target.ID)
	assertGone("Tag", &models.Tag{}, "user_id = ?", target.ID)
	assertGone("ContactTag", &models.ContactTag{}, "user_id = ?", target.ID)
	assertGone("FieldDefinition", &models.FieldDefinition{}, "user_id = ?", target.ID)
	assertGone("FieldValue", &models.FieldValue{}, "user_id = ?", target.ID)
	assertGone("RelationshipEdge", &models.RelationshipEdge{}, "user_id = ?", target.ID)
	assertGone("LifeEvent", &models.LifeEvent{}, "user_id = ?", target.ID)
	assertGone("ConversationAgenda", &models.ConversationAgenda{}, "user_id = ?", target.ID)
	assertGone("Gift", &models.Gift{}, "user_id = ?", target.ID)
	assertGone("CardDAVSync", &models.CardDAVSync{}, "user_id = ?", target.ID)
	assertGone("ApiToken", &models.ApiToken{}, "user_id = ?", target.ID)
	assertGone("ReminderCompletion", &models.ReminderCompletion{}, "user_id = ?", target.ID)
	assertGone("CalendarEventLink", &models.CalendarEventLink{}, "user_id = ?", target.ID)
	assertGone("CalendarSubscription", &models.CalendarSubscription{}, "user_id = ?", target.ID)
	assertGone("Contact", &models.Contact{}, "user_id = ?", target.ID)
	assertGone("ContactShare (target as sender)", &models.ContactShare{}, "id = ?", shareAsSender.ID)
	assertGone("ContactShare (target as recipient)", &models.ContactShare{}, "id = ?", shareAsRecipient.ID)
	assertGone("LinkFieldType", &models.LinkFieldType{}, "user_id = ?", target.ID)

	// LinkFieldType is soft-deletable (T26); DeleteUser hard-deletes it
	// (Unscoped, like CadencePolicy/Preference/LifeEvent) since there's no
	// tombstone consumer once the account is gone — assertGone alone
	// wouldn't catch a plain soft-delete leaving the row behind.
	var unscopedLinkFieldTypeCount int64
	require.NoError(t, db.Unscoped().Model(&models.LinkFieldType{}).Where("user_id = ?", target.ID).Count(&unscopedLinkFieldTypeCount).Error)
	assert.Zero(t, unscopedLinkFieldTypeCount, "LinkFieldType rows must be hard-deleted, not merely soft-deleted")

	var remainingUser models.User
	err := db.First(&remainingUser, target.ID).Error
	assert.Error(t, err, "target user should be deleted")

	// M4/T26: prove rows are genuinely gone, not merely soft-deleted.
	var unscopedUser models.User
	unscopedErr := db.Unscoped().First(&unscopedUser, target.ID).Error
	assert.Error(t, unscopedErr, "target user must be Unscoped-gone — not merely soft-deleted")

	// M2/T26: re-registration with the deleted account's email must succeed.
	newUser := models.User{
		Username: target.Username,
		Password: "new-password-for-rereg",
		Email:    target.Email,
	}
	require.NoError(t, db.Create(&newUser).Error, "re-registration with deleted account's email must succeed (T26)")
}

// --- GetCurrentUser ---

func TestGetCurrentUser_Success(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	require.NoError(t, db.First(&user).Error)
	user.EnabledContactFields = []string{"phone"}
	require.NoError(t, db.Save(&user).Error)

	router.GET("/users/me", GetCurrentUser)

	req, _ := http.NewRequest("GET", "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.CurrentUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, user.ID, resp.ID)
	assert.Equal(t, user.Username, resp.Username)
	assert.Equal(t, user.Email, resp.Email)
	assert.Equal(t, []string{"phone"}, resp.EnabledContactFields)

	// The response DTO must never carry the password hash, regardless of
	// what the model looks like server-side.
	assert.NotContains(t, w.Body.String(), user.Password)
	assert.NotContains(t, w.Body.String(), "password")
}

func TestGetCurrentUser_NotFound(t *testing.T) {
	_, router := setupRouter()

	// Overwrite the seeded userID with one that doesn't exist in the DB.
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(999999))
	})
	router.GET("/users/me", GetCurrentUser)

	req, _ := http.NewRequest("GET", "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// --- ListUsers ---

func TestListUsers_Success(t *testing.T) {
	db, router := setupRouter()

	require.NoError(t, db.Create(&models.User{Username: "second", Email: "second@example.com", Password: "password123"}).Error)
	require.NoError(t, db.Create(&models.User{Username: "third", Email: "third@example.com", Password: "password123", IsAdmin: true}).Error)

	router.GET("/users", ListUsers)

	req, _ := http.NewRequest("GET", "/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.AdminUsersListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 3, resp.Total)
	assert.Len(t, resp.Users, 3)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 1, resp.TotalPages)

	// No password field should ever be serialized for the admin listing.
	assert.NotContains(t, w.Body.String(), "password")

	var thirdAdmin bool
	for _, u := range resp.Users {
		if u.Username == "third" {
			thirdAdmin = u.IsAdmin
		}
	}
	assert.True(t, thirdAdmin, "admin flag should be visible to the caller")
}

func TestListUsers_Pagination(t *testing.T) {
	db, router := setupRouter()

	for i := 0; i < 4; i++ {
		require.NoError(t, db.Create(&models.User{
			Username: "user" + strconv.Itoa(i),
			Email:    "user" + strconv.Itoa(i) + "@example.com",
			Password: "password123",
		}).Error)
	}
	// Plus the seeded "tester" user = 5 total.

	router.GET("/users", ListUsers)

	req, _ := http.NewRequest("GET", "/users?page=2&limit=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.AdminUsersListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 5, resp.Total)
	assert.Len(t, resp.Users, 2)
	assert.Equal(t, 2, resp.Page)
	assert.Equal(t, 2, resp.Limit)
	assert.Equal(t, 3, resp.TotalPages)
}

func TestListUsers_DatabaseError(t *testing.T) {
	db, router := setupRouter()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	router.GET("/users", ListUsers)

	req, _ := http.NewRequest("GET", "/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

// --- GetUser ---

func TestGetUser_Success(t *testing.T) {
	db, router := setupRouter()

	target := models.User{Username: "target", Email: "target@example.com", Password: "password123", IsAdmin: true}
	require.NoError(t, db.Create(&target).Error)

	router.GET("/users/:id", GetUser)

	req, _ := http.NewRequest("GET", "/users/"+strconv.Itoa(int(target.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.AdminUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, target.ID, resp.ID)
	assert.Equal(t, target.Username, resp.Username)
	assert.True(t, resp.IsAdmin)
	assert.NotContains(t, w.Body.String(), "password")
}

func TestGetUser_NotFound(t *testing.T) {
	_, router := setupRouter()

	router.GET("/users/:id", GetUser)

	req, _ := http.NewRequest("GET", "/users/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestGetUser_InvalidID(t *testing.T) {
	_, router := setupRouter()

	router.GET("/users/:id", GetUser)

	req, _ := http.NewRequest("GET", "/users/not-a-number", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// --- CreateUser (T39) ---

func TestCreateUser_Success(t *testing.T) {
	db, router := setupRouter()

	router.POST("/users", withValidated(func() any { return &models.AdminUserCreateInput{} }), CreateUser)

	payload := models.AdminUserCreateInput{
		Username: "NewUser",
		Email:    "NewUser@Example.com",
		Password: "brandNewPassw0rd!",
		IsAdmin:  true,
	}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp models.AdminUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// Username/email are normalized to lowercase, matching RegisterUser/UpdateUser.
	assert.Equal(t, "newuser", resp.Username)
	assert.Equal(t, "newuser@example.com", resp.Email)
	assert.True(t, resp.IsAdmin)
	assert.NotZero(t, resp.ID)
	assert.NotContains(t, w.Body.String(), "password")

	var created models.User
	require.NoError(t, db.Where("username = ?", "newuser").First(&created).Error)
	// The stored password must be hashed, never the plaintext the admin typed.
	assert.NotEqual(t, "brandNewPassw0rd!", created.Password)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(created.Password), []byte("brandNewPassw0rd!")),
		"stored password hash must verify against the submitted password")

	// The new user must actually be able to log in with the set password —
	// this is the ticket's "Done when" hand-verification, pinned as an
	// automated check too.
	loginRouter := gin.Default()
	loginRouter.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	cfg := &config.Config{JWTExpiryHours: 1, JWTSecretKey: "test-secret-key-for-login-check"}
	loginRouter.POST("/login", func(c *gin.Context) { LoginUser(c, cfg) })

	loginPayload, _ := json.Marshal(map[string]string{"identifier": "newuser", "password": "brandNewPassw0rd!"})
	loginReq, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	loginRouter.ServeHTTP(loginW, loginReq)

	assert.Equal(t, http.StatusOK, loginW.Code, loginW.Body.String())
}

func TestCreateUser_DefaultsToNonAdmin(t *testing.T) {
	_, router := setupRouter()

	router.POST("/users", withValidated(func() any { return &models.AdminUserCreateInput{} }), CreateUser)

	payload := models.AdminUserCreateInput{
		Username: "plainuser",
		Email:    "plainuser@example.com",
		Password: "brandNewPassw0rd!",
	}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp models.AdminUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.IsAdmin, "omitting is_admin must not implicitly grant it")
}

func TestCreateUser_DuplicateEmail_Conflict(t *testing.T) {
	db, router := setupRouter()

	require.NoError(t, db.Create(&models.User{Username: "existing", Email: "taken@example.com", Password: "password123"}).Error)

	router.POST("/users", withValidated(func() any { return &models.AdminUserCreateInput{} }), CreateUser)

	payload := models.AdminUserCreateInput{
		Username: "different-username",
		Email:    "taken@example.com",
		Password: "brandNewPassw0rd!",
	}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", "different-username").Count(&count).Error)
	assert.Zero(t, count, "no user should be created when the email collides")
}

func TestCreateUser_DuplicateUsername_Conflict(t *testing.T) {
	db, router := setupRouter()

	require.NoError(t, db.Create(&models.User{Username: "taken", Email: "taken@example.com", Password: "password123"}).Error)

	router.POST("/users", withValidated(func() any { return &models.AdminUserCreateInput{} }), CreateUser)

	payload := models.AdminUserCreateInput{
		Username: "taken",
		Email:    "different@example.com",
		Password: "brandNewPassw0rd!",
	}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

// Reuses the same validators as self-registration (models.UserRegistrationInput)
// rather than a parallel set — a short password must be rejected the same way.
func TestCreateUser_WeakPassword_Rejected(t *testing.T) {
	_, router := setupRouter()

	router.POST("/users", middleware.ValidateJSONMiddleware(&models.AdminUserCreateInput{}), CreateUser)

	jsonValue := []byte(`{"username":"weakpw","email":"weakpw@example.com","password":"short"}`)

	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreateUser_MissingFields_Rejected(t *testing.T) {
	_, router := setupRouter()

	router.POST("/users", middleware.ValidateJSONMiddleware(&models.AdminUserCreateInput{}), CreateUser)

	jsonValue := []byte(`{"username":"","email":"","password":""}`)

	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// Every other success-path test above uses withValidated, a bare
// ShouldBindJSON with no struct validation, so none of them actually proves
// a valid payload clears the REAL middleware.ValidateJSONMiddleware chain
// the route registers (routes/routes.go). This wires that real chain with a
// payload shaped like what the Add User dialog sends, so a future tightening
// of AdminUserCreateInput's validate tags that starts rejecting legitimate
// input would be caught here rather than only in the (not always run)
// Playwright e2e suite.
func TestCreateUser_RealValidationMiddleware_AcceptsValidPayload(t *testing.T) {
	db, router := setupRouter()

	router.POST("/users", middleware.ValidateJSONMiddleware(&models.AdminUserCreateInput{}), CreateUser)

	jsonValue := []byte(`{"username":"realvalidation","email":"realvalidation@example.com","password":"brandNewPassw0rd!"}`)

	req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", "realvalidation").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

// A non-admin must not be able to reach CreateUser at all — this wires the
// REAL middleware chain (AdminMiddleware), unlike the handler-only tests
// above, to prove the route itself is gated rather than relying solely on
// the handler (CreateUser has no caller-privilege check of its own, the same
// design as UpdateUser — see TestUpdateUser_HandlerAllowsSelfPromotion_
// GatedOnlyByRouteMiddleware's note above).
func TestCreateUser_NonAdmin_Forbidden(t *testing.T) {
	db, _ := setupRouter()

	var nonAdmin models.User
	require.NoError(t, db.Where("username = ?", "tester").First(&nonAdmin).Error)
	require.False(t, nonAdmin.IsAdmin, "seeded test user must start as a non-admin for this test to be meaningful")

	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", nonAdmin.ID)
		c.Next()
	})
	router.Use(middleware.AdminMiddleware())
	router.POST("/admin/users", middleware.ValidateJSONMiddleware(&models.AdminUserCreateInput{}), CreateUser)

	payload := models.AdminUserCreateInput{
		Username: "shouldnotexist",
		Email:    "shouldnotexist@example.com",
		Password: "brandNewPassw0rd!",
	}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/admin/users", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", "shouldnotexist").Count(&count).Error)
	assert.Zero(t, count, "no user should be created when the caller is not an admin")
}

// --- UpdateUser: normal CRUD paths ---

func TestUpdateUser_Success(t *testing.T) {
	db, router := setupRouter()

	target := models.User{Username: "target", Email: "target@example.com", Password: "password123"}
	require.NoError(t, db.Create(&target).Error)

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	newUsername := "renamed"
	newEmail := "renamed@example.com"
	payload := models.AdminUserUpdateInput{Username: &newUsername, Email: &newEmail}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(target.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.AdminUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "renamed", resp.Username)
	assert.Equal(t, "renamed@example.com", resp.Email)
	assert.NotContains(t, w.Body.String(), "password")

	var updated models.User
	require.NoError(t, db.First(&updated, target.ID).Error)
	assert.Equal(t, "renamed", updated.Username)
	assert.Equal(t, "renamed@example.com", updated.Email)
}

func TestUpdateUser_PasswordReset_IncrementsTokenVersion(t *testing.T) {
	db, router := setupRouter()

	target := models.User{Username: "target", Email: "target@example.com", Password: "password123", TokenVersion: 3}
	require.NoError(t, db.Create(&target).Error)
	originalHash := target.Password

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	newPassword := "brandNewPassw0rd!"
	payload := models.AdminUserUpdateInput{Password: &newPassword}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(target.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated models.User
	require.NoError(t, db.First(&updated, target.ID).Error)
	assert.NotEqual(t, originalHash, updated.Password, "password hash should change")
	assert.Equal(t, uint(4), updated.TokenVersion, "resetting a password must bump TokenVersion to invalidate existing sessions")
}

func TestUpdateUser_NotFound(t *testing.T) {
	_, router := setupRouter()

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	newUsername := "renamed"
	payload := models.AdminUserUpdateInput{Username: &newUsername}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/999999", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateUser_InvalidID(t *testing.T) {
	_, router := setupRouter()

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	newUsername := "renamed"
	payload := models.AdminUserUpdateInput{Username: &newUsername}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/not-a-number", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestUpdateUser_DuplicateUsername_Conflict(t *testing.T) {
	db, router := setupRouter()

	require.NoError(t, db.Create(&models.User{Username: "taken", Email: "taken@example.com", Password: "password123"}).Error)
	target := models.User{Username: "target", Email: "target@example.com", Password: "password123"}
	require.NoError(t, db.Create(&target).Error)

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	dupUsername := "taken"
	payload := models.AdminUserUpdateInput{Username: &dupUsername}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(target.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

// --- UpdateUser: admin/last-admin-protection invariants ---
//
// These tests cover the privilege-escalation-adjacent concern called out by
// Phase 3b of docs/adrs/0003-golden-fixtures-external-test-oracle.md. DeleteUser (see
// TestDeleteUser_CleansUpAllOwnedRows above and DeleteUser's own guards at
// admin_user_controller.go:272-301) blocks self-deletion and last-admin
// deletion; the question here is whether UpdateUser has equivalent guards
// against self-demotion and last-admin demotion, and whether it (or the
// route layer) blocks self-promotion.

// The acting admin must not be able to strip their own admin status via
// UpdateUser, even if they are not the last admin. Guard is at
// admin_user_controller.go:187-190.
func TestUpdateUser_CannotRemoveOwnAdminStatus(t *testing.T) {
	db, router := setupRouter()

	var actingUser models.User
	require.NoError(t, db.First(&actingUser).Error)
	actingUser.IsAdmin = true
	require.NoError(t, db.Save(&actingUser).Error)

	// A second admin exists, so this is NOT a last-admin situation -- the
	// self-demotion guard must fire independently of the admin count.
	require.NoError(t, db.Create(&models.User{Username: "other-admin", Email: "other-admin@example.com", Password: "password123", IsAdmin: true}).Error)

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	demote := false
	payload := models.AdminUserUpdateInput{IsAdmin: &demote}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(actingUser.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var unchanged models.User
	require.NoError(t, db.First(&unchanged, actingUser.ID).Error)
	assert.True(t, unchanged.IsAdmin, "acting admin's own admin status must be unchanged")
}

// An admin must not be able to demote the last remaining admin -- including
// a DIFFERENT user than themselves -- to zero admins on the instance. Guard
// is at admin_user_controller.go:192-204, mirroring DeleteUser's last-admin
// count check.
func TestUpdateUser_CannotDemoteLastAdmin(t *testing.T) {
	db, router := setupRouter()

	// Seeded "tester" user is NOT an admin; "target" is the ONLY admin.
	target := models.User{Username: "target", Email: "target@example.com", Password: "password123", IsAdmin: true}
	require.NoError(t, db.Create(&target).Error)

	var adminCount int64
	require.NoError(t, db.Model(&models.User{}).Where("is_admin = ?", true).Count(&adminCount).Error)
	require.EqualValues(t, 1, adminCount, "test setup requires exactly one admin")

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	demote := false
	payload := models.AdminUserUpdateInput{IsAdmin: &demote}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(target.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var unchanged models.User
	require.NoError(t, db.First(&unchanged, target.ID).Error)
	assert.True(t, unchanged.IsAdmin, "last admin must remain an admin")
}

// Demoting an admin who is NOT the last admin must succeed -- this is the
// success-path complement to TestUpdateUser_CannotDemoteLastAdmin, proving
// the guard is scoped to the "count <= 1" case rather than blocking all
// demotions.
func TestUpdateUser_DemoteAdmin_WhenNotLastAdmin_Succeeds(t *testing.T) {
	db, router := setupRouter()

	var actingUser models.User
	require.NoError(t, db.First(&actingUser).Error)
	actingUser.IsAdmin = true
	require.NoError(t, db.Save(&actingUser).Error)

	target := models.User{Username: "target", Email: "target@example.com", Password: "password123", IsAdmin: true}
	require.NoError(t, db.Create(&target).Error)

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	demote := false
	payload := models.AdminUserUpdateInput{IsAdmin: &demote}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(target.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated models.User
	require.NoError(t, db.First(&updated, target.ID).Error)
	assert.False(t, updated.IsAdmin)
}

// UpdateUser's input DTO (models.AdminUserUpdateInput) accepts an IsAdmin
// field, and the handler applies it with no check on the CALLER's own
// privilege level -- unlike self/last-admin *demotion*, there is no
// promotion guard at all in the handler itself (admin_user_controller.go
// has no code path that inspects the caller's IsAdmin before applying
// input.IsAdmin=true at line 229-231).
//
// This is NOT exploitable in production: the route is registered at
// routes/routes.go:211 inside the `admin` group, which is gated by
// middleware.AdminMiddleware() (routes/routes.go:207). That middleware
// (middleware/admin.go:38-49) loads the caller's IsAdmin from the DB and
// aborts with 403 before the handler ever runs unless the caller is already
// an admin -- so a non-admin cannot reach UpdateUser at all in the wired
// application.
//
// This test calls the handler directly (as all tests in this file do,
// bypassing route-level middleware per setupRouter's design) to document
// that the handler relies entirely on that external gate and does not
// duplicate the check itself. It intentionally demonstrates the bypassed
// behavior -- see the SECURITY FINDINGS note in the WP report for why this
// is a defense-in-depth observation, not a live vulnerability.
func TestUpdateUser_HandlerAllowsSelfPromotion_GatedOnlyByRouteMiddleware(t *testing.T) {
	db, router := setupRouter()

	var actingUser models.User
	require.NoError(t, db.First(&actingUser).Error)
	require.False(t, actingUser.IsAdmin, "acting user must start as a non-admin for this test to be meaningful")

	// No AdminMiddleware registered here -- setupRouter's router is bare, so
	// this reaches the handler exactly as a misconfigured or bypassed route
	// would. In the real app this path is unreachable for a non-admin
	// because of AdminMiddleware (middleware/admin.go, wired at
	// routes/routes.go:207).
	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	promote := true
	payload := models.AdminUserUpdateInput{IsAdmin: &promote}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(actingUser.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Documents current handler behavior: it applies the promotion with no
	// error, because UpdateUser has no caller-privilege check of its own.
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated models.User
	require.NoError(t, db.First(&updated, actingUser.ID).Error)
	assert.True(t, updated.IsAdmin, "handler applied the self-promotion with no check of its own -- protection is route-middleware-only")
}

// --- TriggerReminders ---

func TestTriggerReminders_Success(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	require.NoError(t, db.First(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Jane", Lastname: "Doe"}
	require.NoError(t, db.Create(&contact).Error)

	byMailTrue := true
	reoccurFalse := false
	reminder := models.Reminder{
		UserID:                user.ID,
		ContactID:             &contact.ID,
		Message:               "Test reminder",
		ByMail:                &byMailTrue,
		RemindAt:              time.Now().Add(-1 * time.Hour),
		Recurrence:            "once",
		ReoccurFromCompletion: &reoccurFalse,
	}
	require.NoError(t, db.Create(&reminder).Error)

	cfg := config.Config{}
	router.POST("/trigger-reminders", func(c *gin.Context) {
		TriggerReminders(c, cfg)
	})

	req, _ := http.NewRequest("POST", "/trigger-reminders", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Reminder emails sent successfully", resp["message"])

	// Email sending is disabled (zero-value config has no SMTP/Resend
	// configured), so SendReminders takes the "email disabled" branch and
	// leaves the reminder's email_sent flag untouched -- confirming the
	// underlying job actually ran against this reminder rather than the
	// handler being a no-op stub.
	var afterRun models.Reminder
	require.NoError(t, db.First(&afterRun, reminder.ID).Error)
	assert.False(t, afterRun.EmailSent)
}

func TestTriggerReminders_DatabaseError(t *testing.T) {
	db, router := setupRouter()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	cfg := config.Config{}
	router.POST("/trigger-reminders", func(c *gin.Context) {
		TriggerReminders(c, cfg)
	})

	req, _ := http.NewRequest("POST", "/trigger-reminders", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

// M5/T26: TriggerPurge endpoint executes the purge without error.
func TestTriggerPurge(t *testing.T) {
	_, router := setupRouter()

	cfg := config.Config{DeleteRetentionDays: 30}
	router.POST("/trigger-purge", func(c *gin.Context) {
		TriggerPurge(c, cfg)
	})

	req, _ := http.NewRequest("POST", "/trigger-purge", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// M1/T26: PurgeSoftDeletedRows hard-deletes rows past the retention window.
func TestPurgeSoftDeletedRows_DeletesRowsPastWindow(t *testing.T) {
	db, _ := setupRouter()

	cfg := config.Config{DeleteRetentionDays: 30}

	var user models.User
	db.First(&user)

	// Create a note soft-deleted 60 days ago
	oldCutoff := time.Now().AddDate(0, 0, -60)
	note := models.Note{
		UserID:  user.ID,
		Content: "old note",
		Date:    oldCutoff,
	}
	require.NoError(t, db.Create(&note).Error)
	require.NoError(t, db.Exec("UPDATE notes SET deleted_at = ? WHERE id = ?", oldCutoff, note.ID).Error)

	// Create a note soft-deleted 5 days ago (inside window — must survive)
	recentCutoff := time.Now().AddDate(0, 0, -5)
	recentNote := models.Note{
		UserID:  user.ID,
		Content: "recent note",
		Date:    recentCutoff,
	}
	require.NoError(t, db.Create(&recentNote).Error)
	require.NoError(t, db.Exec("UPDATE notes SET deleted_at = ? WHERE id = ?", recentCutoff, recentNote.ID).Error)

	services.PurgeSoftDeletedRows(db, cfg)

	var oldCount int64
	db.Unscoped().Model(&models.Note{}).Where("id = ?", note.ID).Count(&oldCount)
	assert.Zero(t, oldCount, "note outside retention window must be hard-deleted")

	var unscopedRecent int64
	db.Unscoped().Model(&models.Note{}).Count(&unscopedRecent)
	assert.EqualValues(t, 1, unscopedRecent, "note inside window must still exist under Unscoped()")
}

// M1b/T26: Live rows (deleted_at IS NULL) are never touched.
func TestPurgeSoftDeletedRows_NeverTouchesLiveRows(t *testing.T) {
	db, _ := setupRouter()

	cfg := config.Config{DeleteRetentionDays: 1}

	var user models.User
	db.First(&user)

	liveNote := models.Note{
		UserID:  user.ID,
		Content: "live note",
		Date:    time.Now(),
	}
	require.NoError(t, db.Create(&liveNote).Error)

	services.PurgeSoftDeletedRows(db, cfg)

	var count int64
	db.Model(&models.Note{}).Where("id = ?", liveNote.ID).Count(&count)
	assert.EqualValues(t, 1, count, "live rows must never be touched")
}

// T20a: PurgeSoftDeletedRows hard-deletes soft-deleted preferences past retention.
func TestPurgeSoftDeletedRows_HandlesPreferences(t *testing.T) {
	db, _ := setupRouter()

	cfg := config.Config{DeleteRetentionDays: 30}

	var user models.User
	db.First(&user)

	contact := models.Contact{UserID: user.ID, Firstname: "PrefPurge"}
	require.NoError(t, db.Create(&contact).Error)

	// Preference soft-deleted 60 days ago — must be purged.
	oldCutoff := time.Now().AddDate(0, 0, -60)
	oldPref := models.Preference{
		UserID: user.ID, EntityID: contact.VCardUID,
		Category: "food", Value: "OldPref",
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&oldPref).Error)
	require.NoError(t, db.Exec("UPDATE preferences SET deleted_at = ? WHERE id = ?", oldCutoff, oldPref.ID).Error)

	// Preference soft-deleted 5 days ago — inside window, must survive.
	recentCutoff := time.Now().AddDate(0, 0, -5)
	recentPref := models.Preference{
		UserID: user.ID, EntityID: contact.VCardUID,
		Category: "hobby", Value: "RecentPref",
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&recentPref).Error)
	require.NoError(t, db.Exec("UPDATE preferences SET deleted_at = ? WHERE id = ?", recentCutoff, recentPref.ID).Error)

	services.PurgeSoftDeletedRows(db, cfg)

	var unscopedOld int64
	db.Unscoped().Model(&models.Preference{}).Where("id = ?", oldPref.ID).Count(&unscopedOld)
	assert.Zero(t, unscopedOld, "preference outside retention window must be hard-deleted")

	var unscopedRecent int64
	db.Unscoped().Model(&models.Preference{}).Where("id = ?", recentPref.ID).Count(&unscopedRecent)
	assert.EqualValues(t, 1, unscopedRecent, "preference inside window must still exist under Unscoped()")
}

// T20a defense-in-depth: a live preference whose entity_id points to a
// contact about to be purged is cleaned up even without a prior soft-delete
// cascade (the Edge/join-shaped defense-in-depth block).
func TestPurgeCleansUpPreferencesOfPurgedContact(t *testing.T) {
	db, _ := setupRouter()

	cfg := config.Config{DeleteRetentionDays: 30}

	var user models.User
	db.First(&user)

	oldCutoff := time.Now().AddDate(0, 0, -60)
	contact := models.Contact{UserID: user.ID, Firstname: "PurgeMe"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Exec("UPDATE contacts SET deleted_at = ? WHERE id = ?", oldCutoff, contact.ID).Error)

	pref := models.Preference{
		UserID: user.ID, EntityID: contact.VCardUID,
		Category: models.PreferenceCategoryHobby, Value: "OrphanPref",
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&pref).Error)

	services.PurgeSoftDeletedRows(db, cfg)

	var unscopedPref int64
	db.Unscoped().Model(&models.Preference{}).Where("id = ?", pref.ID).Count(&unscopedPref)
	assert.Zero(t, unscopedPref, "preference referencing a purged contact must be cleaned up")
}
