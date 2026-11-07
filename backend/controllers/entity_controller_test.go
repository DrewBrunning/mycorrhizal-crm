package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Entity CRUD, restore and entity field values (issue #2296, ADR 0043) against
// the real migrated schema, with the real validation middleware.

type entityEnv struct {
	db     *gorm.DB
	router *gin.Engine
	userID uint
}

func newEntityEnv(t *testing.T) *entityEnv {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.POST("/entities", middleware.ValidateJSONMiddleware(&models.EntityInput{}), CreateEntity)
	router.GET("/entities", ListEntities)
	router.GET("/entities/:id", GetEntity)
	router.PUT("/entities/:id", middleware.ValidateJSONMiddleware(&models.EntityInput{}), UpdateEntity)
	router.DELETE("/entities/:id", DeleteEntity)
	router.POST("/entities/:id/restore", RestoreEntity)
	router.GET("/entities/:id/contacts", ListEntityContacts)
	router.GET("/entities/:id/field-values", ListEntityFieldValues)
	router.PUT("/entities/:id/field-values", middleware.ValidateJSONMiddleware(&models.ContactFieldValuesInput{}), ReplaceEntityFieldValues)
	router.POST("/field-definitions", middleware.ValidateJSONMiddleware(&models.FieldDefinitionInput{}), CreateFieldDefinition)
	router.PUT("/field-definitions/:id", middleware.ValidateJSONMiddleware(&models.FieldDefinitionInput{}), UpdateFieldDefinition)
	router.PUT("/contacts/:id/field-values", middleware.ValidateJSONMiddleware(&models.ContactFieldValuesInput{}), ReplaceContactFieldValues)
	return &entityEnv{db: db, router: router, userID: user.ID}
}

func (e *entityEnv) do(method, path string, body any, hdr ...string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		if s, ok := body.(string); ok {
			buf.WriteString(s)
		} else {
			_ = json.NewEncoder(&buf).Encode(body)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func (e *entityEnv) create(t *testing.T, body any) models.EntityResponse {
	t.Helper()
	w := e.do(http.MethodPost, "/entities", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var r models.EntityResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	return r
}

func entityReason(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			ReasonCode string `json:"reason_code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Error.ReasonCode
}

func TestEntity_CreateGetUpdateRoundTrip(t *testing.T) {
	e := newEntityEnv(t)
	r := e.create(t, map[string]any{"kind": "organisation", "name": "  Acme Corp ", "sensitivity": "private",
		"core": map[string]any{"organization": map[string]any{"name": "Acme Corp"}, "url": "https://acme.example"}})
	assert.Equal(t, "Acme Corp", r.Name)
	assert.Equal(t, "private", r.Sensitivity)
	assert.EqualValues(t, 1, r.Revision)
	assert.JSONEq(t, `{"organization":{"name":"Acme Corp"},"url":"https://acme.example"}`, string(r.Core))

	w := e.do(http.MethodGet, "/entities/"+r.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var got models.EntityResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, r.ID, got.ID)
	require.NotNil(t, got.LinkCount)
	assert.Equal(t, 0, *got.LinkCount)

	w = e.do(http.MethodPut, "/entities/"+r.ID, map[string]any{"name": "Acme Inc"}, "If-Match", "1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var upd models.EntityResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &upd))
	assert.Equal(t, "Acme Inc", upd.Name)
	assert.EqualValues(t, 2, upd.Revision)
	assert.JSONEq(t, string(r.Core), string(upd.Core), "omitted core is kept")
	assert.Equal(t, "private", upd.Sensitivity, "omitted sensitivity is kept")
}

func TestEntity_CreateValidation(t *testing.T) {
	e := newEntityEnv(t)
	cases := map[string]struct {
		body   any
		reason string
	}{
		"missing kind":     {map[string]any{"name": "X"}, string(apperrors.ReasonEntityKindRequired)},
		"unknown core key": {map[string]any{"kind": "pet", "name": "Rex", "core": map[string]any{"bogus": 1}}, string(apperrors.ReasonEntityCoreInvalid)},
		"place addr sens":  {map[string]any{"kind": "place", "name": "Cabin", "core": map[string]any{"address": map[string]any{"sensitivity": "private"}}}, string(apperrors.ReasonEntityCoreInvalid)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := e.do(http.MethodPost, "/entities", tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Equal(t, tc.reason, entityReason(t, w))
		})
	}
	// struct-tag failures: bad kind, blank name, long name, bad sensitivity.
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	for name, body := range map[string]any{
		"bad kind":   map[string]any{"kind": "robot", "name": "X"},
		"no name":    map[string]any{"kind": "pet"},
		"long name":  map[string]any{"kind": "pet", "name": string(long)},
		"bad sens":   map[string]any{"kind": "pet", "name": "X", "sensitivity": "top"},
		"blank name": map[string]any{"kind": "pet", "name": "   "},
	} {
		t.Run(name, func(t *testing.T) {
			w := e.do(http.MethodPost, "/entities", body)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
	var n int64
	require.NoError(t, e.db.Model(&models.Entity{}).Count(&n).Error)
	assert.Zero(t, n, "rejected payloads write nothing")
}

func TestEntity_KindImmutableAndNoMassAssignment(t *testing.T) {
	e := newEntityEnv(t)
	r := e.create(t, map[string]any{"kind": "pet", "name": "Rex"})
	w := e.do(http.MethodPut, "/entities/"+r.ID, map[string]any{"kind": "place", "name": "Rex"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, string(apperrors.ReasonEntityKindImmutable), entityReason(t, w))

	// Unknown/forbidden keys (id, user_id, name_key, revision) are not writable.
	w = e.do(http.MethodPut, "/entities/"+r.ID, `{"name":"Rexy","id":"x","user_id":999,"name_key":"zz","revision":50}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var ent models.Entity
	require.NoError(t, e.db.First(&ent, "id = ?", r.ID).Error)
	assert.Equal(t, e.userID, ent.UserID)
	assert.Equal(t, "rexy", ent.NameKey)
	assert.EqualValues(t, 2, ent.Revision)
}

func TestEntity_IfMatch(t *testing.T) {
	e := newEntityEnv(t)
	r := e.create(t, map[string]any{"kind": "pet", "name": "Rex"})
	w := e.do(http.MethodPut, "/entities/"+r.ID, map[string]any{"name": "Rexy"}, "If-Match", "7")
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
	w = e.do(http.MethodDelete, "/entities/"+r.ID, nil, "If-Match", "7")
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
	var n int64
	require.NoError(t, e.db.Model(&models.Entity{}).Where("id = ?", r.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "stale delete changed nothing")
}

func TestEntity_ListFilters(t *testing.T) {
	e := newEntityEnv(t)
	e.create(t, map[string]any{"kind": "organisation", "name": "Acme Corp"})
	e.create(t, map[string]any{"kind": "organisation", "name": "100%_Club", "sensitivity": "secret"})
	e.create(t, map[string]any{"kind": "pet", "name": "Acme the Dog"})
	list := func(q string) models.EntityListResponse {
		w := e.do(http.MethodGet, "/entities"+q, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var l models.EntityListResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &l))
		return l
	}
	assert.Len(t, list("").Entities, 3)
	assert.Len(t, list("?kind=organisation").Entities, 2)
	assert.Len(t, list("?q=acme").Entities, 2)
	assert.Len(t, list("?q=ACME&kind=pet").Entities, 1)
	assert.Len(t, list("?sensitivity=secret").Entities, 1, "owner sees secret rows when asked")
	assert.Len(t, list("?q=%25_").Entities, 1, "LIKE metacharacters are literal")
	assert.Len(t, list("?q=%25").Entities, 1)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities?kind=robot", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities?sensitivity=x", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities?cursor=not-a-cursor", nil).Code)

	page := list("?limit=2")
	require.Len(t, page.Entities, 2)
	require.NotEmpty(t, page.NextCursor)
	rest := list("?limit=2&cursor=" + page.NextCursor)
	require.Len(t, rest.Entities, 1)
	assert.Empty(t, rest.NextCursor)

	var raw map[string]json.RawMessage
	w := e.do(http.MethodGet, "/entities?kind=place", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["entities"]), "empty collection is [] not null")
}

// Ownership: another user's entity is a 404 on every route and nothing changes.
func TestEntity_CrossUserIsNotFound(t *testing.T) {
	e := newEntityEnv(t)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	foreign := models.Entity{UserID: other.ID, Kind: models.EntityKindOrganisation, Name: "Theirs"}
	require.NoError(t, e.db.Create(&foreign).Error)
	def := models.FieldDefinition{UserID: e.userID, Label: "I", Key: "i", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&def).Error)

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/entities/" + foreign.ID, nil},
		{http.MethodPut, "/entities/" + foreign.ID, map[string]any{"name": "Mine now"}},
		{http.MethodDelete, "/entities/" + foreign.ID, nil},
		{http.MethodGet, "/entities/" + foreign.ID + "/contacts", nil},
		{http.MethodGet, "/entities/" + foreign.ID + "/field-values", nil},
		{http.MethodPut, "/entities/" + foreign.ID + "/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": def.ID, "value": "x"}}}},
		{http.MethodGet, "/entities/does-not-exist", nil},
	} {
		w := e.do(tc.method, tc.path, tc.body)
		assert.Equal(t, http.StatusNotFound, w.Code, tc.method+" "+tc.path+" "+w.Body.String())
	}
	var ent models.Entity
	require.NoError(t, e.db.First(&ent, "id = ?", foreign.ID).Error)
	assert.Equal(t, "Theirs", ent.Name)
	var vals int64
	require.NoError(t, e.db.Model(&models.FieldValue{}).Count(&vals).Error)
	assert.Zero(t, vals)

	// A foreign entity is not listed, and cannot be restored.
	require.NoError(t, e.db.Delete(&foreign).Error)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/entities/"+foreign.ID+"/restore", nil).Code)
	assert.Len(t, func() []models.Entity {
		var l []models.Entity
		e.db.Unscoped().Where("id = ? AND deleted_at IS NOT NULL", foreign.ID).Find(&l)
		return l
	}(), 1, "foreign tombstone untouched")
}

