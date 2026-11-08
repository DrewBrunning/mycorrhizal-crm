package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2298: every leaving surface -- REST detail, VCF/JSContact export,
// contact share and MCP get_contact -- carries the primary entity link's ORG/ADR
// through the one shared projection, filters above-normal sensitivity, and never
// carries anything else entity-shaped.

const (
	entNotesToken = "ENTITY-NOTES-TOKEN-2298"
	entRoleToken  = "ROLE-LABEL-TOKEN-2298"
)

// seedProjectedContact gives userID a contact with an own ORG, a primary
// organisation link ("Acme Corp") and a primary place link ("Springfield"),
// both at the given sensitivity. The entities carry notes/url and the links a
// role label that must never leave.
func seedProjectedContact(t *testing.T, db *gorm.DB, userID uint, sensitivity string) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: "Proj", Lastname: "Subject"}
	require.NoError(t, db.Create(&contact).Error)
	models.ApplyRecordToContact(&contact, &contactmodel.Record{Card: contactmodel.Card{
		Name:          &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Proj"}, {Kind: "surname", Value: "Subject"}}},
		Organizations: []contactmodel.Organization{{ID: "own-org", Name: "Own Co"}},
	}}, "")
	require.NoError(t, db.Save(&contact).Error)

	org := models.Entity{UserID: userID, Kind: models.EntityKindOrganisation, Name: "Acme Corp", Sensitivity: sensitivity}
	require.NoError(t, org.SetCore(&models.OrganisationCore{Organization: contactmodel.Organization{Name: "Acme Corp"}, URL: "https://entity-url.example", Notes: entNotesToken}))
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, db.Create(&models.ContactEntityLink{UserID: userID, ContactVCardUID: contact.VCardUID, EntityID: org.ID,
		Kind: org.Kind, Role: "contractor", RoleLabel: entRoleToken, IsPrimary: true}).Error)
	place := models.Entity{UserID: userID, Kind: models.EntityKindPlace, Name: "Cabin", Sensitivity: sensitivity}
	require.NoError(t, place.SetCore(&models.PlaceCore{Address: contactmodel.Address{
		Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}, {Kind: "locality", Value: "Springfield"}}}, Notes: entNotesToken}))
	require.NoError(t, db.Create(&place).Error)
	require.NoError(t, db.Create(&models.ContactEntityLink{UserID: userID, ContactVCardUID: contact.VCardUID, EntityID: place.ID,
		Kind: place.Kind, Role: "resident", IsPrimary: true}).Error)
	return contact
}

func assertNoEntityLeak(t *testing.T, surface, body string) {
	t.Helper()
	for _, secret := range []string{entNotesToken, entRoleToken, "entity-url.example", "contractor"} {
		assert.NotContains(t, body, secret, "%s must not carry %q", surface, secret)
	}
}

func TestEntityProjection_VCFAndJSContactExport(t *testing.T) {
	db, router := setupRouter(t)
	registerVCFRoute(router, "")
	registerJSContactRoute(router, "")
	var user models.User
	require.NoError(t, db.First(&user).Error)
	contact := seedProjectedContact(t, db, user.ID, "normal")

	get := func(path string) string {
		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return w.Body.String()
	}
	for name, path := range map[string]string{
		"vcf4":      "/export/vcf?vcard_uid=" + contact.VCardUID,
		"jscontact": "/export/jscontact?vcard_uid=" + contact.VCardUID,
	} {
		body := get(path)
		assert.Contains(t, body, "Own Co", name)
		assert.Contains(t, body, "Acme Corp", name)
		assert.Contains(t, body, "Springfield", name)
		assert.Contains(t, body, "ent-", name)
		assertNoEntityLeak(t, name, body)
	}

	v3 := get("/export/vcf?version=3&vcard_uid=" + contact.VCardUID)
	assert.Contains(t, v3, "Own Co")
	assert.NotContains(t, v3, "Acme Corp", "3.0 keeps the own ORG and drops the projected one")
	assert.Contains(t, v3, "Springfield")
	assertNoEntityLeak(t, "vcf3", v3)
}

func TestEntityProjection_ExportWithholdsAboveNormalUnlessOptedIn(t *testing.T) {
	for _, sens := range []string{"private", "secret"} {
		db, router := setupRouter(t)
		registerVCFRoute(router, "")
		registerJSContactRoute(router, "")
		var user models.User
		require.NoError(t, db.First(&user).Error)
		contact := seedProjectedContact(t, db, user.ID, sens)

		get := func(path string) string {
			req, _ := http.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			return w.Body.String()
		}
		for _, p := range []string{"/export/vcf?vcard_uid=", "/export/vcf?version=3&vcard_uid=", "/export/jscontact?vcard_uid="} {
			body := get(p + contact.VCardUID)
			assert.NotContains(t, body, "Acme Corp", "%s %s", sens, p)
			assert.NotContains(t, body, "Springfield", "%s %s", sens, p)
			assert.NotContains(t, body, "ent-", "%s %s", sens, p)
		}
		// The explicit include flag brings the ORG back (the place's ADR still
		// rides with its stamped sensitivity token, so the selection default-denies it).
		opt := get("/export/jscontact?include_sensitive=true&vcard_uid=" + contact.VCardUID)
		assert.Contains(t, opt, "Acme Corp", sens)
		assertNoEntityLeak(t, "opt-in "+sens, opt)
	}
}

