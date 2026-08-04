package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// calendar_controller.go had zero test coverage before this file (found in
// the pre-v0.2.0 e2e/test-coverage audit): the underlying sync engine
// (services/calendar_sync_service_test.go) and its SSRF-blocking dialer are
// both thoroughly tested, but the HTTP layer wrapping them -- ownership
// scoping, credential-on-create/update handling, input validation, the
// subscription-count limit -- was not. These tests mirror
// contact_subscription_controller_test.go's coverage of the structurally
// identical ContactSubscription surface.

func seedCalendarSubscription(db *gorm.DB, userID uint, url string) models.CalendarSubscription {
	sub := models.CalendarSubscription{
		UserID:      userID,
		Name:        "Test calendar",
		URL:         url,
		SyncEnabled: true,
	}
	db.Create(&sub)
	return sub
}

func TestListCalendarSubscriptions(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.GET("/calendars", ListCalendarSubscriptions)

	seedCalendarSubscription(db, user.ID, "https://example.com/calendars/a/")
	seedCalendarSubscription(db, user.ID, "https://example.com/calendars/b/")

	req, _ := http.NewRequest("GET", "/calendars", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Len(t, body["calendars"], 2)
}

func TestCreateCalendarSubscription(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.POST("/calendars", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), CreateCalendarSubscription)

	input := models.CalendarSubscriptionInput{
		Name:     "My Nextcloud calendar",
		URL:      "https://nextcloud.example.com/remote.php/dav/calendars/alice/personal/",
		Username: "alice",
		Password: "hunter2",
	}
	body, _ := json.Marshal(input)

	req, _ := http.NewRequest("POST", "/calendars", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp models.CalendarSubscriptionResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "My Nextcloud calendar", resp.Name)
	assert.True(t, resp.HasPassword)
	assert.True(t, resp.SyncEnabled)
	assert.Equal(t, 5, resp.PastDays, "default PastDays")
	assert.Equal(t, 10, resp.FutureDays, "default FutureDays")

	var stored models.CalendarSubscription
	require.NoError(t, db.First(&stored, resp.ID).Error)
	assert.NotEmpty(t, stored.PasswordEncrypted)
	assert.NotEqual(t, "hunter2", stored.PasswordEncrypted, "password must be encrypted at rest")
}

func TestCreateCalendarSubscriptionCustomWindow(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.POST("/calendars", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), CreateCalendarSubscription)

	past, future := 30, 90
	input := models.CalendarSubscriptionInput{Name: "Wide window", URL: "https://example.com/cal/", PastDays: &past, FutureDays: &future}
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", "/calendars", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp models.CalendarSubscriptionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 30, resp.PastDays)
	assert.Equal(t, 90, resp.FutureDays)
}

func TestCreateCalendarSubscriptionRejectsInvalidURL(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.POST("/calendars", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), CreateCalendarSubscription)

	input := models.CalendarSubscriptionInput{Name: "Bad", URL: "ftp://example.com/x"}
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", "/calendars", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateCalendarSubscriptionLimit(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.POST("/calendars", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), CreateCalendarSubscription)

	for i := 0; i < maxCalendarSubscriptionsPerUser; i++ {
		seedCalendarSubscription(db, user.ID, "https://example.com/calendars/"+strconv.Itoa(i)+"/")
	}

	input := models.CalendarSubscriptionInput{Name: "One Too Many", URL: "https://example.com/calendars/extra/"}
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", "/calendars", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUpdateCalendarSubscription(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.PUT("/calendars/:id", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), UpdateCalendarSubscription)

	sub := seedCalendarSubscription(db, user.ID, "https://example.com/calendars/a/")

	falseVal := false
	update := models.CalendarSubscriptionInput{
		Name:        "Renamed",
		URL:         "https://example.com/calendars/new/",
		SyncEnabled: &falseVal,
	}
	body, _ := json.Marshal(update)
	req, _ := http.NewRequest("PUT", "/calendars/"+strconv.Itoa(int(sub.ID)), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.CalendarSubscriptionResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "Renamed", resp.Name)
	assert.False(t, resp.SyncEnabled)

	var stored models.CalendarSubscription
	require.NoError(t, db.First(&stored, sub.ID).Error)
	assert.Equal(t, "https://example.com/calendars/new/", stored.URL)
}

