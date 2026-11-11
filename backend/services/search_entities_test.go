package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2472 (part of #2174): reusable entities are a fourth search section.

func entUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{Username: name, Password: "password123!A", Email: name + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func entMake(t *testing.T, db *gorm.DB, userID uint, kind, name string) models.Entity {
	t.Helper()
	e := models.Entity{UserID: userID, Kind: kind, Name: name}
	require.NoError(t, db.Create(&e).Error)
	return e
}

func entNames(hits []SearchEntityHit) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h.Name)
	}
	return out
}

func TestSearchEntities_FindsByNameAllKinds(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-find")
	org := entMake(t, db, u.ID, models.EntityKindOrganisation, "Quokka Holdings")
	entMake(t, db, u.ID, models.EntityKindPlace, "Quokka Cottage")
	entMake(t, db, u.ID, models.EntityKindPet, "Quokka")
	entMake(t, db, u.ID, models.EntityKindPet, "Unrelated")

	res, err := Search(db, u.ID, "quokk", 0, nil)
	require.NoError(t, err)
	require.Len(t, res.Entities, 3)
	kinds := map[string]bool{}
	for _, h := range res.Entities {
		kinds[h.Kind] = true
		assert.Equal(t, models.RelationshipSensitivityNormal, h.Sensitivity)
		assert.NotEmpty(t, h.ID)
	}
	assert.Len(t, kinds, 3)

	res, err = Search(db, u.ID, "QUOKKA hold", 0, nil)
	require.NoError(t, err)
	require.Len(t, res.Entities, 1)
	assert.Equal(t, org.ID, res.Entities[0].ID)
	assert.Equal(t, "Quokka Holdings", res.Entities[0].Name)
}

func TestSearchEntities_NFCQueryMatchesNFDInput(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-nfc")
	entMake(t, db, u.ID, models.EntityKindOrganisation, "Café Zeta") // stored NFC by BeforeSave
	res, err := Search(db, u.ID, "café zeta", 0, nil)               // NFD query
	require.NoError(t, err)
	assert.Len(t, res.Entities, 1)
}

func TestSearchEntities_CrossUserReturnsNothing(t *testing.T) {
	db := dbtest.New(t)
	a, b := entUser(t, db, "ent-a"), entUser(t, db, "ent-b")
	entMake(t, db, a.ID, models.EntityKindOrganisation, "Shared Name Ltd")
	entMake(t, db, b.ID, models.EntityKindOrganisation, "Shared Name Ltd")
	ra, err := Search(db, a.ID, "shared", 0, nil)
	require.NoError(t, err)
	require.Len(t, ra.Entities, 1)
	rb, err := Search(db, b.ID, "shared", 0, nil)
	require.NoError(t, err)
	require.Len(t, rb.Entities, 1)
	assert.NotEqual(t, ra.Entities[0].ID, rb.Entities[0].ID)
	c := entUser(t, db, "ent-c")
	rc, err := Search(db, c.ID, "shared", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, rc.Entities)
}

func TestSearchEntities_SoftDeletedMergedAndRestored(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-life")
	gone := entMake(t, db, u.ID, models.EntityKindOrganisation, "Vanish Corp")
	loser := entMake(t, db, u.ID, models.EntityKindOrganisation, "Mergeable Inc")
	survivor := entMake(t, db, u.ID, models.EntityKindOrganisation, "Mergeable Incorporated")
	now := time.Now()

	require.NoError(t, DeleteEntity(db, u.ID, gone.ID, now))
	res, err := Search(db, u.ID, "vanish", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, res.Entities, "a soft-deleted entity is not findable")

	_, err = MergeEntities(db, u.ID, loser.ID, survivor.ID, nil, now)
	require.NoError(t, err)
	res, err = Search(db, u.ID, "mergeable", 0, nil)
	require.NoError(t, err)
	require.Len(t, res.Entities, 1, "the merged-away loser drops out; the survivor stays")
	assert.Equal(t, survivor.ID, res.Entities[0].ID)

	_, err = RestoreEntity(db, u.ID, gone.ID, now)
	require.NoError(t, err)
	res, err = Search(db, u.ID, "vanish", 0, nil)
	require.NoError(t, err)
	assert.Len(t, res.Entities, 1, "a restored entity is findable again")
}

