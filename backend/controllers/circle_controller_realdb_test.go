package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newCircleRealDBRouter mirrors newTagRealDBRouter (tag_controller_realdb_test.go)
// for the Circle routes, against a dbtest.New(t) real-migrated-schema
// database. Kept local to this file rather than reusing setupRouter/
// withValidated from circle_controller_test.go, which another agent is
// rewriting concurrently.
func newCircleRealDBRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Next()
	})
	router.POST("/circles", middleware.ValidateJSONMiddleware(&models.CircleInput{}), CreateCircle)
	router.GET("/circles", ListCircles)
	router.GET("/circles/:id", GetCircle)
	router.PUT("/circles/:id", middleware.ValidateJSONMiddleware(&models.CircleInput{}), UpdateCircle)
	router.DELETE("/circles/:id", DeleteCircle)
	router.POST("/circles/:id/members", middleware.ValidateJSONMiddleware(&models.CircleMemberInput{}), AddCircleMember)
	router.DELETE("/circles/:id/members/:vcard_uid", RemoveCircleMember)
	return router
}

func circleDoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequest(method, path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func createCircleTestUsers(t *testing.T, db *gorm.DB) (owner, other models.User) {
	t.Helper()
	owner = models.User{Username: "circle-owner", Password: "password123!A", Email: "circle-owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	other = models.User{Username: "circle-other", Password: "password123!A", Email: "circle-other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	return owner, other
}

// TestCircleController_CrossUserScoping proves another user's Circle is
// invisible: GET, PUT and DELETE by its exact ID all 404 and never mutate
// the row.
func TestCircleController_CrossUserScoping(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createCircleTestUsers(t, db)

	circle := models.Circle{UserID: owner.ID, Name: "owner-only"}
	require.NoError(t, db.Create(&circle).Error)

	otherRouter := newCircleRealDBRouter(db, other.ID)

	getResp := circleDoJSON(t, otherRouter, "GET", "/circles/"+circle.ID, nil)
	require.Equal(t, http.StatusNotFound, getResp.Code, getResp.Body.String())

	putResp := circleDoJSON(t, otherRouter, "PUT", "/circles/"+circle.ID, models.CircleInput{Name: "hijacked"})
	require.Equal(t, http.StatusNotFound, putResp.Code, putResp.Body.String())

	deleteResp := circleDoJSON(t, otherRouter, "DELETE", "/circles/"+circle.ID, nil)
	require.Equal(t, http.StatusNotFound, deleteResp.Code, deleteResp.Body.String())

	var reloaded models.Circle
	require.NoError(t, db.First(&reloaded, "id = ?", circle.ID).Error)
	require.Equal(t, "owner-only", reloaded.Name, "a 404'd cross-user PUT must not have mutated the row")
}

// TestCircleController_CrossUserScoping_ListDoesNotLeak proves ListCircles
// never returns another user's circles, with or without include_members.
func TestCircleController_CrossUserScoping_ListDoesNotLeak(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createCircleTestUsers(t, db)

	ownerContact := models.Contact{UserID: owner.ID, Firstname: "Mine"}
	require.NoError(t, db.Create(&ownerContact).Error)
	ownerCircle := models.Circle{UserID: owner.ID, Name: "mine"}
	require.NoError(t, db.Create(&ownerCircle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: ownerCircle.ID, UserID: owner.ID, MemberVCardUID: ownerContact.VCardUID}).Error)

	otherContact := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&otherContact).Error)
	otherCircle := models.Circle{UserID: other.ID, Name: "theirs"}
	require.NoError(t, db.Create(&otherCircle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: otherCircle.ID, UserID: other.ID, MemberVCardUID: otherContact.VCardUID}).Error)

	ownerRouter := newCircleRealDBRouter(db, owner.ID)

	listResp := circleDoJSON(t, ownerRouter, "GET", "/circles?include_members=true", nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())

	var body struct {
		Circles []models.Circle       `json:"circles"`
		Total   int64                 `json:"total"`
		Members []models.CircleMember `json:"members"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &body))

	require.Len(t, body.Circles, 1, "must only see the caller's own circle")
	require.Equal(t, "mine", body.Circles[0].Name)
	require.EqualValues(t, 1, body.Total)

	require.Len(t, body.Members, 1, "include_members must only include the caller's own membership rows")
	require.Equal(t, ownerCircle.ID, body.Members[0].CircleID)
	require.Equal(t, ownerContact.VCardUID, body.Members[0].MemberVCardUID)
}

// TestCircleController_ListCircles_IncludeMembersFalseOmitsField pins the
// other half of the include_members contract: absent the flag, the response
// carries no "members" key.
func TestCircleController_ListCircles_IncludeMembersFalseOmitsField(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	require.NoError(t, db.Create(&models.Circle{UserID: owner.ID, Name: "solo"}).Error)

	router := newCircleRealDBRouter(db, owner.ID)
	resp := circleDoJSON(t, router, "GET", "/circles", nil)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	_, hasMembers := body["members"]
	require.False(t, hasMembers, "members key must be absent when include_members is not requested")
}

// TestCircleController_ListCircles_CursorPagination exercises the
// next_cursor branch across two pages.
func TestCircleController_ListCircles_CursorPagination(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)

	for _, name := range []string{"alpha", "bravo", "charlie"} {
		circle := models.Circle{UserID: owner.ID, Name: name}
		require.NoError(t, db.Create(&circle).Error)
	}

	router := newCircleRealDBRouter(db, owner.ID)

	firstResp := circleDoJSON(t, router, "GET", "/circles?limit=2", nil)
	require.Equal(t, http.StatusOK, firstResp.Code, firstResp.Body.String())
	var firstPage struct {
		Circles    []models.Circle `json:"circles"`
		NextCursor string          `json:"next_cursor"`
		Total      int64           `json:"total"`
	}
	require.NoError(t, json.Unmarshal(firstResp.Body.Bytes(), &firstPage))
	require.Len(t, firstPage.Circles, 2)
	require.EqualValues(t, 3, firstPage.Total)
	require.NotEmpty(t, firstPage.NextCursor)

	secondResp := circleDoJSON(t, router, "GET", "/circles?limit=2&cursor="+firstPage.NextCursor, nil)
	require.Equal(t, http.StatusOK, secondResp.Code, secondResp.Body.String())
	var secondPage struct {
		Circles    []models.Circle `json:"circles"`
		NextCursor string          `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(secondResp.Body.Bytes(), &secondPage))
	require.Len(t, secondPage.Circles, 1)
	require.Empty(t, secondPage.NextCursor)
}

