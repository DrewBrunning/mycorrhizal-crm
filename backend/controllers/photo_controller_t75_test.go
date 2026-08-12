package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// richCardOnlyRecordCtrl is the controller-package mirror of models'
// richCardOnlyRecord (models/contact_card_merge_test.go): a neutral Record
// whose Card-only data has no flat-field home, used to seed contacts whose
// preservation a handler-level test asserts after a plain save. Duplicated
// across test packages on purpose — test fixtures are not shared API.
func richCardOnlyRecordCtrl() *contactmodel.Record {
	return &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{
				Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}},
			},
			Emails: []contactmodel.Email{{Address: "ada@example.com", Contexts: []string{"work"}, Label: "work"}},
			Phones: []contactmodel.Phone{{Number: "+15550100100", Label: "cell", Features: []string{"cell", "voice"}}},
			Addresses: []contactmodel.Address{{
				Components: []contactmodel.AddressComponent{
					{Kind: "name", Value: "123 Main St"},
					{Kind: "apartment", Value: "Apt 3B"},
					{Kind: "postOfficeBox", Value: "PO Box 42"},
					{Kind: "locality", Value: "Springfield"},
				},
				Contexts: []string{"home"},
			}},
			SpeakToAs: &contactmodel.SpeakToAs{
				Pronouns: []contactmodel.Pronouns{{Pronouns: "she/her"}},
			},
			PersonalInfo: []contactmodel.PersonalInfo{{Kind: "hobby", Value: "sailing"}},
		},
		Envelope: contactmodel.CRMEnvelope{Kind: "pet"},
		Passthrough: contactmodel.Passthrough{
			VCard: []contactmodel.JCardProp{{Name: "X-CUSTOM", Type: "text", Value: json.RawMessage(`"keep-me"`)}},
		},
	}
}

// assertCardOnlyDataPreserved is the shared assertion block for the three
// T75 trigger tests: after a plain save, a contact's SpeakToAs, PersonalInfo,
// unprojected address components, rich phone features, pet kind and imported
// Passthrough must all still be there.
func assertCardOnlyDataPreserved(t *testing.T, c models.Contact) {
	t.Helper()
	if c.Card.SpeakToAs == nil || len(c.Card.SpeakToAs.Pronouns) != 1 || c.Card.SpeakToAs.Pronouns[0].Pronouns != "she/her" {
		t.Errorf("SpeakToAs = %+v, want the created pronouns preserved", c.Card.SpeakToAs)
	}
	if len(c.Card.PersonalInfo) != 1 || c.Card.PersonalInfo[0].Value != "sailing" {
		t.Errorf("PersonalInfo = %+v, want the created hobby preserved", c.Card.PersonalInfo)
	}
	require.Len(t, c.Card.Addresses, 1, "the address must survive")
	components := map[string]string{}
	for _, comp := range c.Card.Addresses[0].Components {
		components[comp.Kind] = comp.Value
	}
	for _, kind := range []string{"apartment", "postOfficeBox"} {
		if components[kind] == "" {
			t.Errorf("address component kind=%q missing (components=%v)", kind, components)
		}
	}
	if len(c.Card.Phones[0].Features) != 2 {
		t.Errorf("Card.Phones[0] = %+v, want the created features preserved", c.Card.Phones[0])
	}
	if c.CRM.Kind != "pet" {
		t.Errorf("CRM.Kind = %q, want the pet kind preserved", c.CRM.Kind)
	}
	if len(c.Passthrough.VCard) != 1 || c.Passthrough.VCard[0].Name != "X-CUSTOM" {
		t.Errorf("Passthrough.VCard = %+v, want the imported X-CUSTOM property preserved", c.Passthrough.VCard)
	}
}

// TestAddPhotoToContact_PreservesCardOnlyData pins T75 trigger 1 at the
// handler level against the real migrated schema (CLAUDE.md trap 1):
// AddPhotoToContact loads the contact, sets Photo/PhotoThumbnail, and calls
// db.Save(&contact) with no ApplyRecordToContact — the exact "plain save"
// shape that, before T75's BeforeSave merge, silently destroyed every
// Card-only member.
func TestAddPhotoToContact_PreservesCardOnlyData(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "t75-photo-ctrl.db"))
	require.NoError(t, err)

	user := models.User{Username: "t75photoc", Password: "password123!A", Email: "t75photoc@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := &models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(contact, richCardOnlyRecordCtrl(), "")
	require.NoError(t, db.Create(contact).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", *cfg)
		c.Next()
	})
	router.POST("/contacts/:id/photo", func(c *gin.Context) { AddPhotoToContact(c, cfg) })

	req := newMultipartPhotoRequest(t, "/contacts/"+strconv.Itoa(int(contact.ID))+"/photo", "photo", "photo.png", newPNGBytes(t, 20, 20))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var persisted models.Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	assert.True(t, len(persisted.Photo) > 0, "the upload must have set a photo path")
	assertCardOnlyDataPreserved(t, persisted)
}
