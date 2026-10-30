package controllers

// Per-contact field visibility error paths (issue #1997): the authenticated
// guard, the defensive GetValidated guard, and the database-error arms of the
// four handlers.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
)

func TestFieldVisibility_NoAuth_Unauthorized(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	router := routerWithoutAuth(e.db)
	router.GET("/contacts/:id/field-visibility", GetContactFieldVisibility)
	router.PUT("/contacts/:id/field-visibility", PutContactFieldVisibility)
	router.DELETE("/contacts/:id/field-visibility", ResetContactFieldVisibility)
	router.PATCH("/users/hide-empty-fields", UpdateHideEmptyFields)

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, e.path()},
		{http.MethodPut, e.path()},
		{http.MethodDelete, e.path()},
		{http.MethodPatch, "/users/hide-empty-fields"},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, mustRequest(t, tc.method, tc.path, nil))
		assertUnauthorizedEnvelope(t, w, tc.method+" "+tc.path)
	}
}

// The route is always registered behind the validation middleware; this pins
// the defensive GetValidated guard for a route that is accidentally wired
// without it.
func TestFieldVisibility_MissingValidatedBodyIs400(t *testing.T) {
	e := newFieldVisibilityEnv(t)
	e.router.PUT("/raw/contacts/:id/field-visibility", PutContactFieldVisibility)
	e.router.PATCH("/raw/hide-empty-fields", UpdateHideEmptyFields)

	w := e.do(t, http.MethodPut, "/raw"+e.path(), `{"overrides":{"emails":"show"}}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	w = e.do(t, http.MethodPatch, "/raw/hide-empty-fields", `{"hide_empty_fields":true}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestFieldVisibility_DatabaseErrors(t *testing.T) {
	t.Run("GET query failure", func(t *testing.T) {
		logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Request error")
		e := newFieldVisibilityEnv(t)
		dbtest.HideTable(t, e.db, "contact_field_overrides")

		w := e.do(t, http.MethodGet, e.path(), nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	})

	t.Run("PUT replace failure", func(t *testing.T) {
		logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Request error")
		e := newFieldVisibilityEnv(t)
		dbtest.HideTable(t, e.db, "contact_field_overrides")

		w := e.do(t, http.MethodPut, e.path(), models.ContactFieldVisibilityInput{
			Overrides: map[string]string{"emails": "hide"},
		})
		assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	})

	t.Run("DELETE clear failure", func(t *testing.T) {
		logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Request error")
		e := newFieldVisibilityEnv(t)
		dbtest.HideTable(t, e.db, "contact_field_overrides")

		w := e.do(t, http.MethodDelete, e.path(), nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	})

	t.Run("PATCH user update failure", func(t *testing.T) {
		logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Request error")
		e := newFieldVisibilityEnv(t)
		dbtest.HideTable(t, e.db, "users")

		w := e.do(t, http.MethodPatch, "/users/hide-empty-fields", `{"hide_empty_fields":true}`)
		assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	})
}
