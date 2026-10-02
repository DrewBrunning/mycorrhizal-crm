package models

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
)

// Contact map address fields (ADR 0031, issue #694): stable IDs, coordinates
// and sensitivity on the flat ContactAddress, threaded through the neutral
// Card both ways and surviving a real save/reload.

func TestAddressMapFieldsThreadThroughBothMappings(t *testing.T) {
	flat := ContactAddress{ID: "a-1", Coordinates: "geo:48.2,16.3", Sensitivity: "private", Street: "1 Main St", City: "Wien", Type: "home"}

	neutral := AddressFromContactAddress(flat)
	assert.Equal(t, "a-1", neutral.ID)
	assert.Equal(t, "geo:48.2,16.3", neutral.Coordinates)
	assert.Equal(t, "private", neutral.Sensitivity)

	assert.Equal(t, flat, contactAddressFromNeutral(neutral), "flat -> neutral -> flat is lossless for the map fields")
}

func TestEnsureAddressIDs_MintsDistinctUUIDsForFlatOnlyAddresses(t *testing.T) {
	c := &Contact{Addresses: []ContactAddress{{Street: "1 Main St"}, {Street: "2 Side St"}}}
	c.ensureAddressIDs(false)

	require.Len(t, c.Addresses, 2)
	for _, a := range c.Addresses {
		_, err := uuid.Parse(a.ID)
		assert.NoError(t, err, "minted ID %q is a UUID", a.ID)
	}
	assert.NotEqual(t, c.Addresses[0].ID, c.Addresses[1].ID)
	assert.Empty(t, c.Card.Addresses, "no Card entry exists to stamp")
}

func TestEnsureAddressIDs_KeepsExistingIDs(t *testing.T) {
	c := &Contact{Addresses: []ContactAddress{{ID: "keep", Street: "1 Main St"}, {Street: "2 Side St"}}}
	c.ensureAddressIDs(false)
	assert.Equal(t, "keep", c.Addresses[0].ID)
	assert.NotEmpty(t, c.Addresses[1].ID)
}

func TestEnsureAddressIDs_InheritsCardIDWhenPostalFieldsMatch(t *testing.T) {
	card := contactmodel.Card{Addresses: []contactmodel.Address{
		{ID: "card-id", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}, {Kind: "building", Value: "Tower B"}}, Full: "1 Main St"},
	}}
	c := &Contact{Card: card, Addresses: []ContactAddress{{Street: "1 Main St"}}}
	c.ensureAddressIDs(false)
	assert.Equal(t, "card-id", c.Addresses[0].ID, "an ID-unaware editor resubmitting an unchanged address keeps its ID")
}

func TestEnsureAddressIDs_StampsTheMatchingCardEntryWithoutMutatingTheOriginal(t *testing.T) {
	original := []contactmodel.Address{{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}}, Full: "1 Main St"}}
	c := &Contact{Card: contactmodel.Card{Addresses: original}, Addresses: []ContactAddress{{Street: "1 Main St"}}}
	c.ensureAddressIDs(false)

	require.NotEmpty(t, c.Addresses[0].ID)
	assert.Equal(t, c.Addresses[0].ID, c.Card.Addresses[0].ID, "flat and Card share the minted ID")
	assert.Empty(t, original[0].ID, "copy-on-write: the caller's backing array is untouched (the probe runs on shallow copies)")
}

func TestEnsureAddressIDs_ChangedAddressGetsAFreshID(t *testing.T) {
	card := contactmodel.Card{Addresses: []contactmodel.Address{
		{ID: "card-id", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}}},
	}}
	c := &Contact{Card: card, Addresses: []ContactAddress{{Street: "9 Different Rd"}}}
	c.ensureAddressIDs(false)
	assert.NotEqual(t, "card-id", c.Addresses[0].ID, "an edited address is a new address")
	assert.NotEmpty(t, c.Addresses[0].ID)
	assert.Equal(t, "card-id", c.Card.Addresses[0].ID, "and the stale Card entry is left alone")
}

