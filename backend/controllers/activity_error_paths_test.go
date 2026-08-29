package controllers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// activityRouterNoUser returns a router with db set but no userID, for the
// handler-level 401 branches.
func activityRouterNoUser(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	return router
}

func TestActivityHandlers_Unauthenticated(t *testing.T) {
	db, _ := setupRouter()
	router := activityRouterNoUser(db)
	router.POST("/activities", withValidated(func() any { return &models.ActivityInput{} }), CreateActivity)
	router.GET("/activities/:id", GetActivity)
	router.GET("/activities", GetActivities)
	router.PUT("/activities/:id", withValidated(func() any { return &models.ActivityInput{} }), UpdateActivity)
	router.DELETE("/activities/:id", DeleteActivity)
	router.GET("/contacts/:id/activities", GetActivitiesForContact)

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/activities", `{"title":"x","date":"2026-01-02T00:00:00Z"}`},
		{http.MethodGet, "/activities/1", ""},
		{http.MethodGet, "/activities", ""},
		{http.MethodPut, "/activities/1", `{"title":"x","date":"2026-01-02T00:00:00Z"}`},
		{http.MethodDelete, "/activities/1", ""},
		{http.MethodGet, "/contacts/1/activities", ""},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
	}
}

func TestCreateActivity_ValidationError(t *testing.T) {
	_, router := setupRouter()
	router.POST("/activities", middleware.ValidateJSONMiddleware(&models.ActivityInput{}), CreateActivity)

	// Missing required title/date fails validation.
	req, _ := http.NewRequest(http.MethodPost, "/activities", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreateActivity_MissingContactIsNotFound(t *testing.T) {
	db, router := setupRouter()
	router.POST("/activities", withValidated(func() any { return &models.ActivityInput{} }), CreateActivity)

	// A ContactIDs list naming a contact that does not exist (or belongs to
	// another user) must 404 rather than silently creating an activity.
	req, _ := http.NewRequest(http.MethodPost, "/activities", bytes.NewBufferString(`{"title":"x","date":"2026-01-02T00:00:00Z","contact_ids":[999999]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.Activity{}).Count(&count).Error)
	assert.Zero(t, count, "no activity may be created when a contact id is missing")
}

func TestCreateActivity_DatabaseError(t *testing.T) {
	db, _ := setupRouter()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	router := activityRouterNoUser(db)
	router.Use(func(c *gin.Context) { c.Set("userID", uint(1)); c.Next() })
	router.POST("/activities", withValidated(func() any { return &models.ActivityInput{} }), CreateActivity)

	req, _ := http.NewRequest(http.MethodPost, "/activities", bytes.NewBufferString(`{"title":"x","date":"2026-01-02T00:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestGetActivity_NotFound(t *testing.T) {
	_, router := setupRouter()
	router.GET("/activities/:id", GetActivity)
	req, _ := http.NewRequest(http.MethodGet, "/activities/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestGetActivity_InvalidID(t *testing.T) {
	_, router := setupRouter()
	router.GET("/activities/:id", GetActivity)
	req, _ := http.NewRequest(http.MethodGet, "/activities/not-a-number", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestGetActivities_InvalidCursor(t *testing.T) {
	_, router := setupRouter()
	router.GET("/activities", GetActivities)
	req, _ := http.NewRequest(http.MethodGet, "/activities?cursor=not-base64url", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestGetActivities_FeedCursorOlderThanRetention(t *testing.T) {
	db, _ := setupRouterWithRetention(30)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", uint(1))
		c.Set("cfg", config.Config{DeleteRetentionDays: 30})
		c.Next()
	})
	router.GET("/activities", GetActivities)

	// A since cursor far in the past is beyond the retention window -> 410.
	old := EncodeCursor(time.Now().AddDate(0, 0, -60), 1)
	req, _ := http.NewRequest(http.MethodGet, "/activities?since="+old, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusGone, w.Code, w.Body.String())
}

// TestGetActivities_SinceFeed returns soft-deleted activities as tombstones
// (deleted:true) and issues a next_cursor when more than one page of changes
// exists.
func TestGetActivities_SinceFeedIncludesTombstones(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	var user models.User
	db.First(&user)

	router.GET("/activities", GetActivities)

	a1 := models.Activity{UserID: user.ID, Title: "One", Date: time.Now()}
	a2 := models.Activity{UserID: user.ID, Title: "Two", Date: time.Now()}
	require.NoError(t, db.Create(&a1).Error)
	require.NoError(t, db.Create(&a2).Error)
	require.NoError(t, db.Delete(&a2).Error) // soft-delete one

	// Since before both, ascending, page size 1 -> returns One then (Two, deleted).
	start := time.Now().Add(-time.Hour)
	since := EncodeCursor(start, 0)
	req, _ := http.NewRequest(http.MethodGet, "/activities?since="+since+"&limit=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var page1 struct {
		Activities []struct {
			ID      uint   `json:"id"`
			Deleted bool   `json:"deleted"`
			Title   string `json:"title"`
		} `json:"activities"`
		NextCursor string `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page1))
	require.Len(t, page1.Activities, 1)
	require.NotEmpty(t, page1.NextCursor, "a full feed page must carry a next_cursor")

	req2, _ := http.NewRequest(http.MethodGet, "/activities?since="+page1.NextCursor, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	var page2 struct {
		Activities []struct {
			ID      uint `json:"id"`
			Deleted bool `json:"deleted"`
		} `json:"activities"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &page2))
	require.Len(t, page2.Activities, 1)
	assert.True(t, page2.Activities[0].Deleted, "the soft-deleted activity must be surfaced as a tombstone")
}

func TestGetActivities_IncludeContacts(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)
	activity := models.Activity{UserID: user.ID, Title: "Coffee", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)
	require.NoError(t, db.Model(&activity).Association("Contacts").Append(&contact))

	router.GET("/activities", GetActivities)
	req, _ := http.NewRequest(http.MethodGet, "/activities?include=contacts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Activities []struct {
			Contacts []models.Contact `json:"contacts"`
		} `json:"activities"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Activities, 1)
	assert.Len(t, body.Activities[0].Contacts, 1, "include=contacts must preload participants")
}

func TestGetActivities_NextCursorOnFullPage(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	for i := 0; i < 3; i++ {
		a := models.Activity{UserID: user.ID, Title: fmt.Sprintf("A%d", i), Date: time.Now()}
		require.NoError(t, db.Create(&a).Error)
		require.NoError(t, db.Model(&a).Update("updated_at", time.Now().Add(time.Duration(i)*time.Minute)).Error)
	}

	router.GET("/activities", GetActivities)
	req, _ := http.NewRequest(http.MethodGet, "/activities?limit=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Activities []models.Activity `json:"activities"`
		NextCursor string            `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Activities, 2)
	assert.NotEmpty(t, body.NextCursor, "a full page must carry a next_cursor")
}

func TestUpdateActivity_NotFound(t *testing.T) {
	_, router := setupRouter()
	router.PUT("/activities/:id", withValidated(func() any { return &models.ActivityInput{} }), UpdateActivity)
	req, _ := http.NewRequest(http.MethodPut, "/activities/999999", bytes.NewBufferString(`{"title":"x","date":"2026-01-02T00:00:00Z"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestUpdateActivity_MissingContactIsNotFound(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	activity := models.Activity{UserID: user.ID, Title: "Old", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)

	router.PUT("/activities/:id", withValidated(func() any { return &models.ActivityInput{} }), UpdateActivity)
	req, _ := http.NewRequest(http.MethodPut, "/activities/"+strconv.Itoa(int(activity.ID)), bytes.NewBufferString(`{"title":"x","date":"2026-01-02T00:00:00Z","contact_ids":[999999]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var reloaded models.Activity
	require.NoError(t, db.First(&reloaded, activity.ID).Error)
	assert.Equal(t, "Old", reloaded.Title, "a failed contact replace must not mutate the activity")
}

func TestDeleteActivity_NotFound(t *testing.T) {
	_, router := setupRouter()
	router.DELETE("/activities/:id", DeleteActivity)
	req, _ := http.NewRequest(http.MethodDelete, "/activities/999999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestGetActivitiesForContact_MalformedCursor(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	router.GET("/contacts/:id/activities", GetActivitiesForContact)
	bad := base64.RawURLEncoding.EncodeToString([]byte("2026-01-01T00:00:00Z|notanumber"))
	req, _ := http.NewRequest(http.MethodGet, "/contacts/"+strconv.Itoa(int(contact.ID))+"/activities?cursor="+bad, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}
