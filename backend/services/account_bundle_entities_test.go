package services

// Account bundle: reusable entities, links and entity field values (issue
// #2300, ADR 0043 decision 7). Real migrated schema; two databases model the
// source and destination instance as in TestAccountBundle_RoundTrip.

import (
	"encoding/json"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbfault"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func pdate(y, m int) *contactmodel.PartialDate {
	return &contactmodel.PartialDate{Year: &y, Month: &m}
}

// seedBundleEntities adds one entity of each kind (each at a different
// sensitivity), links covering primary/non-primary, a dated period and
// per-link sensitivity, an entity-target field definition with a value, and a
// soft-deleted entity (with a soft-deleted link) that must never be exported.
func seedBundleEntities(t *testing.T, db *gorm.DB, user models.User, ada, bob models.Contact, marker string) {
	t.Helper()
	org := models.Entity{UserID: user.ID, Kind: models.EntityKindOrganisation, Name: "Acme-" + marker, Sensitivity: models.RelationshipSensitivityNormal}
	require.NoError(t, org.SetCore(&models.OrganisationCore{
		Organization: contactmodel.Organization{Name: "Acme-" + marker, Units: []contactmodel.OrgUnit{{Name: "R&D"}}},
		URL:          "https://acme-" + marker + ".example", Notes: "org-notes-" + marker,
	}))
	require.NoError(t, db.Create(&org).Error)

	place := models.Entity{UserID: user.ID, Kind: models.EntityKindPlace, Name: "Cabin-" + marker, Sensitivity: models.RelationshipSensitivityPrivate}
	require.NoError(t, place.SetCore(&models.PlaceCore{
		Address: contactmodel.Address{Components: []contactmodel.AddressComponent{{Kind: "locality", Value: "Springfield-" + marker}}, CountryCode: "US"}, Notes: "place-notes-" + marker,
	}))
	require.NoError(t, db.Create(&place).Error)

	pet := models.Entity{UserID: user.ID, Kind: models.EntityKindPet, Name: "Rex-" + marker, Sensitivity: models.RelationshipSensitivitySecret}
	require.NoError(t, pet.SetCore(&models.PetCore{Species: "dog", Breed: "lab-" + marker, Birth: pdate(2019, 4)}))
	require.NoError(t, db.Create(&pet).Error)

	gone := models.Entity{UserID: user.ID, Kind: models.EntityKindOrganisation, Name: "Gone-" + marker, Sensitivity: models.RelationshipSensitivityNormal}
	require.NoError(t, gone.SetCore(&models.OrganisationCore{Organization: contactmodel.Organization{Name: "Gone-" + marker}}))
	require.NoError(t, db.Create(&gone).Error)

	link := func(contact models.Contact, e models.Entity, role, label string, primary bool, period *contactmodel.TemporalRange, sens string) {
		require.NoError(t, db.Create(&models.ContactEntityLink{
			UserID: user.ID, ContactVCardUID: contact.VCardUID, EntityID: e.ID, Kind: e.Kind,
			Role: role, RoleLabel: label, IsPrimary: primary, Period: period, Sensitivity: sens,
		}).Error)
	}
	link(ada, org, "employee", "Engineer-"+marker, true, &contactmodel.TemporalRange{Start: pdate(2020, 3)}, models.RelationshipSensitivityNormal)
	link(bob, org, "contractor", "", false, &contactmodel.TemporalRange{Start: pdate(2015, 1), End: pdate(2017, 6)}, models.RelationshipSensitivityPrivate)
	link(bob, place, "resident", "", true, nil, models.RelationshipSensitivitySecret)
	link(ada, pet, "owner", "", false, nil, models.RelationshipSensitivityNormal)
	link(ada, gone, "member", "", false, nil, models.RelationshipSensitivityNormal)
	require.NoError(t, db.Delete(&models.ContactEntityLink{}, "entity_id = ?", gone.ID).Error)
	require.NoError(t, db.Delete(&gone).Error)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Industry", Key: "industry_" + marker, Target: models.FieldDefinitionTargetOrganisation,
		Type: models.FieldTypeText, Projection: "internal-only", Sensitivity: models.RelationshipSensitivityPrivate,
	}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: def.ID, UserID: user.ID, EntityID: org.ID, Value: json.RawMessage(`"software-` + marker + `"`),
	}).Error)
}

