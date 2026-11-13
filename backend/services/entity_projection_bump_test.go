package services

// Issue #2565 (ADR 0043 amendment, maintainer decision 2026-11-13): a write
// that changes what ProjectPrimaryEntities would emit for a contact bumps that
// contact's revision / ETag / updated_at, in the same transaction, so CardDAV
// and the ?since= feed pick the projected ORG/ADR up. Contacts whose projection
// does not change are untouched.

import (
	"fmt"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type contactStamp struct {
	Revision  int64
	ETag      string `gorm:"column:etag"`
	UpdatedAt time.Time
}

func stampOf(t *testing.T, db *gorm.DB, c models.Contact) contactStamp {
	t.Helper()
	var s contactStamp
	require.NoError(t, db.Unscoped().Table("contacts").Select("revision", "etag", "updated_at").Where("id = ?", c.ID).Scan(&s).Error)
	return s
}

func assertBumped(t *testing.T, db *gorm.DB, c models.Contact, before contactStamp, msg string) {
	t.Helper()
	got := stampOf(t, db, c)
	assert.Equal(t, before.Revision+1, got.Revision, "%s: revision advances exactly once", msg)
	assert.Equal(t, fmt.Sprintf("e-%d-%d", c.ID, got.Revision), got.ETag, "%s: ETag is derived from the revision (ADR 0006)", msg)
	assert.True(t, got.UpdatedAt.After(before.UpdatedAt), "%s: updated_at advances so the since-feed lists the contact", msg)
}

func assertUntouched(t *testing.T, db *gorm.DB, c models.Contact, before contactStamp, msg string) {
	t.Helper()
	assert.Equal(t, before, stampOf(t, db, c), "%s: contact must not be touched", msg)
}

type projFixture struct {
	entityFixture
	c3 models.Contact // never linked
}

func newProjFixture(t *testing.T) projFixture {
	t.Helper()
	f := projFixture{entityFixture: newEntityFixture(t)}
	f.c3 = models.Contact{UserID: f.userID, Firstname: "Three"}
	require.NoError(t, f.db.Create(&f.c3).Error)
	// Spread updated_at into the past so "advances" is unambiguous.
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, f.db.Table("contacts").Where("user_id = ?", f.userID).UpdateColumn("updated_at", old).Error)
	return f
}

func (f projFixture) newLink(t *testing.T, c models.Contact, e models.Entity, primary bool) models.ContactEntityLink {
	t.Helper()
	role := map[string]string{models.EntityKindOrganisation: "employee", models.EntityKindPlace: "resident", models.EntityKindPet: "owner"}[e.Kind]
	l := models.ContactEntityLink{UserID: f.userID, ContactVCardUID: c.VCardUID, EntityID: e.ID, Kind: e.Kind, Role: role, IsPrimary: primary, Sensitivity: "normal"}
	require.NoError(t, CreateContactEntityLink(f.db, &l))
	return l
}

func (f projFixture) place(t *testing.T, name string) models.Entity {
	t.Helper()
	e := models.Entity{UserID: f.userID, Kind: models.EntityKindPlace, Name: name}
	require.NoError(t, e.SetCore(&models.PlaceCore{Address: contactmodel.Address{Components: []contactmodel.AddressComponent{{Kind: "locality", Value: name}}}}))
	require.NoError(t, f.db.Create(&e).Error)
	return e
}

func (f projFixture) pet(t *testing.T) models.Entity {
	t.Helper()
	e := models.Entity{UserID: f.userID, Kind: models.EntityKindPet, Name: "Rex"}
	require.NoError(t, f.db.Create(&e).Error)
	return e
}

// reload returns the persisted row (a service that saves a stale struct would
// otherwise be tested against its own in-memory copy).
func (f projFixture) reloadLink(t *testing.T, id string) models.ContactEntityLink {
	t.Helper()
	var l models.ContactEntityLink
	require.NoError(t, f.db.First(&l, "id = ?", id).Error)
	return l
}

func (f projFixture) reloadEntity(t *testing.T, id string) models.Entity {
	t.Helper()
	var e models.Entity
	require.NoError(t, f.db.First(&e, "id = ?", id).Error)
	return e
}

func TestProjectionBump_LinkCreate(t *testing.T) {
	f := newProjFixture(t)
	pet := f.pet(t)
	s1, s2, s3 := stampOf(t, f.db, f.c1), stampOf(t, f.db, f.c2), stampOf(t, f.db, f.c3)

	f.newLink(t, f.c2, f.org, false)
	assertUntouched(t, f.db, f.c2, s2, "non-primary organisation link")
	f.newLink(t, f.c2, f.pet(t), true)
	assertUntouched(t, f.db, f.c2, s2, "a primary pet link never projects")
	_ = pet

	f.newLink(t, f.c1, f.org, true)
	assertBumped(t, f.db, f.c1, s1, "primary organisation link")
	assertUntouched(t, f.db, f.c3, s3, "unrelated contact")
}

