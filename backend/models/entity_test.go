package models

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func entityTestUser(t *testing.T, db *gorm.DB) User {
	t.Helper()
	u := User{Username: "ent", Password: "x", Email: "ent@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func TestEntityCreate_NormalizesNameDerivesKeyAndStampsRevision(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)

	// "é" as e + combining acute (NFD) must be stored NFC (ADR 0016).
	e := Entity{UserID: u.ID, Kind: EntityKindOrganisation, Name: "  Café   Corp.  "}
	require.NoError(t, db.Create(&e).Error)

	assert.NotEmpty(t, e.ID)
	var got Entity
	require.NoError(t, db.First(&got, "id = ?", e.ID).Error)
	assert.Equal(t, "  Café   Corp.  ", got.Name, "name is NFC-normalized, otherwise stored as typed")
	assert.Equal(t, "café corp", got.NameKey, "key is lower-cased, whitespace-collapsed, punctuation-trimmed")
	assert.Equal(t, "normal", got.Sensitivity, "sensitivity defaults to normal")
	assert.EqualValues(t, 1, got.Revision)
	assert.Equal(t, "e-"+e.ID+"-1", got.ETag)
	assert.Equal(t, "{}", got.Core)

	got.Name = "Renamed Co"
	require.NoError(t, db.Save(&got).Error)
	assert.EqualValues(t, 2, got.Revision, "an update bumps the revision (ADR 0006)")
	assert.Equal(t, "renamed co", got.NameKey, "name_key is re-derived on every save, never client-supplied")
}

func TestEntityNameKey(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"Acme":                    "acme",
		"  ACME,  Inc. ":          "acme, inc",
		"Mum's  house":            "mum's house",
		"\u00c9cole":              "\u00e9cole",
		"E\u0301cole":             "\u00e9cole",
		"---":                     "",
		"Springfield  Elementary": "springfield elementary",
	} {
		assert.Equal(t, want, EntityNameKey(in), "input %q", in)
	}
}

func TestEntityCreate_RejectsBadKindNameAndCore(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)

	assert.Error(t, db.Create(&Entity{UserID: u.ID, Kind: "company", Name: "x"}).Error, "kind is a closed enum")
	assert.Error(t, db.Create(&Entity{UserID: u.ID, Kind: EntityKindPet, Name: ""}).Error, "name is required")
	long := make([]rune, EntityNameMaxLength+1)
	for i := range long {
		long[i] = 'a'
	}
	assert.Error(t, db.Create(&Entity{UserID: u.ID, Kind: EntityKindPet, Name: string(long)}).Error, "name is at most 200 characters")
	assert.NoError(t, db.Create(&Entity{UserID: u.ID, Kind: EntityKindPet, Name: string(long[:EntityNameMaxLength])}).Error)
	assert.Error(t, db.Create(&Entity{UserID: u.ID, Kind: EntityKindPet, Name: "Rex", Sensitivity: "top-secret"}).Error, "sensitivity CHECK")
	assert.Error(t, db.Create(&Entity{UserID: u.ID, Kind: EntityKindPet, Name: "Rex", Core: `{"favourite_toy":"ball"}`}).Error, "unknown core key: no free-form core")
}

func TestEntityKindIsImmutable(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	e := Entity{UserID: u.ID, Kind: EntityKindPlace, Name: "Cabin"}
	require.NoError(t, db.Create(&e).Error)

	err := db.Exec("UPDATE entities SET kind = 'pet' WHERE id = ?", e.ID).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entities.kind is immutable", "the database trigger refuses a kind change")

	e.Kind = EntityKindPet
	require.NoError(t, db.Save(&e).Error, "the model never writes kind after create (<-:create)")
	var got Entity
	require.NoError(t, db.First(&got, "id = ?", e.ID).Error)
	assert.Equal(t, EntityKindPlace, got.Kind)
}