func TestEnsureAddressIDs_AddressBeyondTheCardIsMinted(t *testing.T) {
	c := &Contact{
		Card:      contactmodel.Card{Addresses: []contactmodel.Address{{ID: "card-id", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}}}}},
		Addresses: []ContactAddress{{Street: "1 Main St"}, {Street: "2 Side St"}},
	}
	c.ensureAddressIDs(false)
	assert.Equal(t, "card-id", c.Addresses[0].ID)
	assert.NotEmpty(t, c.Addresses[1].ID)
	assert.Len(t, c.Card.Addresses, 1)
}

func TestEnsureAddressIDs_DirectCardMintsOnCardAndMirrorsToFlat(t *testing.T) {
	card := contactmodel.Card{Addresses: []contactmodel.Address{
		{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}}},
		{ID: "given", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "2 Side St"}}},
	}}
	c := &Contact{Card: card}
	applyAddresses(c, card) // what ApplyRecordToContact does before the save
	c.ensureAddressIDs(true)

	require.Len(t, c.Card.Addresses, 2)
	require.Len(t, c.Addresses, 2)
	assert.NotEmpty(t, c.Card.Addresses[0].ID)
	assert.Equal(t, "given", c.Card.Addresses[1].ID)
	for i := range c.Addresses {
		assert.Equal(t, c.Card.Addresses[i].ID, c.Addresses[i].ID, "flat[%d] mirrors the Card", i)
	}
	assert.Empty(t, card.Addresses[0].ID, "the caller's card slice is not mutated")
}

func TestEnsureAddressIDs_DirectCardWithNoAddressesIsANoOp(t *testing.T) {
	c := &Contact{}
	c.ensureAddressIDs(true)
	assert.Empty(t, c.Addresses)
}

func TestFindAddressByID(t *testing.T) {
	c := &Contact{Addresses: []ContactAddress{{ID: "a", Street: "1"}, {ID: "b", Street: "2"}}}
	a, idx, ok := c.FindAddressByID("b")
	assert.True(t, ok)
	assert.Equal(t, 1, idx)
	assert.Equal(t, "2", a.Street)

	_, idx, ok = c.FindAddressByID("zzz")
	assert.False(t, ok)
	assert.Equal(t, -1, idx)

	_, _, ok = c.FindAddressByID("")
	assert.False(t, ok, "the empty ID never matches an unset ID")
	_, _, ok = (&Contact{Addresses: []ContactAddress{{Street: "no id"}}}).FindAddressByID("")
	assert.False(t, ok)
}

func TestSetAddressCoordinates(t *testing.T) {
	origFlat := []ContactAddress{{ID: "a", Street: "1"}, {ID: "b", Street: "2"}}
	origCard := []contactmodel.Address{{ID: "a"}, {ID: "b"}}
	c := &Contact{Addresses: origFlat, Card: contactmodel.Card{Addresses: origCard}}

	require.True(t, c.SetAddressCoordinates("b", "geo:1,2"))
	assert.Equal(t, "geo:1,2", c.Addresses[1].Coordinates)
	assert.Equal(t, "geo:1,2", c.Card.Addresses[1].Coordinates, "both copies, so the T75 merge sees no edit")
	assert.Empty(t, c.Addresses[0].Coordinates)
	assert.Empty(t, c.Card.Addresses[0].Coordinates)
	assert.Empty(t, origFlat[1].Coordinates, "copy-on-write on the flat slice")
	assert.Empty(t, origCard[1].Coordinates, "copy-on-write on the Card slice")

	assert.False(t, c.SetAddressCoordinates("nope", "geo:3,4"))
	assert.Equal(t, "geo:1,2", c.Addresses[1].Coordinates, "an unknown ID changes nothing")

	// A flat address with no Card counterpart (flat-only legacy contact) still updates.
	flatOnly := &Contact{Addresses: []ContactAddress{{ID: "x"}}}
	require.True(t, flatOnly.SetAddressCoordinates("x", "geo:5,6"))
	assert.Equal(t, "geo:5,6", flatOnly.Addresses[0].Coordinates)
}

