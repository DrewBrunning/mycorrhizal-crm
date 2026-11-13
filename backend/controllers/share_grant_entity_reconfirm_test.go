package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2564, ADR 0042 decision 5: raising the sensitivity of a primary
// organisation/place link, or of the entity behind it, withholds the
// projected organizations/addresses section until the owner re-confirms, so
// flipping it back to normal never silently re-surfaces it.

type entityGrantFixture struct {
	e            *sgEnv
	contact      models.Contact
	grantID      string
	org, place   models.Entity
	orgL, placeL models.ContactEntityLink
}

func newEntityGrantFixture(t *testing.T) *entityGrantFixture {
	t.Helper()
	e := newSGEnv(t)
	f := &entityGrantFixture{e: e}
	f.contact = e.contact(e.A, "Linked")
	f.grantID = e.grant(e.A, e.B, "contact", f.contact.VCardUID, []string{"addresses", "organizations"})
	require.Equal(t, 200, e.do(e.B, http.MethodPost, "/share-grants/"+f.grantID+"/accept", nil).Code)

	f.org = models.Entity{UserID: e.A.ID, Kind: models.EntityKindOrganisation, Name: "Acme Corp"}
	require.NoError(t, f.org.SetCore(&models.OrganisationCore{Organization: contactmodel.Organization{Name: "Acme Corp"}}))
	require.NoError(t, e.db.Create(&f.org).Error)
	f.place = models.Entity{UserID: e.A.ID, Kind: models.EntityKindPlace, Name: "Cabin"}
	require.NoError(t, f.place.SetCore(&models.PlaceCore{Address: contactmodel.Address{
		Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}, {Kind: "locality", Value: "Springfield"}}}}))
	require.NoError(t, e.db.Create(&f.place).Error)
	f.orgL = e.link(f.contact, f.org, "employee")
	f.placeL = e.link(f.contact, f.place, "resident")
	return f
}

func (e *sgEnv) link(c models.Contact, ent models.Entity, role string) models.ContactEntityLink {
	l := models.ContactEntityLink{UserID: e.A.ID, ContactVCardUID: c.VCardUID, EntityID: ent.ID, Kind: ent.Kind, Role: role, IsPrimary: true, Sensitivity: "normal"}
	require.NoError(e.t, e.db.Create(&l).Error)
	return l
}

func (f *entityGrantFixture) body() string {
	return f.e.do(f.e.B, http.MethodGet, "/share-grants/"+f.grantID+"/content", nil).Body.String()
}

func (f *entityGrantFixture) reconfirmSections() []string {
	var g models.ShareGrant
	require.NoError(f.e.t, f.e.db.First(&g, "id = ?", f.grantID).Error)
	return g.ReconfirmSections
}

func TestReconfirm_PrimaryLinkSensitivityFlipWithholdsUntilReconfirmed(t *testing.T) {
	for _, tc := range []struct {
		name, section, token string
		link                 func(f *entityGrantFixture) *models.ContactEntityLink
	}{
		{"place link", "addresses", "1 Main St", func(f *entityGrantFixture) *models.ContactEntityLink { return &f.placeL }},
		{"organisation link", "organizations", "Acme Corp", func(f *entityGrantFixture) *models.ContactEntityLink { return &f.orgL }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEntityGrantFixture(t)
			require.Contains(t, f.body(), tc.token, "precondition: the projected entry is shared")
			require.Empty(t, f.reconfirmSections())

			l := tc.link(f)
			require.NoError(t, f.e.db.Model(l).Update("sensitivity", "secret").Error)
			assert.NotContains(t, f.body(), tc.token)
			assert.Equal(t, []string{tc.section}, f.reconfirmSections())

			require.NoError(t, f.e.db.Model(l).Update("sensitivity", "normal").Error)
			assert.NotContains(t, f.body(), tc.token, "back to normal stays withheld until reconfirmed")

			require.Equal(t, 200, f.e.do(f.e.A, http.MethodPost, "/share-grants/"+f.grantID+"/reconfirm", nil).Code)
			assert.Contains(t, f.body(), tc.token)
		})
	}
}