func TestUpdateCalendarSubscription_ClearPassword(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	cfg := config.Config{JWTSecretKey: "test-jwt-secret-key-with-32-chars!!"}
	encrypted, err := services.EncryptCredential(cfg.JWTSecretKey, "hunter2")
	require.NoError(t, err)
	sub := models.CalendarSubscription{UserID: user.ID, Name: "Has Password", URL: "https://example.com/a/", PasswordEncrypted: encrypted, SyncEnabled: true}
	require.NoError(t, db.Create(&sub).Error)

	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) { c.Set("cfg", cfg); c.Next() })
	router.PUT("/calendars/:id", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), UpdateCalendarSubscription)

	input := models.CalendarSubscriptionInput{Name: "Has Password", URL: sub.URL, ClearPassword: true}
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("PUT", "/calendars/"+strconv.Itoa(int(sub.ID)), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.CalendarSubscriptionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.HasPassword)

	var stored models.CalendarSubscription
	require.NoError(t, db.First(&stored, sub.ID).Error)
	assert.Empty(t, stored.PasswordEncrypted)
}

func TestUpdateCalendarSubscription_ReplacesPassword(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	cfg := config.Config{JWTSecretKey: "test-jwt-secret-key-with-32-chars!!"}
	encrypted, err := services.EncryptCredential(cfg.JWTSecretKey, "oldpass")
	require.NoError(t, err)
	sub := models.CalendarSubscription{UserID: user.ID, Name: "Sub", URL: "https://example.com/a/", PasswordEncrypted: encrypted, SyncEnabled: true}
	require.NoError(t, db.Create(&sub).Error)

	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) { c.Set("cfg", cfg); c.Next() })
	router.PUT("/calendars/:id", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), UpdateCalendarSubscription)

	input := models.CalendarSubscriptionInput{Name: "Sub", URL: sub.URL, Password: "newpass"}
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("PUT", "/calendars/"+strconv.Itoa(int(sub.ID)), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.CalendarSubscription
	require.NoError(t, db.First(&stored, sub.ID).Error)
	assert.NotEqual(t, encrypted, stored.PasswordEncrypted)
	decrypted, err := services.DecryptCredential(cfg.JWTSecretKey, stored.PasswordEncrypted)
	require.NoError(t, err)
	assert.Equal(t, "newpass", decrypted)
}

func TestUpdateCalendarSubscription_RejectsInvalidURL(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	sub := seedCalendarSubscription(db, user.ID, "https://example.com/a/")

	router := routerForUser(db, user.ID)
	router.PUT("/calendars/:id", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), UpdateCalendarSubscription)

	input := models.CalendarSubscriptionInput{Name: "Sub", URL: "ftp://not-http.example.com"}
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("PUT", "/calendars/"+strconv.Itoa(int(sub.ID)), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestDeleteCalendarSubscription(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.DELETE("/calendars/:id", DeleteCalendarSubscription)

	sub := seedCalendarSubscription(db, user.ID, "https://example.com/a/")
	require.NoError(t, db.Create(&models.CalendarEventLink{SubscriptionID: sub.ID, UserID: user.ID, UID: "evt-1", ActivityID: 1, ContentHash: "h"}).Error)

	req, _ := http.NewRequest("DELETE", "/calendars/"+strconv.Itoa(int(sub.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var deleted models.CalendarSubscription
	result := db.Unscoped().First(&deleted, sub.ID)
	require.NoError(t, result.Error)
	assert.NotNil(t, deleted.DeletedAt)

	var linkCount int64
	db.Model(&models.CalendarEventLink{}).Where("subscription_id = ?", sub.ID).Count(&linkCount)
	assert.Equal(t, int64(0), linkCount, "event links should be cleaned up on delete")
}

func TestCalendarSubscriptionUserIsolation(t *testing.T) {
	db, _ := setupRouter()
	var user1 models.User
	db.First(&user1)

	user2 := models.User{Username: "other-cal", Password: "pass", Email: "other-cal@example.com"}
	db.Create(&user2)

	sub := seedCalendarSubscription(db, user1.ID, "https://example.com/a/")

	router := routerForUser(db, user2.ID)
	router.DELETE("/calendars/:id", DeleteCalendarSubscription)

	req, _ := http.NewRequest("DELETE", "/calendars/"+strconv.Itoa(int(sub.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// --- SyncCalendarSubscription: the manual-sync HTTP route ---
//
// services/calendar_sync_service_test.go already thoroughly tests
// CalendarSyncService.SyncSubscription's own logic directly; these tests
// instead prove the controller/route wiring -- that hitting
// POST /calendars/:id/sync over real gin routing actually invokes the
// service, reflects its result in the HTTP response, and persists the
// subscription's LastSyncStatus/LastSyncedAt bookkeeping to the database.

func calendarMultistatusResponseForTest(calendars ...string) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	sb.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` + "\n")
	for i, cal := range calendars {
		escaped := strings.ReplaceAll(cal, "&", "&amp;")
		escaped = strings.ReplaceAll(escaped, "<", "&lt;")
		fmt.Fprintf(&sb, `<d:response><d:href>/calendars/test/event%d.ics</d:href>
<d:propstat><d:prop><c:calendar-data>%s</c:calendar-data></d:prop>
<d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, i, escaped)
	}
	sb.WriteString(`</d:multistatus>`)
	return sb.String()
}

func TestSyncCalendarSubscription_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, calendarMultistatusResponseForTest(
			"BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:route-event-1\nSUMMARY:Route Sync Test\nDTSTART:"+
				calendarICalDateForTest(2)+"\nEND:VEVENT\nEND:VCALENDAR",
		))
	}))
	defer server.Close()

	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	cfg := config.Config{JWTSecretKey: "test-jwt-secret-key-with-32-chars!!"}
	sub := models.CalendarSubscription{
		UserID: user.ID, Name: "Route sync test", URL: server.URL + "/calendars/test/",
		SyncEnabled: true, PastDays: 5, FutureDays: 10,
	}
	require.NoError(t, db.Create(&sub).Error)

	router := routerForContactSync(db, user.ID, cfg)
	router.POST("/calendars/:id/sync", SyncCalendarSubscription)

	req, _ := http.NewRequest("POST", "/calendars/"+strconv.Itoa(int(sub.ID))+"/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["created"], "response body: %s", w.Body.String())

	var stored models.CalendarSubscription
	require.NoError(t, db.First(&stored, sub.ID).Error)
	assert.Equal(t, models.CalendarSyncStatusSuccess, stored.LastSyncStatus)
	assert.NotNil(t, stored.LastSyncedAt, "LastSyncedAt should be set by hitting the real route")

	var activity models.Activity
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&activity).Error)
	assert.Equal(t, "Route Sync Test", activity.Title)
}

