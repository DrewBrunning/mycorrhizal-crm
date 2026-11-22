package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPreviewContactMerge_ListsNeverSerializeAsNull pins issue #2604: two
// contacts with no addresses/urls/impps/circles/field-value conflicts used to
// produce `null` for those keys (Go nil slices), which strict clients (the
// Android Moshi adapter) reject as "Unexpected server response". Every list in
// the resolution must be a JSON array.
func TestPreviewContactMerge_ListsNeverSerializeAsNull(t *testing.T) {
	db := dbtest.NewAt(t, filepath.Join(t.TempDir(), "merge-preview-nulls.db"))
	closeTestDBAtTeardown(t, db)
	user := models.User{Username: "nullmerge", Password: "password123!A", Email: "nullmerge@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/contacts/merge/preview", withValidated(func() any { return &models.ContactMergeRequest{} }), PreviewContactMerge)

	keeper := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}
	loser := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace-King"}
	require.NoError(t, db.Create(&keeper).Error)
	require.NoError(t, db.Create(&loser).Error)

	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(models.ContactMergeRequest{KeepID: keeper.ID, MergeID: loser.ID}))
	req, _ := http.NewRequest("POST", "/contacts/merge/preview", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var raw struct {
		Resolution map[string]json.RawMessage `json:"resolution"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	for _, key := range []string{"emails", "phones", "addresses", "urls", "impps", "circles", "conflicts", "field_value_conflicts"} {
		v, ok := raw.Resolution[key]
		require.True(t, ok, "resolution.%s must be present", key)
		assert.NotEqual(t, "null", string(v), "resolution.%s must be [] not null", key)
		assert.True(t, len(v) > 0 && v[0] == '[', "resolution.%s must be a JSON array, got %s", key, v)
	}
}
