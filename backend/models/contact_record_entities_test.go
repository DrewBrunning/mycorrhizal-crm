package models

import (
	"encoding/json"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func pinProjectionDay(t *testing.T, y, m, d int) {
	t.Helper()
	restore := SetEntityProjectionToday(func() *contactmodel.PartialDate {
		return &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}
	})
	t.Cleanup(restore)
}

type projFixture struct {
	db      *gorm.DB
	user    User
	contact Contact
}

func newProjFixture(t *testing.T) *projFixture {
	t.Helper()
	db := dbtest.New(t)
	u := User{Username: "proj", Password: "x", Email: "proj@example.com"}
	require.NoError(t, db.Create(&u).Error)
	c := Contact{UserID: u.ID, Firstname: "Ann"}
	require.NoError(t, db.Create(&c).Error)
	return &projFixture{db: db, user: u, contact: c}
}

func (f *projFixture) contactWithCard(t *testing.T, card contactmodel.Card) *Contact {
	t.Helper()
	c := f.contact
	card.UID = c.VCardUID
	card.Name = &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ann"}}}
	ApplyRecordToContact(&c, &contactmodel.Record{UID: c.VCardUID, Card: card}, "")
	require.NoError(t, f.db.Save(&c).Error)
	return &c
}

func (f *projFixture) org(t *testing.T, name, sens string) Entity {
	t.Helper()
	e := Entity{UserID: f.user.ID, Kind: EntityKindOrganisation, Name: name, Sensitivity: sens}
	require.NoError(t, e.SetCore(&OrganisationCore{Organization: contactmodel.Organization{Name: name, Units: []contactmodel.OrgUnit{{Name: "R&D"}}}}))
	require.NoError(t, f.db.Create(&e).Error)
	return e
}

func (f *projFixture) place(t *testing.T, sens string) Entity {
	t.Helper()
	e := Entity{UserID: f.user.ID, Kind: EntityKindPlace, Name: "Home", Sensitivity: sens}
	require.NoError(t, e.SetCore(&PlaceCore{Address: contactmodel.Address{Components: []contactmodel.AddressComponent{
		{Kind: "name", Value: "1 Main St"}, {Kind: "locality", Value: "Springfield"}}}}))
	require.NoError(t, f.db.Create(&e).Error)
	return e
}

func (f *projFixture) link(t *testing.T, e Entity, primary bool, sens string, period *contactmodel.TemporalRange) ContactEntityLink {
	t.Helper()
	role := map[string]string{EntityKindOrganisation: "employee", EntityKindPlace: "resident", EntityKindPet: "owner"}[e.Kind]
	l := ContactEntityLink{UserID: f.user.ID, ContactVCardUID: f.contact.VCardUID, EntityID: e.ID, Kind: e.Kind,
		Role: role, IsPrimary: primary, Sensitivity: sens, Period: period}
	require.NoError(t, f.db.Create(&l).Error)
	return l
}

func (f *projFixture) record(t *testing.T, sel *FieldSelection) *contactmodel.Record {
	t.Helper()
	var c Contact
	require.NoError(t, f.db.First(&c, f.contact.ID).Error)
	return RecordForContactFiltered(&c, "", f.db, sel)
}

func TestProjectPrimaryEntities_OrganisationAndPlaceProject(t *testing.T) {
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{})
	org := f.org(t, "Acme", "normal")
	place := f.place(t, "normal")
	lo := f.link(t, org, true, "normal", nil)
	lp := f.link(t, place, true, "normal", nil)

	rec := f.record(t, FieldSelectionAll())
	require.Len(t, rec.Card.Organizations, 1)
	assert.Equal(t, "ent-"+lo.ID, rec.Card.Organizations[0].ID)
	assert.Equal(t, "Acme", rec.Card.Organizations[0].Name)
	require.Len(t, rec.Card.Addresses, 1)
	assert.Equal(t, "ent-"+lp.ID, rec.Card.Addresses[0].ID)
	require.NotNil(t, rec.Card.Addresses[0].Pref)
	assert.Equal(t, 1, *rec.Card.Addresses[0].Pref)

	// REST detail (nil selection) reads through the same path.
	assert.Len(t, f.record(t, nil).Card.Organizations, 1)
}

