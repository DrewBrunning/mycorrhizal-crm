package controllers

// Issue #1738: a contact share is a copy that leaves the sender's account, so
// it must withhold personalInfo entries whose CRM metadata is above normal,
// exactly like the vCard/JSContact exports. PersonalInfoMeta itself never
// reaches a Card, so a share carries neither the hidden entries nor their
// notes. The explicit IncludeSensitive opt-in re-includes them.

import (
	"net/http"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateContactShare_PersonalInfoSensitivity(t *testing.T) {
	db, cfg, sender, recipient, _ := newContactShareFixtures(t)

	record := &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			PersonalInfo: []contactmodel.PersonalInfo{
				{ID: "pi-normal", Kind: "hobby", Value: "Normal Chess"},
				{ID: "pi-private", Kind: "hobby", Value: "Private Birdwatching"},
				{ID: "pi-secret", Kind: "hobby", Value: "Secret Lockpicking"},
			},
		},
		Envelope: contactmodel.CRMEnvelope{PersonalInfoMeta: map[string]contactmodel.PersonalInfoMeta{
			"pi-normal":  {Favorite: true, Notes: "normal hobby notes"},
			"pi-private": {Sensitivity: models.RelationshipSensitivityPrivate, Notes: "private hobby notes"},
			"pi-secret":  {Sensitivity: models.RelationshipSensitivitySecret, Notes: "secret hobby notes"},
		}},
	}
	contact := models.Contact{UserID: sender.ID}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, db.Create(&contact).Error)

	share := func(includeSensitive bool) string {
		t.Helper()
		router := contactShareRouterFor(db, cfg, sender.ID)
		w := doJSON(t, router, http.MethodPost, "/contact-shares", models.ContactShareInput{
			VCardUID:         contact.VCardUID,
			ToUserID:         recipient.ID,
			Sections:         []string{models.SectionPersonalInfo},
			IncludeSensitive: includeSensitive,
		})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var stored models.ContactShare
		require.NoError(t, db.Where("from_user_id = ?", sender.ID).Order("created_at DESC").First(&stored).Error)
		return stored.Payload
	}

	defaultPayload := share(false)
	assert.Contains(t, defaultPayload, "Normal Chess", "the normal control must cross")
	assert.NotContains(t, defaultPayload, "Private Birdwatching")
	assert.NotContains(t, defaultPayload, "Secret Lockpicking")
	assert.NotContains(t, defaultPayload, "private hobby notes", "CRM metadata never reaches a Card")
	assert.NotContains(t, defaultPayload, "secret hobby notes")

	optedIn := share(true)
	assert.True(t, strings.Contains(optedIn, "Private Birdwatching"))
	assert.True(t, strings.Contains(optedIn, "Secret Lockpicking"))
	assert.NotContains(t, optedIn, "private hobby notes",
		"even with the opt-in, PersonalInfoMeta is CRM-only and never projected into a Card")
}