func seedContactLink(t *testing.T, e *entityEnv, entityID, kind string, primary bool) (models.Contact, models.ContactEntityLink) {
	t.Helper()
	c := models.Contact{UserID: e.userID, Firstname: "Ada", Lastname: "L"}
	require.NoError(t, e.db.Create(&c).Error)
	var ent models.Entity
	require.NoError(t, e.db.First(&ent, "id = ?", entityID).Error)
	role := "employee"
	l := models.ContactEntityLink{UserID: e.userID, ContactVCardUID: c.VCardUID, EntityID: entityID, Kind: kind,
		Role: role, Sensitivity: "normal", IsPrimary: primary}
	require.NoError(t, e.db.Create(&l).Error)
	return c, l
}

func TestEntity_DeleteRestoreRoundTrip(t *testing.T) {
	e := newEntityEnv(t)
	org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
	contact, link := seedContactLink(t, e, org.ID, "organisation", true)
	def := models.FieldDefinition{UserID: e.userID, Label: "Industry", Key: "industry", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&def).Error)
	w := e.do(http.MethodPut, "/entities/"+org.ID+"/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": def.ID, "value": "Widgets"}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = e.do(http.MethodGet, "/entities/"+org.ID+"/contacts", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var cl models.EntityContactsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cl))
	require.Len(t, cl.Contacts, 1)
	assert.Equal(t, contact.ID, cl.Contacts[0].ContactID)
	assert.True(t, cl.Contacts[0].IsPrimary)

	w = e.do(http.MethodDelete, "/entities/"+org.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var del models.EntityDeleteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &del))
	assert.Equal(t, 1, del.LinkCount)
	assert.Equal(t, 1, del.ProjectionCount, "one contact loses its projected organisation")

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/entities/"+org.ID, nil).Code)
	var live int64
	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Where("id = ?", link.ID).Count(&live).Error)
	assert.Zero(t, live, "link tombstoned")
	var vals int64
	require.NoError(t, e.db.Model(&models.FieldValue{}).Where("entity_id = ?", org.ID).Count(&vals).Error)
	assert.Zero(t, vals, "field values hard-deleted")
	var snaps int64
	require.NoError(t, e.db.Model(&models.EntityDeletionSnapshot{}).Where("entity_id = ?", org.ID).Count(&snaps).Error)
	assert.EqualValues(t, 1, snaps)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodDelete, "/entities/"+org.ID, nil).Code, "double delete")

	w = e.do(http.MethodPost, "/entities/"+org.ID+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var rest models.EntityRestoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rest))
	assert.Equal(t, org.ID, rest.Entity.ID)
	assert.False(t, rest.SameNameLive)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["skipped"]))

	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Where("id = ? AND is_primary = 1", link.ID).Count(&live).Error)
	assert.EqualValues(t, 1, live, "link revived, still primary")
	w = e.do(http.MethodGet, "/entities/"+org.ID+"/field-values", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Widgets")
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/entities/"+org.ID+"/restore", nil).Code, "snapshot consumed; live entity is not restorable")
}