func TestValidateAddressMapFields(t *testing.T) {
	assert.Empty(t, ValidateAddressMapFields(nil))
	assert.Empty(t, ValidateAddressMapFields([]contactmodel.Address{
		{ID: "a", Coordinates: "geo:48.2,16.3", Sensitivity: "private"},
		{ID: "b", Sensitivity: "secret"},
		{ID: "c", Sensitivity: "normal"},
		{}, {}, // empty IDs may repeat: the server mints them
	}))

	got := ValidateAddressMapFields([]contactmodel.Address{
		{Coordinates: "48.2,16.3"},
		{Coordinates: "geo:95,0"},
		{Sensitivity: "classified"},
		{ID: strings.Repeat("x", 129)},
		{ID: "dup"}, {ID: "dup"},
	})
	assert.Contains(t, got, "card.addresses[0].coordinates")
	assert.Contains(t, got, "card.addresses[1].coordinates")
	assert.Contains(t, got, "card.addresses[2].sensitivity")
	assert.Contains(t, got, "card.addresses[3].id")
	assert.Contains(t, got, "card.addresses[5].id")
	assert.NotContains(t, got, "card.addresses[4].id", "the first occurrence of an ID is fine")
}

func TestMergeAddresses(t *testing.T) {
	mk := func(id, street string) contactmodel.Address {
		return contactmodel.Address{ID: id, Components: []contactmodel.AddressComponent{{Kind: "name", Value: street}, {Kind: "building", Value: "Tower B"}}, Full: street}
	}
	fresh := func(id, street string) contactmodel.Address {
		return AddressFromContactAddress(ContactAddress{ID: id, Street: street})
	}

	t.Run("same projection keeps the loaded entry whole", func(t *testing.T) {
		got := mergeAddresses([]contactmodel.Address{mk("a", "1 Main St")}, []contactmodel.Address{fresh("a", "1 Main St")})
		assert.Equal(t, mk("a", "1 Main St"), got[0], "unprojected 'building' component survives")
	})
	t.Run("an ID on only the loaded side is not an edit", func(t *testing.T) {
		got := mergeAddresses([]contactmodel.Address{mk("a", "1 Main St")}, []contactmodel.Address{fresh("", "1 Main St")})
		assert.Equal(t, mk("a", "1 Main St"), got[0])
	})
	t.Run("an ID on only the fresh side is not an edit either", func(t *testing.T) {
		got := mergeAddresses([]contactmodel.Address{mk("", "1 Main St")}, []contactmodel.Address{fresh("new", "1 Main St")})
		assert.Equal(t, mk("", "1 Main St"), got[0])
	})
	t.Run("two different IDs are a real difference", func(t *testing.T) {
		got := mergeAddresses([]contactmodel.Address{mk("a", "1 Main St")}, []contactmodel.Address{fresh("b", "1 Main St")})
		assert.Equal(t, fresh("b", "1 Main St"), got[0])
	})
	t.Run("an edited street takes the fresh entry", func(t *testing.T) {
		got := mergeAddresses([]contactmodel.Address{mk("a", "1 Main St")}, []contactmodel.Address{fresh("a", "9 Other Rd")})
		assert.Equal(t, fresh("a", "9 Other Rd"), got[0])
	})
	t.Run("changed coordinates or sensitivity are an edit", func(t *testing.T) {
		f := fresh("a", "1 Main St")
		f.Coordinates = "geo:1,2"
		assert.Equal(t, f, mergeAddresses([]contactmodel.Address{mk("a", "1 Main St")}, []contactmodel.Address{f})[0])
		g := fresh("a", "1 Main St")
		g.Sensitivity = "secret"
		assert.Equal(t, g, mergeAddresses([]contactmodel.Address{mk("a", "1 Main St")}, []contactmodel.Address{g})[0])
	})
	t.Run("length follows fresh", func(t *testing.T) {
		assert.Len(t, mergeAddresses([]contactmodel.Address{mk("a", "1")}, nil), 0)
		got := mergeAddresses(nil, []contactmodel.Address{fresh("a", "1"), fresh("b", "2")})
		assert.Len(t, got, 2)
	})
}

// --- real migrated schema ---------------------------------------------------