func TestBuildAccountBundle_Entities(t *testing.T) {
	db := dbtest.New(t)
	owner := bundleTestUser(t, db, "ent-bundle-owner")
	other := bundleTestUser(t, db, "ent-bundle-other")
	seedFullAccount(t, db, owner, "OWN")
	seedFullAccount(t, db, other, "OTH")

	bundle, _, err := BuildAccountBundle(db, owner.ID, "")
	require.NoError(t, err)
	assert.Equal(t, 1, bundle.Version, "the version stays 1: the collections are additive")

	byName := map[string]models.AccountBundleEntity{}
	for _, e := range bundle.Plan.Entities {
		byName[e.Name] = e
	}
	require.Len(t, bundle.Plan.Entities, 3, "live entities only, owner only")
	assert.Equal(t, "normal", byName["Acme-OWN"].Sensitivity)
	assert.Equal(t, "private", byName["Cabin-OWN"].Sensitivity)
	assert.Equal(t, "secret", byName["Rex-OWN"].Sensitivity, "nothing is filtered: the bundle is the owner's backup")
	assert.Equal(t, models.EntityKindPlace, byName["Cabin-OWN"].Kind)
	assert.Contains(t, string(byName["Acme-OWN"].Core), "org-notes-OWN")
	assert.NotEmpty(t, byName["Acme-OWN"].ID)

	require.Len(t, bundle.Plan.EntityLinks, 4, "live links only; the soft-deleted one is omitted")
	var primary, dated int
	linkSens := map[string]bool{}
	for _, l := range bundle.Plan.EntityLinks {
		assert.Equal(t, byName[entityNameByID(bundle, l.EntityID)].ID, l.EntityID, "links reference entities by UUID")
		linkSens[l.Sensitivity] = true
		if l.IsPrimary {
			primary++
		}
		if l.Start != nil {
			dated++
		}
	}
	assert.Equal(t, 2, primary)
	assert.Equal(t, 2, dated)
	assert.True(t, linkSens["private"] && linkSens["secret"], "link sensitivity travels")

	require.Len(t, bundle.Plan.EntityFieldValues, 1)
	assert.Equal(t, byName["Acme-OWN"].ID, bundle.Plan.EntityFieldValues[0].EntityID)
	assert.JSONEq(t, `"software-OWN"`, string(bundle.Plan.EntityFieldValues[0].Value))
	for _, v := range bundle.Plan.CustomFieldValues {
		assert.NotEqual(t, byName["Acme-OWN"].ID, v.EntityID, "an entity's value is in entity_field_values, not custom_field_values")
	}

	raw, err := json.Marshal(bundle.Plan)
	require.NoError(t, err)
	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &generic))
	for _, k := range []string{"entities", "entity_links", "entity_field_values"} {
		assert.NotEqual(t, "null", string(generic[k]), "%s marshals as an array, never null", k)
	}
}

func entityNameByID(b *models.AccountBundle, id string) string {
	for _, e := range b.Plan.Entities {
		if e.ID == id {
			return e.Name
		}
	}
	return ""
}

func TestBuildAccountBundle_NoEntitiesMarshalsEmptyArrays(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "ent-bundle-empty")
	bundle, _, err := BuildAccountBundle(db, u.ID, "")
	require.NoError(t, err)
	raw, err := json.Marshal(bundle.Plan)
	require.NoError(t, err)
	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &generic))
	for _, k := range []string{"entities", "entity_links", "entity_field_values"} {
		assert.JSONEq(t, "[]", string(generic[k]))
	}
}

