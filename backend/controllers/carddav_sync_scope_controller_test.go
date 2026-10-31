package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newCardDAVSyncScopeRouter(t *testing.T, db *gorm.DB, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Next()
	})
	r.GET("/users/me/carddav-sync-scope", GetCardDAVSyncScope)
	r.PUT("/users/me/carddav-sync-scope", middleware.ValidateJSONMiddleware(&models.CardDAVSyncScopeInput{}), PutCardDAVSyncScope)
	r.POST("/users/me/carddav-sync-scope/preview", middleware.ValidateJSONMiddleware(&models.CardDAVSyncScopeInput{}), PreviewCardDAVSyncScope)
	return r
}

func cardDAVSyncScopeDo(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequest(method, path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeCardDAVSyncScope(t *testing.T, w *httptest.ResponseRecorder) models.CardDAVSyncScopeResponse {
	t.Helper()
	var resp models.CardDAVSyncScopeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func seedScopeFixture(t *testing.T, db *gorm.DB) (owner, other models.User, c1, c2, c3 models.Contact, a, b models.Circle) {
	t.Helper()
	owner = models.User{Username: "scope-owner", Password: "password123!A", Email: "scope-owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	other = models.User{Username: "scope-other", Password: "password123!A", Email: "scope-other@example.com"}
	require.NoError(t, db.Create(&other).Error)

	a = models.Circle{UserID: owner.ID, Name: "A"}
	b = models.Circle{UserID: owner.ID, Name: "B"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)

	c1 = models.Contact{UserID: owner.ID, Firstname: "One"}
	c2 = models.Contact{UserID: owner.ID, Firstname: "Two"}
	c3 = models.Contact{UserID: owner.ID, Firstname: "Three"}
	require.NoError(t, db.Create(&c1).Error)
	require.NoError(t, db.Create(&c2).Error)
	require.NoError(t, db.Create(&c3).Error)

	require.NoError(t, db.Create(&models.CircleMember{CircleID: a.ID, UserID: owner.ID, MemberVCardUID: c1.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: a.ID, UserID: owner.ID, MemberVCardUID: c2.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: b.ID, UserID: owner.ID, MemberVCardUID: c2.VCardUID}).Error)
	return owner, other, c1, c2, c3, a, b
}

// A user with no row is "all" with no circles, and every contact is in scope.
func TestCardDAVSyncScope_DefaultIsAll(t *testing.T) {
	db := dbtest.New(t)
	owner, _, _, _, _, _, _ := seedScopeFixture(t, db)
	r := newCardDAVSyncScopeRouter(t, db, owner.ID)

	w := cardDAVSyncScopeDo(t, r, http.MethodGet, "/users/me/carddav-sync-scope", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp := decodeCardDAVSyncScope(t, w)
	require.Equal(t, models.CardDAVSyncScopeModeAll, resp.Mode)
	require.NotNil(t, resp.CircleIDs)
	require.Empty(t, resp.CircleIDs)
	require.Nil(t, resp.DefaultCircleID)
	require.Equal(t, 3, resp.InScopeCount)
	require.Equal(t, 3, resp.TotalCount)
	require.Empty(t, resp.Warnings)
	require.Nil(t, resp.UpdatedAt)
}

// Every write-time rule in ADR 0041 decision 2, plus ownership: an unknown id
// and another user's id are both the same 400 and nothing is persisted.
func TestCardDAVSyncScope_ValidationAndOwnership(t *testing.T) {
	db := dbtest.New(t)
	owner, other, _, _, _, a, b := seedScopeFixture(t, db)
	otherCircle := models.Circle{UserID: other.ID, Name: "theirs"}
	require.NoError(t, db.Create(&otherCircle).Error)
	r := newCardDAVSyncScopeRouter(t, db, owner.ID)

	missing := "00000000-0000-4000-8000-000000000000"
	cases := map[string]models.CardDAVSyncScopeInput{
		"all with circle":             {Mode: "all", CircleIDs: []string{a.ID}},
		"all with default":            {Mode: "all", DefaultCircleID: &a.ID},
		"include empty":               {Mode: "include"},
		"include no default":          {Mode: "include", CircleIDs: []string{a.ID}},
		"include default not chosen":  {Mode: "include", CircleIDs: []string{a.ID}, DefaultCircleID: &b.ID},
		"exclude empty":               {Mode: "exclude"},
		"exclude default excluded":    {Mode: "exclude", CircleIDs: []string{a.ID}, DefaultCircleID: &a.ID},
		"include unknown circle":      {Mode: "include", CircleIDs: []string{missing}, DefaultCircleID: &missing},
		"include another user circle": {Mode: "include", CircleIDs: []string{otherCircle.ID}, DefaultCircleID: &otherCircle.ID},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			w := cardDAVSyncScopeDo(t, r, http.MethodPut, "/users/me/carddav-sync-scope", input)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}

	var scopes, rows int64
	require.NoError(t, db.Model(&models.CardDAVSyncScope{}).Where("user_id = ?", owner.ID).Count(&scopes).Error)
	require.NoError(t, db.Model(&models.CardDAVSyncScopeCircle{}).Where("user_id = ?", owner.ID).Count(&rows).Error)
	require.Zero(t, scopes)
	require.Zero(t, rows)
}

// include/exclude counts, duplicate ids, full replacement and derived warnings.
func TestCardDAVSyncScope_RoundTripCountsAndWarnings(t *testing.T) {
	db := dbtest.New(t)
	owner, _, _, _, _, a, b := seedScopeFixture(t, db)
	r := newCardDAVSyncScopeRouter(t, db, owner.ID)

	put := cardDAVSyncScopeDo(t, r, http.MethodPut, "/users/me/carddav-sync-scope", models.CardDAVSyncScopeInput{
		Mode: "include", CircleIDs: []string{a.ID, a.ID}, DefaultCircleID: &a.ID,
	})
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())
	resp := decodeCardDAVSyncScope(t, put)
	require.Equal(t, models.CardDAVSyncScopeModeInclude, resp.Mode)
	require.Equal(t, []string{a.ID}, resp.CircleIDs, "duplicate ids are deduplicated")
	require.Equal(t, 2, resp.InScopeCount, "c1 and c2 are in A")
	require.Equal(t, 3, resp.TotalCount)
	require.Empty(t, resp.Warnings)
	require.NotNil(t, resp.UpdatedAt)

	// Exclude B: only c2 is in B, so c1 and c3 sync.
	put = cardDAVSyncScopeDo(t, r, http.MethodPut, "/users/me/carddav-sync-scope", models.CardDAVSyncScopeInput{
		Mode: "exclude", CircleIDs: []string{b.ID},
	})
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())
	resp = decodeCardDAVSyncScope(t, put)
	require.Equal(t, models.CardDAVSyncScopeModeExclude, resp.Mode)
	require.Equal(t, 2, resp.InScopeCount)
	require.Nil(t, resp.DefaultCircleID)
	require.Empty(t, resp.Warnings)

	// PUT replaces the whole object: switching to all clears the circles.
	put = cardDAVSyncScopeDo(t, r, http.MethodPut, "/users/me/carddav-sync-scope", models.CardDAVSyncScopeInput{Mode: "all"})
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())
	resp = decodeCardDAVSyncScope(t, put)
	require.Equal(t, models.CardDAVSyncScopeModeAll, resp.Mode)
	require.Empty(t, resp.CircleIDs)
	require.Equal(t, 3, resp.InScopeCount)
}

// Preview returns counts for a proposal and persists nothing.
func TestCardDAVSyncScope_PreviewPersistsNothing(t *testing.T) {
	db := dbtest.New(t)
	owner, _, _, _, _, a, _ := seedScopeFixture(t, db)
	r := newCardDAVSyncScopeRouter(t, db, owner.ID)

	w := cardDAVSyncScopeDo(t, r, http.MethodPost, "/users/me/carddav-sync-scope/preview", models.CardDAVSyncScopeInput{
		Mode: "include", CircleIDs: []string{a.ID}, DefaultCircleID: &a.ID,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var preview models.CardDAVSyncScopePreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
	require.Equal(t, 2, preview.InScopeCount)
	require.Equal(t, 3, preview.TotalCount)

	var count int64
	require.NoError(t, db.Model(&models.CardDAVSyncScope{}).Where("user_id = ?", owner.ID).Count(&count).Error)
	require.Zero(t, count)

	get := decodeCardDAVSyncScope(t, cardDAVSyncScopeDo(t, r, http.MethodGet, "/users/me/carddav-sync-scope", nil))
	require.Equal(t, models.CardDAVSyncScopeModeAll, get.Mode)
}

// Deleting a selected circle drops it from the scope and nulls the default;
// include never falls back to all, and the warning is derived on read.
func TestCardDAVSyncScope_CircleDeleteDerivesWarnings(t *testing.T) {
	db := dbtest.New(t)
	owner, _, _, _, _, a, _ := seedScopeFixture(t, db)
	r := newCardDAVSyncScopeRouter(t, db, owner.ID)

	put := cardDAVSyncScopeDo(t, r, http.MethodPut, "/users/me/carddav-sync-scope", models.CardDAVSyncScopeInput{
		Mode: "include", CircleIDs: []string{a.ID}, DefaultCircleID: &a.ID,
	})
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())

	require.NoError(t, db.Delete(&models.Circle{}, "id = ?", a.ID).Error)

	resp := decodeCardDAVSyncScope(t, cardDAVSyncScopeDo(t, r, http.MethodGet, "/users/me/carddav-sync-scope", nil))
	require.Equal(t, models.CardDAVSyncScopeModeInclude, resp.Mode, "never falls back to all")
	require.Empty(t, resp.CircleIDs)
	require.Nil(t, resp.DefaultCircleID)
	require.Zero(t, resp.InScopeCount)
	require.Equal(t, 3, resp.TotalCount)
	require.Len(t, resp.Warnings, 1)
	require.Equal(t, models.CardDAVSyncScopeWarningEmpty, resp.Warnings[0].Code)
}

// A different user cannot read or overwrite the caller's scope: each request
// is scoped by the authenticated user id.
func TestCardDAVSyncScope_PerUserIsolation(t *testing.T) {
	db := dbtest.New(t)
	owner, other, _, _, _, a, _ := seedScopeFixture(t, db)
	ownerRouter := newCardDAVSyncScopeRouter(t, db, owner.ID)
	otherRouter := newCardDAVSyncScopeRouter(t, db, other.ID)

	require.Equal(t, http.StatusOK, cardDAVSyncScopeDo(t, ownerRouter, http.MethodPut, "/users/me/carddav-sync-scope", models.CardDAVSyncScopeInput{
		Mode: "include", CircleIDs: []string{a.ID}, DefaultCircleID: &a.ID,
	}).Code)

	otherResp := decodeCardDAVSyncScope(t, cardDAVSyncScopeDo(t, otherRouter, http.MethodGet, "/users/me/carddav-sync-scope", nil))
	require.Equal(t, models.CardDAVSyncScopeModeAll, otherResp.Mode)
	require.Empty(t, otherResp.CircleIDs)
}
