package services

// Entity delete cascade and restore (issue #2295, ADR 0043 decision 6).
// Real migrated schema (trap #1), injected timestamps.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var entityNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// entityRefColumns are the column names through which a table can reference an
// entities.id.
var entityRefColumns = []string{"entity_id", "id_low", "id_high"}

// entityCascadeNotRegistered lists tables that reference entities.id yet are
// deliberately not part of the entity delete cascade. Every entry needs a reason.
var entityCascadeNotRegistered = map[string]string{
	"entity_deletion_snapshots": "written BY the entity delete and consumed by restore; reaped by the purge",
}

// entityReferencingTables finds every table whose entity-shaped column holds an
// entities.id: a foreign key to entities, the dismissed-pair id columns, and
// the contact-keyed tables that overload entity_id (those hold contact UIDs
// and are identified by ContactCascadeRegistry, so they are excluded unless
// they are field_values, whose entity_id holds either).
func entityReferencingTables(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	var fks []struct{ T string }
	require.NoError(t, db.Raw(`SELECT DISTINCT m.name AS t FROM sqlite_master m, pragma_foreign_key_list(m.name) f
		WHERE m.type = 'table' AND f."table" = 'entities'`).Scan(&fks).Error)
	for _, r := range fks {
		out[r.T] = "foreign key to entities"
	}
	var cols []struct{ T, C string }
	require.NoError(t, db.Raw(`SELECT m.name AS t, p.name AS c FROM sqlite_master m, pragma_table_info(m.name) p
		WHERE m.type = 'table' AND (m.name = 'entities' OR m.name GLOB 'entity_*' OR m.name GLOB '*_entity_*') AND p.name IN ?`, entityRefColumns).Scan(&cols).Error)
	for _, r := range cols {
		out[r.T] = "column " + r.C
	}
	out["field_values"] = "entity_id holds an entities.id for entity-target definitions"
	return out
}

func TestEntityCascadeRegistry_CoversEverySchemaTableReferencingAnEntity(t *testing.T) {
	db := dbtest.New(t)
	registered := map[string]bool{}
	for _, s := range EntityCascadeRegistry() {
		registered[s.Table] = true
	}
	var unclassified []string
	for table, why := range entityReferencingTables(t, db) {
		_, exempt := entityCascadeNotRegistered[table]
		switch {
		case registered[table]:
			assert.False(t, exempt, "%s is registered AND exempt; drop the stale exemption", table)
		case exempt:
		default:
			unclassified = append(unclassified, table+" ("+why+")")
		}
	}
	sort.Strings(unclassified)
	require.Empty(t, unclassified,
		"tables referencing an entity with no EntityCascadeRegistry step (services/entity_delete.go) and no "+
			"entityCascadeNotRegistered entry; an entity delete would leave their rows behind (ADR 0035, ADR 0043):\n  %s",
		strings.Join(unclassified, "\n  "))

	var names []string
	require.NoError(t, db.Raw(`SELECT name FROM sqlite_master WHERE type='table'`).Scan(&names).Error)
	schema := map[string]bool{}
	for _, n := range names {
		schema[n] = true
	}
	for table := range registered {
		assert.True(t, schema[table], "registry step %s is not in the migrated schema (stale)", table)
	}
	for table := range entityCascadeNotRegistered {
		assert.True(t, schema[table], "exemption %s is not in the migrated schema (stale)", table)
	}
}

// The coverage scan must actually fail for a new entities-referencing table
// (the acceptance criterion): prove it by adding one.
func TestEntityCascadeRegistry_CoverageTestWouldCatchANewTable(t *testing.T) {
	db := dbtest.New(t)
	require.NoError(t, db.Exec(`CREATE TABLE entity_widgets (id INTEGER PRIMARY KEY, user_id INTEGER, entity_id TEXT,
		FOREIGN KEY (entity_id) REFERENCES entities(id))`).Error)
	refs := entityReferencingTables(t, db)
	registered := map[string]bool{}
	for _, s := range EntityCascadeRegistry() {
		registered[s.Table] = true
	}
	_, found := refs["entity_widgets"]
	assert.True(t, found, "the scan sees a new table that references entities")
	assert.False(t, registered["entity_widgets"], "and it has no registry step, which is what fails the coverage test")
}

