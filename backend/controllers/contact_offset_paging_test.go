package controllers

import (
	"encoding/json"
	"fmt"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func offsetPage(t *testing.T, router *gin.Engine, query string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("GET", "/contacts?"+query, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

// #2188: ?offset= + ?include_total=true let the web client render page
// numbers and a total count over the cursor-paged list.
func TestGetContacts_OffsetAndTotal(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	db.First(&user)
	router.GET("/contacts", GetContacts)
	for i := 0; i < 7; i++ {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: fmt.Sprintf("C%d", i), Email: "x@example.com"}).Error)
	}
	// Another user's contact must never be counted.
	other := models.User{Username: "other-offset", Email: "o@example.com", Password: "x"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: other.ID, Firstname: "Foreign"}).Error)

	t.Run("total absent unless requested", func(t *testing.T) {
		code, body := offsetPage(t, router, "limit=3")
		require.Equal(t, 200, code)
		assert.NotContains(t, body, "total")
	})

	t.Run("total is whole filtered set, not the page", func(t *testing.T) {
		code, body := offsetPage(t, router, "limit=3&include_total=true&offset=3&sort=name&order=asc")
		require.Equal(t, 200, code)
		assert.Equal(t, float64(7), body["total"])
		items := body["contacts"].([]any)
		require.Len(t, items, 3)
		assert.Equal(t, "C3", items[0].(map[string]any)["firstname"])
		assert.NotEmpty(t, body["next_cursor"])
	})

	t.Run("last partial page has no next_cursor", func(t *testing.T) {
		_, body := offsetPage(t, router, "limit=3&offset=6&sort=name&order=asc&include_total=true")
		assert.Len(t, body["contacts"].([]any), 1)
		assert.Empty(t, body["next_cursor"])
	})

	t.Run("offset past the end is an empty page", func(t *testing.T) {
		code, body := offsetPage(t, router, "limit=3&offset=300&include_total=true")
		require.Equal(t, 200, code)
		assert.Empty(t, body["contacts"])
		assert.Equal(t, float64(7), body["total"])
	})

	t.Run("total honours filters", func(t *testing.T) {
		_, body := offsetPage(t, router, "limit=3&include_total=true&search=C1")
		assert.Equal(t, float64(1), body["total"])
	})

	t.Run("total and offset work with includes", func(t *testing.T) {
		code, body := offsetPage(t, router, "limit=3&offset=3&include_total=true&includes=notes&sort=name&order=asc")
		require.Equal(t, 200, code)
		assert.Equal(t, float64(7), body["total"])
		assert.Len(t, body["contacts"].([]any), 3)
	})

	t.Run("offset with a valid cursor or since is 400", func(t *testing.T) {
		_, first := offsetPage(t, router, "limit=2&sort=name&order=asc")
		cur, _ := first["next_cursor"].(string)
		require.NotEmpty(t, cur)
		code, _ := offsetPage(t, router, "limit=2&sort=name&order=asc&offset=2&cursor="+cur)
		assert.Equal(t, http.StatusBadRequest, code)
		code, _ = offsetPage(t, router, "offset=2&since=2020-01-01T00:00:00Z")
		assert.Equal(t, http.StatusBadRequest, code)
	})

	t.Run("bad values are 400", func(t *testing.T) {
		for _, q := range []string{"offset=-1", "offset=abc", "include_total=maybe", "offset=2&cursor=abc"} {
			code, _ := offsetPage(t, router, q)
			assert.Equal(t, http.StatusBadRequest, code, q)
		}
	})
}

// A storage failure surfaces as a 500 on both the count and the page query.
func TestGetContacts_OffsetAndTotalDatabaseErrors(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Request error")
	db, router := setupRouter(t)
	router.GET("/contacts", GetContacts)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = OFF").Error)
	require.NoError(t, db.Migrator().DropTable("contacts"))

	code, _ := offsetPage(t, router, "include_total=true")
	assert.Equal(t, http.StatusInternalServerError, code)
	code, _ = offsetPage(t, router, "offset=5")
	assert.Equal(t, http.StatusInternalServerError, code)
}
