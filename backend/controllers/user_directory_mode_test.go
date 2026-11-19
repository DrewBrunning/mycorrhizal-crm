package controllers

import (
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func directoryRouter(t *testing.T, mode string) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, router := setupRouter(t)
	router.Use(func(c *gin.Context) {
		c.Set("cfg", config.Config{ShareDirectoryMode: mode})
		c.Next()
	})
	router.GET("/users/directory", ListUserDirectory)
	return db, router
}

func directoryNames(t *testing.T, router *gin.Engine, q string) (int, []string, string) {
	t.Helper()
	path := "/users/directory"
	if q != "" {
		path += "?q=" + url.QueryEscape(q)
	}
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp struct {
		Users []UserDirectoryEntry `json:"users"`
	}
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.NotNil(t, resp.Users)
	}
	names := []string{}
	for _, u := range resp.Users {
		names = append(names, u.Username)
	}
	return w.Code, names, w.Body.String()
}

func seedDirectoryUsers(t *testing.T, db *gorm.DB, names ...string) {
	t.Helper()
	for _, n := range names {
		require.NoError(t, db.Create(&models.User{Username: n, Email: n + "@example.com", Password: "pw-hash"}).Error)
	}
}

func TestListUserDirectory_AllModeIgnoresQAndListsEveryone(t *testing.T) {
	for _, mode := range []string{"", "all"} {
		db, router := directoryRouter(t, mode)
		seedDirectoryUsers(t, db, "alice", "bob")
		code, names, _ := directoryNames(t, router, "")
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, []string{"alice", "bob"}, names, "mode %q", mode)
		_, names, _ = directoryNames(t, router, "zz")
		assert.Equal(t, []string{"alice", "bob"}, names, "all mode keeps its unchanged behaviour regardless of q")
	}
}

func TestListUserDirectory_SearchModeRequiresThreeCharPrefix(t *testing.T) {
	db, router := directoryRouter(t, "search")
	seedDirectoryUsers(t, db, "alice", "alicia", "bob", "Alberto")

	code, names, body := directoryNames(t, router, "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, []string{}, names, "empty q returns nothing")
	assert.JSONEq(t, `{"users":[]}`, body)

	for _, q := range []string{"a", "al", "  al ", "  "} {
		code, names, _ = directoryNames(t, router, q)
		assert.Equal(t, http.StatusOK, code)
		assert.Empty(t, names, "q %q is under 3 characters", q)
	}

	_, names, _ = directoryNames(t, router, "ali")
	assert.Equal(t, []string{"alice", "alicia"}, names)
	_, names, _ = directoryNames(t, router, "ALI")
	assert.Equal(t, []string{"alice", "alicia"}, names, "case-insensitive")
	_, names, _ = directoryNames(t, router, "alb")
	assert.Equal(t, []string{"Alberto"}, names)
	_, names, _ = directoryNames(t, router, "lic")
	assert.Empty(t, names, "prefix only, never substring")
}

func TestListUserDirectory_SearchModeNeverMatchesEmailOrDisplayName(t *testing.T) {
	db, router := directoryRouter(t, "search")
	seedDirectoryUsers(t, db, "carol")
	require.NoError(t, db.Model(&models.User{}).Where("username = ?", "carol").Update("email", "secretmail@corp.example").Error)
	for _, q := range []string{"secretmail", "corp", "carol@", "@"} {
		_, names, body := directoryNames(t, router, q)
		assert.Empty(t, names, "q %q", q)
		assert.NotContains(t, body, "secretmail")
	}
	_, _, body := directoryNames(t, router, "car")
	assert.NotContains(t, body, "secretmail")
	assert.NotContains(t, body, "pw-hash")
}

func TestListUserDirectory_SearchModeTreatsWildcardsLiterally(t *testing.T) {
	db, router := directoryRouter(t, "search")
	seedDirectoryUsers(t, db, "abc", "a_c", "a%c")
	for _, q := range []string{"%%%", "___", "a%c", "a_c"} {
		_, names, _ := directoryNames(t, router, q)
		for _, n := range names {
			assert.Equal(t, q, n, "q %q may only match itself literally", q)
		}
	}
	_, names, _ := directoryNames(t, router, "a_c")
	assert.Equal(t, []string{"a_c"}, names)
	_, names, _ = directoryNames(t, router, "%%%")
	assert.Empty(t, names)
}

func TestListUserDirectory_SearchModeCapsResultsAndExcludesCaller(t *testing.T) {
	db, router := directoryRouter(t, "search")
	var names []string
	for i := 0; i < 15; i++ {
		names = append(names, "member"+string(rune('a'+i)))
	}
	seedDirectoryUsers(t, db, names...)
	_, got, _ := directoryNames(t, router, "mem")
	assert.Len(t, got, UserDirectorySearchLimit)
	assert.Equal(t, "membera", got[0], "ordered by username before the cap")

	_, got, _ = directoryNames(t, router, "tes") // the seeded caller "tester"
	assert.Empty(t, got, "the caller is never listed")
}
