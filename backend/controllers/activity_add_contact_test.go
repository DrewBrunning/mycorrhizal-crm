package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type addContactResp struct {
	Results []struct {
		ActivityID uint   `json:"activity_id"`
		Status     string `json:"status"`
	} `json:"results"`
}

func postAddToActivities(t *testing.T, router http.Handler, contactID uint, ids []uint) (*httptest.ResponseRecorder, addContactResp) {
	t.Helper()
	body, _ := json.Marshal(models.ContactActivitiesInput{ActivityIDs: ids})
	req, _ := http.NewRequest("POST", fmt.Sprintf("/contacts/%d/activities", contactID), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp addContactResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func joinCount(t *testing.T, db *gorm.DB, activityID, contactID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("activity_contacts").Where("activity_id = ? AND contact_id = ?", activityID, contactID).Count(&n).Error)
	return n
}

func TestAddContactToActivities(t *testing.T) {
	db, router := setupRouter(t)
	router.POST("/contacts/:id/activities", withValidated(func() any { return &models.ContactActivitiesInput{} }), AddContactToActivities)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	me := models.Contact{UserID: user.ID, Firstname: "Me"}
	other := models.Contact{UserID: user.ID, Firstname: "Other"}
	require.NoError(t, db.Create(&me).Error)
	require.NoError(t, db.Create(&other).Error)

	when := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	a1 := models.Activity{UserID: user.ID, Title: "Dinner", Description: "d", Location: "L", Date: when}
	a2 := models.Activity{UserID: user.ID, Title: "Trip", Date: when}
	require.NoError(t, db.Create(&a1).Error)
	require.NoError(t, db.Create(&a2).Error)
	require.NoError(t, db.Model(&a1).Association("Contacts").Append(&other))

	t.Run("adds to several, leaves other fields and members untouched", func(t *testing.T) {
		w, resp := postAddToActivities(t, router, me.ID, []uint{a1.ID, a2.ID})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Len(t, resp.Results, 2)
		assert.Equal(t, "added", resp.Results[0].Status)
		assert.Equal(t, "added", resp.Results[1].Status)
		assert.EqualValues(t, 1, joinCount(t, db, a1.ID, me.ID))
		assert.EqualValues(t, 1, joinCount(t, db, a2.ID, me.ID))
		assert.EqualValues(t, 1, joinCount(t, db, a1.ID, other.ID), "existing member kept")

		var after models.Activity
		require.NoError(t, db.First(&after, a1.ID).Error)
		assert.Equal(t, "Dinner", after.Title)
		assert.Equal(t, "d", after.Description)
		assert.Equal(t, "L", after.Location)
		assert.True(t, after.Date.Equal(when))
		assert.Greater(t, after.Revision, int64(1), "membership change bumps revision")
	})

	t.Run("repeat is idempotent: no duplicate row, no revision bump", func(t *testing.T) {
		var before models.Activity
		require.NoError(t, db.First(&before, a1.ID).Error)
		w, resp := postAddToActivities(t, router, me.ID, []uint{a1.ID, a1.ID})
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Results, 1, "duplicate ids collapse")
		assert.Equal(t, "already_member", resp.Results[0].Status)
		assert.EqualValues(t, 1, joinCount(t, db, a1.ID, me.ID))
		var after models.Activity
		require.NoError(t, db.First(&after, a1.ID).Error)
		assert.Equal(t, before.Revision, after.Revision)
	})

	t.Run("foreign activity reported not_found, own ones still applied", func(t *testing.T) {
		stranger := models.User{Username: "x", Password: "password123", Email: "x@example.com"}
		require.NoError(t, db.Create(&stranger).Error)
		foreign := models.Activity{UserID: stranger.ID, Title: "Theirs", Date: when}
		require.NoError(t, db.Create(&foreign).Error)
		mine := models.Activity{UserID: user.ID, Title: "Mine", Date: when}
		require.NoError(t, db.Create(&mine).Error)

		w, resp := postAddToActivities(t, router, me.ID, []uint{foreign.ID, mine.ID, 999999})
		require.Equal(t, http.StatusOK, w.Code)
		require.Len(t, resp.Results, 3)
		assert.Equal(t, "not_found", resp.Results[0].Status)
		assert.Equal(t, "added", resp.Results[1].Status)
		assert.Equal(t, "not_found", resp.Results[2].Status)
		assert.EqualValues(t, 0, joinCount(t, db, foreign.ID, me.ID))
	})

	t.Run("foreign contact is 404 and writes nothing", func(t *testing.T) {
		stranger := models.User{Username: "y", Password: "password123", Email: "y@example.com"}
		require.NoError(t, db.Create(&stranger).Error)
		theirs := models.Contact{UserID: stranger.ID, Firstname: "Theirs"}
		require.NoError(t, db.Create(&theirs).Error)
		w, _ := postAddToActivities(t, router, theirs.ID, []uint{a2.ID})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.EqualValues(t, 0, joinCount(t, db, a2.ID, theirs.ID))
	})

	t.Run("soft-deleted activity is not_found", func(t *testing.T) {
		gone := models.Activity{UserID: user.ID, Title: "Gone", Date: when}
		require.NoError(t, db.Create(&gone).Error)
		require.NoError(t, db.Delete(&gone).Error)
		_, resp := postAddToActivities(t, router, me.ID, []uint{gone.ID})
		require.Len(t, resp.Results, 1)
		assert.Equal(t, "not_found", resp.Results[0].Status)
	})

	t.Run("empty and oversized lists are rejected", func(t *testing.T) {
		w, _ := postAddToActivities(t, router, me.ID, []uint{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		big := make([]uint, models.MaxContactActivitiesBatch+1)
		for i := range big {
			big[i] = uint(i + 1)
		}
		w, _ = postAddToActivities(t, router, me.ID, big)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAddContactToActivities_ErrorPaths(t *testing.T) {
	t.Run("unauthenticated is 401", func(t *testing.T) {
		db, _ := setupRouter(t)
		router := activityRouterNoUser(db)
		router.POST("/contacts/:id/activities", withValidated(func() any { return &models.ContactActivitiesInput{} }), AddContactToActivities)
		w, _ := postAddToActivities(t, router, 1, []uint{1})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("malformed contact id is 400", func(t *testing.T) {
		_, router := setupRouter(t)
		router.POST("/contacts/:id/activities", withValidated(func() any { return &models.ContactActivitiesInput{} }), AddContactToActivities)
		req, _ := http.NewRequest("POST", "/contacts/abc/activities", bytes.NewBufferString(`{"activity_ids":[1]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	for _, table := range []string{"contacts", "activities", "activity_contacts"} {
		t.Run("database failure on "+table+" is 500 with no partial write", func(t *testing.T) {
			db, router := setupRouter(t)
			logtest.AllowWarnings(t, "the path under test logs the database failure")
			router.POST("/contacts/:id/activities", withValidated(func() any { return &models.ContactActivitiesInput{} }), AddContactToActivities)
			var user models.User
			require.NoError(t, db.First(&user).Error)
			c := models.Contact{UserID: user.ID, Firstname: "C"}
			require.NoError(t, db.Create(&c).Error)
			a := models.Activity{UserID: user.ID, Title: "A", Date: time.Now()}
			require.NoError(t, db.Create(&a).Error)
			dbtest.HideTable(t, db, table)

			w, _ := postAddToActivities(t, router, c.ID, []uint{a.ID})
			assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
		})
	}
}

func TestAddContactToActivities_WriteFailuresRollBack(t *testing.T) {
	for _, tc := range []struct{ name, trigger string }{
		{"join insert fails", `CREATE TRIGGER t_fail_ins BEFORE INSERT ON activity_contacts BEGIN SELECT RAISE(ABORT, 'forced'); END`},
		{"activity save fails", `CREATE TRIGGER t_fail_upd BEFORE UPDATE ON activities BEGIN SELECT RAISE(ABORT, 'forced'); END`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, router := setupRouter(t)
			logtest.AllowWarnings(t, "the path under test logs the database failure")
			router.POST("/contacts/:id/activities", withValidated(func() any { return &models.ContactActivitiesInput{} }), AddContactToActivities)
			var user models.User
			require.NoError(t, db.First(&user).Error)
			c := models.Contact{UserID: user.ID, Firstname: "C"}
			require.NoError(t, db.Create(&c).Error)
			a := models.Activity{UserID: user.ID, Title: "A", Date: time.Now()}
			require.NoError(t, db.Create(&a).Error)
			require.NoError(t, db.Exec(tc.trigger).Error)

			w, _ := postAddToActivities(t, router, c.ID, []uint{a.ID})
			assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
			assert.EqualValues(t, 0, joinCount(t, db, a.ID, c.ID), "failed call leaves no join row")
		})
	}
}

func TestAddContactToActivities_MissingValidatedBody(t *testing.T) {
	db, router := setupRouter(t)
	router.POST("/contacts/:id/activities", AddContactToActivities)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	c := models.Contact{UserID: user.ID, Firstname: "C"}
	require.NoError(t, db.Create(&c).Error)
	w, _ := postAddToActivities(t, router, c.ID, []uint{1})
	assert.GreaterOrEqual(t, w.Code, 400)
}