func TestProjectPrimaryEntities_NeverWrittenBackToStoredCard(t *testing.T) {
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	_ = f.record(t, FieldSelectionAll())
	var stored Contact
	require.NoError(t, f.db.First(&stored, f.contact.ID).Error)
	assert.Empty(t, stored.Card.Organizations)
	assert.Empty(t, stored.Card.Addresses)
}

func TestProjectPrimaryEntities_OnlyCurrentPrimaryOrgOrPlace(t *testing.T) {
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{})
	pinProjectionDay(t, 2026, 6, 1)
	y2020, y2024, y2030 := 2020, 2024, 2030
	pet := Entity{UserID: f.user.ID, Kind: EntityKindPet, Name: "Rex"}
	require.NoError(t, f.db.Create(&pet).Error)
	f.link(t, pet, true, "normal", nil)                               // pets never project
	f.link(t, f.org(t, "NotPrimary", "normal"), false, "normal", nil) // non-primary
	ended := f.org(t, "Ended", "normal")
	f.link(t, ended, true, "normal", &contactmodel.TemporalRange{End: &contactmodel.PartialDate{Year: &y2024}})
	// A second primary organisation is impossible (unique index), so move the
	// ended one aside to prove the future-start case separately.
	assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Organizations, "ended, pet, non-primary: nothing projects")

	setPeriod(t, f, ended.ID, &contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: &y2030}})
	assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Organizations, "future start is not current")

	setPeriod(t, f, ended.ID, &contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: &y2020}})
	assert.Len(t, f.record(t, FieldSelectionAll()).Card.Organizations, 1, "started in the past, open end: current")
}

func TestProjectPrimaryEntities_SoftDeletedAndCrossUserNeverProject(t *testing.T) {
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{})
	org := f.org(t, "Acme", "normal")
	l := f.link(t, org, true, "normal", nil)
	require.NoError(t, f.db.Delete(&l).Error)
	assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Organizations, "soft-deleted link")

	l2 := f.link(t, org, true, "normal", nil)
	require.NoError(t, f.db.Delete(&org).Error)
	assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Organizations, "soft-deleted entity")
	require.NoError(t, f.db.Delete(&l2).Error)

	// An entity of another owner never projects even if a link row points at it.
	other := User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, f.db.Create(&other).Error)
	foreign := Entity{UserID: other.ID, Kind: EntityKindOrganisation, Name: "Foreign"}
	require.NoError(t, f.db.Create(&foreign).Error)
	require.NoError(t, f.db.Create(&ContactEntityLink{UserID: f.user.ID, ContactVCardUID: f.contact.VCardUID,
		EntityID: foreign.ID, Kind: EntityKindOrganisation, Role: "employee", IsPrimary: true}).Error)
	assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Organizations)
}

func TestProjectPrimaryEntities_OwnValuesLeadAndDedupe(t *testing.T) {
	f := newProjFixture(t)
	pref5 := 5
	f.contactWithCard(t, contactmodel.Card{
		Organizations: []contactmodel.Organization{{ID: "own-org", Name: "Own Co"}},
		Addresses:     []contactmodel.Address{{ID: "own-adr", Pref: &pref5, Components: []contactmodel.AddressComponent{{Kind: "name", Value: "9 Elm"}}}},
	})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	f.link(t, f.place(t, "normal"), true, "normal", nil)

	rec := f.record(t, FieldSelectionAll())
	require.Len(t, rec.Card.Organizations, 2)
	assert.Equal(t, "own-org", rec.Card.Organizations[0].ID, "own value leads")
	assert.True(t, IsEntityProjectionID(rec.Card.Organizations[1].ID))
	require.Len(t, rec.Card.Addresses, 2)
	assert.Equal(t, "own-adr", rec.Card.Addresses[0].ID)
	assert.Equal(t, 5, *rec.Card.Addresses[0].Pref, "own pref untouched")
	assert.Equal(t, 6, *rec.Card.Addresses[1].Pref, "projected pref is greater than any own pref")
}

