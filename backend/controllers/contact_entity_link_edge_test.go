package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Error paths and edge cases of the contact-entity link routes (issue #2297).

func TestContactEntityLink_DatabaseFailuresAre500(t *testing.T) {
	logtest.AllowWarnings(t, "injected database failures are logged as request errors")
	for _, table := range []string{"contacts", "contact_entity_links", "entities"} {
		t.Run(table, func(t *testing.T) {
			e := newLinkEnv(t)
			c := e.contact(t, func(c *models.Contact) {
				c.Card = contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "o1", Name: "Initech"}}}
			})
			org := e.org(t, "Acme")
			l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
			pl := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "owner", "is_primary": true})
			dbtest.HideTable(t, e.db, table)
			for _, call := range []struct {
				method, path string
				body         any
			}{
				{http.MethodGet, linkPath(c), nil},
				{http.MethodPost, linkPath(c), map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": true}},
				{http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "owner", "is_primary": true}},
				{http.MethodDelete, linkPath(c, pl.ID), nil},
				{http.MethodPost, linkPath(c, l.ID, "adopt"), nil},
				{http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": map[string]any{"kind": "organization", "id": "o1"}, "role": "employee"}},
			} {
				w := e.do(call.method, call.path, call.body)
				if call.method == http.MethodDelete && table == "entities" {
					continue // a link delete never reads the entity
				}
				assert.GreaterOrEqual(t, w.Code, 500, "%s %s with %s hidden: %s", call.method, call.path, table, w.Body.String())
			}
		})
	}
}

