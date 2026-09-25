package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// failDBTableOn makes every subsequent GORM operation of the given kinds
// ("create", "query", "update", "delete", "row") against the given table
// fail, simulating a storage error. This is the only way to exercise a
// handler's generic "Failed to X" 500 branch — a happy-path or
// validation-error test can never reach it — and mirrors the fault-injection
// pattern backend/main_test.go's failRawExec and
// backend/database/fault_injection_test.go already use in this repo.
// Registering per-kind (rather than blanket-failing every operation on a
// table) lets a test fail only the specific step under test — e.g. faulting
// only "update" so a handler's preceding First() lookup still succeeds and
// the fault is isolated to the Save() call that follows it.
func failDBTableOn(t *testing.T, db *gorm.DB, table string, kinds ...string) {
	t.Helper()
	fail := func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			tx.AddError(fmt.Errorf("simulated db failure: %s", table))
		}
	}
	for _, kind := range kinds {
		name := fmt.Sprintf("fail_%s_%s", kind, table)
		var regErr error
		switch kind {
		case "create":
			regErr = db.Callback().Create().Before("gorm:create").Register(name, fail)
		case "query":
			regErr = db.Callback().Query().Before("gorm:query").Register(name, fail)
		case "update":
			regErr = db.Callback().Update().Before("gorm:update").Register(name, fail)
		case "delete":
			regErr = db.Callback().Delete().Before("gorm:delete").Register(name, fail)
		case "row":
			regErr = db.Callback().Row().Before("gorm:row").Register(name, fail)
		default:
			t.Fatalf("failDBTableOn: unknown kind %q", kind)
		}
		require.NoError(t, regErr)
	}
}

// failDBTableAfterNCalls fails every Query-kind GORM call against the given
// table starting with the (skipCalls+1)-th one, letting the first skipCalls
// calls succeed. Needed where a handler issues two or more Query-kind calls
// against the same table in sequence (e.g. a Count followed by a Find) and a
// test wants to isolate the fault to a later one without the earlier ones
// also failing — failDBTableOn's per-kind granularity can't distinguish
// between two calls of the *same* kind.
func failDBTableAfterNCalls(t *testing.T, db *gorm.DB, table string, skipCalls int) {
	t.Helper()
	calls := 0
	fail := func(tx *gorm.DB) {
		if tx.Statement.Table != table {
			return
		}
		calls++
		if calls > skipCalls {
			tx.AddError(fmt.Errorf("simulated db failure: %s (call %d)", table, calls))
		}
	}
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(fmt.Sprintf("fail_query_after_%d_%s", skipCalls, table), fail))
}

// newTagRealDBRouter wires the real Tag routes (the exact middleware chain
// routes.go uses: ValidateJSONMiddleware, not a test shim) against a
// dbtest.New(t) real-migrated-schema database (CLAUDE.md backend trap #1),
// with a fixed caller identity injected the way AuthMiddleware normally
// would. Kept local to this file rather than reusing setupRouter/
// withValidated from tag_controller_test.go, which another agent is
// rewriting concurrently.
func newTagRealDBRouter(db *gorm.DB, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", userID)
		c.Next()
	})
	router.POST("/tags", middleware.ValidateJSONMiddleware(&models.TagInput{}), CreateTag)
	router.GET("/tags", ListTags)
	router.GET("/tags/:id", GetTag)
	router.PUT("/tags/:id", middleware.ValidateJSONMiddleware(&models.TagInput{}), UpdateTag)
	router.DELETE("/tags/:id", DeleteTag)
	router.POST("/tags/:id/contacts", middleware.ValidateJSONMiddleware(&models.ContactTagInput{}), AddContactTag)
	router.DELETE("/tags/:id/contacts/:vcard_uid", RemoveContactTag)
	return router
}