func TestEntityCascadeRegistry_StepsAreWellFormed(t *testing.T) {
	db := dbtest.New(t)
	seen := map[string]bool{}
	require.NotEmpty(t, EntityCascadeRegistry())
	for _, s := range EntityCascadeRegistry() {
		assert.False(t, seen[s.Table], "%s appears twice", s.Table)
		seen[s.Table] = true
		assert.NotEmpty(t, s.Reason, "%s needs a recorded reason", s.Table)
		require.NotNil(t, s.Match, "%s needs a Match", s.Table)
		q, args := s.Match(EntityCascadeTarget{ID: "e", UserID: 2})
		assert.Contains(t, q, "user_id", "%s: every Match must scope by user_id (trap #5)", s.Table)
		assert.NotEmpty(t, args)
		if s.Mode == CascadeHard && s.Table != "field_values" {
			assert.NotEmpty(t, s.SnapshotSkip, "%s: a hard step is snapshotted or says why not", s.Table)
		}
		if s.NewModel != nil {
			stmt := &gorm.Statement{DB: db}
			require.NoError(t, stmt.Parse(s.NewModel()))
			assert.Equal(t, s.Table, stmt.Schema.Table)
			assert.Equal(t, s.Mode == CascadeSoft, stmt.Schema.LookUpField("DeletedAt") != nil,
				"%s: mode %q disagrees with the model (trap #7)", s.Table, s.Mode)
		}
	}
}

func TestEntityCascadeRegistry_ReturnsFreshSlice(t *testing.T) {
	a := EntityCascadeRegistry()
	a[0].Table = "tampered"
	assert.NotEqual(t, "tampered", EntityCascadeRegistry()[0].Table)
}

type entityFixture struct {
	db      *gorm.DB
	userID  uint
	org     models.Entity
	other   models.Entity
	c1, c2  models.Contact
	defID   string
	linkIDs map[string]string
}

func newEntityFixture(t *testing.T) entityFixture {
	t.Helper()
	db, userID := newPurgeDB(t)
	f := entityFixture{db: db, userID: userID, linkIDs: map[string]string{}}
	f.org = models.Entity{UserID: userID, Kind: models.EntityKindOrganisation, Name: "Acme"}
	f.other = models.Entity{UserID: userID, Kind: models.EntityKindOrganisation, Name: "Beta"}
	require.NoError(t, db.Create(&f.org).Error)
	require.NoError(t, db.Create(&f.other).Error)
	f.c1 = models.Contact{UserID: userID, Firstname: "One"}
	f.c2 = models.Contact{UserID: userID, Firstname: "Two"}
	require.NoError(t, db.Create(&f.c1).Error)
	require.NoError(t, db.Create(&f.c2).Error)
	def := models.FieldDefinition{UserID: userID, Label: "Notes", Key: "notes", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, db.Create(&def).Error)
	f.defID = def.ID
	return f
}

func (f entityFixture) link(t *testing.T, c models.Contact, e models.Entity, primary bool) models.ContactEntityLink {
	t.Helper()
	l := models.ContactEntityLink{UserID: f.userID, ContactVCardUID: c.VCardUID, EntityID: e.ID, Kind: e.Kind, Role: "employee", IsPrimary: primary, Sensitivity: "normal"}
	require.NoError(t, f.db.Create(&l).Error)
	return l
}

