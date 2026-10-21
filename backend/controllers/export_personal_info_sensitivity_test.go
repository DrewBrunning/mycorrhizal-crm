package controllers

// Issue #1738: personalInfo entries carry CRM-side sensitivity metadata
// (CRMEnvelope.PersonalInfoMeta). The outward surfaces (vCard 4, JSContact,
// contact shares, CardDAV) must withhold above-normal entries; the owner's
// full-fidelity copies (CSV, account bundle) must keep them, metadata
// included. This file pins both halves over one seeded contact.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedPersonalInfoSensitivityContact(t *testing.T) (models.Contact, *gorm.DB, func(path string) string) {
	t.Helper()
	db, router := setupRouter(t)
	registerVCFRoute(router, "")
	registerJSContactRoute(router, "")
	router.GET("/export", ExportData)

	var user models.User
	require.NoError(t, db.First(&user).Error)

	record := &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			PersonalInfo: []contactmodel.PersonalInfo{
				{ID: "pi-normal", Kind: "hobby", Value: "Normal Chess", Level: "high"},
				{ID: "pi-private", Kind: "hobby", Value: "Private Birdwatching", Level: "medium"},
				{ID: "pi-secret", Kind: "hobby", Value: "Secret Lockpicking", Level: "low"},
			},
		},
		Envelope: contactmodel.CRMEnvelope{PersonalInfoMeta: map[string]contactmodel.PersonalInfoMeta{
			"pi-normal":  {Favorite: true, Notes: "normal hobby notes"},
			"pi-private": {Sensitivity: models.RelationshipSensitivityPrivate, Notes: "private hobby notes"},
			"pi-secret":  {Sensitivity: models.RelationshipSensitivitySecret, Notes: "secret hobby notes"},
		}},
	}
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
	return c, db, get
}

func TestPersonalInfoSensitivity_OutwardExportsWithholdAboveNormal(t *testing.T) {
	_, _, get := seedPersonalInfoSensitivityContact(t)

	for _, path := range []string{"/export/vcf", "/export/jscontact"} {
		t.Run(path, func(t *testing.T) {
			body := get(path)
			assert.Contains(t, body, "Normal Chess", "the normal control must export")
			assert.NotContains(t, body, "Private Birdwatching")
			assert.NotContains(t, body, "Secret Lockpicking")
		})
	}

	// vCard 3 cannot represent personalInfo at all; the control assertion is
	// the non-hobby identity field, and no entry may appear regardless of
	// sensitivity.
	body := get("/export/vcf?version=3")
	assert.NotContains(t, body, "Normal Chess")
	assert.Contains(t, body, "Ada")

	for _, path := range []string{"/export/vcf?include_sensitive=true", "/export/jscontact?include_sensitive=true"} {
		body := get(path)
		assert.Contains(t, body, "Normal Chess")
		assert.Contains(t, body, "Private Birdwatching")
		assert.Contains(t, body, "Secret Lockpicking")
	}
}

func TestPersonalInfoSensitivity_CSVAndAccountBundleKeepEverything(t *testing.T) {
	contact, db, get := seedPersonalInfoSensitivityContact(t)

	csvBody := get("/export")
	start := strings.Index(csvBody, "=== PERSONAL_INFO ===")
	require.GreaterOrEqual(t, start, 0, "the CSV export must have a personal info section")
	section := csvBody[start:]
	if next := strings.Index(section, "\n=== "); next >= 0 {
		section = section[:next]
	}
	for _, want := range []string{
		"Normal Chess", "Private Birdwatching", "Secret Lockpicking",
		"normal hobby notes", "private hobby notes", "secret hobby notes",
	} {
		assert.Contains(t, section, want, "the owner's CSV backup must keep %q", want)
	}
	assert.Contains(t, section, "private")
	assert.Contains(t, section, "secret")

	var stored models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", contact.VCardUID).First(&stored).Error)
	bundle, _, err := services.BuildAccountBundle(db, stored.UserID, "")
	require.NoError(t, err)
	require.Len(t, bundle.Plan.Contacts, 1)

	values := map[string]bool{}
	for _, entry := range bundle.Plan.Contacts[0].Card.PersonalInfo {
		values[entry.Value] = true
	}
	assert.True(t, values["Normal Chess"])
	assert.True(t, values["Private Birdwatching"])
	assert.True(t, values["Secret Lockpicking"])
	assert.Equal(t, models.RelationshipSensitivityPrivate,
		bundle.Plan.Contacts[0].CRM.PersonalInfoMeta["pi-private"].Sensitivity)
	assert.Equal(t, "secret hobby notes",
		bundle.Plan.Contacts[0].CRM.PersonalInfoMeta["pi-secret"].Notes)
}
