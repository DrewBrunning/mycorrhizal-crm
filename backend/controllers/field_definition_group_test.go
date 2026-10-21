package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldDefinitionGroup covers issue #1730: the optional display-only
// `group` label round-trips through create/update/get/list on the real
// migrated schema, is trimmed and length-limited by the real validation
// middleware, can be cleared, is kept when an update omits it, and stays
// ownership-scoped.
func TestFieldDefinitionGroup(t *testing.T) {
	db := dbtest.New(t)
	owner := models.User{Username: "grpowner", Password: "password123!A", Email: "grpowner@example.com"}
	other := models.User{Username: "grpother", Password: "password123!A", Email: "grpother@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)

	gin.SetMode(gin.ReleaseMode)
	newRouter := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("db", db)
			c.Set("userID", userID)
			c.Next()
		})
		r.POST("/field-definitions", middleware.ValidateJSONMiddleware(&models.FieldDefinitionInput{}), CreateFieldDefinition)
		r.GET("/field-definitions", ListFieldDefinitions)
		r.GET("/field-definitions/:id", GetFieldDefinition)
		r.PUT("/field-definitions/:id", middleware.ValidateJSONMiddleware(&models.FieldDefinitionInput{}), UpdateFieldDefinition)
		return r
	}
	ownerRouter, otherRouter := newRouter(owner.ID), newRouter(other.ID)

	do := func(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	create := func(key string, extra map[string]any) *httptest.ResponseRecorder {
		body := map[string]any{"label": key, "key": key, "type": "string"}
		for k, v := range extra {
			body[k] = v
		}
		return do(ownerRouter, "POST", "/field-definitions", body)
	}
	decodeDef := func(w *httptest.ResponseRecorder) models.FieldDefinition {
		var out struct {
			FieldDefinition models.FieldDefinition `json:"field_definition"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		return out.FieldDefinition
	}

	t.Run("create with group is trimmed and persisted", func(t *testing.T) {
		w := create("airline", map[string]any{"group": "  Travel profile  "})
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		def := decodeDef(w)
		assert.Equal(t, "Travel profile", def.Group)

		var stored models.FieldDefinition
		require.NoError(t, db.First(&stored, "id = ?", def.ID).Error)
		assert.Equal(t, "Travel profile", stored.Group)
	})

	t.Run("create without group is ungrouped and group is always on the wire", func(t *testing.T) {
		w := create("plain", nil)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var raw struct {
			FieldDefinition map[string]json.RawMessage `json:"field_definition"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
		g, present := raw.FieldDefinition["group"]
		require.True(t, present, "group must not be omitempty")
		assert.JSONEq(t, `""`, string(g))
	})

	t.Run("whitespace-only group means ungrouped", func(t *testing.T) {
		w := create("blankgroup", map[string]any{"group": "   "})
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, "", decodeDef(w).Group)
	})

	t.Run("101 characters is a 400, 100 is accepted", func(t *testing.T) {
		w := create("toolong", map[string]any{"group": strings.Repeat("g", 101)})
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		w = create("maxlen", map[string]any{"group": strings.Repeat("g", 100)})
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		w = do(ownerRouter, "PUT", "/field-definitions/"+decodeDef(w).ID, map[string]any{
			"label": "maxlen", "key": "maxlen", "type": "string", "group": strings.Repeat("g", 101),
		})
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("the 100 limit counts characters, not bytes", func(t *testing.T) {
		// "é" is 2 bytes and "✈" is 3 in UTF-8: 100 of either is 200/300
		// bytes but still 100 characters, so it must be accepted; 101 is not.
		for i, ch := range []string{"é", "✈"} {
			w := create("mb"+strconv.Itoa(i), map[string]any{"group": strings.Repeat(ch, 100)})
			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			assert.Equal(t, strings.Repeat(ch, 100), decodeDef(w).Group)
			w = create("mbx"+strconv.Itoa(i), map[string]any{"group": strings.Repeat(ch, 101)})
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		}
	})

	t.Run("update sets, keeps on omission, and clears", func(t *testing.T) {
		def := decodeDef(create("upd", nil))
		put := func(extra map[string]any) models.FieldDefinition {
			body := map[string]any{"label": "upd", "key": "upd", "type": "string"}
			for k, v := range extra {
				body[k] = v
			}
			w := do(ownerRouter, "PUT", "/field-definitions/"+def.ID, body)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var out models.FieldDefinition
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
			return out
		}
		assert.Equal(t, "Family", put(map[string]any{"group": " Family "}).Group)
		assert.Equal(t, "Family", put(nil).Group, "omitting group on update must keep it")
		assert.Equal(t, "", put(map[string]any{"group": ""}).Group, "empty string clears the group")
	})

	t.Run("get and list return the group", func(t *testing.T) {
		def := decodeDef(create("listed", map[string]any{"group": "Listed group"}))
		w := do(ownerRouter, "GET", "/field-definitions/"+def.ID, nil)
		require.Equal(t, http.StatusOK, w.Code)
		var got models.FieldDefinition
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, "Listed group", got.Group)

		w = do(ownerRouter, "GET", "/field-definitions?limit=100", nil)
		require.Equal(t, http.StatusOK, w.Code)
		var list struct {
			FieldDefinitions []models.FieldDefinition `json:"field_definitions"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		groups := map[string]string{}
		keys := make([]string, 0, len(list.FieldDefinitions))
		for _, d := range list.FieldDefinitions {
			groups[d.Key] = d.Group
			keys = append(keys, d.Key)
		}
		assert.Equal(t, "Listed group", groups["listed"])
		assert.Equal(t, "Travel profile", groups["airline"])
		assert.Equal(t, "", groups["plain"])
		// Order is still by position (creation order), not by group.
		assert.Less(t, indexOfKey(keys, "airline"), indexOfKey(keys, "plain"))
	})

	t.Run("another user's definition is 404 for get and update", func(t *testing.T) {
		def := decodeDef(create("private-owner", map[string]any{"group": "Mine"}))
		assert.Equal(t, http.StatusNotFound, do(otherRouter, "GET", "/field-definitions/"+def.ID, nil).Code)
		w := do(otherRouter, "PUT", "/field-definitions/"+def.ID, map[string]any{
			"label": "x", "key": "private-owner", "type": "string", "group": "Stolen",
		})
		assert.Equal(t, http.StatusNotFound, w.Code)
		var stored models.FieldDefinition
		require.NoError(t, db.First(&stored, "id = ?", def.ID).Error)
		assert.Equal(t, "Mine", stored.Group)
	})
}

func indexOfKey(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