// A bundle written before 1.7 has no entity keys at all: it must import as
// none, with no issue.
func TestAccountBundle_AbsentEntityCollectionsImportAsNone(t *testing.T) {
	srcDB, dstDB := dbtest.New(t), dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "ent-absent-owner")
	dest := bundleTestUser(t, dstDB, "ent-absent-dest")
	seedFullAccount(t, srcDB, owner, "ABS")
	useBundlePhotoDir(t)
	bundle, _, err := BuildAccountBundle(srcDB, owner.ID, "")
	require.NoError(t, err)

	raw, err := json.Marshal(bundle)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	plan := doc["plan"].(map[string]any)
	for _, k := range []string{"entities", "entity_links", "entity_field_values"} {
		require.Contains(t, plan, k)
		delete(plan, k)
	}
	old, err := json.Marshal(doc)
	require.NoError(t, err)
	var parsed models.AccountBundle
	require.NoError(t, json.Unmarshal(old, &parsed))

	report, _, err := ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(&parsed), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	assert.Empty(t, report.Issues)
	var n int64
	require.NoError(t, dstDB.Model(&models.Entity{}).Where("user_id = ?", dest.ID).Count(&n).Error)
	assert.Zero(t, n)
}

func TestAccountBundle_EntityRoundTripPreservesEverything(t *testing.T) {
	srcDB, dstDB := dbtest.New(t), dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "ent-rt-owner")
	dest := bundleTestUser(t, dstDB, "ent-rt-dest")
	seedFullAccount(t, srcDB, owner, "ENT")
	useBundlePhotoDir(t)
	exported, _, err := BuildAccountBundle(srcDB, owner.ID, "")
	require.NoError(t, err)

	report, _, err := ExecuteSourceImportWithActions(t.Context(), dstDB, dest.ID, MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	require.Empty(t, report.Issues)

	var ents []models.Entity
	require.NoError(t, dstDB.Where("user_id = ?", dest.ID).Find(&ents).Error)
	require.Len(t, ents, 3)
	sens := map[string]string{}
	for _, e := range ents {
		sens[e.Name] = e.Sensitivity
	}
	assert.Equal(t, map[string]string{"Acme-ENT": "normal", "Cabin-ENT": "private", "Rex-ENT": "secret"}, sens)

	var links []models.ContactEntityLink
	require.NoError(t, dstDB.Where("user_id = ?", dest.ID).Find(&links).Error)
	require.Len(t, links, 4)
	primaries := 0
	var contractor models.ContactEntityLink
	for _, l := range links {
		if l.IsPrimary {
			primaries++
		}
		if l.Role == "contractor" {
			contractor = l
		}
	}
	assert.Equal(t, 2, primaries)
	require.NotNil(t, contractor.Period)
	assert.Equal(t, 2017, *contractor.Period.End.Year)
	assert.Equal(t, "private", contractor.Sensitivity)

	reexported, _, err := BuildAccountBundle(dstDB, dest.ID, "")
	require.NoError(t, err)
	assert.Equal(t, canonicalPlanJSON(t, exported), canonicalPlanJSON(t, reexported))
}

// A collision with an entity id the account already owns is a per-row
// conflict: nothing is overwritten, nothing is duplicated, and the row is
// named on the report.
func TestAccountBundle_EntityIDCollisionIsAConflictNotAnOverwrite(t *testing.T) {
	db := dbtest.New(t)
	owner := bundleTestUser(t, db, "ent-conflict-owner")
	seedFullAccount(t, db, owner, "CON")
	useBundlePhotoDir(t)
	exported, _, err := BuildAccountBundle(db, owner.ID, "")
	require.NoError(t, err)

	// Rename the live entity locally, then restore the bundle into the SAME
	// account: the ids exist, so the bundle's (older) name must not win.
	var acme models.Entity
	require.NoError(t, db.Where("user_id = ? AND name = ?", owner.ID, "Acme-CON").First(&acme).Error)
	require.NoError(t, db.Model(&acme).Update("name", "Acme renamed").Error)
	var before int64
	require.NoError(t, db.Model(&models.Entity{}).Where("user_id = ?", owner.ID).Count(&before).Error)

	report, _, err := ExecuteSourceImportWithActions(t.Context(), db, owner.ID, MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)

	var conflicts int
	for _, i := range report.Issues {
		if i.Field == "entity" && i.Category == ImportIssueCategorySkipped {
			conflicts++
			assert.Contains(t, i.Message, "already exists")
		}
	}
	assert.Equal(t, 3, conflicts, "one conflict per colliding entity")
	var after int64
	require.NoError(t, db.Model(&models.Entity{}).Where("user_id = ?", owner.ID).Count(&after).Error)
	assert.Equal(t, before, after, "no duplicates")
	require.NoError(t, db.First(&acme, "id = ?", acme.ID).Error)
	assert.Equal(t, "Acme renamed", acme.Name, "an existing entity is never overwritten")
}