func TestEntityCore_TypedPerKind(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)

	org := Entity{UserID: u.ID, Kind: EntityKindOrganisation, Name: "Acme"}
	require.NoError(t, org.SetCore(&OrganisationCore{
		Organization: contactmodel.Organization{Name: "Acme", Units: []contactmodel.OrgUnit{{Name: "R&D"}}},
		URL:          "https://acme.example", Notes: "n",
	}))
	require.NoError(t, db.Create(&org).Error)

	var got Entity
	require.NoError(t, db.First(&got, "id = ?", org.ID).Error)
	core, err := got.DecodeCore()
	require.NoError(t, err)
	oc, ok := core.(*OrganisationCore)
	require.True(t, ok)
	assert.Equal(t, "R&D", oc.Organization.Units[0].Name)

	pet := Entity{UserID: u.ID, Kind: EntityKindPet, Name: "Rex"}
	assert.Error(t, pet.SetCore(&OrganisationCore{}), "a core of the wrong kind is refused")
	y := 2020
	require.NoError(t, pet.SetCore(&PetCore{Species: "dog", Birth: &contactmodel.PartialDate{Year: &y}}))

	place := Entity{UserID: u.ID, Kind: EntityKindPlace, Name: "Cabin"}
	assert.Error(t, place.SetCore(&PlaceCore{Address: contactmodel.Address{Sensitivity: "private"}}),
		"a place's address sensitivity lives in the entity column, not in core")
	assert.NoError(t, place.SetCore(&PlaceCore{Address: contactmodel.Address{CountryCode: "GB"}, Notes: "lake"}))
	assert.Error(t, place.SetCore(&PlaceCore{Notes: strings.Repeat("x", 6000)}), "validate tags apply")
}

func linkFor(t *testing.T, db *gorm.DB, u User, e Entity, uid, role string, primary bool) (ContactEntityLink, error) {
	t.Helper()
	l := ContactEntityLink{UserID: u.ID, ContactVCardUID: uid, EntityID: e.ID, Kind: e.Kind, Role: role, IsPrimary: primary, Sensitivity: "normal"}
	return l, db.Create(&l).Error
}

func TestContactEntityLink_RoleAndKindChecks(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	org := Entity{UserID: u.ID, Kind: EntityKindOrganisation, Name: "Acme"}
	pet := Entity{UserID: u.ID, Kind: EntityKindPet, Name: "Rex"}
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, db.Create(&pet).Error)

	for _, role := range EntityLinkRoles(EntityKindOrganisation) {
		_, err := linkFor(t, db, u, org, "uid-1", role, false)
		assert.NoError(t, err, "organisation role %s", role)
	}
	_, err := linkFor(t, db, u, org, "uid-1", "carer", false)
	assert.Error(t, err, "a pet role is not an organisation role (model)")
	_, err = linkFor(t, db, u, pet, "uid-1", "employee", false)
	assert.Error(t, err, "an organisation role is not a pet role (model)")

	// The database enforces the same closed vocabulary, bypassing the model.
	err = db.Exec(`INSERT INTO contact_entity_links (id, user_id, contact_vcard_uid, entity_id, kind, role)
		VALUES ('raw-1', ?, 'uid-1', ?, 'pet', 'employee')`, u.ID, pet.ID).Error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CHECK constraint failed")
	err = db.Exec(`INSERT INTO contact_entity_links (id, user_id, contact_vcard_uid, entity_id, kind, role)
		VALUES ('raw-2', ?, 'uid-1', ?, 'pet', 'owner')`, u.ID, org.ID).Error
	require.Error(t, err, "a link's kind must equal its entity's kind (composite FK)")
	err = db.Exec(`INSERT INTO contact_entity_links (id, user_id, contact_vcard_uid, entity_id, kind, role, role_label)
		VALUES ('raw-3', ?, 'uid-1', ?, 'pet', 'owner', ?)`, u.ID, pet.ID, strings.Repeat("x", 101)).Error
	require.Error(t, err, "role_label is at most 100 characters")
}