func TestReconfirm_EntitySensitivityFlipFansOutToEveryLinkedContact(t *testing.T) {
	f := newEntityGrantFixture(t)
	// A second contact linked to the same place, with its own live grant.
	other := f.e.contact(f.e.A, "Neighbour")
	otherGrant := f.e.grant(f.e.A, f.e.B, "contact", other.VCardUID, []string{"addresses"})
	require.Equal(t, 200, f.e.do(f.e.B, http.MethodPost, "/share-grants/"+otherGrant+"/accept", nil).Code)
	f.e.link(other, f.place, "resident")
	// A third contact that is not linked stays untouched.
	loner := f.e.contact(f.e.A, "Loner")
	lonerGrant := f.e.grant(f.e.A, f.e.B, "contact", loner.VCardUID, []string{"addresses"})

	require.NoError(t, f.e.db.Model(&f.place).Update("sensitivity", "private").Error)
	require.NoError(t, f.e.db.Model(&f.place).Update("sensitivity", "normal").Error)

	assert.Equal(t, []string{"addresses"}, f.reconfirmSections())
	assert.NotContains(t, f.body(), "1 Main St", "withheld after returning to normal")
	var linked, unlinked models.ShareGrant
	require.NoError(t, f.e.db.First(&linked, "id = ?", otherGrant).Error)
	assert.Equal(t, []string{"addresses"}, linked.ReconfirmSections, "every linked contact's grant is marked")
	require.NoError(t, f.e.db.First(&unlinked, "id = ?", lonerGrant).Error)
	assert.Empty(t, unlinked.ReconfirmSections, "an unlinked contact is not affected")

	// An organisation entity flip marks organizations, not addresses.
	require.NoError(t, f.e.db.Model(&f.org).Update("sensitivity", "secret").Error)
	assert.ElementsMatch(t, []string{"addresses", "organizations"}, f.reconfirmSections())
}

func TestReconfirm_EntityOrLinkFlipIsScopedToTheOwnersGrants(t *testing.T) {
	f := newEntityGrantFixture(t)
	// The same sensitivity flip on another owner's entity must not touch A's grant.
	foreign := models.Entity{UserID: f.e.C.ID, Kind: models.EntityKindPlace, Name: "Elsewhere"}
	require.NoError(t, foreign.SetCore(&models.PlaceCore{}))
	require.NoError(t, f.e.db.Create(&foreign).Error)
	require.NoError(t, f.e.db.Model(&foreign).Update("sensitivity", "secret").Error)
	assert.Empty(t, f.reconfirmSections())
}

// Only a normal -> private/secret move marks a section: re-saving a link or
// entity that was already non-normal (after the owner re-confirmed) must not
// withhold it again.
func TestReconfirm_ResavingAnAlreadyPrivateLinkOrEntityDoesNotRemark(t *testing.T) {
	f := newEntityGrantFixture(t)
	require.NoError(t, f.e.db.Model(&f.placeL).Update("sensitivity", "private").Error)
	require.NoError(t, f.e.db.Model(&f.place).Update("sensitivity", "private").Error)
	require.Equal(t, 200, f.e.do(f.e.A, http.MethodPost, "/share-grants/"+f.grantID+"/reconfirm", nil).Code)
	require.Empty(t, f.reconfirmSections())

	require.NoError(t, f.e.db.Model(&f.placeL).Update("role", "visitor").Error)
	require.Empty(t, f.reconfirmSections(), "step role")
	require.NoError(t, f.e.db.Model(&f.place).Update("name", "Cabin 2").Error)
	require.Empty(t, f.reconfirmSections(), "step name")
	require.NoError(t, f.e.db.Model(&f.placeL).Update("sensitivity", "secret").Error) // private -> secret
	assert.Empty(t, f.reconfirmSections(), "an already-non-normal item is not re-marked")
}