func TestEntity_RestoreWarnsOnSameName(t *testing.T) {
	e := newEntityEnv(t)
	a := e.create(t, map[string]any{"kind": "place", "name": "Cabin"})
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+a.ID, nil).Code)
	e.create(t, map[string]any{"kind": "place", "name": "cabin."})
	w := e.do(http.MethodPost, "/entities/"+a.ID+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var rest models.EntityRestoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rest))
	assert.True(t, rest.SameNameLive)
}

func TestEntity_FieldDefinitionTargets(t *testing.T) {
	e := newEntityEnv(t)
	mk := func(body map[string]any) *httptest.ResponseRecorder {
		return e.do(http.MethodPost, "/field-definitions", body)
	}

	w := mk(map[string]any{"label": "Industry", "key": "industry", "type": "string", "target": "organisation", "projection": "internal-only"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		FieldDefinition models.FieldDefinition `json:"field_definition"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// vcard projection is rejected on an entity target, accepted on contact.
	w = mk(map[string]any{"label": "X", "key": "x", "type": "string", "target": "pet", "projection": "vcard:X-FOO"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, string(apperrors.ReasonContactFieldDefinitionProjectionTargetInvalid), entityReason(t, w))
	assert.Equal(t, http.StatusCreated, mk(map[string]any{"label": "X", "key": "x", "type": "string", "projection": "vcard:X-FOO"}).Code)

	// Same key on a different target is fine; same target+key is a 409.
	assert.Equal(t, http.StatusCreated, mk(map[string]any{"label": "Industry", "key": "industry", "type": "string"}).Code)
	assert.Equal(t, http.StatusConflict, mk(map[string]any{"label": "Industry", "key": "industry", "type": "string", "target": "organisation"}).Code)
	assert.Equal(t, http.StatusBadRequest, mk(map[string]any{"label": "Y", "key": "y", "type": "string", "target": "robot"}).Code)

	// Target is immutable; projection can't be moved onto an entity definition.
	path := "/field-definitions/" + created.FieldDefinition.ID
	w = e.do(http.MethodPut, path, map[string]any{"label": "Industry", "key": "industry", "type": "string", "target": "place"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, string(apperrors.ReasonContactFieldDefinitionTargetImmutable), entityReason(t, w))
	w = e.do(http.MethodPut, path, map[string]any{"label": "Industry", "key": "industry", "type": "string", "projection": "vcard:X-IND"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, http.StatusOK, e.do(http.MethodPut, path, map[string]any{"label": "Sector", "key": "industry", "type": "string"}).Code)
}

func TestEntity_FieldValuesTargetAndValidation(t *testing.T) {
	e := newEntityEnv(t)
	org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
	pet := e.create(t, map[string]any{"kind": "pet", "name": "Rex"})
	orgDef := models.FieldDefinition{UserID: e.userID, Label: "Employees", Key: "employees", Target: "organisation", Type: "number", Projection: "internal-only", Sensitivity: "normal"}
	contactDef := models.FieldDefinition{UserID: e.userID, Label: "Nick", Key: "nick", Target: "contact", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&orgDef).Error)
	require.NoError(t, e.db.Create(&contactDef).Error)
	put := func(id, defID string, v any) *httptest.ResponseRecorder {
		return e.do(http.MethodPut, "/entities/"+id+"/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": defID, "value": v}}})
	}
	assert.Equal(t, http.StatusOK, put(org.ID, orgDef.ID, 12).Code)
	w := put(org.ID, orgDef.ID, "twelve")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, string(apperrors.ReasonContactFieldValueInvalid), entityReason(t, w))
	w = put(pet.ID, orgDef.ID, 3)
	assert.Equal(t, http.StatusBadRequest, w.Code, "org definition on a pet")
	assert.Equal(t, string(apperrors.ReasonContactFieldDefinitionTargetMismatch), entityReason(t, w))
	assert.Equal(t, http.StatusBadRequest, put(org.ID, contactDef.ID, "x").Code, "contact definition on an entity")

	// A contact cannot take an entity-target definition's value.
	c := models.Contact{UserID: e.userID, Firstname: "Z"}
	require.NoError(t, e.db.Create(&c).Error)
	w = e.do(http.MethodPut, "/contacts/"+strconv.FormatUint(uint64(c.ID), 10)+"/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": orgDef.ID, "value": 1}}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, string(apperrors.ReasonContactFieldDefinitionTargetMismatch), entityReason(t, w))
}