func TestProjectionBump_LinkCreate_PrimaryPlaceAndPastLink(t *testing.T) {
	f := newProjFixture(t)
	s1, s2 := stampOf(t, f.db, f.c1), stampOf(t, f.db, f.c2)

	f.newLink(t, f.c1, f.place(t, "Springfield"), true)
	assertBumped(t, f.db, f.c1, s1, "primary place link")

	// A primary link that ended in the past is not current and does not project.
	y, m, d := 2001, 1, 1
	past := models.ContactEntityLink{UserID: f.userID, ContactVCardUID: f.c2.VCardUID, EntityID: f.org.ID, Kind: f.org.Kind, Role: "employee", IsPrimary: true, Sensitivity: "normal",
		Period: &contactmodel.TemporalRange{End: &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}}}
	require.NoError(t, CreateContactEntityLink(f.db, &past))
	assertUntouched(t, f.db, f.c2, s2, "a primary link whose period has ended")
}

func TestProjectionBump_LinkPrimaryFlipDemotesAndBumpsOnce(t *testing.T) {
	f := newProjFixture(t)
	a := f.newLink(t, f.c1, f.org, true)
	b := f.newLink(t, f.c1, f.other, false)
	s1 := stampOf(t, f.db, f.c1)

	b = f.reloadLink(t, b.ID)
	b.IsPrimary = true
	require.NoError(t, SaveContactEntityLink(f.db, &b))
	assertBumped(t, f.db, f.c1, s1, "promoting a second primary (demotes the first)")
	assert.False(t, f.reloadLink(t, a.ID).IsPrimary)

	s1 = stampOf(t, f.db, f.c1)
	b = f.reloadLink(t, b.ID)
	b.IsPrimary = false
	require.NoError(t, SaveContactEntityLink(f.db, &b))
	assertBumped(t, f.db, f.c1, s1, "clearing the primary flag")
}

func TestProjectionBump_LinkUpdate(t *testing.T) {
	f := newProjFixture(t)
	l := f.newLink(t, f.c1, f.org, true)
	save := func(mut func(l *models.ContactEntityLink)) {
		t.Helper()
		got := f.reloadLink(t, l.ID)
		mut(&got)
		require.NoError(t, SaveContactEntityLink(f.db, &got))
	}

	s := stampOf(t, f.db, f.c1)
	save(func(l *models.ContactEntityLink) { l.Role, l.RoleLabel = "owner", "Founder" })
	assertUntouched(t, f.db, f.c1, s, "role and role label are CRM-only, not projected")

	save(func(l *models.ContactEntityLink) { l.Sensitivity = "private" })
	assertBumped(t, f.db, f.c1, s, "normal -> private withholds the entry from the sync view")

	s = stampOf(t, f.db, f.c1)
	save(func(l *models.ContactEntityLink) { l.Sensitivity = "normal" })
	assertBumped(t, f.db, f.c1, s, "private -> normal exposes it again")

	s = stampOf(t, f.db, f.c1)
	y, m, d := 2001, 1, 1
	save(func(l *models.ContactEntityLink) {
		l.Period = &contactmodel.TemporalRange{End: &contactmodel.PartialDate{Year: &y, Month: &m, Day: &d}}
	})
	assertBumped(t, f.db, f.c1, s, "ending the link stops the projection")
}

func TestProjectionBump_LinkDelete(t *testing.T) {
	f := newProjFixture(t)
	primary := f.newLink(t, f.c1, f.org, true)
	plain := f.newLink(t, f.c2, f.org, false)
	s1, s2 := stampOf(t, f.db, f.c1), stampOf(t, f.db, f.c2)

	require.NoError(t, DeleteContactEntityLink(f.db, &plain))
	assertUntouched(t, f.db, f.c2, s2, "deleting a non-primary link")
	require.NoError(t, DeleteContactEntityLink(f.db, &primary))
	assertBumped(t, f.db, f.c1, s1, "deleting the primary link")
}

func TestProjectionBump_EntityRename(t *testing.T) {
	f := newProjFixture(t)
	f.newLink(t, f.c1, f.org, true)
	f.newLink(t, f.c2, f.org, false) // linked, not primary
	f.newLink(t, f.c3, f.other, true)
	s1, s2, s3 := stampOf(t, f.db, f.c1), stampOf(t, f.db, f.c2), stampOf(t, f.db, f.c3)

	e := f.reloadEntity(t, f.org.ID)
	e.Name = "Acme Holdings"
	require.NoError(t, SaveEntity(f.db, &e))

	assertBumped(t, f.db, f.c1, s1, "rename of the projected organisation")
	assertUntouched(t, f.db, f.c2, s2, "non-primary link")
	assertUntouched(t, f.db, f.c3, s3, "other entity")
}

