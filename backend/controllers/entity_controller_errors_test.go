package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Error paths and edge cases of the entity routes (issue #2296).

func (e *entityEnv) seedOrgWithLink(t *testing.T, primary bool) (models.EntityResponse, models.Contact) {
	t.Helper()
	org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
	c, _ := seedContactLink(t, e, org.ID, "organisation", primary)
	return org, c
}

func TestEntity_DatabaseFailuresAre500(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	type call struct {
		method, path string
		body         any
	}
	cases := []struct {
		name  string
		hide  []string
		setup func(t *testing.T, e *entityEnv) []call
	}{
		{"entities table", []string{"entities"}, func(t *testing.T, e *entityEnv) []call {
			return []call{
				{http.MethodPost, "/entities", map[string]any{"kind": "pet", "name": "Rex"}},
				{http.MethodGet, "/entities", nil},
				{http.MethodGet, "/entities/x", nil},
				{http.MethodPut, "/entities/x", map[string]any{"name": "N"}},
				{http.MethodDelete, "/entities/x", nil},
				{http.MethodGet, "/entities/x/contacts", nil},
				{http.MethodGet, "/entities/x/field-values", nil},
				{http.MethodPut, "/entities/x/field-values", map[string]any{"field_values": []any{}}},
			}
		}},
		{"links table", []string{"contact_entity_links"}, func(t *testing.T, e *entityEnv) []call {
			org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
			return []call{
				{http.MethodGet, "/entities/" + org.ID, nil},
				{http.MethodDelete, "/entities/" + org.ID, nil},
				{http.MethodGet, "/entities/" + org.ID + "/contacts", nil},
			}
		}},
		{"contacts table (primary link)", []string{"contacts"}, func(t *testing.T, e *entityEnv) []call {
			org, _ := e.seedOrgWithLink(t, true)
			return []call{
				{http.MethodGet, "/entities/" + org.ID, nil},
				{http.MethodGet, "/entities/" + org.ID + "/contacts", nil},
			}
		}},
		{"field_values table", []string{"field_values"}, func(t *testing.T, e *entityEnv) []call {
			org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
			return []call{
				{http.MethodGet, "/entities/" + org.ID + "/field-values", nil},
				{http.MethodPut, "/entities/" + org.ID + "/field-values", map[string]any{"field_values": []any{}}},
			}
		}},
		{"field_definitions table", []string{"field_definitions"}, func(t *testing.T, e *entityEnv) []call {
			org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
			return []call{{http.MethodPut, "/entities/" + org.ID + "/field-values", map[string]any{"field_values": []any{}}}}
		}},
		{"snapshots table on restore", []string{"entity_deletion_snapshots"}, func(t *testing.T, e *entityEnv) []call {
			org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
			require.NoError(t, e.db.Exec("UPDATE entities SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?", org.ID).Error)
			return []call{{http.MethodPost, "/entities/" + org.ID + "/restore", nil}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEntityEnv(t)
			calls := tc.setup(t, e)
			for _, h := range tc.hide {
				dbtest.HideTable(t, e.db, h)
			}
			for _, cl := range calls {
				w := e.do(cl.method, cl.path, cl.body)
				assert.Equal(t, http.StatusInternalServerError, w.Code, cl.method+" "+cl.path+" "+w.Body.String())
			}
		})
	}
}

func TestEntity_WriteFailuresAre500(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	e := newEntityEnv(t)
	org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
	def := models.FieldDefinition{UserID: e.userID, Label: "N", Key: "n", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&def).Error)
	put := func(v string) int {
		return e.do(http.MethodPut, "/entities/"+org.ID+"/field-values",
			map[string]any{"field_values": []map[string]any{{"field_definition_id": def.ID, "value": v}}}).Code
	}
	require.Equal(t, http.StatusOK, put("a"))
	require.Equal(t, http.StatusOK, put("b"), "second PUT updates the existing value in place")

	require.NoError(t, e.db.Exec(`CREATE TRIGGER fv_no_update BEFORE UPDATE ON field_values BEGIN SELECT RAISE(ABORT, 'no'); END`).Error)
	assert.Equal(t, http.StatusInternalServerError, put("c"))
	require.NoError(t, e.db.Exec(`DROP TRIGGER fv_no_update`).Error)

	require.NoError(t, e.db.Exec(`DELETE FROM field_values`).Error)
	require.NoError(t, e.db.Exec(`CREATE TRIGGER fv_no_insert BEFORE INSERT ON field_values BEGIN SELECT RAISE(ABORT, 'no'); END`).Error)
	assert.Equal(t, http.StatusInternalServerError, put("d"))
	require.NoError(t, e.db.Exec(`DROP TRIGGER fv_no_insert`).Error)

	// An update and a delete the database refuses.
	require.NoError(t, e.db.Exec(`CREATE TRIGGER ent_no_update BEFORE UPDATE ON entities BEGIN SELECT RAISE(ABORT, 'no'); END`).Error)
	assert.Equal(t, http.StatusInternalServerError, e.do(http.MethodPut, "/entities/"+org.ID, map[string]any{"name": "Z"}).Code)
	assert.Equal(t, http.StatusInternalServerError, e.do(http.MethodDelete, "/entities/"+org.ID, nil).Code)
}

func TestEntity_UpdateEdgeCases(t *testing.T) {
	e := newEntityEnv(t)
	r := e.create(t, map[string]any{"kind": "place", "name": "Cabin"})
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPut, "/entities/"+r.ID, map[string]any{"name": "   "}).Code, "blank name")

	w := e.do(http.MethodPut, "/entities/"+r.ID, map[string]any{"name": "Cabin", "core": map[string]any{"bogus": true}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "entity_core_invalid", entityReason(t, w))

	w = e.do(http.MethodPut, "/entities/"+r.ID, map[string]any{"name": "Cabin", "sensitivity": "secret",
		"core": map[string]any{"address": map[string]any{"full": "1 Lake Rd"}, "notes": "keys under the mat"}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "1 Lake Rd")
	assert.Contains(t, w.Body.String(), `"sensitivity":"secret"`)
	var ent models.Entity
	require.NoError(t, e.db.First(&ent, "id = ?", r.ID).Error)
	assert.Equal(t, "secret", ent.Sensitivity)

	// A trashed entity is not updatable.
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+r.ID, nil).Code)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPut, "/entities/"+r.ID, map[string]any{"name": "Back"}).Code)
}