func TestSyncCalendarSubscription_UnauthorizedReflectsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	cfg := config.Config{JWTSecretKey: "test-jwt-secret-key-with-32-chars!!"}
	encrypted, err := services.EncryptCredential(cfg.JWTSecretKey, "wrongpass")
	require.NoError(t, err)

	sub := models.CalendarSubscription{
		UserID: user.ID, Name: "Route sync failure test", URL: server.URL + "/calendars/test/",
		Username: "caluser", PasswordEncrypted: encrypted, SyncEnabled: true, PastDays: 5, FutureDays: 10,
	}
	require.NoError(t, db.Create(&sub).Error)

	router := routerForContactSync(db, user.ID, cfg)
	router.POST("/calendars/:id/sync", SyncCalendarSubscription)

	req, _ := http.NewRequest("POST", "/calendars/"+strconv.Itoa(int(sub.ID))+"/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// calendarSyncError maps ErrCalendarUnauthorized to apperrors.ErrExternal,
	// which carries http.StatusServiceUnavailable.
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())

	var stored models.CalendarSubscription
	require.NoError(t, db.First(&stored, sub.ID).Error)
	assert.Equal(t, models.CalendarSyncStatusError, stored.LastSyncStatus, "the failed sync must still be recorded on the subscription")
	assert.NotEmpty(t, stored.LastSyncError)
	assert.NotNil(t, stored.LastSyncedAt)
}

func TestSyncCalendarSubscription_NotFoundForOtherUser(t *testing.T) {
	db, _ := setupRouter()
	var user1 models.User
	db.First(&user1)

	user2 := models.User{Username: "other-cal-sync", Password: "pass", Email: "othercalsync@example.com"}
	require.NoError(t, db.Create(&user2).Error)

	sub := seedCalendarSubscription(db, user1.ID, "https://example.com/calendars/a/")

	cfg := config.Config{JWTSecretKey: "test-jwt-secret-key-with-32-chars!!"}
	router := routerForContactSync(db, user2.ID, cfg)
	router.POST("/calendars/:id/sync", SyncCalendarSubscription)

	req, _ := http.NewRequest("POST", "/calendars/"+strconv.Itoa(int(sub.ID))+"/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestCalendarSyncError_AllSentinelsMapped exercises every branch of
// calendarSyncError directly (pure function, no HTTP layer needed) -- only
// ErrCalendarUnauthorized was reachable through the HTTP-level tests above.
func TestCalendarSyncError_AllSentinelsMapped(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"InvalidURL", services.ErrCalendarInvalidURL, http.StatusBadRequest},
		{"Unauthorized", services.ErrCalendarUnauthorized, http.StatusServiceUnavailable},
		{"NotFound", services.ErrCalendarNotFound, http.StatusServiceUnavailable},
		{"PrivateAddress", services.ErrCalendarPrivateAddress, http.StatusServiceUnavailable},
		{"TooLarge", services.ErrCalendarTooLarge, http.StatusServiceUnavailable},
		{"InvalidData", services.ErrCalendarInvalidData, http.StatusServiceUnavailable},
		{"Unreachable", services.ErrCalendarUnreachable, http.StatusServiceUnavailable},
		{"UnknownError_FallsBackToOperationFailed", fmt.Errorf("some other failure"), http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appErr := calendarSyncError(tc.err)
			require.NotNil(t, appErr)
			assert.Equal(t, tc.wantStatus, appErr.HTTPStatus, "status for %v", tc.err)
		})
	}
}