func TestContactEntityLink_PrimaryIsUniquePerContactAndKindWhileLive(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	a := Entity{UserID: u.ID, Kind: EntityKindOrganisation, Name: "A"}
	b := Entity{UserID: u.ID, Kind: EntityKindOrganisation, Name: "B"}
	home := Entity{UserID: u.ID, Kind: EntityKindPlace, Name: "Home"}
	for _, e := range []*Entity{&a, &b, &home} {
		require.NoError(t, db.Create(e).Error)
	}

	first, err := linkFor(t, db, u, a, "uid-1", "employee", true)
	require.NoError(t, err)
	_, err = linkFor(t, db, u, b, "uid-1", "employee", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "UNIQUE constraint failed", "second live primary of the same kind")

	_, err = linkFor(t, db, u, b, "uid-1", "employee", false)
	assert.NoError(t, err, "non-primary links are unlimited")
	_, err = linkFor(t, db, u, a, "uid-1", "employee", false)
	assert.NoError(t, err, "no natural-key unique index: a re-hire is a second link to the same entity")
	_, err = linkFor(t, db, u, home, "uid-1", "resident", true)
	assert.NoError(t, err, "primary is per kind")
	_, err = linkFor(t, db, u, b, "uid-2", "employee", true)
	assert.NoError(t, err, "primary is per contact")

	// A soft-deleted primary does not occupy the index (the index is partial).
	require.NoError(t, db.Delete(&first).Error)
	_, err = linkFor(t, db, u, b, "uid-1", "owner", true)
	assert.NoError(t, err, "a tombstoned primary frees the slot")
}

func TestContactEntityLink_EmptyPeriodStoresNullAndPeriodRoundTrips(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	e := Entity{UserID: u.ID, Kind: EntityKindOrganisation, Name: "A"}
	require.NoError(t, db.Create(&e).Error)

	l := ContactEntityLink{UserID: u.ID, ContactVCardUID: "uid-1", EntityID: e.ID, Kind: e.Kind, Role: "employee", Sensitivity: "normal", Period: &contactmodel.TemporalRange{}}
	require.NoError(t, db.Create(&l).Error)
	var isNull bool
	require.NoError(t, db.Raw("SELECT period IS NULL FROM contact_entity_links WHERE id = ?", l.ID).Scan(&isNull).Error)
	assert.True(t, isNull, "an all-empty range stores NULL")

	y := 2019
	l2 := ContactEntityLink{UserID: u.ID, ContactVCardUID: "uid-1", EntityID: e.ID, Kind: e.Kind, Role: "employee", Sensitivity: "normal",
		Period: &contactmodel.TemporalRange{Start: &contactmodel.PartialDate{Year: &y}}}
	require.NoError(t, db.Create(&l2).Error)
	var got ContactEntityLink
	require.NoError(t, db.First(&got, "id = ?", l2.ID).Error)
	require.NotNil(t, got.Period)
	assert.Equal(t, 2019, *got.Period.Start.Year)
	assert.Nil(t, got.Period.End)
	assert.False(t, got.IsPrimary, "an explicit false is written (issue #1240)")
	assert.EqualValues(t, 1, got.Revision)
}

func TestFieldDefinitionKeyIsUniquePerTarget(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	mk := func(target string) error {
		return db.Create(&FieldDefinition{UserID: u.ID, Label: "Notes", Key: "notes", Target: target, Type: FieldTypeString,
			Projection: "internal-only", Sensitivity: RelationshipSensitivityNormal}).Error
	}
	require.NoError(t, mk(FieldDefinitionTargetContact))
	assert.NoError(t, mk("organisation"), `"notes" may exist on both a contact and an organisation`)
	assert.Error(t, mk("organisation"), "but not twice on the same target")
}