func TestEntity_MissingAuthAndBinding(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	gin.SetMode(gin.ReleaseMode)
	r := gin.New() // no userID, no db needed: handlers stop at currentUserID
	r.Use(func(c *gin.Context) {
		c.Set("db", dbtest.New(t))
		if c.FullPath() == "/entities/:id/field-values" {
			c.Set("validated", &models.ContactFieldValuesInput{})
		} else {
			c.Set("validated", &models.EntityInput{Kind: "pet", Name: "x"})
		}
		c.Next()
	})
	r.POST("/entities", CreateEntity)
	r.GET("/entities", ListEntities)
	r.GET("/entities/:id", GetEntity)
	r.PUT("/entities/:id", UpdateEntity)
	r.DELETE("/entities/:id", DeleteEntity)
	r.POST("/entities/:id/restore", RestoreEntity)
	r.GET("/entities/:id/contacts", ListEntityContacts)
	r.GET("/entities/:id/field-values", ListEntityFieldValues)
	r.PUT("/entities/:id/field-values", ReplaceEntityFieldValues)
	e := &entityEnv{router: r}
	for _, c := range []struct{ m, p string }{
		{"POST", "/entities"}, {"GET", "/entities"}, {"GET", "/entities/x"}, {"PUT", "/entities/x"}, {"DELETE", "/entities/x"},
		{"POST", "/entities/x/restore"}, {"GET", "/entities/x/contacts"}, {"GET", "/entities/x/field-values"}, {"PUT", "/entities/x/field-values"},
	} {
		assert.Equal(t, http.StatusUnauthorized, e.do(c.m, c.p, nil).Code, c.m+" "+c.p)
	}

	// Authenticated but the validation middleware is missing: a 400, not a panic.
	e2 := newEntityEnv(t)
	org := e2.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
	e2.router.POST("/bare/entities", CreateEntity)
	e2.router.PUT("/bare/entities/:id", UpdateEntity)
	e2.router.PUT("/bare/entities/:id/field-values", ReplaceEntityFieldValues)
	assert.Equal(t, http.StatusBadRequest, e2.do("POST", "/bare/entities", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, e2.do("PUT", "/bare/entities/"+org.ID, `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, e2.do("PUT", "/bare/entities/"+org.ID+"/field-values", `{}`).Code)
}

func TestEntity_FieldValueRequestErrors(t *testing.T) {
	e := newEntityEnv(t)
	org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
	def := models.FieldDefinition{UserID: e.userID, Label: "N", Key: "n", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&def).Error)
	path := "/entities/" + org.ID + "/field-values"
	w := e.do(http.MethodPut, path, map[string]any{"field_values": []map[string]any{{"field_definition_id": "nope", "value": "x"}}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "contact_field_definition_unknown", entityReason(t, w))
	w = e.do(http.MethodPut, path, map[string]any{"field_values": []map[string]any{
		{"field_definition_id": def.ID, "value": "x"}, {"field_definition_id": def.ID, "value": "y"}}})
	assert.Equal(t, http.StatusBadRequest, w.Code, "the same definition twice")
	// Another user's definition is "unknown", never usable.
	other := models.User{Username: "o2", Password: "password123", Email: "o2@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	foreign := models.FieldDefinition{UserID: other.ID, Label: "F", Key: "f", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&foreign).Error)
	w = e.do(http.MethodPut, path, map[string]any{"field_values": []map[string]any{{"field_definition_id": foreign.ID, "value": "x"}}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	// Clearing with an empty array removes everything.
	require.Equal(t, http.StatusOK, e.do(http.MethodPut, path, map[string]any{"field_values": []map[string]any{{"field_definition_id": def.ID, "value": "x"}}}).Code)
	require.Equal(t, http.StatusOK, e.do(http.MethodPut, path, map[string]any{"field_values": []any{}}).Code)
	var n int64
	require.NoError(t, e.db.Model(&models.FieldValue{}).Count(&n).Error)
	assert.Zero(t, n)
}

func TestEntity_RestoreReportsDemotionsAndUnsupportedSnapshot(t *testing.T) {
	e := newEntityEnv(t)
	org, contact := e.seedOrgWithLink(t, true)
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+org.ID, nil).Code)
	// While the first org is in the trash the contact gains another primary org.
	other := e.create(t, map[string]any{"kind": "organisation", "name": "Globex"})
	require.NoError(t, e.db.Create(&models.ContactEntityLink{UserID: e.userID, ContactVCardUID: contact.VCardUID,
		EntityID: other.ID, Kind: "organisation", Role: "employee", Sensitivity: "normal", IsPrimary: true}).Error)

	w := e.do(http.MethodPost, "/entities/"+org.ID+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"reason":"primary_demoted"`)
	var primaries int64
	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Where("contact_vcard_uid = ? AND is_primary = 1", contact.VCardUID).Count(&primaries).Error)
	assert.EqualValues(t, 1, primaries, "exactly one primary survives")

	// A snapshot from a newer binary is refused with 409 and nothing changes.
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+other.ID, nil).Code)
	require.NoError(t, e.db.Exec("UPDATE entity_deletion_snapshots SET payload_version = 99 WHERE entity_id = ?", other.ID).Error)
	w = e.do(http.MethodPost, "/entities/"+other.ID+"/restore", nil)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "entity_snapshot_unsupported", entityReason(t, w))
	var live int64
	require.NoError(t, e.db.Model(&models.Entity{}).Where("id = ?", other.ID).Count(&live).Error)
	assert.Zero(t, live)
}