func TestDeleteEntity_CascadesAndSnapshots(t *testing.T) {
	f := newEntityFixture(t)
	l1 := f.link(t, f.c1, f.org, true)
	l2 := f.link(t, f.c2, f.org, false)
	earlier := f.link(t, f.c2, f.org, false) // deleted by the user before the entity
	require.NoError(t, f.db.Delete(&earlier).Error)
	keep := f.link(t, f.c1, f.other, false)
	require.NoError(t, f.db.Create(&models.FieldValue{FieldDefinitionID: f.defID, UserID: f.userID, EntityID: f.org.ID, Value: json.RawMessage(`"hello"`)}).Error)
	require.NoError(t, f.db.Create(&models.DismissedEntityPair{UserID: f.userID, IDLow: min(f.org.ID, f.other.ID), IDHigh: max(f.org.ID, f.other.ID)}).Error)

	require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))

	assert.Zero(t, count(t, f.db, false, &models.Entity{}, "id = ?", f.org.ID), "entity is soft-deleted")
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Entity{}, "id = ? AND deleted_at IS NOT NULL", f.org.ID))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Entity{}, "id = ?", f.other.ID), "other entities are untouched")

	var snap models.EntityDeletionSnapshot
	require.NoError(t, f.db.Where("entity_id = ?", f.org.ID).First(&snap).Error)
	for _, id := range []string{l1.ID, l2.ID} {
		var got models.ContactEntityLink
		require.NoError(t, f.db.Unscoped().First(&got, "id = ?", id).Error)
		assert.True(t, got.DeletedAt.Valid, "link soft-deleted")
		require.NotNil(t, got.DeletionBatchID)
		assert.Equal(t, snap.DeletionBatchID, *got.DeletionBatchID, "stamped with the one new batch id")
		assert.EqualValues(t, 2, got.Revision, "tombstoning bumps the revision")
	}
	var prior models.ContactEntityLink
	require.NoError(t, f.db.Unscoped().First(&prior, "id = ?", earlier.ID).Error)
	assert.Nil(t, prior.DeletionBatchID, "a link deleted earlier is not part of the batch")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", keep.ID), "other entity's link untouched")

	assert.Zero(t, count(t, f.db, true, &models.FieldValue{}, "entity_id = ?", f.org.ID), "field values are hard-deleted")
	assert.Zero(t, count(t, f.db, true, &models.DismissedEntityPair{}, "user_id = ?", f.userID))
	assert.Contains(t, snap.Payload, "hello")
	var raw string
	require.NoError(t, f.db.Raw("SELECT payload FROM entity_deletion_snapshots WHERE id = ?", snap.ID).Scan(&raw).Error)
}

func TestDeleteEntity_OwnerScopedAndRollsBack(t *testing.T) {
	f := newEntityFixture(t)
	other := models.User{Username: "intruder", Email: "i@example.com", Password: "x"}
	require.NoError(t, f.db.Create(&other).Error)
	assert.ErrorIs(t, DeleteEntity(f.db, other.ID, f.org.ID, entityNow), gorm.ErrRecordNotFound, "another user's entity is not found")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Entity{}, "id = ?", f.org.ID))

	f.link(t, f.c1, f.org, false)
	dbtest.HideTable(t, f.db, "dismissed_entity_pairs")
	require.Error(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Entity{}, "id = ?", f.org.ID), "a failed step rolls the whole delete back")
	assert.Zero(t, count(t, f.db, true, &models.EntityDeletionSnapshot{}, "user_id = ?", f.userID), "no snapshot without the delete")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactEntityLink{}, "entity_id = ?", f.org.ID))
}

func TestRestoreEntity_RevivesBatchLinksAndFieldValues(t *testing.T) {
	f := newEntityFixture(t)
	l1 := f.link(t, f.c1, f.org, true)
	earlier := f.link(t, f.c2, f.org, false)
	require.NoError(t, f.db.Delete(&earlier).Error)
	require.NoError(t, f.db.Create(&models.FieldValue{FieldDefinitionID: f.defID, UserID: f.userID, EntityID: f.org.ID, Value: json.RawMessage(`"hello"`)}).Error)
	require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))

	res, err := RestoreEntity(f.db, f.userID, f.org.ID, entityNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, res.Skipped)
	assert.False(t, res.SameNameLive)
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Entity{}, "id = ?", f.org.ID))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", l1.ID), "batch link revived")
	assert.Zero(t, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", earlier.ID), "an earlier-deleted link stays deleted")
	var fv models.FieldValue
	require.NoError(t, f.db.First(&fv, "entity_id = ?", f.org.ID).Error)
	assert.JSONEq(t, `"hello"`, string(fv.Value), "field value re-created from the snapshot")
	assert.Zero(t, count(t, f.db, true, &models.EntityDeletionSnapshot{}, "entity_id = ?", f.org.ID), "snapshot consumed")

	var revived models.ContactEntityLink
	require.NoError(t, f.db.First(&revived, "id = ?", l1.ID).Error)
	assert.True(t, revived.IsPrimary, "primary survives when nothing collides")
	assert.Nil(t, revived.DeletionBatchID)

	_, err = RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
	assert.ErrorIs(t, err, ErrEntityNotInTrash, "a live entity is not restorable")
}