// TestCalendarSubscriptionHandlers_NoAuth_Unauthorized exercises the
// currentUserID !ok early-return every handler in this file checks first.
func TestCalendarSubscriptionHandlers_NoAuth_Unauthorized(t *testing.T) {
	db, _ := setupRouter()
	router := routerWithoutAuth(db)
	router.GET("/calendars", ListCalendarSubscriptions)
	router.POST("/calendars", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), CreateCalendarSubscription)
	router.PUT("/calendars/:id", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), UpdateCalendarSubscription)
	router.DELETE("/calendars/:id", DeleteCalendarSubscription)
	router.POST("/calendars/:id/sync", SyncCalendarSubscription)

	for _, req := range []*http.Request{
		mustRequest(t, "GET", "/calendars", nil),
		mustRequest(t, "POST", "/calendars", strings.NewReader(`{"name":"x","url":"https://example.com"}`)),
		mustRequest(t, "PUT", "/calendars/1", strings.NewReader(`{"name":"x","url":"https://example.com"}`)),
		mustRequest(t, "DELETE", "/calendars/1", nil),
		mustRequest(t, "POST", "/calendars/1/sync", nil),
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusOK, w.Code, "%s %s should not succeed without auth", req.Method, req.URL.Path)
		assert.NotEqual(t, http.StatusCreated, w.Code, "%s %s should not succeed without auth", req.Method, req.URL.Path)
	}
}

// TestFindCalendarSubscription_NonNumericID_InvalidInput exercises the
// strconv.ParseUint error branch in findCalendarSubscription.
func TestFindCalendarSubscription_NonNumericID_InvalidInput(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.DELETE("/calendars/:id", DeleteCalendarSubscription)

	req, _ := http.NewRequest("DELETE", "/calendars/not-a-number", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestListCalendarSubscriptions_DBError exercises the db.Find error branch by
// closing the underlying *sql.DB out from under gorm before the request.
func TestListCalendarSubscriptions_DBError(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.GET("/calendars", ListCalendarSubscriptions)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	req, _ := http.NewRequest("GET", "/calendars", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestCreateCalendarSubscription_DBError exercises the subscription-count
// db.Count error branch (the first DB call CreateCalendarSubscription makes).
func TestCreateCalendarSubscription_DBError(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.POST("/calendars", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), CreateCalendarSubscription)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	input := models.CalendarSubscriptionInput{Name: "X", URL: "https://example.com/a/"}
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", "/calendars", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestCreateCalendarSubscription_RealValidation_MissingRequiredFields wires
// the real middleware.ValidateJSONMiddleware (not the withValidated bypass
// used elsewhere in this file) to prove the CalendarSubscriptionInput struct
// tags (name/url required) are actually enforced end-to-end.
func TestCreateCalendarSubscription_RealValidation_MissingRequiredFields(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	router := routerForUser(db, user.ID)
	router.Use(apperrors.ErrorHandlerMiddleware())
	router.POST("/calendars", middleware.ValidateJSONMiddleware(&models.CalendarSubscriptionInput{}), CreateCalendarSubscription)

	req, _ := http.NewRequest("POST", "/calendars", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var count int64
	db.Model(&models.CalendarSubscription{}).Count(&count)
	assert.Equal(t, int64(0), count, "no subscription should be created when validation fails")
}

// calendarICalDateForTest renders a UTC datetime a given number of days from
// now, so test events reliably fall inside the subscription's sync window.
func calendarICalDateForTest(daysFromNow int) string {
	return time.Now().UTC().AddDate(0, 0, daysFromNow).Format("20060102T150405Z")
}
