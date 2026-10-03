package controllers

// Issue #1433: an address marked private/secret carries a CRM-side
// classification (ADR 0031) and, with it, a geo: coordinate. The neutral-Card
// exporters (vCard 3/4, JSContact) are "copies that leave the instance", so
// they must default-deny such an address exactly as they already do for
// relationship edges, hobby preferences and vCard custom fields — the rule
// ASVS 1.5.1 states and contact_map_controller.go's own comment claims
// ("The vCard/JSContact exports still exclude them"). Before this fix the
// address filter did not exist at all: /export/vcf carried the secret
// street line and its GEO by default.
//
// The second test pins the other half of the boundary: the owner's own REST
// detail (what the edit form loads, and therefore what a full-overwrite save
// would echo back) must keep every address. The fix lives in
// ApplyFieldSelection, which only the export/share surfaces call — never the
// nil-FieldSelection RecordForContact read path.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
)

func TestNeutralCardExports_ExcludeSensitiveAddresses(t *testing.T) {
	db, router := setupRouter(t)
	registerVCFRoute(router, "")
	registerJSContactRoute(router, "")

	var user models.User
	require.NoError(t, db.First(&user).Error)

	record := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Addresses: []contactmodel.Address{
			{Full: "1 Normal St", Coordinates: "geo:1,1"},
			{Full: "2 Secret St", Coordinates: "geo:2,2", Sensitivity: models.RelationshipSensitivitySecret},
			{Full: "3 Private Ave", Coordinates: "geo:3,3", Sensitivity: models.RelationshipSensitivityPrivate},
		},
	}}
	c := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&c, record, "")
	require.NoError(t, db.Create(&c).Error)

	get := func(path string) string {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "%s: %s", path, w.Body.String())
		return w.Body.String()
	}

	for _, path := range []string{"/export/vcf", "/export/jscontact"} {
		t.Run(path, func(t *testing.T) {
			body := get(path)

			// Control first: a normal address (and its coordinate) must be
			// present, so the absences below are the sensitivity filter and not
			// a broken export path.
			assert.Contains(t, body, "1 Normal St", "the normal address control must export")
			assert.Contains(t, body, "geo:1,1", "the normal address's coordinate control must export")

			assert.NotContains(t, body, "2 Secret St", "a secret address must not leave by default")
			assert.NotContains(t, body, "geo:2,2", "a secret address's GEO must not leave by default")
			assert.NotContains(t, body, "3 Private Ave", "a private address must not leave by default")
			assert.NotContains(t, body, "geo:3,3", "a private address's GEO must not leave by default")
		})
	}

	t.Run("include_sensitive opt-in re-includes them", func(t *testing.T) {
		for _, path := range []string{"/export/vcf?include_sensitive=true", "/export/jscontact?include_sensitive=true"} {
			body := get(path)
			assert.Contains(t, body, "2 Secret St", "%s: opt-in must include the secret address", path)
			assert.Contains(t, body, "geo:2,2", "%s: opt-in must include the secret GEO", path)
			assert.Contains(t, body, "3 Private Ave", "%s: opt-in must include the private address", path)
			assert.Contains(t, body, "geo:3,3", "%s: opt-in must include the private GEO", path)
		}
	})
}

// TestContactReadPaths_KeepSensitiveAddresses pins that the owner's own REST
// detail is not filtered. A regression here would be silent data loss: the
// edit form would load without the address and a full-overwrite save would
// delete it.
func TestContactReadPaths_KeepSensitiveAddresses(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/contacts/:id", GetContact)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	record := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Addresses: []contactmodel.Address{
			{Full: "1 Normal St", Coordinates: "geo:1,1"},
			{Full: "2 Secret St", Coordinates: "geo:2,2", Sensitivity: models.RelationshipSensitivitySecret},
		},
	}}
	c := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&c, record, "")
	require.NoError(t, db.Create(&c).Error)

	req, _ := http.NewRequest(http.MethodGet, "/contacts/"+fmt.Sprint(c.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := w.Body.String()
	assert.Contains(t, body, "2 Secret St", "the owner's own REST detail must keep the secret address")
	assert.Contains(t, body, "geo:2,2", "the owner's own REST detail must keep the secret GEO")
}