func TestProjectPrimaryEntities_ValueEqualAfterNormalizationIsDropped(t *testing.T) {
	f := newProjFixture(t)
	// Own ORG typed in NFD; the entity holds the NFC form (ADR 0016).
	f.contactWithCard(t, contactmodel.Card{Organizations: []contactmodel.Organization{
		{ID: "own", Name: "Café Corp", Units: []contactmodel.OrgUnit{{Name: "R&D"}}}}})
	f.link(t, f.org(t, "Café Corp", "normal"), true, "normal", nil)
	rec := f.record(t, FieldSelectionAll())
	require.Len(t, rec.Card.Organizations, 1, "value-equal projection is a duplicate and is dropped")
	assert.Equal(t, "own", rec.Card.Organizations[0].ID)
}

func TestProjectPrimaryEntities_SensitivityIsMaxOfEntityAndLink(t *testing.T) {
	for _, tc := range []struct{ name, entity, link string }{
		{"private entity", "private", "normal"},
		{"private link", "normal", "private"},
		{"secret entity", "secret", "normal"},
		{"secret link", "normal", "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProjFixture(t)
			f.contactWithCard(t, contactmodel.Card{})
			f.link(t, f.org(t, "Acme", tc.entity), true, tc.link, nil)
			pl := f.place(t, tc.entity)
			f.link(t, pl, true, tc.link, nil)

			for _, sel := range []*FieldSelection{FieldSelectionAll(), nil} {
				rec := f.record(t, sel)
				assert.Empty(t, rec.Card.Organizations, "above-normal never projects by default")
				assert.Empty(t, rec.Card.Addresses)
			}
			// Only the explicit include flag brings it back, and a place carries the token.
			rec := f.record(t, &FieldSelection{IncludeSensitive: true, Sections: FieldSelectionAll().Sections})
			require.Len(t, rec.Card.Organizations, 1)
			require.Len(t, rec.Card.Addresses, 1)
			want := tc.entity
			if tc.link != "normal" {
				want = tc.link
			}
			assert.Equal(t, want, rec.Card.Addresses[0].Sensitivity)
		})
	}
}

func TestProjectedPlaceAbovePlaceThresholdIsStrippedByFieldSelection(t *testing.T) {
	// A place with a stamped Address.Sensitivity is dropped by ApplyFieldSelection
	// exactly like an own address, even if it somehow reached the Record.
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{})
	f.link(t, f.place(t, "private"), true, "normal", nil)
	rec := f.record(t, &FieldSelection{IncludeSensitive: true, Sections: FieldSelectionAll().Sections})
	require.Len(t, rec.Card.Addresses, 1)
	filtered := ApplyFieldSelection(&contactmodel.Record{Card: rec.Card}, FieldSelectionAll())
	assert.Empty(t, filtered.Card.Addresses)
}

func TestStripProjectedEntityIDs_AndApplyRecordDropsProjection(t *testing.T) {
	c := Contact{Firstname: "Ann"}
	rec := &contactmodel.Record{Card: contactmodel.Card{
		Organizations: []contactmodel.Organization{{ID: "own", Name: "Own"}, {ID: "ent-1", Name: "Acme"}},
		Addresses:     []contactmodel.Address{{ID: "ent-2", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "x"}}}},
		Titles:        []contactmodel.Title{{ID: "t", Name: "Dev", Kind: "title", OrganizationID: "ent-1"}},
	}, Envelope: contactmodel.CRMEnvelope{Periods: []contactmodel.EntryPeriod{{Kind: "organizations", EntryID: "ent-1"}, {Kind: "organizations", EntryID: "own"}}}}
	ApplyRecordToContact(&c, rec, "")
	require.Len(t, c.Card.Organizations, 1)
	assert.Equal(t, "own", c.Card.Organizations[0].ID)
	assert.Empty(t, c.Card.Addresses)
	assert.Equal(t, "", c.Card.Titles[0].OrganizationID, "title association to a dropped projection is cleared")
	require.Len(t, c.CRM.Periods, 1)
	assert.Equal(t, "own", c.CRM.Periods[0].EntryID)
	assert.Len(t, rec.Card.Organizations, 2, "the caller's Record is not mutated")
}