func tagDoJSON(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

// createTagTestUsers seeds two users against a real migrated schema, for
// every cross-user-scoping test in this file.
func createTagTestUsers(t *testing.T, db *gorm.DB) (owner, other models.User) {
	t.Helper()
	owner = models.User{Username: "tag-owner", Password: "password123!A", Email: "tag-owner@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	other = models.User{Username: "tag-other", Password: "password123!A", Email: "tag-other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	return owner, other
}

// TestTagController_CrossUserScoping proves the owning user's Tag is
// completely invisible to another authenticated user: GET, PUT and DELETE
// by its exact ID must all 404, never reveal existence via a different
// status, and never actually mutate the row.
func TestTagController_CrossUserScoping(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createTagTestUsers(t, db)

	tag := models.Tag{UserID: owner.ID, Name: "owner-only"}
	require.NoError(t, db.Create(&tag).Error)

	otherRouter := newTagRealDBRouter(db, other.ID)

	getResp := tagDoJSON(t, otherRouter, "GET", "/tags/"+tag.ID, nil)
	require.Equal(t, http.StatusNotFound, getResp.Code, getResp.Body.String())

	putResp := tagDoJSON(t, otherRouter, "PUT", "/tags/"+tag.ID, models.TagInput{Name: "hijacked"})
	require.Equal(t, http.StatusNotFound, putResp.Code, putResp.Body.String())

	deleteResp := tagDoJSON(t, otherRouter, "DELETE", "/tags/"+tag.ID, nil)
	require.Equal(t, http.StatusNotFound, deleteResp.Code, deleteResp.Body.String())

	var reloaded models.Tag
	require.NoError(t, db.First(&reloaded, "id = ?", tag.ID).Error)
	require.Equal(t, "owner-only", reloaded.Name, "a 404'd cross-user PUT must not have mutated the row")
}

// TestTagController_CrossUserScoping_ListDoesNotLeak proves ListTags never
// returns another user's tags, with or without include_contacts.
func TestTagController_CrossUserScoping_ListDoesNotLeak(t *testing.T) {
	db := dbtest.New(t)
	owner, other := createTagTestUsers(t, db)

	ownerContact := models.Contact{UserID: owner.ID, Firstname: "Mine"}
	require.NoError(t, db.Create(&ownerContact).Error)
	ownerTag := models.Tag{UserID: owner.ID, Name: "mine"}
	require.NoError(t, db.Create(&ownerTag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: ownerTag.ID, UserID: owner.ID, ContactVCardUID: ownerContact.VCardUID}).Error)

	otherContact := models.Contact{UserID: other.ID, Firstname: "Theirs"}
	require.NoError(t, db.Create(&otherContact).Error)
	otherTag := models.Tag{UserID: other.ID, Name: "theirs"}
	require.NoError(t, db.Create(&otherTag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: otherTag.ID, UserID: other.ID, ContactVCardUID: otherContact.VCardUID}).Error)

	ownerRouter := newTagRealDBRouter(db, owner.ID)

	listResp := tagDoJSON(t, ownerRouter, "GET", "/tags?include_contacts=true", nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())

	var body struct {
		Tags     []models.Tag        `json:"tags"`
		Total    int64               `json:"total"`
		Contacts []models.ContactTag `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &body))

	require.Len(t, body.Tags, 1, "must only see the caller's own tag")
	require.Equal(t, "mine", body.Tags[0].Name)
	require.EqualValues(t, 1, body.Total)

	require.Len(t, body.Contacts, 1, "include_contacts must only include the caller's own contact-tag rows")
	require.Equal(t, ownerTag.ID, body.Contacts[0].TagID)
	require.Equal(t, ownerContact.VCardUID, body.Contacts[0].ContactVCardUID)
}

// TestTagController_ListTags_IncludeContactsFalseOmitsField pins the other
// half of the include_contacts contract: when the query flag is absent, the
// response carries no "contacts" key at all (the branch at
// tag_controller.go's ListTags is skipped entirely).
func TestTagController_ListTags_IncludeContactsFalseOmitsField(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createTagTestUsers(t, db)
	require.NoError(t, db.Create(&models.Tag{UserID: owner.ID, Name: "solo"}).Error)

	router := newTagRealDBRouter(db, owner.ID)
	resp := tagDoJSON(t, router, "GET", "/tags", nil)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	_, hasContacts := body["contacts"]
	require.False(t, hasContacts, "contacts key must be absent when include_contacts is not requested")
}

// TestTagController_ListTags_CursorPagination exercises the next_cursor
// branch: a page smaller than the total set must report a non-empty cursor
// and, when followed, return the remaining rows with an empty cursor.
func TestTagController_ListTags_CursorPagination(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createTagTestUsers(t, db)

	for _, name := range []string{"alpha", "bravo", "charlie"} {
		tag := models.Tag{UserID: owner.ID, Name: name}
		require.NoError(t, db.Create(&tag).Error)
	}

	router := newTagRealDBRouter(db, owner.ID)

	firstResp := tagDoJSON(t, router, "GET", "/tags?limit=2", nil)
	require.Equal(t, http.StatusOK, firstResp.Code, firstResp.Body.String())
	var firstPage struct {
		Tags       []models.Tag `json:"tags"`
		NextCursor string       `json:"next_cursor"`
		Total      int64        `json:"total"`
	}
	require.NoError(t, json.Unmarshal(firstResp.Body.Bytes(), &firstPage))
	require.Len(t, firstPage.Tags, 2)
	require.EqualValues(t, 3, firstPage.Total)
	require.NotEmpty(t, firstPage.NextCursor, "a page smaller than the total set must carry a next_cursor")

	secondResp := tagDoJSON(t, router, "GET", "/tags?limit=2&cursor="+firstPage.NextCursor, nil)
	require.Equal(t, http.StatusOK, secondResp.Code, secondResp.Body.String())
	var secondPage struct {
		Tags       []models.Tag `json:"tags"`
		NextCursor string       `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(secondResp.Body.Bytes(), &secondPage))
	require.Len(t, secondPage.Tags, 1, "the remaining row must come back on the second page")
	require.Empty(t, secondPage.NextCursor, "the final page must carry no next_cursor")
}

