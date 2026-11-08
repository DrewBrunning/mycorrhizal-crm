package carddav

// Issue #2298 (ADR 0043 decision 3) on the CardDAV/WebDAV surface: a primary
// entity link's ORG/ADR is served in the card with a reserved ent-<linkId>
// element ID, and a client PUT of that card (IDs kept, or IDs stripped as
// vCard 3.0 / native clients do) never writes the projection into the stored
// Card.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type projectedContact struct {
	backend *Backend
	contact models.Contact
	linkOrg models.ContactEntityLink
	ctx     context.Context
	path    string
}

func newProjectedContact(t *testing.T, entitySensitivity string) projectedContact {
	t.Helper()
	backend, db := newTestBackend(t)
	// Entities have a real foreign key to users (contacts in these tests do not).
	user := models.User{Username: "proj-user", Email: "proj-user@example.com", Password: "unused"}
	require.NoError(t, db.Create(&user).Error)
	require.EqualValues(t, 1, user.ID, "the DAV context below is user 1")
	record := &contactmodel.Record{Card: contactmodel.Card{
		UID:           "proj-" + strings.ReplaceAll(t.Name(), "/", "-"),
		Name:          &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Organizations: []contactmodel.Organization{{ID: "own-org", Name: "Own Co"}},
	}}
	contact := models.Contact{UserID: 1}
	models.ApplyRecordToContact(&contact, record, "")
	require.NoError(t, db.Create(&contact).Error)

	org := models.Entity{UserID: 1, Kind: models.EntityKindOrganisation, Name: "Acme Corp", Sensitivity: entitySensitivity}
	require.NoError(t, org.SetCore(&models.OrganisationCore{Organization: contactmodel.Organization{Name: "Acme Corp", Units: []contactmodel.OrgUnit{{Name: "Rocketry"}}}}))
	require.NoError(t, db.Create(&org).Error)
	lo := models.ContactEntityLink{UserID: 1, ContactVCardUID: contact.VCardUID, EntityID: org.ID, Kind: org.Kind, Role: "employee", IsPrimary: true}
	require.NoError(t, db.Create(&lo).Error)
	place := models.Entity{UserID: 1, Kind: models.EntityKindPlace, Name: "Cabin", Sensitivity: entitySensitivity}
	require.NoError(t, place.SetCore(&models.PlaceCore{Address: contactmodel.Address{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}, {Kind: "locality", Value: "Springfield"}}}}))
	require.NoError(t, db.Create(&place).Error)
	require.NoError(t, db.Create(&models.ContactEntityLink{UserID: 1, ContactVCardUID: contact.VCardUID, EntityID: place.ID, Kind: place.Kind, Role: "resident", IsPrimary: true}).Error)

	return projectedContact{
		backend: backend, contact: contact, linkOrg: lo,
		ctx:  ContextWithUser(context.Background(), 1, "tester", db, backend.photoDir, ""),
		path: "/carddav/addressbooks/tester/contacts/" + contact.VCardUID + ".vcf",
	}
}

func (p projectedContact) storedCardJSON(t *testing.T) string {
	t.Helper()
	var stored models.Contact
	require.NoError(t, p.backend.db.Where("user_id = ? AND vcard_uid = ?", uint(1), p.contact.VCardUID).First(&stored).Error)
	b, err := json.Marshal(stored.Card)
	require.NoError(t, err)
	return string(b)
}

func TestCardDAV_GetServesProjectedORGAndADR(t *testing.T) {
	p := newProjectedContact(t, "normal")
	obj, err := p.backend.GetAddressObject(p.ctx, p.path, nil)
	require.NoError(t, err)
	body := encodeCard(t, obj.Card)
	assert.Contains(t, body, "Own Co")
	assert.Contains(t, body, "Acme Corp;Rocketry")
	assert.Contains(t, body, "PROP-ID=ent-"+p.linkOrg.ID)
	assert.Contains(t, body, "Springfield")
	assert.Less(t, strings.Index(body, "Own Co"), strings.Index(body, "Acme Corp"), "own ORG leads")
	assert.NotContains(t, body, "Employee", "the link role never leaves")
}

func TestCardDAV_GetWithholdsAboveNormalEntity(t *testing.T) {
	for _, sens := range []string{"private", "secret"} {
		p := newProjectedContact(t, sens)
		obj, err := p.backend.GetAddressObject(p.ctx, p.path, nil)
		require.NoError(t, err)
		body := encodeCard(t, obj.Card)
		assert.NotContains(t, body, "Acme Corp", sens)
		assert.NotContains(t, body, "Springfield", sens)
		assert.NotContains(t, body, "ent-")
	}
}

// GET then PUT of the unchanged card is a no-op on the stored Card, whether the
// client kept the PROP-IDs or stripped them.
func TestCardDAV_UnchangedPutOfProjectedCardIsNoOp(t *testing.T) {
	for _, strip := range []bool{false, true} {
		p := newProjectedContact(t, "normal")
		before := p.storedCardJSON(t)
		obj, err := p.backend.GetAddressObject(p.ctx, p.path, nil)
		require.NoError(t, err)
		card := obj.Card
		if strip {
			for _, fields := range card {
				for _, f := range fields {
					delete(f.Params, "PROP-ID")
					delete(f.Params, "PREF")
				}
			}
		}
		_, err = p.backend.PutAddressObject(p.ctx, p.path, card, nil)
		require.NoError(t, err)

		var after contactmodel.Card
		require.NoError(t, json.Unmarshal([]byte(p.storedCardJSON(t)), &after))
		var was contactmodel.Card
		require.NoError(t, json.Unmarshal([]byte(before), &was))
		require.Len(t, after.Organizations, 1, "strip=%v: projection must not be stored as an ORG", strip)
		assert.Equal(t, "Own Co", after.Organizations[0].Name)
		assert.Empty(t, after.Addresses, "strip=%v: projected ADR must not be stored", strip)
		if !strip { // an id-stripping client makes the adapter mint a synthetic element ID
			assert.Equal(t, was.Organizations, after.Organizations)
		}
	}
}

// A client that edits the projected value (IDs stripped) creates an ordinary
// contact value; nothing is lost and the edit is not echo-suppressed.
func TestCardDAV_ClientEditOfProjectedValueBecomesOrdinaryValue(t *testing.T) {
	p := newProjectedContact(t, "normal")
	obj, err := p.backend.GetAddressObject(p.ctx, p.path, nil)
	require.NoError(t, err)
	card := obj.Card
	for _, f := range card["ORG"] {
		delete(f.Params, "PROP-ID")
		if strings.HasPrefix(f.Value, "Acme Corp") {
			f.Value = "Acme Renamed;Rocketry"
		}
	}
	_, err = p.backend.PutAddressObject(p.ctx, p.path, card, nil)
	require.NoError(t, err)

	var stored contactmodel.Card
	require.NoError(t, json.Unmarshal([]byte(p.storedCardJSON(t)), &stored))
	var names []string
	for _, o := range stored.Organizations {
		names = append(names, o.Name)
	}
	assert.Equal(t, []string{"Own Co", "Acme Renamed"}, names)
}