func TestAccountBundle_EntityImportValidationIssues(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "ent-invalid")
	c := models.Contact{VCardUID: "c0ffee00-0000-4000-8000-000000000001"}

	good := models.AccountBundleEntity{ID: "11111111-1111-4111-8111-111111111111", Kind: "organisation", Name: "Good", Sensitivity: "normal", Core: json.RawMessage(`{"organization":{"name":"Good"}}`)}
	badKind := models.AccountBundleEntity{ID: "22222222-2222-4222-8222-222222222222", Kind: "robot", Name: "X", Sensitivity: "normal", Core: json.RawMessage(`{}`)}
	badCore := models.AccountBundleEntity{ID: "33333333-3333-4333-8333-333333333333", Kind: "pet", Name: "Y", Sensitivity: "normal", Core: json.RawMessage(`{"unknown":1}`)}
	badSens := models.AccountBundleEntity{ID: "44444444-4444-4444-8444-444444444444", Kind: "pet", Name: "Z", Sensitivity: "top", Core: json.RawMessage(`{}`)}
	noName := models.AccountBundleEntity{ID: "55555555-5555-4555-8555-555555555555", Kind: "pet", Name: "", Sensitivity: "normal", Core: json.RawMessage(`{}`)}
	bundle := &models.AccountBundle{Format: models.AccountBundleFormat, Version: 1}
	bundle.Plan.Contacts = []models.AccountBundleContact{{UID: c.VCardUID, Card: contactmodel.Card{UID: c.VCardUID, Name: &contactmodel.Name{Full: "Zed Q", Components: []contactmodel.NameComponent{{Kind: "given", Value: "Zed"}, {Kind: "surname", Value: "Q"}}}}}}
	bundle.Plan.Entities = []models.AccountBundleEntity{good, badKind, badCore, badSens, noName}
	bundle.Plan.EntityLinks = []models.AccountBundleEntityLink{
		{ID: "a1111111-1111-4111-8111-111111111111", ContactVCardUID: c.VCardUID, EntityID: good.ID, Role: "employee", Sensitivity: "normal"},
		{ID: "a2222222-2222-4222-8222-222222222222", ContactVCardUID: c.VCardUID, EntityID: good.ID, Role: "astronaut", Sensitivity: "normal"},
		{ID: "a3333333-3333-4333-8333-333333333333", ContactVCardUID: "no-such-contact", EntityID: good.ID, Role: "employee", Sensitivity: "normal"},
		{ID: "a4444444-4444-4444-8444-444444444444", ContactVCardUID: c.VCardUID, EntityID: badKind.ID, Role: "other", Sensitivity: "normal"},
	}

	report, _, err := ExecuteSourceImportWithActions(t.Context(), db, u.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)

	var ents []models.Entity
	require.NoError(t, db.Where("user_id = ?", u.ID).Find(&ents).Error)
	require.Len(t, ents, 1)
	assert.Equal(t, "Good", ents[0].Name)
	var links []models.ContactEntityLink
	require.NoError(t, db.Where("user_id = ?", u.ID).Find(&links).Error)
	require.Len(t, links, 1, "an invalid role, unknown contact and dangling entity each drop their link")

	cats := map[string]int{}
	for _, i := range report.Issues {
		cats[i.Field+"/"+i.Category]++
	}
	assert.Equal(t, 4, cats["entity/"+ImportIssueCategoryInvalid], "%+v", report.Issues)
	assert.Equal(t, 1, cats["entity_link.role/"+ImportIssueCategoryInvalid], "%+v", report.Issues)
	assert.GreaterOrEqual(t, cats["entity_link.entity_id/"+ImportIssueCategoryUnsupported], 1, "%+v", report.Issues)
}