func TestSearchEntities_RenameAndHardDelete(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-rename")
	e := entMake(t, db, u.ID, models.EntityKindPlace, "Oldname House")
	e.Name = "Newname House"
	require.NoError(t, db.Save(&e).Error)
	res, _ := Search(db, u.ID, "oldname", 0, nil)
	assert.Empty(t, res.Entities)
	res, _ = Search(db, u.ID, "newname", 0, nil)
	assert.Len(t, res.Entities, 1)
	require.NoError(t, db.Unscoped().Delete(&e).Error)
	res, _ = Search(db, u.ID, "newname", 0, nil)
	assert.Empty(t, res.Entities)
	assertFTSCanonical(t, db)
}

func TestSearchEntities_LimitAndOffsetApplyPerSection(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-limit")
	for _, n := range []string{"Limitco A", "Limitco B", "Limitco C"} {
		entMake(t, db, u.ID, models.EntityKindOrganisation, n)
	}
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Limitco", Lastname: "One"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Limitco", Lastname: "Two"}).Error)

	res, err := Search(db, u.ID, "limitco", 2, nil)
	require.NoError(t, err)
	assert.Len(t, res.Entities, 2)
	assert.Len(t, res.Contacts, 2)

	res, err = SearchPage(db, u.ID, "limitco", 2, 2, nil)
	require.NoError(t, err)
	assert.Len(t, res.Entities, 1, "offset applies to the entities section")

	res, err = Search(db, u.ID, "limitco", 0, nil)
	require.NoError(t, err)
	assert.Len(t, res.Entities, 3)
	assert.Equal(t, []string{"Limitco A", "Limitco B", "Limitco C"}, entNames(res.Entities), "equal ranks are ordered by name")
}

// Only the plaintext name column is indexed: core text (encrypted at rest),
// custom field values and the other sensitivity-bearing content are not
// findable, matching TestSearch_SecretSensitivityNotFindable.
func TestSearchEntities_OnlyNameIsIndexedAndSecretIsOwnerVisible(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-sens")
	e := models.Entity{UserID: u.ID, Kind: models.EntityKindPet, Name: "Biscuit", Sensitivity: models.RelationshipSensitivitySecret}
	require.NoError(t, e.SetCore(&models.PetCore{Species: "axolotl", Breed: "leucistic", Notes: "hidden-codeword"}))
	require.NoError(t, db.Create(&e).Error)
	def := models.FieldDefinition{UserID: u.ID, Label: "Secret", Key: "secret", Target: "pet", Type: "text", Sensitivity: models.RelationshipSensitivitySecret}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: def.ID, UserID: u.ID, EntityID: e.ID, Value: []byte(`"field-codeword"`)}).Error)

	for _, term := range []string{"axolotl", "leucistic", "hidden-codeword", "field-codeword"} {
		res, err := Search(db, u.ID, term, 0, nil)
		require.NoError(t, err)
		assert.Empty(t, res.Entities, "%q must not make an entity findable", term)
	}
	res, err := Search(db, u.ID, "biscuit", 0, nil)
	require.NoError(t, err)
	require.Len(t, res.Entities, 1, "the owner finds a secret entity by its name")
	assert.Equal(t, models.RelationshipSensitivitySecret, res.Entities[0].Sensitivity)
}

func TestSearchEntities_ShortAndNulTermsReturnEmptySliceNotNull(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-short")
	entMake(t, db, u.ID, models.EntityKindPet, "Xy")
	for _, term := range []string{"x", "\x00", "\x00\x00", ""} {
		res, err := Search(db, u.ID, term, 0, nil)
		require.NoError(t, err, "%q", term)
		require.NotNil(t, res.Entities)
		assert.Empty(t, res.Entities)
	}
}

// MCP (ADR 0043 §4): the scoped search used by the MCP tool never returns entities.
func TestSearchPageScoped_DoesNotReturnEntities(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-mcp")
	entMake(t, db, u.ID, models.EntityKindOrganisation, "Mcpcorp")
	res, err := SearchPageScoped(db, u.ID, "mcpcorp", 10, 0, nil, false)
	require.NoError(t, err)
	assert.Empty(t, res.Entities)
}

