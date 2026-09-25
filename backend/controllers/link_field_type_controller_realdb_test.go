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

// newLinkFieldTypeRealDBRouter mirrors newTagRealDBRouter
// (tag_controller_realdb_test.go) for the LinkFieldType routes, against a
// dbtest.New(t) real-migrated-schema database. Kept local to this file
// rather than the existing link_field_type_real_db_test.go's single-user
// story, so cross-user scoping and a few untested branches get their own
// focused router/tests.
func newLinkFieldTypeRealDBRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Next()
	})
	router.POST("/link-field-types", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeInput{}), CreateLinkFieldType)
	router.GET("/link-field-types", ListLinkFieldTypes)
	router.GET("/link-field-types/:id", GetLinkFieldType)
	router.PUT("/link-field-types/reorder", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeReorderInput{}), ReorderLinkFieldTypes)
	router.PUT("/link-field-types/:id", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeInput{}), UpdateLinkFieldType)
	router.DELETE("/link-field-types/:id", DeleteLinkFieldType)
	return router
}

func linkFieldTypeDoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func createLinkFieldTypeTestUsers(t *testing.T, db *gorm.DB) (owner, other models.User) {
	t.Helper()
	owner = models.User{Username: "lft-owner", Password: "password123!A", Email: "lft-owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	other = models.User{Username: "lft-other", Password: "password123!A", Email: "lft-other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	return owner, other
}

// TestLinkFieldTypeController_CrossUserScoping proves another user's
// LinkFieldType is invisible: GET, PUT and DELETE by its exact ID all 404
// and never mutate the row. Deliberately creates the row directly (not via
// the seed defaults) so this is unambiguously "owner's row, not caller's."
func TestLinkFieldTypeController_CrossUserScoping(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createLinkFieldTypeTestUsers(t, db)

	linkType := models.LinkFieldType{UserID: owner.ID, Name: "Owner Only", Protocol: "https://example.com/{value}", Category: models.LinkFieldTypeCategoryOther}
	require.NoError(t, db.Create(&linkType).Error)

	otherRouter := newLinkFieldTypeRealDBRouter(db, other.ID)

	getResp := linkFieldTypeDoJSON(t, otherRouter, "GET", "/link-field-types/"+linkType.ID, nil)
	require.Equal(t, http.StatusNotFound, getResp.Code, getResp.Body.String())

	putResp := linkFieldTypeDoJSON(t, otherRouter, "PUT", "/link-field-types/"+linkType.ID, models.LinkFieldTypeInput{
		Name: "Hijacked", Category: models.LinkFieldTypeCategoryOther,
	})
	require.Equal(t, http.StatusNotFound, putResp.Code, putResp.Body.String())

	deleteResp := linkFieldTypeDoJSON(t, otherRouter, "DELETE", "/link-field-types/"+linkType.ID, nil)
	require.Equal(t, http.StatusNotFound, deleteResp.Code, deleteResp.Body.String())

	var reloaded models.LinkFieldType
	require.NoError(t, db.First(&reloaded, "id = ?", linkType.ID).Error)
	require.Equal(t, "Owner Only", reloaded.Name, "a 404'd cross-user PUT must not have mutated the row")
}

// TestLinkFieldTypeController_ListDoesNotLeak proves ListLinkFieldTypes
// (after lazily seeding each user's own defaults) never returns another
// user's rows.
func TestLinkFieldTypeController_ListDoesNotLeak(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createLinkFieldTypeTestUsers(t, db)

	ownerRouter := newLinkFieldTypeRealDBRouter(db, owner.ID)
	otherRouter := newLinkFieldTypeRealDBRouter(db, other.ID)

	ownerListResp := linkFieldTypeDoJSON(t, ownerRouter, "GET", "/link-field-types", nil)
	require.Equal(t, http.StatusOK, ownerListResp.Code, ownerListResp.Body.String())
	var ownerListed struct {
		LinkFieldTypes []models.LinkFieldType `json:"link_field_types"`
	}
	require.NoError(t, json.Unmarshal(ownerListResp.Body.Bytes(), &ownerListed))
	require.Len(t, ownerListed.LinkFieldTypes, len(models.LinkFieldTypeDefaults))
	for _, lt := range ownerListed.LinkFieldTypes {
		var row models.LinkFieldType
		require.NoError(t, db.First(&row, "id = ?", lt.ID).Error)
		require.Equal(t, owner.ID, row.UserID, "UserID is json:\"-\", so check ownership against the DB row")
	}

	// The other user's own first fetch seeds their own independent copy of
	// the defaults, not a shared/global set.
	otherListResp := linkFieldTypeDoJSON(t, otherRouter, "GET", "/link-field-types", nil)
	require.Equal(t, http.StatusOK, otherListResp.Code, otherListResp.Body.String())
	var otherListed struct {
		LinkFieldTypes []models.LinkFieldType `json:"link_field_types"`
	}
	require.NoError(t, json.Unmarshal(otherListResp.Body.Bytes(), &otherListed))
	require.Len(t, otherListed.LinkFieldTypes, len(models.LinkFieldTypeDefaults))
	for _, lt := range otherListed.LinkFieldTypes {
		var row models.LinkFieldType
		require.NoError(t, db.First(&row, "id = ?", lt.ID).Error)
		require.Equal(t, other.ID, row.UserID, "UserID is json:\"-\", so check ownership against the DB row")
	}

	var total int64
	require.NoError(t, db.Model(&models.LinkFieldType{}).Count(&total).Error)
	require.EqualValues(t, 2*len(models.LinkFieldTypeDefaults), total, "seeding must be per-user, not shared")
}

// TestLinkFieldTypeController_GetLinkFieldType_Success is the plain
// same-user GET happy path (the big real-db test only ever reaches
// GetLinkFieldType through the soft-deleted-404 case).
func TestLinkFieldTypeController_GetLinkFieldType_Success(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createLinkFieldTypeTestUsers(t, db)
	linkType := models.LinkFieldType{UserID: owner.ID, Name: "Custom", Protocol: "https://example.com/{value}", Category: models.LinkFieldTypeCategoryOther}
	require.NoError(t, db.Create(&linkType).Error)

	router := newLinkFieldTypeRealDBRouter(db, owner.ID)
	resp := linkFieldTypeDoJSON(t, router, "GET", "/link-field-types/"+linkType.ID, nil)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var got models.LinkFieldType
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &got))
	require.Equal(t, "Custom", got.Name)
}