func TestProjectionBump_EntityCoreAndSensitivity(t *testing.T) {
	f := newProjFixture(t)
	f.newLink(t, f.c1, f.org, true)
	s := stampOf(t, f.db, f.c1)

	// Notes / URL are CRM-only: never projected.
	e := f.reloadEntity(t, f.org.ID)
	require.NoError(t, e.SetCore(&models.OrganisationCore{Organization: contactmodel.Organization{}, Notes: "internal only", URL: "https://x.example"}))
	require.NoError(t, SaveEntity(f.db, &e))
	assertUntouched(t, f.db, f.c1, s, "notes/url edits and a save that projects the same ORG")

	e = f.reloadEntity(t, f.org.ID)
	require.NoError(t, e.SetCore(&models.OrganisationCore{Organization: contactmodel.Organization{Name: "Acme", Units: []contactmodel.OrgUnit{{Name: "R&D"}}}}))
	require.NoError(t, SaveEntity(f.db, &e))
	assertBumped(t, f.db, f.c1, s, "projected ORG units change")

	s = stampOf(t, f.db, f.c1)
	e = f.reloadEntity(t, f.org.ID)
	e.Sensitivity = "private"
	require.NoError(t, SaveEntity(f.db, &e))
	assertBumped(t, f.db, f.c1, s, "entity goes private")
}

func TestProjectionBump_PlaceAddressChange(t *testing.T) {
	f := newProjFixture(t)
	pl := f.place(t, "Springfield")
	f.newLink(t, f.c1, pl, true)
	s := stampOf(t, f.db, f.c1)
	e := f.reloadEntity(t, pl.ID)
	require.NoError(t, e.SetCore(&models.PlaceCore{Address: contactmodel.Address{Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "Shelbyville"}}}}))
	require.NoError(t, SaveEntity(f.db, &e))
	assertBumped(t, f.db, f.c1, s, "projected address change")
}

func TestProjectionBump_PetNeverBumps(t *testing.T) {
	f := newProjFixture(t)
	pet := f.pet(t)
	f.newLink(t, f.c1, pet, true)
	s := stampOf(t, f.db, f.c1)
	e := f.reloadEntity(t, pet.ID)
	e.Name = "Rexy"
	require.NoError(t, SaveEntity(f.db, &e))
	assertUntouched(t, f.db, f.c1, s, "pet rename")
	require.NoError(t, DeleteEntity(f.db, f.userID, pet.ID, entityNow))
	assertUntouched(t, f.db, f.c1, s, "pet delete")
	_, err := RestoreEntity(f.db, f.userID, pet.ID, entityNow)
	require.NoError(t, err)
	assertUntouched(t, f.db, f.c1, s, "pet restore")
}

func TestProjectionBump_EntityDeleteAndRestore(t *testing.T) {
	f := newProjFixture(t)
	f.newLink(t, f.c1, f.org, true)
	f.newLink(t, f.c2, f.org, false)
	s1, s2, s3 := stampOf(t, f.db, f.c1), stampOf(t, f.db, f.c2), stampOf(t, f.db, f.c3)

	require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))
	assertBumped(t, f.db, f.c1, s1, "entity delete drops the projected ORG")
	assertUntouched(t, f.db, f.c2, s2, "non-primary link")
	assertUntouched(t, f.db, f.c3, s3, "unrelated contact")

	s1 = stampOf(t, f.db, f.c1)
	_, err := RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
	require.NoError(t, err)
	assertBumped(t, f.db, f.c1, s1, "entity restore brings the projected ORG back")
	assertUntouched(t, f.db, f.c2, s2, "non-primary link after restore")
}

func TestProjectionBump_Merge(t *testing.T) {
	f := newProjFixture(t)
	f.newLink(t, f.c1, f.org, true) // loser, primary: its ORG becomes the survivor's
	f.newLink(t, f.c2, f.other, true)
	f.newLink(t, f.c3, f.org, false)
	s1, s2, s3 := stampOf(t, f.db, f.c1), stampOf(t, f.db, f.c2), stampOf(t, f.db, f.c3)

	_, err := MergeEntities(f.db, f.userID, f.org.ID, f.other.ID, nil, entityNow)
	require.NoError(t, err)
	assertBumped(t, f.db, f.c1, s1, "merge repoints the primary link to the survivor's different value")
	assertUntouched(t, f.db, f.c2, s2, "survivor's own primary: value unchanged")
	assertUntouched(t, f.db, f.c3, s3, "non-primary link repointed")
}