func TestContactEntityLink_ServiceWriteFailuresRollBack(t *testing.T) {
	logtest.AllowWarnings(t, "injected database failures are logged as request errors")
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	a, b := e.org(t, "Acme"), e.org(t, "Beta")
	la := e.addLink(t, c, map[string]any{"entity_id": a.ID, "role": "employee", "is_primary": true})
	require.NoError(t, e.db.Exec(`CREATE TRIGGER block_link_update BEFORE UPDATE ON contact_entity_links BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)

	// demoting the previous primary fails: the new primary is not created
	w := e.do(http.MethodPost, linkPath(c), map[string]any{"entity_id": b.ID, "role": "employee", "is_primary": true})
	assert.GreaterOrEqual(t, w.Code, 500, w.Body.String())
	assert.Len(t, e.listLinks(t, c), 1)

	// a plain save failure
	w = e.do(http.MethodPut, linkPath(c, la.ID), map[string]any{"role": "owner"})
	assert.GreaterOrEqual(t, w.Code, 500, w.Body.String())
}

func TestContactEntityLink_HandlersRejectMissingValidatedBody(t *testing.T) {
	// Registered without the validation middleware: the handlers still refuse
	// to run on a request that was never validated.
	e := newLinkEnv(t)
	e.router.POST("/raw/contacts/:id/entity-links", CreateContactEntityLink)
	e.router.PUT("/raw/contacts/:id/entity-links/:linkId", UpdateContactEntityLink)
	e.router.POST("/raw/contacts/:id/entity-links/from-existing", LinkContactEntityFromExisting)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	for _, call := range []struct{ method, path string }{
		{http.MethodPost, fmt.Sprintf("/raw/contacts/%d/entity-links", c.ID)},
		{http.MethodPut, fmt.Sprintf("/raw/contacts/%d/entity-links/%s", c.ID, l.ID)},
		{http.MethodPost, fmt.Sprintf("/raw/contacts/%d/entity-links/from-existing", c.ID)},
	} {
		w := e.do(call.method, call.path, map[string]any{"role": "employee"})
		assert.GreaterOrEqual(t, w.Code, 400, "%s %s", call.method, call.path)
	}
	// and a path with an unknown contact aborts before the body is read
	for _, call := range []struct{ method, path string }{
		{http.MethodPost, "/raw/contacts/999999/entity-links"},
		{http.MethodPut, "/raw/contacts/999999/entity-links/x"},
		{http.MethodPost, "/raw/contacts/999999/entity-links/from-existing"},
	} {
		assert.Equal(t, http.StatusNotFound, e.do(call.method, call.path, map[string]any{"role": "employee"}).Code)
	}
}

func TestContactEntityLink_CreateRequiresEntityIDAndSensitivityUpdates(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	w := e.do(http.MethodPost, linkPath(c), map[string]any{"role": "employee"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "validation_required", entityReason(t, w))

	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	w = e.do(http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "employee", "sensitivity": "private"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "private", e.listLinks(t, c)[0].Sensitivity)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "employee", "sensitivity": "top-secret"}).Code)
}

func TestContactEntityLink_ListSkipsALinkWhoseEntityIsGone(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	require.NoError(t, e.db.Delete(&models.Entity{}, "id = ?", org.ID).Error) // bare soft delete, links left live
	assert.Empty(t, e.listLinks(t, c))
}

func TestContactEntityLink_AdoptEdgeCases(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)

	// organisation whose core carries no name falls back to the entity name
	org := e.create(t, map[string]any{"kind": "organisation", "name": "Nameless Core Ltd"})
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, linkPath(c, l.ID, "adopt"), nil).Code)
	assert.Equal(t, "Nameless Core Ltd", contactCardJSON(t, e, c.ID).Organizations[0].Name)

	// a place with no address has nothing to adopt
	empty := e.create(t, map[string]any{"kind": "place", "name": "Nowhere"})
	pl := e.addLink(t, c, map[string]any{"entity_id": empty.ID, "role": "visitor"})
	w := e.do(http.MethodPost, linkPath(c, pl.ID, "adopt"), nil)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Equal(t, "entity_link_kind_not_adoptable", entityReason(t, w))

	// a repeat address adopt finds the value-equal own address and writes nothing
	place := e.place(t, "Cabin")
	al := e.addLink(t, c, map[string]any{"entity_id": place.ID, "role": "resident"})
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, linkPath(c, al.ID, "adopt"), nil).Code)
	var before models.Contact
	require.NoError(t, e.db.First(&before, c.ID).Error)
	w = e.do(http.MethodPost, linkPath(c, al.ID, "adopt"), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"adopted":false`)
	var after models.Contact
	require.NoError(t, e.db.First(&after, c.ID).Error)
	assert.Equal(t, before.Revision, after.Revision)
	assert.Len(t, contactCardJSON(t, e, c.ID).Addresses, 1)
}

func TestContactEntityLink_AdoptSurfacesAContactWriteFailure(t *testing.T) {
	logtest.AllowWarnings(t, "injected database failures are logged as request errors")
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	require.NoError(t, e.db.Exec(`CREATE TRIGGER block_contact_update BEFORE UPDATE ON contacts BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)
	w := e.do(http.MethodPost, linkPath(c, l.ID, "adopt"), nil)
	assert.GreaterOrEqual(t, w.Code, 500, w.Body.String())
}

func TestContactEntityLink_FromExistingSourceVariants(t *testing.T) {
	e := newLinkEnv(t)
	long := strings.Repeat("é", 250)
	c := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{
			Organizations: []contactmodel.Organization{{Name: "No Id Inc"}},
			Addresses: []contactmodel.Address{
				{ID: "a-full", Full: "Cabin\n1 Lake Rd"},
				{ID: "a-comps", Components: []contactmodel.AddressComponent{{Kind: "name", Value: long}}},
				{ID: "a-blank", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "  "}}},
			},
		}
	})
	idx := func(i int) map[string]any { return map[string]any{"kind": "organization", "index": i} }

	// by position (imported organisations often have no element id)
	w := e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": idx(0), "role": "employee"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"name":"No Id Inc"`)

	// out of range, and both / neither of id and index
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": idx(5), "role": "employee"}).Code)
	for _, src := range []map[string]any{
		{"kind": "organization"},
		{"kind": "organization", "id": "x", "index": 0},
	} {
		assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": src, "role": "employee"}).Code)
	}

	// place names: Full label, long components truncated to 200 runes, blank falls back
	for id, check := range map[string]func(string){
		"a-full":  func(n string) { assert.Equal(t, "Cabin\n1 Lake Rd", n) },
		"a-comps": func(n string) { assert.Equal(t, 200, len([]rune(n))) },
		"a-blank": func(n string) { assert.Equal(t, "Address", n) },
	} {
		w = e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": map[string]any{"kind": "address", "id": id}, "role": "visitor"})
		require.Equal(t, http.StatusCreated, w.Code, "%s: %s", id, w.Body.String())
		var resp models.ContactEntityLinkFromExistingResponse
		require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &resp))
		check(resp.Entity.Name)
	}

	// an existing place of the right kind is matched for an address value too
	place := e.place(t, "Cabin")
	w = e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": map[string]any{"kind": "address", "id": "a-full"}, "entity_id": place.ID, "role": "visitor"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"entity_created":false`)
	// but an organisation entity for an address value is a kind mismatch
	org := e.org(t, "Acme")
	w = e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": map[string]any{"kind": "address", "id": "a-full"}, "entity_id": org.ID, "role": "employee"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestContactEntityLink_FromExistingRejectsBadPeriodAndOrganisationCoreProblems(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "o1", Name: strings.Repeat("x", 201)}}}
	})
	// the 201-character name cannot become an entity (max 200)
	w := e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": map[string]any{"kind": "organization", "id": "o1"}, "role": "employee"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	c2 := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "o1", Name: "Ok"}}}
	})
	w = e.do(http.MethodPost, linkPath(c2, "from-existing"), map[string]any{"source": map[string]any{"kind": "organization", "id": "o1"}, "role": "employee",
		"period": map[string]any{"start": map[string]any{"year": 2024}, "end": map[string]any{"year": 2020}}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAbortLinkWrite_MapsErrors(t *testing.T) {
	logtest.AllowWarnings(t, "the database-error mapping logs the request error")
	gin.SetMode(gin.TestMode)
	run := func(err error) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
		abortLinkWrite(c, err)
		return w
	}
	w := run(errors.New("constraint failed: UNIQUE constraint failed: contact_entity_links.user_id"))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "entity_link_primary_conflict")
	w = run(apperrors.ErrNotFound("Entity link"))
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = run(&models.ErrRevisionConflict{})
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
	w = run(gorm.ErrInvalidDB)
	assert.GreaterOrEqual(t, w.Code, 500)
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func TestContactEntityLink_RoutesRequireAnAuthenticatedUser(t *testing.T) {
	db, _ := setupRouter(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	r.GET("/contacts/:id/entity-links", ListContactEntityLinks)
	r.POST("/contacts/:id/entity-links", CreateContactEntityLink)
	r.POST("/contacts/:id/entity-links/from-existing", LinkContactEntityFromExisting)
	r.PUT("/contacts/:id/entity-links/:linkId", UpdateContactEntityLink)
	r.DELETE("/contacts/:id/entity-links/:linkId", DeleteContactEntityLink)
	r.POST("/contacts/:id/entity-links/:linkId/adopt", AdoptContactEntityLink)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/contacts/1/entity-links"}, {http.MethodPost, "/contacts/1/entity-links"},
		{http.MethodPost, "/contacts/1/entity-links/from-existing"}, {http.MethodPut, "/contacts/1/entity-links/x"},
		{http.MethodDelete, "/contacts/1/entity-links/x"}, {http.MethodPost, "/contacts/1/entity-links/x/adopt"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(call.method, call.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", call.method, call.path)
	}
}

func TestContactEntityLink_DeleteFailureIs500(t *testing.T) {
	logtest.AllowWarnings(t, "injected database failures are logged as request errors")
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	require.NoError(t, e.db.Exec(`CREATE TRIGGER block_link_update BEFORE UPDATE ON contact_entity_links BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)
	assert.GreaterOrEqual(t, e.do(http.MethodDelete, linkPath(c, l.ID), nil).Code, 500)
}