func TestEntityProjection_ContactShare(t *testing.T) {
	for _, sens := range []string{"normal", "private"} {
		db, cfg, sender, recipient, _ := newContactShareFixtures(t)
		contact := seedProjectedContact(t, db, sender.ID, sens)
		router := contactShareRouterFor(db, cfg, sender.ID)
		w := doJSON(t, router, "POST", "/contact-shares", models.ContactShareInput{
			ToUserID: recipient.ID, VCardUID: contact.VCardUID,
			Sections: []string{models.SectionOrganizations, models.SectionAddresses},
		})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var share models.ContactShare
		require.NoError(t, db.Where("from_user_id = ?", sender.ID).First(&share).Error)
		if sens == "normal" {
			assert.Contains(t, share.Payload, "Acme Corp")
			assert.Contains(t, share.Payload, "Springfield")
		} else {
			assert.NotContains(t, share.Payload, "Acme Corp")
			assert.NotContains(t, share.Payload, "Springfield")
		}
		assertNoEntityLeak(t, "share "+sens, share.Payload)
		assert.NotContains(t, share.Payload, "ent-", "the sender's link id never reaches another account")
		assert.NotContains(t, share.Payload, "Cabin", "the entity's own name never leaves, only the projected value")
	}
}

func TestEntityProjection_MCPGetContact(t *testing.T) {
	f := newMCPFixture(t)
	contact := seedProjectedContact(t, f.db, f.alice.ID, "normal")
	r := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": contact.ID})
	require.False(t, r.isError, r.text)
	body := r.text
	assert.Contains(t, body, "Acme Corp")
	assert.Contains(t, body, "Springfield")
	assertNoEntityLeak(t, "mcp", body)
	card := r.structured["contact"].(map[string]any)["card"].(map[string]any)
	assert.Len(t, card["organizations"], 2)

	hidden := seedProjectedContact(t, f.db, f.alice.ID, "private")
	priv := f.call(t, f.alice.ID, "get_contact", map[string]any{"id": hidden.ID})
	require.False(t, priv.isError, priv.text)
	assert.NotContains(t, priv.text, "Acme Corp", "above-normal entity never reaches MCP by default")
	assert.NotContains(t, priv.text, "Springfield")
}

// REST detail projects; a PUT of the unchanged response (IDs kept, or stripped)
// leaves the stored Card without the projection, and a client edit becomes an
// ordinary contact value.
func TestEntityProjection_RESTDetailAndUnchangedPut(t *testing.T) {
	for _, strip := range []bool{false, true} {
		db, router := setupRouter(t)
		router.GET("/contacts/:id", GetContact)
		router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
		var user models.User
		require.NoError(t, db.First(&user).Error)
		contact := seedProjectedContact(t, db, user.ID, "normal")
		path := "/contacts/" + itoa(contact.ID)

		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var detail models.ContactRecordResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
		require.Len(t, detail.Card.Organizations, 2)
		assert.True(t, strings.HasPrefix(detail.Card.Organizations[1].ID, "ent-"))
		require.Len(t, detail.Card.Addresses, 1)

		if strip {
			for i := range detail.Card.Organizations {
				if strings.HasPrefix(detail.Card.Organizations[i].ID, "ent-") {
					detail.Card.Organizations[i].ID = ""
				}
			}
			for i := range detail.Card.Addresses {
				detail.Card.Addresses[i].ID, detail.Card.Addresses[i].Pref = "", nil
			}
		}
		body, err := json.Marshal(map[string]any{"card": detail.Card, "crm": detail.CRM, "gender": detail.Gender})
		require.NoError(t, err)
		req, _ = http.NewRequest("PUT", path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "strip=%v %s", strip, w.Body.String())

		var stored models.Contact
		require.NoError(t, db.First(&stored, contact.ID).Error)
		require.Len(t, stored.Card.Organizations, 1, "strip=%v: projected ORG must not be stored", strip)
		assert.Equal(t, "Own Co", stored.Card.Organizations[0].Name)
		assert.Empty(t, stored.Card.Addresses, "strip=%v: projected ADR must not be stored", strip)
	}
}