// TestTagController_GetUpdateDelete_NotFoundForUnknownID pins the plain
// (same-user) 404 path for an ID that simply doesn't exist, distinct from
// the cross-user case above.
func TestTagController_GetUpdateDelete_NotFoundForUnknownID(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createTagTestUsers(t, db)
	router := newTagRealDBRouter(db, owner.ID)

	const bogusID = "does-not-exist"

	getResp := tagDoJSON(t, router, "GET", "/tags/"+bogusID, nil)
	require.Equal(t, http.StatusNotFound, getResp.Code, getResp.Body.String())

	putResp := tagDoJSON(t, router, "PUT", "/tags/"+bogusID, models.TagInput{Name: "whatever"})
	require.Equal(t, http.StatusNotFound, putResp.Code, putResp.Body.String())

	deleteResp := tagDoJSON(t, router, "DELETE", "/tags/"+bogusID, nil)
	require.Equal(t, http.StatusNotFound, deleteResp.Code, deleteResp.Body.String())
}

// TestTagController_AddContactTag_TagNotFound pins the "tag itself doesn't
// exist" 404 branch of AddContactTag, distinct from the
// "contact doesn't belong to caller" 404 already covered by
// TestAddContactTagRejectsContactFromAnotherUser in tag_controller_test.go.
func TestTagController_AddContactTag_TagNotFound(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createTagTestUsers(t, db)
	contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	router := newTagRealDBRouter(db, owner.ID)
	resp := tagDoJSON(t, router, "POST", "/tags/does-not-exist/contacts", models.ContactTagInput{ContactVCardUID: contact.VCardUID})
	require.Equal(t, http.StatusNotFound, resp.Code, resp.Body.String())
}

// TestTagController_RemoveContactTag_NotFound pins RemoveContactTag's two
// distinct not-found branches: the tag itself missing, and an existing tag
// with no matching tagging row (RowsAffected == 0).
func TestTagController_RemoveContactTag_NotFound(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createTagTestUsers(t, db)
	contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)
	tag := models.Tag{UserID: owner.ID, Name: "poly"}
	require.NoError(t, db.Create(&tag).Error)

	router := newTagRealDBRouter(db, owner.ID)

	missingTagResp := tagDoJSON(t, router, "DELETE", "/tags/does-not-exist/contacts/"+contact.VCardUID, nil)
	require.Equal(t, http.StatusNotFound, missingTagResp.Code, missingTagResp.Body.String())

	noTaggingResp := tagDoJSON(t, router, "DELETE", "/tags/"+tag.ID+"/contacts/"+contact.VCardUID, nil)
	require.Equal(t, http.StatusNotFound, noTaggingResp.Code, noTaggingResp.Body.String())
}