// TestLinkFieldTypeController_CreateLinkFieldType_ExplicitPosition pins the
// input.Position != nil branch of CreateLinkFieldType — the big real-db
// test only ever exercises the "compute the next position" branch.
func TestLinkFieldTypeController_CreateLinkFieldType_ExplicitPosition(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createLinkFieldTypeTestUsers(t, db)
	router := newLinkFieldTypeRealDBRouter(db, owner.ID)

	explicit := 7
	resp := linkFieldTypeDoJSON(t, router, "POST", "/link-field-types", models.LinkFieldTypeInput{
		Name: "Explicit Position", Category: models.LinkFieldTypeCategoryOther, Position: &explicit,
	})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var created struct {
		LinkFieldType models.LinkFieldType `json:"link_field_type"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, explicit, created.LinkFieldType.Position)
}

// TestLinkFieldTypeController_UpdateLinkFieldType_NotFound pins the plain
// (same-user) 404 path for UpdateLinkFieldType.
func TestLinkFieldTypeController_UpdateLinkFieldType_NotFound(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createLinkFieldTypeTestUsers(t, db)
	router := newLinkFieldTypeRealDBRouter(db, owner.ID)

	resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/does-not-exist", models.LinkFieldTypeInput{
		Name: "Whatever", Category: models.LinkFieldTypeCategoryOther,
	})
	require.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

// TestLinkFieldTypeController_UpdateLinkFieldType_DuplicateNameConflict
// pins the "renamed to an existing other type's name" 409 branch of
// UpdateLinkFieldType, distinct from CreateLinkFieldType's own duplicate
// check the big real-db test already covers.
func TestLinkFieldTypeController_UpdateLinkFieldType_DuplicateNameConflict(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createLinkFieldTypeTestUsers(t, db)
	first := models.LinkFieldType{UserID: owner.ID, Name: "First", Category: models.LinkFieldTypeCategoryOther}
	require.NoError(t, db.Create(&first).Error)
	second := models.LinkFieldType{UserID: owner.ID, Name: "Second", Category: models.LinkFieldTypeCategoryOther}
	require.NoError(t, db.Create(&second).Error)

	router := newLinkFieldTypeRealDBRouter(db, owner.ID)
	resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/"+second.ID, models.LinkFieldTypeInput{
		Name: "First", Category: models.LinkFieldTypeCategoryOther,
	})
	require.Equal(t, http.StatusConflict, resp.Code, resp.Body.String())

	var reloaded models.LinkFieldType
	require.NoError(t, db.First(&reloaded, "id = ?", second.ID).Error)
	require.Equal(t, "Second", reloaded.Name, "a 409'd rename must not have mutated the row")
}

// TestLinkFieldTypeController_DeleteLinkFieldType_NotFound pins the plain
// (same-user) 404 path for DeleteLinkFieldType.
func TestLinkFieldTypeController_DeleteLinkFieldType_NotFound(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createLinkFieldTypeTestUsers(t, db)
	router := newLinkFieldTypeRealDBRouter(db, owner.ID)

	resp := linkFieldTypeDoJSON(t, router, "DELETE", "/link-field-types/does-not-exist", nil)
	require.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

// TestLinkFieldTypeController_DatabaseErrorBranches mirrors
// TestTagController_DatabaseErrorBranches (tag_controller_realdb_test.go)
// for link_field_type_controller.go's "Failed to X" 500 branches, using the
// same failDBTableOn fault injector.
func TestLinkFieldTypeController_DatabaseErrorBranches(t *testing.T) {
	t.Run("ListLinkFieldTypes_SeedCountError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		failDBTableOn(t, db, "link_field_types", "query", "row")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "GET", "/link-field-types", nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ListLinkFieldTypes_ListError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		require.NoError(t, db.Create(&models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}).Error)
		// Already-seeded (count>0): skip 1 successful Query call (the seed's
		// own Count) so the fault lands on the handler's own listing Find.
		failDBTableAfterNCalls(t, db, "link_field_types", 1)
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "GET", "/link-field-types", nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("CreateLinkFieldType_PositionScanError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		// Row-only fault: the duplicate-name check (a Query-kind First())
		// still succeeds; nextLinkFieldTypePosition's Scan (Row-kind) fails.
		failDBTableOn(t, db, "link_field_types", "row")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "POST", "/link-field-types", models.LinkFieldTypeInput{
			Name: "x", Category: models.LinkFieldTypeCategoryOther,
		})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("GetLinkFieldType_LookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		failDBTableOn(t, db, "link_field_types", "query")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "GET", "/link-field-types/"+linkType.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("CreateLinkFieldType_ExistingLookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		failDBTableOn(t, db, "link_field_types", "query", "row")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "POST", "/link-field-types", models.LinkFieldTypeInput{
			Name: "x", Category: models.LinkFieldTypeCategoryOther,
		})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("CreateLinkFieldType_CreateError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		failDBTableOn(t, db, "link_field_types", "create")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "POST", "/link-field-types", models.LinkFieldTypeInput{
			Name: "x", Category: models.LinkFieldTypeCategoryOther,
		})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("UpdateLinkFieldType_LookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		failDBTableOn(t, db, "link_field_types", "query")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/"+linkType.ID, models.LinkFieldTypeInput{
			Name: "y", Category: models.LinkFieldTypeCategoryOther,
		})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("UpdateLinkFieldType_SaveError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		failDBTableOn(t, db, "link_field_types", "update")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/"+linkType.ID, models.LinkFieldTypeInput{
			Name: "x", Category: models.LinkFieldTypeCategoryOther,
		})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("DeleteLinkFieldType_DeleteError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		failDBTableOn(t, db, "link_field_types", "delete")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "DELETE", "/link-field-types/"+linkType.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ReorderLinkFieldTypes_TotalCountError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		failDBTableOn(t, db, "link_field_types", "query", "row")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/reorder", models.LinkFieldTypeReorderInput{Order: []string{linkType.ID}})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ReorderLinkFieldTypes_UpdateError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		failDBTableOn(t, db, "link_field_types", "update")
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/reorder", models.LinkFieldTypeReorderInput{Order: []string{linkType.ID}})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ReorderLinkFieldTypes_OwnedIDCountError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		// Skip 1 successful Query call (the total count) so the fault lands
		// on the second Count (the owned-and-listed-ID check).
		failDBTableAfterNCalls(t, db, "link_field_types", 1)
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/reorder", models.LinkFieldTypeReorderInput{Order: []string{linkType.ID}})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ReorderLinkFieldTypes_FinalListError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)
		// Skip the two successful Count calls (total, then owned-ID check) so
		// the fault lands on the post-transaction re-listing Find.
		failDBTableAfterNCalls(t, db, "link_field_types", 2)
		router := newLinkFieldTypeRealDBRouter(db, owner.ID)
		resp := linkFieldTypeDoJSON(t, router, "PUT", "/link-field-types/reorder", models.LinkFieldTypeReorderInput{Order: []string{linkType.ID}})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})
}

// TestLinkFieldTypeController_MissingValidatedData mirrors
// TestTagController_MissingValidatedData for CreateLinkFieldType/
// UpdateLinkFieldType/ReorderLinkFieldTypes's own defensive GetValidated
// error branch.
func TestLinkFieldTypeController_MissingValidatedData(t *testing.T) {
	t.Run("CreateLinkFieldType", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Request, _ = http.NewRequest("POST", "/link-field-types", nil)

		CreateLinkFieldType(c)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("UpdateLinkFieldType", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)
		linkType := models.LinkFieldType{UserID: owner.ID, Name: "x", Category: models.LinkFieldTypeCategoryOther}
		require.NoError(t, db.Create(&linkType).Error)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Params = gin.Params{{Key: "id", Value: linkType.ID}}
		c.Request, _ = http.NewRequest("PUT", "/link-field-types/"+linkType.ID, nil)

		UpdateLinkFieldType(c)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("ReorderLinkFieldTypes", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createLinkFieldTypeTestUsers(t, db)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Request, _ = http.NewRequest("PUT", "/link-field-types/reorder", nil)

		ReorderLinkFieldTypes(c)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})
}