func TestRestoreEntity_ReportsPrimaryDanglingAndSameName(t *testing.T) {
	f := newEntityFixture(t)
	primary := f.link(t, f.c1, f.org, true)
	gone := f.link(t, f.c2, f.org, false)
	require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))

	// A new primary of the kind appears meanwhile; one linked contact is deleted;
	// another live entity takes the same name.
	f.link(t, f.c1, f.other, true)
	require.NoError(t, DeleteContact(f.db, f.c2, f.userID, entityNow))
	require.NoError(t, f.db.Create(&models.Entity{UserID: f.userID, Kind: models.EntityKindOrganisation, Name: "ACME"}).Error)

	res, err := RestoreEntity(f.db, f.userID, f.org.ID, entityNow.Add(time.Hour))
	require.NoError(t, err, "restore reports, never fails on a primary collision")
	assert.True(t, res.SameNameLive, "a live entity with the same name_key is a warning, not a conflict")
	reasons := map[string]int{}
	for _, s := range res.Skipped {
		assert.Equal(t, "contact_entity_links", s.Table)
		reasons[s.Reason] += s.Count
	}
	assert.Equal(t, 1, reasons[RestoreReasonPrimaryDemoted], "the colliding primary is revived demoted and reported")
	assert.Equal(t, 1, reasons[RestoreReasonContactGone], "the link whose contact is gone is skipped and reported")
	var got models.ContactEntityLink
	require.NoError(t, f.db.First(&got, "id = ?", primary.ID).Error)
	assert.False(t, got.IsPrimary)
	assert.Zero(t, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", gone.ID))
}

func TestRestoreEntity_OwnerScoped(t *testing.T) {
	f := newEntityFixture(t)
	require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))
	other := models.User{Username: "intruder", Email: "i@example.com", Password: "x"}
	require.NoError(t, f.db.Create(&other).Error)
	_, err := RestoreEntity(f.db, other.ID, f.org.ID, entityNow)
	assert.ErrorIs(t, err, ErrEntityNotInTrash, "another user's trashed entity is not found")
	require.NoError(t, f.db.Exec("UPDATE entity_deletion_snapshots SET payload_version = 99").Error)
	_, err = RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
	assert.ErrorIs(t, err, ErrEntitySnapshotUnsupported)
}

func TestDeleteContact_SoftDeletesLinksWithBatchAndRestoreRevivesThem(t *testing.T) {
	f := newEntityFixture(t)
	a := f.link(t, f.c1, f.org, true)
	earlier := f.link(t, f.c1, f.other, false)
	require.NoError(t, f.db.Delete(&earlier).Error)

	require.NoError(t, DeleteContact(f.db, f.c1, f.userID, entityNow))
	var got models.ContactEntityLink
	require.NoError(t, f.db.Unscoped().First(&got, "id = ?", a.ID).Error)
	assert.True(t, got.DeletedAt.Valid)
	require.NotNil(t, got.DeletionBatchID, "stamped with the contact deletion's batch id")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.Entity{}, "id = ?", f.org.ID), "the entity itself is untouched")
	var prior models.ContactEntityLink
	require.NoError(t, f.db.Unscoped().First(&prior, "id = ?", earlier.ID).Error)
	assert.Nil(t, prior.DeletionBatchID)

	res, err := RestoreContact(f.db, f.userID, f.c1.ID, entityNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, res.Skipped)
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", a.ID), "link revived")
	assert.Zero(t, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", earlier.ID), "an earlier-deleted link stays deleted")
}