func TestProjectionBump_SoftDeletedContactIsNeverTouched(t *testing.T) {
	f := newProjFixture(t)
	f.newLink(t, f.c1, f.org, true)
	require.NoError(t, f.db.Delete(&f.c1).Error)
	s := stampOf(t, f.db, f.c1)

	e := f.reloadEntity(t, f.org.ID)
	e.Name = "Renamed"
	require.NoError(t, SaveEntity(f.db, &e))
	assert.Equal(t, s, stampOf(t, f.db, f.c1), "a soft-deleted contact keeps its tombstone stamp")
	var n int64
	require.NoError(t, f.db.Model(&models.Contact{}).Where("id = ?", f.c1.ID).Count(&n).Error)
	assert.Zero(t, n, "and stays deleted")
}

func TestProjectionBump_OwnerScoped(t *testing.T) {
	f := newProjFixture(t)
	other := models.User{Username: "u2", Email: "u2@example.com", Password: "x"}
	require.NoError(t, f.db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Foreign", VCardUID: f.c1.VCardUID + "-x"}
	require.NoError(t, f.db.Create(&foreign).Error)
	f.newLink(t, f.c1, f.org, true)
	sf := stampOf(t, f.db, foreign)
	e := f.reloadEntity(t, f.org.ID)
	e.Name = "Renamed"
	require.NoError(t, SaveEntity(f.db, &e))
	assertUntouched(t, f.db, foreign, sf, "another user's contact")
}

// A rename of an entity shared by many contacts must not issue per-contact
// statements: the work is chunked, so the statement count is bounded by
// ceil(N/chunk), not N.
func TestProjectionBump_FanOutIsBounded(t *testing.T) {
	f := newProjFixture(t)
	const n = 1200
	contacts := make([]models.Contact, n)
	for i := range contacts {
		contacts[i] = models.Contact{UserID: f.userID, Firstname: fmt.Sprintf("Bulk%d", i)}
	}
	require.NoError(t, f.db.CreateInBatches(&contacts, 200).Error)
	require.NoError(t, f.db.Table("contacts").Where("firstname LIKE 'Bulk%'").UpdateColumn("updated_at", time.Now().Add(-48*time.Hour)).Error)
	links := make([]models.ContactEntityLink, n)
	for i := range links {
		links[i] = models.ContactEntityLink{UserID: f.userID, ContactVCardUID: contacts[i].VCardUID, EntityID: f.org.ID, Kind: f.org.Kind, Role: "employee", IsPrimary: true, Sensitivity: "normal"}
	}
	require.NoError(t, f.db.CreateInBatches(&links, 200).Error)
	first, last := stampOf(t, f.db, contacts[0]), stampOf(t, f.db, contacts[n-1])

	var stmts int
	count := func(*gorm.DB) { stmts++ }
	require.NoError(t, f.db.Callback().Query().Register("test:count_q", count))
	require.NoError(t, f.db.Callback().Update().Register("test:count_u", count))
	require.NoError(t, f.db.Callback().Raw().Register("test:count_r", count))
	require.NoError(t, f.db.Callback().Row().Register("test:count_w", count))

	e := f.reloadEntity(t, f.org.ID)
	e.Name = "Acme Renamed"
	stmts = 0
	require.NoError(t, SaveEntity(f.db, &e))

	assertBumped(t, f.db, contacts[0], first, "first of the fan-out")
	assertBumped(t, f.db, contacts[n-1], last, "last of the fan-out")
	assert.Less(t, stmts, 40, "statements for %d contacts must be bounded by the chunk count, not by the contact count", n)
}

// A primary link whose entity only repeats an ORG the contact already stores
// emits the same card (the precedence rule drops the duplicate), so it bumps
// nothing; a link to a different organisation does.
func TestProjectionBump_ValueEqualToOwnEntryChangesNoCard(t *testing.T) {
	f := newProjFixture(t)
	own := models.Contact{UserID: f.userID, Firstname: "Own"}
	models.ApplyRecordToContact(&own, &contactmodel.Record{Card: contactmodel.Card{
		Organizations: []contactmodel.Organization{{ID: "o1", Name: "Acme"}}}}, "")
	require.NoError(t, f.db.Create(&own).Error)
	require.NoError(t, f.db.Table("contacts").Where("id = ?", own.ID).UpdateColumn("updated_at", time.Now().Add(-48*time.Hour)).Error)
	s := stampOf(t, f.db, own)

	f.newLink(t, own, f.org, true) // entity "Acme" == own value
	assertUntouched(t, f.db, own, s, "entity value-equal to the contact's own ORG")
	l := f.newLink(t, own, f.other, false)
	l.IsPrimary = true // "Beta" differs from the own value and now projects
	require.NoError(t, SaveContactEntityLink(f.db, &l))
	assertBumped(t, f.db, own, s, "a different organisation projects")
}