// An entity value whose definition targets another kind is rejected (ADR 0043
// decision 5: the value must resolve to an entity of the kind its definition
// targets).
func TestAccountBundle_EntityFieldValueDefinitionTargetMustMatchKind(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "ent-fv-target")
	bundle := &models.AccountBundle{Format: models.AccountBundleFormat, Version: 1}
	eid := "11111111-1111-4111-8111-111111111111"
	bundle.Plan.Entities = []models.AccountBundleEntity{{ID: eid, Kind: "pet", Name: "Rex", Sensitivity: "normal", Core: json.RawMessage(`{}`)}}
	bundle.Plan.CustomFieldDefinitions = []models.AccountBundleFieldDefinition{
		{ID: "d1", Label: "Org field", Key: "orgf", Target: "organisation", Type: "text", Projection: "internal-only", Sensitivity: "normal"},
		{ID: "d2", Label: "Pet field", Key: "petf", Target: "pet", Type: "text", Projection: "internal-only", Sensitivity: "normal"},
	}
	bundle.Plan.EntityFieldValues = []models.AccountBundleEntityFieldValue{
		{FieldDefinitionID: "d1", EntityID: eid, Value: json.RawMessage(`"wrong kind"`)},
		{FieldDefinitionID: "d2", EntityID: eid, Value: json.RawMessage(`"ok"`)},
		{FieldDefinitionID: "d2", EntityID: "99999999-9999-4999-8999-999999999999", Value: json.RawMessage(`"dangling"`)},
	}
	report, _, err := ExecuteSourceImportWithActions(t.Context(), db, u.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	var vals []models.FieldValue
	require.NoError(t, db.Where("user_id = ?", u.ID).Find(&vals).Error)
	require.Len(t, vals, 1)
	assert.JSONEq(t, `"ok"`, string(vals[0].Value))
	assert.Len(t, report.Issues, 2, "%+v", report.Issues)
}

func TestMapAccountBundle_EntityTimestampsAreKept(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "ent-ts")
	ts := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	bundle := &models.AccountBundle{Format: models.AccountBundleFormat, Version: 1}
	bundle.Plan.Entities = []models.AccountBundleEntity{{ID: "11111111-1111-4111-8111-111111111111", Kind: "pet", Name: "Rex", Sensitivity: "normal", Core: json.RawMessage(`{}`), CreatedAt: ts, UpdatedAt: ts}}
	_, _, err := ExecuteSourceImportWithActions(t.Context(), db, u.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	var e models.Entity
	require.NoError(t, db.Where("user_id = ?", u.ID).First(&e).Error)
	assert.True(t, e.CreatedAt.Equal(ts), "created_at is restored, got %s", e.CreatedAt)
}

func entityRestoreBundle(uid string) *models.AccountBundle {
	b := &models.AccountBundle{Format: models.AccountBundleFormat, Version: 1}
	b.Plan.Contacts = []models.AccountBundleContact{{UID: uid, Card: contactmodel.Card{UID: uid, Name: &contactmodel.Name{Full: "Zed Q", Components: []contactmodel.NameComponent{{Kind: "given", Value: "Zed"}, {Kind: "surname", Value: "Q"}}}}}}
	b.Plan.Entities = []models.AccountBundleEntity{{ID: "11111111-1111-4111-8111-111111111111", Kind: "organisation", Name: "Acme", Sensitivity: "normal", Core: json.RawMessage(`{}`)}}
	return b
}