func TestSuppressEntityProjectionEcho_IDStrippingClient(t *testing.T) {
	f := newProjFixture(t)
	stored := f.contactWithCard(t, contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "own", Name: "Own Co"}}})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	f.link(t, f.place(t, "normal"), true, "normal", nil)

	// The id-stripped echo of the projected card.
	echo := f.record(t, FieldSelectionAll())
	for i := range echo.Card.Organizations {
		echo.Card.Organizations[i].ID = ""
	}
	for i := range echo.Card.Addresses {
		echo.Card.Addresses[i].ID, echo.Card.Addresses[i].Pref = "", nil
	}
	require.NoError(t, SuppressEntityProjectionEcho(f.db, stored, echo))
	require.Len(t, echo.Card.Organizations, 1)
	assert.Equal(t, "Own Co", echo.Card.Organizations[0].Name)
	assert.Empty(t, echo.Card.Addresses)

	// A client edit of the projected value is an ordinary new value (no loss).
	edited := &contactmodel.Record{Card: contactmodel.Card{Organizations: []contactmodel.Organization{
		{Name: "Own Co"}, {Name: "Acme Renamed", Units: []contactmodel.OrgUnit{{Name: "R&D"}}}}}}
	require.NoError(t, SuppressEntityProjectionEcho(f.db, stored, edited))
	assert.Len(t, edited.Card.Organizations, 2)
}

func TestSuppressEntityProjectionEcho_ValueTheContactAlsoOwnsIsKept(t *testing.T) {
	f := newProjFixture(t)
	stored := f.contactWithCard(t, contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "own", Name: "Acme", Units: []contactmodel.OrgUnit{{Name: "R&D"}}}}})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	rec := &contactmodel.Record{Card: contactmodel.Card{Organizations: []contactmodel.Organization{{Name: "Acme", Units: []contactmodel.OrgUnit{{Name: "R&D"}}}}}}
	require.NoError(t, SuppressEntityProjectionEcho(f.db, stored, rec))
	assert.Len(t, rec.Card.Organizations, 1, "the contact owns that value; it is not an echo")
}

func TestSuppressEntityProjectionEcho_NilSafe(t *testing.T) {
	assert.NoError(t, SuppressEntityProjectionEcho(nil, nil, nil))
	f := newProjFixture(t)
	require.NoError(t, SuppressEntityProjectionEcho(f.db, nil, &contactmodel.Record{}))
	require.NoError(t, SuppressEntityProjectionEcho(f.db, &f.contact, nil))
	SuppressEntityProjectionEcho(f.db, &f.contact, &contactmodel.Record{}) // no links: no-op
}

// Project -> write back -> stored Card unchanged (the unchanged-PUT no-op).
func TestProjectionRoundTrip_UnchangedWriteBackLeavesStoredCardUntouched(t *testing.T) {
	f := newProjFixture(t)
	stored := f.contactWithCard(t, contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "own", Name: "Own Co"}}})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	f.link(t, f.place(t, "normal"), true, "normal", nil)
	before, err := json.Marshal(stored.Card)
	require.NoError(t, err)

	for _, stripIDs := range []bool{false, true} {
		var c Contact
		require.NoError(t, f.db.First(&c, f.contact.ID).Error)
		rec := RecordForContactForSync(&c, "", f.db)
		require.Len(t, rec.Card.Organizations, 2)
		if stripIDs {
			for i := range rec.Card.Organizations {
				if IsEntityProjectionID(rec.Card.Organizations[i].ID) {
					rec.Card.Organizations[i].ID = ""
				}
			}
			for i := range rec.Card.Addresses {
				rec.Card.Addresses[i].ID, rec.Card.Addresses[i].Pref = "", nil
			}
		}
		require.NoError(t, SuppressEntityProjectionEcho(f.db, &c, rec))
		ApplyRecordToContact(&c, rec, "")
		require.NoError(t, f.db.Save(&c).Error)

		var after Contact
		require.NoError(t, f.db.First(&after, f.contact.ID).Error)
		got, err := json.Marshal(after.Card)
		require.NoError(t, err)
		assert.JSONEq(t, string(before), string(got), "stripIDs=%v: stored Card must be unchanged", stripIDs)
	}
}

func TestIsEntityProjectionID(t *testing.T) {
	assert.True(t, IsEntityProjectionID("ent-abc"))
	assert.False(t, IsEntityProjectionID("org1"))
	assert.False(t, IsEntityProjectionID(""))
}

func TestMaxSensitivity(t *testing.T) {
	assert.Equal(t, "normal", maxSensitivity("", ""))
	assert.Equal(t, "private", maxSensitivity("normal", "private"))
	assert.Equal(t, "secret", maxSensitivity("secret", "private"))
}