func TestEntityKindsAndCoreValidationEdges(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"organisation", "place", "pet"}, EntityKinds())

	assert.Error(t, ValidateEntityCore("company", "{}"), "unknown kind")
	assert.NoError(t, ValidateEntityCore(EntityKindPet, ""), "an empty core is the empty struct")
	assert.Error(t, ValidateEntityCore(EntityKindPet, "{not json"), "malformed JSON")

	e := Entity{Kind: "company"}
	_, err := e.DecodeCore()
	assert.Error(t, err, "unknown kind has no core type")
	e = Entity{Kind: EntityKindPet}
	core, err := e.DecodeCore()
	require.NoError(t, err, "an empty stored core decodes to the empty struct")
	assert.IsType(t, &PetCore{}, core)
	e.Core = "{bad"
	_, err = e.DecodeCore()
	assert.Error(t, err)
	assert.Error(t, (&Entity{Kind: "company"}).SetCore(&PetCore{}), "SetCore needs a known kind")
}

func TestEntityHooks_BulkUpdateAndRevisionConflict(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	e := Entity{UserID: u.ID, Kind: EntityKindOrganisation, Name: "Acme"}
	require.NoError(t, db.Create(&e).Error)

	// A bulk update runs the hooks on a zero-value receiver: nothing is derived
	// and no revision is written for a row-less receiver.
	require.NoError(t, db.Model(&Entity{}).Where("id = ?", e.ID).Update("sensitivity", "private").Error)
	var got Entity
	require.NoError(t, db.First(&got, "id = ?", e.ID).Error)
	assert.Equal(t, "private", got.Sensitivity)
	assert.Equal(t, "acme", got.NameKey, "a bulk update does not clobber name_key")

	// A save from a stale revision loses (CON-01 compare-and-swap).
	stale := got
	got.Name = "Acme 2"
	require.NoError(t, db.Save(&got).Error)
	stale.Name = "Acme 3"
	err := db.Save(&stale).Error
	require.Error(t, err)
	var conflict *ErrRevisionConflict
	assert.ErrorAs(t, err, &conflict)
}

func TestContactEntityLinkHooks_UpdateBumpsRevisionAndBulkUpdateIsNoop(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	e := Entity{UserID: u.ID, Kind: EntityKindPlace, Name: "Cabin"}
	require.NoError(t, db.Create(&e).Error)
	l, err := linkFor(t, db, u, e, "uid-1", "resident", false)
	require.NoError(t, err)

	l.RoleLabel = "weekends"
	require.NoError(t, db.Save(&l).Error)
	assert.EqualValues(t, 2, l.Revision)
	assert.Equal(t, "e-"+l.ID+"-2", l.ETag)

	l.Role = "pilot"
	assert.Error(t, db.Save(&l).Error, "an update to a role outside the closed vocabulary is refused")

	require.NoError(t, db.Model(&ContactEntityLink{}).Where("id = ?", l.ID).Update("sensitivity", "private").Error)

	stale := l
	l.RoleLabel = "again"
	l.Role = "visitor"
	require.NoError(t, db.Save(&l).Error)
	stale.Role = "owner"
	var conflict *ErrRevisionConflict
	assert.ErrorAs(t, db.Save(&stale).Error, &conflict)

	assert.Equal(t, []string{"resident", "owner", "visitor", "other"}, EntityLinkRoles(EntityKindPlace))
	assert.Empty(t, EntityLinkRoles("company"))
	assert.False(t, ValidEntityLinkRole("company", "owner"))
}

func TestEntity_SaveWithUnknownKindIsLeftToTheDatabaseAndSetCoreMarshals(t *testing.T) {
	t.Parallel()
	// A zero-kind receiver with an ID (a row-less partial update shape) derives
	// nothing and validates nothing; the CHECK constraint is the backstop.
	db := dbtest.New(t)
	u := entityTestUser(t, db)
	e := Entity{ID: "11111111-1111-4111-8111-111111111111", UserID: u.ID, Name: "No kind"}
	assert.Error(t, db.Create(&e).Error)

	pet := Entity{Kind: EntityKindPet, Name: "Rex"}
	require.NoError(t, pet.SetCore(&PetCore{Species: "dog"}))
	assert.JSONEq(t, `{"species":"dog"}`, pet.Core)
}