func TestRestoreContact_DemotesCollidingPrimaryAndSkipsDeletedEntity(t *testing.T) {
	f := newEntityFixture(t)
	primary := f.link(t, f.c1, f.org, false)
	dangling := f.link(t, f.c1, f.other, false)
	require.NoError(t, DeleteContact(f.db, f.c1, f.userID, entityNow))

	// Meanwhile the contact's slot is taken by a new primary of the same kind,
	// and one linked entity is deleted.
	require.NoError(t, f.db.Create(&models.ContactEntityLink{UserID: f.userID, ContactVCardUID: f.c1.VCardUID, EntityID: f.other.ID,
		Kind: f.other.Kind, Role: "owner", IsPrimary: true, Sensitivity: "normal"}).Error)
	require.NoError(t, f.db.Delete(&f.other).Error)

	res, err := RestoreContact(f.db, f.userID, f.c1.ID, entityNow.Add(time.Hour))
	require.NoError(t, err, "the unique primary index must not fail the restore")
	reasons := map[string]int{}
	for _, s := range res.Skipped {
		if s.Table == "contact_entity_links" {
			reasons[s.Reason] += s.Count
		}
	}
	assert.Equal(t, 1, reasons[RestoreReasonEntityGone], "the link whose entity was deleted is skipped and reported")
	assert.Zero(t, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", dangling.ID), "dangling link stays deleted")
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", primary.ID), "the other link is revived")
}

func TestRestoreContact_DemotesPrimaryWhenSlotWasTaken(t *testing.T) {
	f := newEntityFixture(t)
	primary := f.link(t, f.c1, f.org, true)
	require.NoError(t, DeleteContact(f.db, f.c1, f.userID, entityNow))
	// RestoreContact needs the contact UID free; a live primary link for the same
	// UID is created before the restore (e.g. by a re-created contact's link).
	require.NoError(t, f.db.Create(&models.ContactEntityLink{UserID: f.userID, ContactVCardUID: f.c1.VCardUID, EntityID: f.other.ID,
		Kind: f.other.Kind, Role: "owner", IsPrimary: true, Sensitivity: "normal"}).Error)

	res, err := RestoreContact(f.db, f.userID, f.c1.ID, entityNow.Add(time.Hour))
	require.NoError(t, err)
	var got models.ContactEntityLink
	require.NoError(t, f.db.First(&got, "id = ?", primary.ID).Error)
	assert.False(t, got.IsPrimary, "revived demoted, not failed")
	found := false
	for _, s := range res.Skipped {
		if s.Table == "contact_entity_links" && s.Reason == RestoreReasonPrimaryDemoted && s.Count == 1 {
			found = true
		}
	}
	assert.True(t, found, "the demotion is reported: %+v", res.Skipped)
}

// Every statement of the delete and restore transactions reports its failure and
// rolls the whole transaction back.
func TestDeleteAndRestoreEntity_EveryTableFailurePropagates(t *testing.T) {
	for _, table := range []string{"contact_entity_links", "field_values", "dismissed_entity_pairs", "entity_deletion_snapshots"} {
		t.Run("delete/"+table, func(t *testing.T) {
			f := newEntityFixture(t)
			f.link(t, f.c1, f.org, true)
			dbtest.HideTable(t, f.db, table)
			require.Error(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))
			assert.EqualValues(t, 1, count(t, f.db, false, &models.Entity{}, "id = ?", f.org.ID), "entity still live after the rollback")
		})
	}

	for _, table := range []string{"entity_deletion_snapshots", "contact_entity_links", "field_definitions", "field_values", "entities", "contacts"} {
		t.Run("restore/"+table, func(t *testing.T) {
			f := newEntityFixture(t)
			f.link(t, f.c1, f.org, true)
			require.NoError(t, f.db.Create(&models.FieldValue{FieldDefinitionID: f.defID, UserID: f.userID, EntityID: f.org.ID, Value: json.RawMessage(`"v"`)}).Error)
			require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))
			dbtest.HideTable(t, f.db, table)
			_, err := RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
			require.Error(t, err)
		})
	}
}