func TestEntity_ProjectionCountHonoursPeriodAndContact(t *testing.T) {
	e := newEntityEnv(t)
	org := e.create(t, map[string]any{"kind": "organisation", "name": "Acme"})
	year := func(y int) *contactmodel.PartialDate { return &contactmodel.PartialDate{Year: &y} }
	mk := func(period *contactmodel.TemporalRange, primary bool, uid string) {
		c := models.Contact{UserID: e.userID, Firstname: "P"}
		require.NoError(t, e.db.Create(&c).Error)
		if uid == "" {
			uid = c.VCardUID
		}
		require.NoError(t, e.db.Create(&models.ContactEntityLink{UserID: e.userID, ContactVCardUID: uid, EntityID: org.ID,
			Kind: "organisation", Role: "employee", Sensitivity: "normal", IsPrimary: primary, Period: period}).Error)
	}
	mk(nil, true, "")                                                             // counts
	mk(&contactmodel.TemporalRange{Start: year(2000)}, true, "")                  // started, open end: counts
	mk(&contactmodel.TemporalRange{Start: year(2999)}, true, "")                  // future: no
	mk(&contactmodel.TemporalRange{Start: year(1990), End: year(1995)}, true, "") // ended: no
	mk(nil, false, "")                                                            // not primary: no
	mk(nil, true, "00000000-0000-4000-8000-000000000000")                         // contact gone: no
	w := e.do(http.MethodGet, "/entities/"+org.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"link_count":6`)
	assert.Contains(t, w.Body.String(), `"projection_count":2`)
}