// A link that is new in a later run resolves to an entity an earlier run
// already imported (the ledger remembers it); if that entity has since been
// removed, the link is reported rather than pointed at nothing.
func TestAccountBundle_NewLinkResolvesToEntityFromEarlierRun(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "ent-later-run")
	uid := "c0ffee00-0000-4000-8000-000000000002"
	run := func(b *models.AccountBundle) *ImportReport {
		report, _, err := ExecuteSourceImportWithActions(t.Context(), db, u.ID, MapAccountBundle(b), map[string]SourceContactAction{}, nil)
		require.NoError(t, err)
		return report
	}
	first := entityRestoreBundle(uid)
	run(first)

	// The contact of the first run is skipped (already imported), so the new
	// link belongs to a second contact.
	uid2 := "c0ffee00-0000-4000-8000-0000000000b2"
	second := entityRestoreBundle(uid)
	second.Plan.Contacts = append(second.Plan.Contacts, models.AccountBundleContact{UID: uid2, Card: contactmodel.Card{UID: uid2, Name: &contactmodel.Name{Full: "Yan P", Components: []contactmodel.NameComponent{{Kind: "given", Value: "Yan"}, {Kind: "surname", Value: "P"}}}}})
	second.Plan.EntityLinks = []models.AccountBundleEntityLink{{ID: "a1111111-1111-4111-8111-111111111111", ContactVCardUID: uid2, EntityID: first.Plan.Entities[0].ID, Role: "owner", Sensitivity: "normal"}}
	report := run(second)
	var links []models.ContactEntityLink
	require.NoError(t, db.Where("user_id = ?", u.ID).Find(&links).Error)
	require.Len(t, links, 1, "issues: %+v", report.Issues)
	assert.Equal(t, models.EntityKindOrganisation, links[0].Kind, "the kind comes from the entity row, not the bundle")

	// The entity vanishes (hard delete), then another new link arrives.
	require.NoError(t, db.Unscoped().Delete(&models.Entity{}, "user_id = ?", u.ID).Error)
	uid3 := "c0ffee00-0000-4000-8000-0000000000b3"
	third := entityRestoreBundle(uid)
	third.Plan.Contacts = append(third.Plan.Contacts, models.AccountBundleContact{UID: uid3, Card: contactmodel.Card{UID: uid3, Name: &contactmodel.Name{Full: "Xi O", Components: []contactmodel.NameComponent{{Kind: "given", Value: "Xi"}, {Kind: "surname", Value: "O"}}}}})
	third.Plan.EntityLinks = []models.AccountBundleEntityLink{{ID: "a2222222-2222-4222-8222-222222222222", ContactVCardUID: uid3, EntityID: first.Plan.Entities[0].ID, Role: "owner", Sensitivity: "normal"}}
	report = run(third)
	var found bool
	for _, i := range report.Issues {
		if i.Field == "entity_link.entity_id" && i.Category == ImportIssueCategoryUnsupported {
			found = true
		}
	}
	assert.True(t, found, "%+v", report.Issues)
}

func TestAccountBundle_EntityLinkValidationAndConflicts(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "ent-link-rules")
	uid := "c0ffee00-0000-4000-8000-000000000003"

	// An owned link id (on an unrelated entity) collides with the bundle's.
	existing := models.Entity{UserID: u.ID, Kind: models.EntityKindOrganisation, Name: "Existing", Sensitivity: "normal"}
	require.NoError(t, existing.SetCore(&models.OrganisationCore{}))
	require.NoError(t, db.Create(&existing).Error)
	linkID := "a3333333-3333-4333-8333-333333333333"
	require.NoError(t, db.Create(&models.ContactEntityLink{ID: linkID, UserID: u.ID, ContactVCardUID: "someone-else", EntityID: existing.ID, Kind: existing.Kind, Role: "employee", Sensitivity: "normal"}).Error)

	b := entityRestoreBundle(uid)
	eid := b.Plan.Entities[0].ID
	b.Plan.EntityLinks = []models.AccountBundleEntityLink{
		{ID: "a1111111-1111-4111-8111-111111111111", ContactVCardUID: uid, EntityID: eid, Role: "employee", Sensitivity: "bogus"},
		{ID: linkID, ContactVCardUID: uid, EntityID: eid, Role: "employee", Sensitivity: "normal"},
	}
	// Two values for the same (definition, entity) are one row: the plan keys
	// them identically, so the second is skipped, not written twice.
	b.Plan.CustomFieldDefinitions = []models.AccountBundleFieldDefinition{{ID: "d1", Label: "Industry", Key: "industry", Target: "organisation", Type: "text", Projection: "internal-only", Sensitivity: "normal"}}
	b.Plan.EntityFieldValues = []models.AccountBundleEntityFieldValue{
		{FieldDefinitionID: "d1", EntityID: eid, Value: json.RawMessage(`"a"`)},
		{FieldDefinitionID: "d1", EntityID: eid, Value: json.RawMessage(`"b"`)},
	}
	report, _, err := ExecuteSourceImportWithActions(t.Context(), db, u.ID, MapAccountBundle(b), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	got := map[string]string{}
	for _, i := range report.Issues {
		got[i.Field] = i.Category
	}
	assert.Equal(t, ImportIssueCategoryInvalid, got["entity_link.sensitivity"])
	assert.Equal(t, ImportIssueCategorySkipped, got["entity_link"], "an owned link id is a conflict")
	var vals int64
	require.NoError(t, db.Model(&models.FieldValue{}).Where("user_id = ?", u.ID).Count(&vals).Error)
	assert.EqualValues(t, 1, vals)
	var existingLink models.ContactEntityLink
	require.NoError(t, db.First(&existingLink, "id = ?", linkID).Error)
	assert.Equal(t, "someone-else", existingLink.ContactVCardUID, "the owned link is untouched")
}

