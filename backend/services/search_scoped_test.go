package services

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SearchPageScoped(includeSensitive=false) must not let a hit be explained
// only by a private/secret address (contacts_fts indexes addresses_flat across
// every sensitivity). Exercised here, not just via the MCP controller, because
// the per-file coverage ratchet measures the services package on its own.
func TestSearchPageScoped_SensitiveAddressIsNotAnExistenceOracle(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "scoped-1", Password: "password123!A", Email: "scoped-1@example.com"}
	require.NoError(t, db.Create(&user).Error)

	seed := func(first, street, sens string) models.Contact {
		c := models.Contact{UserID: user.ID, Firstname: first, Lastname: "Oracle"}
		require.NoError(t, db.Create(&c).Error)
		models.ApplyRecordToContact(&c, &contactmodel.Record{Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: first}}},
			Addresses: []contactmodel.Address{{ID: "a", Full: street, Sensitivity: sens,
				Components: []contactmodel.AddressComponent{{Kind: "name", Value: street}}}},
		}}, "")
		require.NoError(t, db.Save(&c).Error)
		return c
	}
	secretOnly := seed("Secretive", "Quillfeather Lane", models.RelationshipSensitivitySecret)
	openOnly := seed("Openly", "Quillfeather Lane", "")
	byName := seed("Quillfeather", "Hidden Road", models.RelationshipSensitivitySecret)

	ids := func(include bool) map[uint]bool {
		r, err := SearchPageScoped(db, user.ID, "Quillfeather", 20, 0, nil, include)
		require.NoError(t, err)
		out := map[uint]bool{}
		for _, h := range r.Contacts {
			out[h.ID] = true
		}
		return out
	}

	def := ids(false)
	assert.False(t, def[secretOnly.ID], "secret-address-only match is hidden")
	assert.True(t, def[openOnly.ID])
	assert.True(t, def[byName.ID], "name match survives despite a secret address")

	all := ids(true)
	assert.True(t, all[secretOnly.ID])
	assert.True(t, all[openOnly.ID])
	assert.True(t, all[byName.ID])

	// Household scope + filter compose (arg ordering in the raw query).
	hh := "no-such-household"
	r, err := SearchPageScoped(db, user.ID, "Quillfeather", 20, 0, &hh, false)
	require.NoError(t, err)
	assert.Empty(t, r.Contacts)
}