func TestProjectionLookupFailureDegradesToNoProjection(t *testing.T) {
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	dbtest.HideTable(t, f.db, "contact_entity_links")
	assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Organizations)
}

func setPeriod(t *testing.T, f *projFixture, entityID string, p *contactmodel.TemporalRange) {
	t.Helper()
	var l ContactEntityLink
	require.NoError(t, f.db.First(&l, "entity_id = ?", entityID).Error)
	l.Period = p
	require.NoError(t, f.db.Save(&l).Error)
}

// Internal read-edit-apply paths (bulk field update, wedding sync, ...) build a
// Record with RecordForContact and apply it straight back, with no echo helper:
// the ent- ID drop inside ApplyRecordToContact alone must keep the projection
// out of the stored Card.
func TestApplyRecordToContact_InternalReadEditApplyNeverStoresProjection(t *testing.T) {
	f := newProjFixture(t)
	f.contactWithCard(t, contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "own", Name: "Own Co"}}})
	f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
	f.link(t, f.place(t, "normal"), true, "normal", nil)

	var c Contact
	require.NoError(t, f.db.First(&c, f.contact.ID).Error)
	rec := RecordForContact(&c, "", f.db)
	require.Len(t, rec.Card.Organizations, 2)
	ApplyRecordToContact(&c, rec, "")
	require.NoError(t, f.db.Save(&c).Error)

	var after Contact
	require.NoError(t, f.db.First(&after, f.contact.ID).Error)
	require.Len(t, after.Card.Organizations, 1)
	assert.Equal(t, "own", after.Card.Organizations[0].ID)
	assert.Empty(t, after.Card.Addresses)
}

func TestSuppressEntityProjectionEcho_LookupFailureFailsClosed(t *testing.T) {
	f := newProjFixture(t)
	stored := f.contactWithCard(t, contactmodel.Card{})
	dbtest.HideTable(t, f.db, "contact_entity_links")
	err := SuppressEntityProjectionEcho(f.db, stored, &contactmodel.Record{})
	assert.Error(t, err, "the write path must not fail open and store an echo")
}

func TestDetachProjectedEntityIDs(t *testing.T) {
	assert.Nil(t, DetachProjectedEntityIDs(nil))
	plain := &contactmodel.Record{Card: contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "own", Name: "A"}}}}
	assert.Same(t, plain, DetachProjectedEntityIDs(plain))
	r := &contactmodel.Record{Card: contactmodel.Card{
		Organizations: []contactmodel.Organization{{ID: "own", Name: "A"}, {ID: "ent-1", Name: "B"}},
		Addresses:     []contactmodel.Address{{ID: "ent-2"}, {ID: "x"}},
	}}
	got := DetachProjectedEntityIDs(r)
	assert.Equal(t, []string{"own", ""}, []string{got.Card.Organizations[0].ID, got.Card.Organizations[1].ID})
	assert.Equal(t, []string{"", "x"}, []string{got.Card.Addresses[0].ID, got.Card.Addresses[1].ID})
	assert.Equal(t, "ent-1", r.Card.Organizations[1].ID, "input is not mutated")
}

func TestInstallEntityProjectionClock_UsesOwnerZoneDay(t *testing.T) {
	// 2026-06-01 23:30 UTC is already 2026-06-02 in Auckland.
	loc, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)
	restore := InstallEntityProjectionClock(func() time.Time { return time.Date(2026, 6, 1, 23, 30, 0, 0, time.UTC) }, loc)
	got := projectionToday()
	assert.Equal(t, 2, *got.Day, "the owner's calendar day, not UTC's")
	restore()
	// Restored: the (system, UTC) default is back and returns a full date.
	d := projectionToday()
	assert.NotNil(t, d.Year)
}

func TestLinkIsCurrent_EdgeCases(t *testing.T) {
	y, m, d := 2026, 6, 1
	today := &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}
	assert.True(t, linkIsCurrent(nil, today))
	// Endpoint at the same day is current on both ends.
	assert.True(t, linkIsCurrent(&contactmodel.TemporalRange{Start: today, End: today}, today))
	// Year-only endpoints compare at shared precision: end 2026 includes June 2026.
	assert.True(t, linkIsCurrent(&contactmodel.TemporalRange{End: &contactmodel.PartialDate{Year: &y}}, today))
}