// A link whose contact was removed is not exported: every reference in the
// bundle resolves.
func TestBuildAccountBundle_OmitsLinkOfRemovedContact(t *testing.T) {
	db := dbtest.New(t)
	u := bundleTestUser(t, db, "ent-bundle-gone-contact")
	c := models.Contact{UserID: u.ID, Firstname: "Gone"}
	require.NoError(t, db.Create(&c).Error)
	e := models.Entity{UserID: u.ID, Kind: models.EntityKindPet, Name: "Rex", Sensitivity: "normal"}
	require.NoError(t, e.SetCore(&models.PetCore{}))
	require.NoError(t, db.Create(&e).Error)
	require.NoError(t, db.Create(&models.ContactEntityLink{UserID: u.ID, ContactVCardUID: c.VCardUID, EntityID: e.ID, Kind: e.Kind, Role: "owner", Sensitivity: "normal"}).Error)
	require.NoError(t, db.Delete(&c).Error)

	bundle, _, err := BuildAccountBundle(db, u.ID, "")
	require.NoError(t, err)
	assert.Len(t, bundle.Plan.Entities, 1)
	assert.Empty(t, bundle.Plan.EntityLinks)
}

// A database failure at any entity-phase statement of a bundle restore either
// fails the import (nothing is left behind) or is reported as a stable
// per-row write failure: never raw driver text.
func TestAccountBundle_EntityRestoreDatabaseFailures(t *testing.T) {
	logtest.AllowWarnings(t, "the fault sweep deliberately fails statements; the import logs each failed row")
	srcDB := dbtest.New(t)
	owner := bundleTestUser(t, srcDB, "ent-fault-owner")
	seedFullAccount(t, srcDB, owner, "FLT")
	useBundlePhotoDir(t)
	exported, _, err := BuildAccountBundle(srcDB, owner.ID, "")
	require.NoError(t, err)

	setup := func() (*gorm.DB, models.User) {
		db := dbtest.New(t, dbtest.WithFaults())
		return db, bundleTestUser(t, db, "ent-fault-dest")
	}
	run := func(db *gorm.DB, u models.User) (*ImportReport, error) {
		report, _, err := ExecuteSourceImportWithActions(t.Context(), db, u.ID, MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
		return report, err
	}

	db, u := setup()
	inj := dbfault.For(db)
	inj.Record()
	_, err = run(db, u)
	require.NoError(t, err)
	stmts := inj.Statements()
	inj.Disarm()

	var targets []int
	firstEntity := -1
	for i, s := range stmts {
		if s.Table == "entities" && firstEntity < 0 {
			firstEntity = i
		}
	}
	require.GreaterOrEqual(t, firstEntity, 0)
	for i := firstEntity; i < len(stmts); i++ {
		targets = append(targets, i+1)
	}
	for _, n := range targets {
		db, u := setup()
		inj := dbfault.For(db)
		inj.FailNth(n)
		report, err := run(db, u)
		inj.Disarm()
		if err != nil {
			var rows int64
			require.NoError(t, db.Model(&models.Entity{}).Where("user_id = ?", u.ID).Count(&rows).Error)
			assert.Zero(t, rows, "statement %d (%s) failed the import: nothing may be left behind", n, stmts[n-1])
			continue
		}
		for _, issue := range report.Issues {
			assert.NotContains(t, issue.Message, dbfault.Marker, "statement %d (%s)", n, stmts[n-1])
		}
	}
}