func TestAddressMapFields_PersistThroughRealDB(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	user := User{Username: "adrmap", Password: "password123!A", Email: "adrmap@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Create through the nested path (ApplyRecordToContact, trap #2), omitting IDs.
	record := &contactmodel.Record{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
		Addresses: []contactmodel.Address{
			{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}, {Kind: "building", Value: "Tower B"}}, Full: "1 Main St", Coordinates: "geo:48.2,16.3", Sensitivity: "private", Contexts: []string{"private"}},
			{Components: []contactmodel.AddressComponent{{Kind: "name", Value: "2 Side St"}}, Full: "2 Side St"},
		},
	}}
	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, record, "")
	require.NoError(t, db.Create(contact).Error)

	var loaded Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	require.Len(t, loaded.Addresses, 2)
	require.Len(t, loaded.Card.Addresses, 2)
	for i := range loaded.Addresses {
		assert.NotEmpty(t, loaded.Addresses[i].ID, "address %d got an ID on create", i)
		assert.Equal(t, loaded.Addresses[i].ID, loaded.Card.Addresses[i].ID, "flat and Card agree on it")
	}
	assert.NotEqual(t, loaded.Addresses[0].ID, loaded.Addresses[1].ID)
	assert.Equal(t, "geo:48.2,16.3", loaded.Addresses[0].Coordinates)
	assert.Equal(t, "private", loaded.Addresses[0].Sensitivity)
	assert.Empty(t, loaded.Addresses[1].Sensitivity)

	ids := []string{loaded.Addresses[0].ID, loaded.Addresses[1].ID}

	// A plain save of the loaded contact (the T75 path: profile photo, import merge,
	// audit undo) must keep the IDs AND the Card-only 'building' component.
	require.NoError(t, db.Save(&loaded).Error)
	var again Contact
	require.NoError(t, db.First(&again, contact.ID).Error)
	assert.Equal(t, ids[0], again.Addresses[0].ID, "IDs are stable across a plain save")
	assert.Equal(t, ids[1], again.Addresses[1].ID)
	assert.Contains(t, again.Card.Addresses[0].Components, contactmodel.AddressComponent{Kind: "building", Value: "Tower B"},
		"the unprojected component survived (the whole point of threading the ID through the merge)")

	// Writing coordinates the way the geocode endpoint does keeps that component too.
	require.True(t, again.SetAddressCoordinates(ids[1], "geo:10,20"))
	require.NoError(t, db.Save(&again).Error)
	var final Contact
	require.NoError(t, db.First(&final, contact.ID).Error)
	assert.Equal(t, "geo:10,20", final.Addresses[1].Coordinates)
	assert.Equal(t, "geo:10,20", final.Card.Addresses[1].Coordinates)
	assert.Contains(t, final.Card.Addresses[0].Components, contactmodel.AddressComponent{Kind: "building", Value: "Tower B"})

	// An ID-unaware flat editor (no ids in the submitted flat addresses) must not churn them.
	stale := final
	stale.Addresses = []ContactAddress{
		{Street: "1 Main St", Type: "home", Coordinates: "geo:48.2,16.3", Sensitivity: "private"},
		{Street: "2 Side St", Coordinates: "geo:10,20"},
	}
	require.NoError(t, db.Save(&stale).Error)
	var inherited Contact
	require.NoError(t, db.First(&inherited, contact.ID).Error)
	assert.Equal(t, ids[0], inherited.Addresses[0].ID)
	assert.Equal(t, ids[1], inherited.Addresses[1].ID)
	assert.Contains(t, inherited.Card.Addresses[0].Components, contactmodel.AddressComponent{Kind: "building", Value: "Tower B"})
}

func TestRecordForContact_ExposesAddressMapFields(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	user := User{Username: "adrrec", Password: "password123!A", Email: "adrrec@example.com"}
	require.NoError(t, db.Create(&user).Error)

	record := &contactmodel.Record{Card: contactmodel.Card{
		Name:      &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Bo"}}},
		Addresses: []contactmodel.Address{{ID: "client-id", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "1 Main St"}}, Coordinates: "geo:1,2", Sensitivity: "secret"}},
	}}
	contact := &Contact{UserID: user.ID}
	ApplyRecordToContact(contact, record, "")
	require.NoError(t, db.Create(contact).Error)

	var loaded Contact
	require.NoError(t, db.First(&loaded, contact.ID).Error)
	got := RecordForContact(&loaded, "", db)
	require.Len(t, got.Card.Addresses, 1)
	assert.Equal(t, "client-id", got.Card.Addresses[0].ID, "a client-supplied ID is kept")
	assert.Equal(t, "geo:1,2", got.Card.Addresses[0].Coordinates)
	assert.Equal(t, "secret", got.Card.Addresses[0].Sensitivity)
}