func TestRestoreEntity_UnreadablePayloadAndMissingDefinition(t *testing.T) {
	f := newEntityFixture(t)
	require.NoError(t, f.db.Create(&models.FieldValue{FieldDefinitionID: f.defID, UserID: f.userID, EntityID: f.org.ID, Value: json.RawMessage(`"v"`)}).Error)
	require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))

	// The definition was deleted while the entity was in the trash: its value
	// is skipped and reported, not an error.
	require.NoError(t, f.db.Delete(&models.FieldDefinition{}, "id = ?", f.defID).Error)
	res, err := RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
	require.NoError(t, err)
	assert.Equal(t, []RestoreSkip{{Table: "field_values", Reason: "definition_gone", Count: 1}}, res.Skipped)

	require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))
	require.NoError(t, f.db.Exec("UPDATE entity_deletion_snapshots SET payload = ?", "{not json").Error)
	_, err = RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode entity deletion snapshot")
}

func TestReviveContactEntityLinks_IgnoresIDsItDoesNotOwn(t *testing.T) {
	f := newEntityFixture(t)
	l := f.link(t, f.c1, f.org, false)
	require.NoError(t, f.db.Delete(&l).Error)
	other := models.User{Username: "x", Email: "x@example.com", Password: "x"}
	require.NoError(t, f.db.Create(&other).Error)

	skips, err := reviveContactEntityLinks(f.db, other.ID, []string{l.ID, "no-such-link"}, entityNow)
	require.NoError(t, err)
	assert.Empty(t, skips)
	assert.Zero(t, count(t, f.db, false, &models.ContactEntityLink{}, "id = ?", l.ID), "another user's link id is never revived")

	dbtest.HideTable(t, f.db, "contacts")
	_, err = reviveContactEntityLinks(f.db, f.userID, []string{l.ID}, entityNow)
	assert.Error(t, err)
}

// failStatement installs a trigger that aborts one kind of statement, to reach
// the error returns of a transaction's later steps.
func failStatement(t *testing.T, db *gorm.DB, name, trigger string) {
	t.Helper()
	require.NoError(t, db.Exec("CREATE TRIGGER "+name+" "+trigger+" BEGIN SELECT RAISE(ABORT, 'forced failure'); END").Error)
}

func TestDeleteAndRestoreEntity_LateStatementFailuresPropagate(t *testing.T) {
	cases := map[string]struct {
		trigger string
		restore bool
	}{
		"soft delete of the entity":  {"BEFORE UPDATE OF deleted_at ON entities WHEN NEW.deleted_at IS NOT NULL", false},
		"un-delete of the entity":    {"BEFORE UPDATE OF deleted_at ON entities WHEN NEW.deleted_at IS NULL", true},
		"save of the revived entity": {"BEFORE UPDATE OF name ON entities", true},
		"snapshot consumption":       {"BEFORE DELETE ON entity_deletion_snapshots", true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newEntityFixture(t)
			if c.restore {
				require.NoError(t, DeleteEntity(f.db, f.userID, f.org.ID, entityNow))
			}
			failStatement(t, f.db, "force_fail", c.trigger)
			var err error
			if c.restore {
				_, err = RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
			} else {
				err = DeleteEntity(f.db, f.userID, f.org.ID, entityNow)
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "forced failure")
		})
	}
}

func TestRestoreEntity_SnapshotMissingMeansNotInTrash(t *testing.T) {
	f := newEntityFixture(t)
	require.NoError(t, f.db.Delete(&f.org).Error) // soft-deleted without a snapshot
	_, err := RestoreEntity(f.db, f.userID, f.org.ID, entityNow)
	assert.ErrorIs(t, err, ErrEntityNotInTrash)
}

