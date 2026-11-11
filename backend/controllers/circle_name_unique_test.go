package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2362: a circle name is unique per user, case-insensitively, on
// create and rename only. Existing duplicate rows are left alone (no index,
// no merge, no rewrite), so these tests also pin that legacy duplicates can
// still be renamed and that a rename keeping the circle's own name succeeds.

func circleNameRouter(t *testing.T) (*models.User, func(method, path, body string) (code int, reason string), func() int64, func(name string) models.Circle) {
	t.Helper()
	db, router := setupRouter(t)
	router.POST("/circles", withValidated(func() any { return &models.CircleInput{} }), CreateCircle)
	router.PUT("/circles/:id", withValidated(func() any { return &models.CircleInput{} }), UpdateCircle)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	do := func(method, path, body string) (int, string) {
		w := serveRaw(router, method, path, body)
		reason, _ := wireReason(t, w)
		return w.Code, reason
	}
	count := func() int64 {
		var n int64
		require.NoError(t, db.Model(&models.Circle{}).Count(&n).Error)
		return n
	}
	seed := func(name string) models.Circle {
		c := models.Circle{UserID: user.ID, Name: name}
		require.NoError(t, db.Create(&c).Error)
		return c
	}
	return &user, do, count, seed
}

func TestCreateCircleRejectsCaseInsensitiveDuplicate(t *testing.T) {
	_, do, count, seed := circleNameRouter(t)
	seed("work")

	for _, name := range []string{"Work", "WORK", "work", "  Work  "} {
		code, reason := do("POST", "/circles", `{"name":"`+name+`"}`)
		assert.Equal(t, http.StatusConflict, code, name)
		assert.Equal(t, "circle_already_exists", reason, name)
	}
	assert.EqualValues(t, 1, count(), "nothing is created on a duplicate")

	code, _ := do("POST", "/circles", `{"name":"Friends"}`)
	assert.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 2, count())
}

func TestCreateCircleDuplicateFoldsNonASCII(t *testing.T) {
	_, do, _, seed := circleNameRouter(t)
	seed("Élan")
	code, reason := do("POST", "/circles", `{"name":"élan"}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "circle_already_exists", reason)
}

func TestCreateCircleSameNameAcrossUsersAllowed(t *testing.T) {
	db, router := setupRouter(t)
	router.POST("/circles", withValidated(func() any { return &models.CircleInput{} }), CreateCircle)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Circle{UserID: other.ID, Name: "Work"}).Error)

	w := serveRaw(router, "POST", "/circles", `{"name":"work"}`)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestUpdateCircleRejectsDuplicateOfAnotherCircle(t *testing.T) {
	_, do, _, seed := circleNameRouter(t)
	seed("Work")
	other := seed("Home")

	code, reason := do("PUT", "/circles/"+other.ID, `{"name":"WORK"}`)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "circle_already_exists", reason)
}

func TestUpdateCircleKeepingOwnNameOrCaseSucceeds(t *testing.T) {
	db, do, _, seed := circleNameRouter(t)
	_ = db
	c := seed("Work")

	code, _ := do("PUT", "/circles/"+c.ID, `{"name":"Work","color":"red"}`)
	assert.Equal(t, http.StatusOK, code, "same name, metadata-only change")
	code, _ = do("PUT", "/circles/"+c.ID, `{"name":"work"}`)
	assert.Equal(t, http.StatusOK, code, "re-casing the circle's own name")
}

func TestUpdateCircleLegacyDuplicatesStillEditable(t *testing.T) {
	_, do, count, seed := circleNameRouter(t)
	a := seed("Family")
	seed("family") // pre-existing duplicate from before the rule

	// Editing metadata while keeping the (duplicated) name must not fail:
	// the rule only guards a name change into another circle's name.
	code, _ := do("PUT", "/circles/"+a.ID, `{"name":"Family","color":"blue"}`)
	assert.Equal(t, http.StatusOK, code)
	// Renaming a legacy duplicate to a free name resolves it.
	code, _ = do("PUT", "/circles/"+a.ID, `{"name":"Family 2"}`)
	assert.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 2, count(), "no merge: both rows survive")
}
