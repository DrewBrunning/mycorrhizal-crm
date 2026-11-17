package services

import (
	"encoding/json"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2744 (item 3): contact hits say which of their own fields the query
// matched, read from the same contacts_fts index the match came from.
func TestSearch_ContactMatchedOn(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "matched-on", Password: "password123!A", Email: "matched-on@example.com"}
	require.NoError(t, db.Create(&user).Error)

	mk := func(c models.Contact) {
		c.UserID = user.ID
		require.NoError(t, db.Create(&c).Error)
	}
	mk(models.Contact{Firstname: "Quillon", Lastname: "Alder"})
	mk(models.Contact{Firstname: "Beatrix", Lastname: "Marlow", Organization: "Zebracorp Holdings"})
	mk(models.Contact{Firstname: "Cyrus", Lastname: "Neal", Email: "cyrus@bazqux.example"})
	mk(models.Contact{Firstname: "Dara", Lastname: "Oakes", Phones: []models.ContactPhone{{Type: "mobile", Value: "+1 555 010 4477"}}})
	mk(models.Contact{Firstname: "Edda", Lastname: "Pike", Addresses: []models.ContactAddress{{Type: "home", Street: "Mangosteen Lane", City: "Clarkston"}}})
	mk(models.Contact{Firstname: "Ravenwood", Lastname: "Quist", Email: "ravenwood@example.org"})
	mk(models.Contact{Firstname: "Fenn", Lastname: "Zebracorp", Organization: "Zebracorp Ltd"})

	matched := func(term string) map[string][]string {
		res, err := Search(db, user.ID, term, 50, nil)
		require.NoError(t, err, term)
		out := map[string][]string{}
		for _, h := range res.Contacts {
			out[h.Firstname] = h.MatchedOn
		}
		return out
	}

	assert.Equal(t, []string{"name"}, matched("quillon")["Quillon"])
	assert.Equal(t, []string{"organization"}, matched("zebracorp")["Beatrix"])
	assert.Equal(t, []string{"email"}, matched("bazqux")["Cyrus"])
	assert.Equal(t, []string{"phone"}, matched("5550104477")["Dara"], "a differently formatted phone query")
	assert.Equal(t, []string{"address"}, matched("mangosteen")["Edda"])
	assert.Equal(t, []string{"name", "email"}, matched("ravenwood")["Ravenwood"], "every field that matched, in a fixed order")
	assert.Equal(t, []string{"name", "organization"}, matched("zebracorp")["Fenn"])

	// A multi-word query that is spread over two fields reports both.
	assert.Equal(t, []string{"name", "organization"}, matched("beatrix zebracorp")["Beatrix"])

	// Enum values are the documented small set.
	for _, v := range []string{SearchMatchName, SearchMatchOrganization, SearchMatchEmail, SearchMatchPhone, SearchMatchAddress} {
		assert.Contains(t, []string{"name", "organization", "email", "phone", "address"}, v)
	}
}

// The scoped (outward-facing, includeSensitive=false) path never reports a
// reason: naming "address" there would confirm text the scope hides.
func TestSearchPageScoped_NoMatchedOnWhenNotIncludingSensitive(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "matched-scoped", Password: "password123!A", Email: "matched-scoped@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Hollis", Lastname: "Quartz"}).Error)

	scoped, err := SearchPageScoped(db, user.ID, "hollis", 10, 0, nil, false)
	require.NoError(t, err)
	require.Len(t, scoped.Contacts, 1)
	assert.Empty(t, scoped.Contacts[0].MatchedOn)
	raw, err := json.Marshal(scoped.Contacts[0])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "matched_on", "the key is absent, not null or []")

	owner, err := SearchPageScoped(db, user.ID, "hollis", 10, 0, nil, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"name"}, owner.Contacts[0].MatchedOn)
}

// Entities are matched on their name only (core is encrypted at rest).
func TestSearch_EntityMatchedOnName(t *testing.T) {
	db := newSearchDB(t)
	u := entUser(t, db, "ent-matched")
	entMake(t, db, u.ID, models.EntityKindOrganisation, "Quokka Holdings")

	res, err := Search(db, u.ID, "quokka", 0, nil)
	require.NoError(t, err)
	require.Len(t, res.Entities, 1)
	assert.Equal(t, []string{"name"}, res.Entities[0].MatchedOn)
}