// TestCircleController_GetUpdateDelete_NotFoundForUnknownID pins the plain
// (same-user) 404 path for an ID that doesn't exist.
func TestCircleController_GetUpdateDelete_NotFoundForUnknownID(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	router := newCircleRealDBRouter(db, owner.ID)

	const bogusID = "does-not-exist"

	getResp := circleDoJSON(t, router, "GET", "/circles/"+bogusID, nil)
	require.Equal(t, http.StatusNotFound, getResp.Code, getResp.Body.String())

	putResp := circleDoJSON(t, router, "PUT", "/circles/"+bogusID, models.CircleInput{Name: "whatever"})
	require.Equal(t, http.StatusNotFound, putResp.Code, putResp.Body.String())

	deleteResp := circleDoJSON(t, router, "DELETE", "/circles/"+bogusID, nil)
	require.Equal(t, http.StatusNotFound, deleteResp.Code, deleteResp.Body.String())
}

// TestCircleController_AddCircleMember_CircleNotFound pins the "circle
// itself doesn't exist" 404 branch of AddCircleMember, distinct from the
// "contact doesn't belong to caller" 404 already covered by
// TestAddCircleMemberRejectsContactFromAnotherUser in circle_controller_test.go.
func TestCircleController_AddCircleMember_CircleNotFound(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	router := newCircleRealDBRouter(db, owner.ID)
	resp := circleDoJSON(t, router, "POST", "/circles/does-not-exist/members", models.CircleMemberInput{MemberVCardUID: contact.VCardUID})
	require.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

// TestCircleController_RemoveCircleMember_CircleNotFound pins the
// "circle itself is missing" 404 branch of RemoveCircleMember, distinct
// from the empty-RowsAffected case circle_controller_test.go already covers.
func TestCircleController_RemoveCircleMember_CircleNotFound(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	router := newCircleRealDBRouter(db, owner.ID)

	resp := circleDoJSON(t, router, "DELETE", "/circles/does-not-exist/members/some-uid", nil)
	require.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

// TestCircleController_CreateCircle_Success is the real-router happy path,
// proving the row lands scoped to the caller.
func TestCircleController_CreateCircle_Success(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createCircleTestUsers(t, db)
	router := newCircleRealDBRouter(db, owner.ID)

	resp := circleDoJSON(t, router, "POST", "/circles", models.CircleInput{Name: "fresh"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.Circle{}).Where("user_id = ? AND name = ?", owner.ID, "fresh").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// TestCircleController_DatabaseErrorBranches mirrors
// TestTagController_DatabaseErrorBranches (tag_controller_realdb_test.go)
// for circle_controller.go's "Failed to X" 500 branches, using the same
// failDBTableOn fault injector.
func TestCircleController_DatabaseErrorBranches(t *testing.T) {
	t.Run("CreateCircle", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		failDBTableOn(t, db, "circles", "create")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "POST", "/circles", models.CircleInput{Name: "x"})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("GetCircle_LookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)
		failDBTableOn(t, db, "circles", "query")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "GET", "/circles/"+circle.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("GetCircle_MembersLookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)
		failDBTableOn(t, db, "circle_members", "query")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "GET", "/circles/"+circle.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ListCircles_CountError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		failDBTableOn(t, db, "circles", "query", "row")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "GET", "/circles", nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ListCircles_IncludeMembersError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		require.NoError(t, db.Create(&models.Circle{UserID: owner.ID, Name: "x"}).Error)
		failDBTableOn(t, db, "circle_members", "query")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "GET", "/circles?include_members=true", nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("UpdateCircle_SaveError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		circle := models.Circle{UserID: owner.ID, Name: "old"}
		require.NoError(t, db.Create(&circle).Error)
		failDBTableOn(t, db, "circles", "update")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "PUT", "/circles/"+circle.ID, models.CircleInput{Name: "new"})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("DeleteCircle_DeleteError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)
		failDBTableOn(t, db, "circles", "delete")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "DELETE", "/circles/"+circle.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("AddCircleMember_ContactLookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)
		failDBTableOn(t, db, "contacts", "query")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "POST", "/circles/"+circle.ID+"/members", models.CircleMemberInput{MemberVCardUID: contact.VCardUID})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("AddCircleMember_ExistingLookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)
		failDBTableOn(t, db, "circle_members", "query")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "POST", "/circles/"+circle.ID+"/members", models.CircleMemberInput{MemberVCardUID: contact.VCardUID})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("AddCircleMember_CreateError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)
		failDBTableOn(t, db, "circle_members", "create")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "POST", "/circles/"+circle.ID+"/members", models.CircleMemberInput{MemberVCardUID: contact.VCardUID})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("RemoveCircleMember_DeleteError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)
		require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: owner.ID, MemberVCardUID: contact.VCardUID}).Error)
		failDBTableOn(t, db, "circle_members", "delete")
		router := newCircleRealDBRouter(db, owner.ID)
		resp := circleDoJSON(t, router, "DELETE", "/circles/"+circle.ID+"/members/"+contact.VCardUID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})
}

// TestCircleController_MissingValidatedData mirrors
// TestTagController_MissingValidatedData for CreateCircle/UpdateCircle's own
// defensive GetValidated error branch.
func TestCircleController_MissingValidatedData(t *testing.T) {
	t.Run("CreateCircle", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Request, _ = http.NewRequest("POST", "/circles", nil)

		CreateCircle(c)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("UpdateCircle", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createCircleTestUsers(t, db)
		circle := models.Circle{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&circle).Error)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Params = gin.Params{{Key: "id", Value: circle.ID}}
		c.Request, _ = http.NewRequest("PUT", "/circles/"+circle.ID, nil)

		UpdateCircle(c)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})
}