func TestReviveContactEntityLinks_StatementFailuresPropagate(t *testing.T) {
	f := newEntityFixture(t)
	l := f.link(t, f.c1, f.org, true)
	require.NoError(t, f.db.Delete(&l).Error)

	failStatement(t, f.db, "force_fail", "BEFORE UPDATE ON contact_entity_links")
	_, err := reviveContactEntityLinks(f.db, f.userID, []string{l.ID}, entityNow)
	require.Error(t, err, "the revive UPDATE failing is reported")
	assert.Contains(t, err.Error(), "forced failure")
	require.NoError(t, f.db.Exec("DROP TRIGGER force_fail").Error)

	dbtest.HideTable(t, f.db, "entities")
	_, err = reviveContactEntityLinks(f.db, f.userID, []string{l.ID}, entityNow)
	assert.Error(t, err, "the entity liveness check failing is reported")
}

func TestRestoreContact_LinkReviveFailureAbortsRestore(t *testing.T) {
	f := newEntityFixture(t)
	f.link(t, f.c1, f.org, false)
	require.NoError(t, DeleteContact(f.db, f.c1, f.userID, entityNow))
	failStatement(t, f.db, "force_fail", "BEFORE UPDATE ON contact_entity_links")
	_, err := RestoreContact(f.db, f.userID, f.c1.ID, entityNow)
	require.Error(t, err)
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ? AND deleted_at IS NOT NULL", f.c1.ID), "the contact stays in the trash")
}

// The revision/etag bump is a second UPDATE of the revived row. The revive map
// never sets revision, so a column-scoped trigger fails only the bump and
// reaches its error return rather than the earlier revive's.
func TestRestoreContact_LinkRevisionBumpFailureAbortsRestore(t *testing.T) {
	f := newEntityFixture(t)
	f.link(t, f.c1, f.org, false)
	require.NoError(t, DeleteContact(f.db, f.c1, f.userID, entityNow))
	failStatement(t, f.db, "force_fail_revision", "BEFORE UPDATE OF revision ON contact_entity_links")
	_, err := RestoreContact(f.db, f.userID, f.c1.ID, entityNow)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced failure")
	assert.EqualValues(t, 1, count(t, f.db, true, &models.Contact{}, "id = ? AND deleted_at IS NOT NULL", f.c1.ID), "the contact stays in the trash")
}

// Restore revives and may demote links via a bulk update; the revision/etag
// must increase so If-Match clients do not keep a stale ETag (issue #2566).
func TestRestoreContact_BumpsLinkRevisionOnReviveAndDemote(t *testing.T) {
	f := newEntityFixture(t)
	plain := f.link(t, f.c1, f.org, false)
	primary := f.link(t, f.c1, f.other, true)
	require.NoError(t, DeleteContact(f.db, f.c1, f.userID, entityNow))
	var deleted map[string]models.ContactEntityLink = map[string]models.ContactEntityLink{}
	for _, id := range []string{plain.ID, primary.ID} {
		var l models.ContactEntityLink
		require.NoError(t, f.db.Unscoped().First(&l, "id = ?", id).Error)
		deleted[id] = l
	}
	// Take the primary slot so the revived primary is demoted.
	third := models.Entity{UserID: f.userID, Kind: models.EntityKindOrganisation, Name: "Gamma"}
	require.NoError(t, f.db.Create(&third).Error)
	require.NoError(t, f.db.Create(&models.ContactEntityLink{UserID: f.userID, ContactVCardUID: f.c1.VCardUID, EntityID: third.ID,
		Kind: third.Kind, Role: "owner", IsPrimary: true, Sensitivity: "normal"}).Error)

	_, err := RestoreContact(f.db, f.userID, f.c1.ID, entityNow.Add(time.Hour))
	require.NoError(t, err)

	for _, id := range []string{plain.ID, primary.ID} {
		var got models.ContactEntityLink
		require.NoError(t, f.db.First(&got, "id = ?", id).Error)
		assert.Greater(t, got.Revision, deleted[id].Revision, "revived link %s revision must increase", id)
		assert.Equal(t, fmt.Sprintf("e-%s-%d", id, got.Revision), got.ETag)
	}
	var demoted models.ContactEntityLink
	require.NoError(t, f.db.First(&demoted, "id = ?", primary.ID).Error)
	assert.False(t, demoted.IsPrimary)
}