func TestProjectPrimaryEntities_EdgeBranches(t *testing.T) {
	t.Run("blank organisation name falls back to the entity name", func(t *testing.T) {
		f := newProjFixture(t)
		f.contactWithCard(t, contactmodel.Card{})
		e := Entity{UserID: f.user.ID, Kind: EntityKindOrganisation, Name: "Fallback Inc"}
		require.NoError(t, f.db.Create(&e).Error) // core "{}": organization name empty
		f.link(t, e, true, "normal", nil)
		rec := f.record(t, FieldSelectionAll())
		require.Len(t, rec.Card.Organizations, 1)
		assert.Equal(t, "Fallback Inc", rec.Card.Organizations[0].Name)
	})
	t.Run("a place with no address never projects", func(t *testing.T) {
		f := newProjFixture(t)
		f.contactWithCard(t, contactmodel.Card{})
		e := Entity{UserID: f.user.ID, Kind: EntityKindPlace, Name: "Nowhere"}
		require.NoError(t, f.db.Create(&e).Error)
		f.link(t, e, true, "normal", nil)
		assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Addresses)
	})
	t.Run("value-equal own address drops the projected one; pref caps at 100", func(t *testing.T) {
		f := newProjFixture(t)
		pl := f.place(t, "normal")
		core, err := pl.DecodeCore()
		require.NoError(t, err)
		same := core.(*PlaceCore).Address
		same.ID = "own"
		f.contactWithCard(t, contactmodel.Card{Addresses: []contactmodel.Address{same}})
		f.link(t, pl, true, "normal", nil)
		assert.Len(t, f.record(t, FieldSelectionAll()).Card.Addresses, 1, "own value-equal address wins")

		// Cap: an own address already at pref 100.
		f2 := newProjFixture(t)
		hundred := 100
		f2.contactWithCard(t, contactmodel.Card{Addresses: []contactmodel.Address{{ID: "own", Pref: &hundred,
			Components: []contactmodel.AddressComponent{{Kind: "name", Value: "Elsewhere"}}}}})
		f2.link(t, f2.place(t, "normal"), true, "normal", nil)
		addrs := f2.record(t, FieldSelectionAll()).Card.Addresses
		require.Len(t, addrs, 2)
		assert.Equal(t, 100, *addrs[1].Pref)
	})
	t.Run("entity lookup failure degrades to no projection and fails the write helper", func(t *testing.T) {
		f := newProjFixture(t)
		stored := f.contactWithCard(t, contactmodel.Card{})
		f.link(t, f.org(t, "Acme", "normal"), true, "normal", nil)
		dbtest.HideTable(t, f.db, "entities")
		assert.Empty(t, f.record(t, FieldSelectionAll()).Card.Organizations)
		assert.Error(t, SuppressEntityProjectionEcho(f.db, stored, &contactmodel.Record{}))
	})
	t.Run("ID-stripped echo of a place is dropped", func(t *testing.T) {
		f := newProjFixture(t)
		stored := f.contactWithCard(t, contactmodel.Card{Addresses: []contactmodel.Address{{ID: "own",
			Components: []contactmodel.AddressComponent{{Kind: "name", Value: "9 Elm"}}}}})
		f.link(t, f.place(t, "normal"), true, "normal", nil)
		echo := f.record(t, FieldSelectionAll())
		for i := range echo.Card.Addresses {
			echo.Card.Addresses[i].ID, echo.Card.Addresses[i].Pref = "", nil
		}
		require.NoError(t, SuppressEntityProjectionEcho(f.db, stored, echo))
		assert.Len(t, echo.Card.Addresses, 1)
	})
	t.Run("strip handles address-only projection and periods", func(t *testing.T) {
		r := &contactmodel.Record{Card: contactmodel.Card{Addresses: []contactmodel.Address{{ID: "ent-9"}, {ID: "own"}}}}
		out := StripProjectedEntityIDs(r)
		require.Len(t, out.Card.Addresses, 1)
		assert.Equal(t, "own", out.Card.Addresses[0].ID)
		assert.Nil(t, StripProjectedEntityIDs(nil))
	})
}