// TestTagController_CreateTag_Success is the real-router (not the withValidated
// shim) happy path for CreateTag, proving the row lands scoped to the caller.
func TestTagController_CreateTag_Success(t *testing.T) {
	db := dbtest.New(t)
	owner, _ := createTagTestUsers(t, db)
	router := newTagRealDBRouter(db, owner.ID)

	resp := tagDoJSON(t, router, "POST", "/tags", models.TagInput{Name: "fresh"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.Tag{}).Where("user_id = ? AND name = ?", owner.ID, "fresh").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// TestTagController_DatabaseErrorBranches exercises every "Failed to X" 500
// branch in tag_controller.go that no happy-path or not-found test can
// reach: each sub-test isolates its fault to the exact step under test (the
// preceding lookups, when any, still succeed) via failDBTableOn.
func TestTagController_DatabaseErrorBranches(t *testing.T) {
	t.Run("CreateTag", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		failDBTableOn(t, db, "tags", "create")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "POST", "/tags", models.TagInput{Name: "x"})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("GetTag_LookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)
		failDBTableOn(t, db, "tags", "query")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "GET", "/tags/"+tag.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("GetTag_ContactTagsLookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)
		failDBTableOn(t, db, "contact_tags", "query")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "GET", "/tags/"+tag.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ListTags_CountError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		failDBTableOn(t, db, "tags", "query", "row")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "GET", "/tags", nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("ListTags_IncludeContactsError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		require.NoError(t, db.Create(&models.Tag{UserID: owner.ID, Name: "x"}).Error)
		failDBTableOn(t, db, "contact_tags", "query")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "GET", "/tags?include_contacts=true", nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("UpdateTag_SaveError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		tag := models.Tag{UserID: owner.ID, Name: "old"}
		require.NoError(t, db.Create(&tag).Error)
		failDBTableOn(t, db, "tags", "update")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "PUT", "/tags/"+tag.ID, models.TagInput{Name: "new"})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("DeleteTag_DeleteError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)
		failDBTableOn(t, db, "tags", "delete")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "DELETE", "/tags/"+tag.ID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("AddContactTag_ContactLookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)
		failDBTableOn(t, db, "contacts", "query")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "POST", "/tags/"+tag.ID+"/contacts", models.ContactTagInput{ContactVCardUID: contact.VCardUID})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("AddContactTag_ExistingLookupError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)
		failDBTableOn(t, db, "contact_tags", "query")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "POST", "/tags/"+tag.ID+"/contacts", models.ContactTagInput{ContactVCardUID: contact.VCardUID})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("AddContactTag_CreateError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)
		failDBTableOn(t, db, "contact_tags", "create")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "POST", "/tags/"+tag.ID+"/contacts", models.ContactTagInput{ContactVCardUID: contact.VCardUID})
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})

	t.Run("RemoveContactTag_DeleteError", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		contact := models.Contact{UserID: owner.ID, Firstname: "Ada"}
		require.NoError(t, db.Create(&contact).Error)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)
		require.NoError(t, db.Create(&models.ContactTag{TagID: tag.ID, UserID: owner.ID, ContactVCardUID: contact.VCardUID}).Error)
		failDBTableOn(t, db, "contact_tags", "delete")
		router := newTagRealDBRouter(db, owner.ID)
		resp := tagDoJSON(t, router, "DELETE", "/tags/"+tag.ID+"/contacts/"+contact.VCardUID, nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code, resp.Body.String())
	})
}

// TestTagController_MissingValidatedData pins CreateTag/UpdateTag's own
// defensive GetValidated error branch: middleware.ValidateJSONMiddleware
// always populates "validated" before these handlers run in production
// (routes.go), so this simulates a route wired without it — the one way to
// reach that branch — and expects the handler's own 400, not a panic on a
// missing context value.
func TestTagController_MissingValidatedData(t *testing.T) {
	t.Run("CreateTag", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Request, _ = http.NewRequest("POST", "/tags", nil)

		CreateTag(c)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("UpdateTag", func(t *testing.T) {
		db := dbtest.New(t)
		owner, _ := createTagTestUsers(t, db)
		tag := models.Tag{UserID: owner.ID, Name: "x"}
		require.NoError(t, db.Create(&tag).Error)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Params = gin.Params{{Key: "id", Value: tag.ID}}
		c.Request, _ = http.NewRequest("PUT", "/tags/"+tag.ID, nil)

		UpdateTag(c)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})
}