func TestSearchEntities_RebuildRepopulatesAndCounts(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-rebuild")
	entMake(t, db, u.ID, models.EntityKindOrganisation, "Rebuildco")
	dead := entMake(t, db, u.ID, models.EntityKindOrganisation, "Rebuilddead")
	require.NoError(t, db.Delete(&dead).Error)
	require.NoError(t, db.Exec("DELETE FROM entities_fts").Error)
	require.NoError(t, db.Exec(`INSERT INTO entities_fts(name, entity_id, user_id, kind) VALUES ('Orphan', 'no-such', ?, 'pet')`, u.ID).Error)

	stats, err := RebuildSearchIndexReport(db)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.Entities)
	assert.Equal(t, stats.Contacts+stats.Notes+stats.Activities+1, stats.Total())
	res, err := Search(db, u.ID, "rebuild", 0, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"Rebuildco"}, entNames(res.Entities))
	var orphans int64
	require.NoError(t, db.Raw("SELECT count(*) FROM entities_fts WHERE entity_id = 'no-such'").Scan(&orphans).Error)
	assert.Zero(t, orphans)
	assertFTSCanonical(t, db)
}

func TestEntitiesFTSConsistency_DetectsEachClass(t *testing.T) {
	setup := func(t *testing.T) (*gorm.DB, models.User, models.Entity) {
		db := dbtest.New(t)
		u := entUser(t, db, "ent-probe")
		e := entMake(t, db, u.ID, models.EntityKindOrganisation, "Probeco")
		res, err := CheckSearchIndexConsistency(db)
		require.NoError(t, err)
		require.True(t, res.Clean(), res.Summary())
		return db, u, e
	}
	classes := func(t *testing.T, db *gorm.DB) map[string]FTSDivergence {
		res, err := CheckSearchIndexConsistency(db)
		require.NoError(t, err)
		out := map[string]FTSDivergence{}
		for _, d := range res.Divergences {
			require.Equal(t, "entities_fts", d.Index)
			out[d.Class] = d
		}
		return out
	}
	t.Run("missing", func(t *testing.T) {
		db, _, e := setup(t)
		require.NoError(t, db.Exec("DELETE FROM entities_fts WHERE entity_id = ?", e.ID).Error)
		d := classes(t, db)[FTSDivergenceMissingFromIndex]
		assert.Equal(t, e.ID, d.EntityID)
	})
	t.Run("orphan", func(t *testing.T) {
		db, u, _ := setup(t)
		require.NoError(t, db.Exec(`INSERT INTO entities_fts(name, entity_id, user_id, kind) VALUES ('X', 'ghost', ?, 'pet')`, u.ID).Error)
		assert.Equal(t, "ghost", classes(t, db)[FTSDivergenceOrphanInIndex].EntityID)
	})
	t.Run("content", func(t *testing.T) {
		db, _, e := setup(t)
		require.NoError(t, db.Exec("UPDATE entities_fts SET name = 'Stale' WHERE entity_id = ?", e.ID).Error)
		d := classes(t, db)[FTSDivergenceContentMismatch]
		assert.Equal(t, e.ID, d.EntityID)
		assert.Equal(t, []string{"name"}, d.Columns)
	})
	t.Run("scope", func(t *testing.T) {
		db, _, e := setup(t)
		require.NoError(t, db.Exec("UPDATE entities_fts SET user_id = 9999 WHERE entity_id = ?", e.ID).Error)
		assert.Equal(t, e.ID, classes(t, db)[FTSDivergenceScopeMismatch].EntityID)
	})
	t.Run("soft-deleted is not a divergence", func(t *testing.T) {
		db, _, e := setup(t)
		require.NoError(t, db.Delete(&e).Error)
		assert.Empty(t, classes(t, db))
	})
}

func TestEntitiesFTS_RebuildMatchesIncremental(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-prop")
	a := entMake(t, db, u.ID, models.EntityKindOrganisation, "Propa")
	entMake(t, db, u.ID, models.EntityKindPlace, "Propb")
	c := entMake(t, db, u.ID, models.EntityKindPet, "Propc")
	a.Name = "Propa Renamed"
	require.NoError(t, db.Save(&a).Error)
	require.NoError(t, db.Delete(&c).Error)
	snap := func() string {
		var s string
		require.NoError(t, db.Raw(`SELECT COALESCE(group_concat(entity_id || '|' || user_id || '|' || kind || '|' || name, ';'), '') FROM (SELECT * FROM entities_fts ORDER BY entity_id)`).Scan(&s).Error)
		return s
	}
	before := snap()
	require.NotEmpty(t, before)
	_, err := RebuildSearchIndexReport(db)
	require.NoError(t, err)
	assert.Equal(t, before, snap())
}

// failOnSQL makes every raw statement containing fragment fail until the
// returned cleanup runs.
func failOnSQL(t *testing.T, db *gorm.DB, name, fragment string) {
	t.Helper()
	fail := func(d *gorm.DB) {
		if strings.Contains(d.Statement.SQL.String(), fragment) {
			_ = d.AddError(fmt.Errorf("injected failure"))
		}
	}
	require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(name, fail))
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(name, fail))
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register(name, fail))
	t.Cleanup(func() {
		_ = db.Callback().Row().Remove(name)
		_ = db.Callback().Raw().Remove(name)
		_ = db.Callback().Query().Remove(name)
	})
}

func TestSearchEntities_QueryErrorsSurface(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-err")
	failOnSQL(t, db, "fail_entities_search", "FROM entities_fts f")
	_, err := Search(db, u.ID, "anything", 0, nil)
	require.ErrorContains(t, err, "search entities")
}

func TestEntitiesFTSConsistency_QueryErrorsSurface(t *testing.T) {
	for i, fragment := range []string{
		"LEFT JOIN entities_fts f",            // missing
		"LEFT JOIN entities b",                // orphan
		"FROM entities_fts f JOIN entities b", // scope
		"AS m_name",                           // content
	} {
		db := dbtest.New(t)
		failOnSQL(t, db, fmt.Sprintf("fail_probe_%d", i), fragment)
		_, err := CheckSearchIndexConsistency(db)
		require.ErrorContains(t, err, "fts consistency (entities_fts", fragment)
	}
}

func TestEntitiesFTSConsistency_CapsEachClass(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-cap")
	for i := 0; i < ftsDivergenceCapPerClass+5; i++ {
		require.NoError(t, db.Exec(`INSERT INTO entities_fts(name, entity_id, user_id, kind) VALUES ('X', ?, ?, 'pet')`, fmt.Sprintf("ghost-%03d", i), u.ID).Error)
	}
	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.True(t, res.Truncated)
	n := 0
	for _, d := range res.Divergences {
		if d.Class == FTSDivergenceOrphanInIndex {
			n++
		}
	}
	assert.Equal(t, ftsDivergenceCapPerClass, n)

	// Content drift over the cap is capped too.
	db2 := dbtest.New(t)
	u2 := entUser(t, db2, "ent-cap2")
	for i := 0; i < ftsDivergenceCapPerClass+2; i++ {
		entMake(t, db2, u2.ID, models.EntityKindPet, fmt.Sprintf("Cap%03d", i))
	}
	require.NoError(t, db2.Exec("UPDATE entities_fts SET name = 'drift', kind = 'place'").Error)
	res, err = CheckSearchIndexConsistency(db2)
	require.NoError(t, err)
	assert.True(t, res.Truncated)
	for _, d := range res.Divergences {
		if d.Class == FTSDivergenceContentMismatch {
			assert.Equal(t, []string{"name", "kind"}, d.Columns)
			break
		}
	}
}

func TestSearchPage_NegativeOffsetIsZeroAndScopedErrorsPropagate(t *testing.T) {
	db := dbtest.New(t)
	u := entUser(t, db, "ent-off")
	entMake(t, db, u.ID, models.EntityKindPet, "Offsetty")
	res, err := SearchPage(db, u.ID, "offsetty", 5, -3, nil)
	require.NoError(t, err)
	assert.Len(t, res.Entities, 1)

	failOnSQL(t, db, "fail_contacts_search", "FROM contacts_fts")
	_, err = SearchPage(db, u.ID, "offsetty", 5, 0, nil)
	require.ErrorContains(t, err, "search contacts")
}
