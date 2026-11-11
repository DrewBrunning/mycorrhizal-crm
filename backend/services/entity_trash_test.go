package services

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func trashedEntity(t *testing.T, db *gorm.DB, userID uint, name string) models.Entity {
	t.Helper()
	e := models.Entity{UserID: userID, Kind: models.EntityKindOrganisation, Name: name}
	require.NoError(t, db.Create(&e).Error)
	require.NoError(t, DeleteEntity(db, userID, e.ID, time.Now()))
	return e
}

func entityTrashUser(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()
	u := models.User{Username: name, Password: "password123", Email: name + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u.ID
}

func TestListTrashedEntities_ScopesByUserAndOrdersNewestFirst(t *testing.T) {
	db := dbtest.New(t)
	uid, other := entityTrashUser(t, db, "a"), entityTrashUser(t, db, "b")
	first := trashedEntity(t, db, uid, "First")
	trashedEntity(t, db, other, "Theirs")
	second := models.Entity{UserID: uid, Kind: models.EntityKindPlace, Name: "Second"}
	require.NoError(t, db.Create(&second).Error)
	require.NoError(t, DeleteEntity(db, uid, second.ID, time.Now().Add(time.Hour)))

	got, total, err := ListTrashedEntities(db, uid, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, got, 2)
	assert.Equal(t, second.ID, got[0].Entity.ID, "newest deletion first")
	assert.Equal(t, first.ID, got[1].Entity.ID)
}

func TestListTrashedEntities_MergeLoserNamesSurvivor(t *testing.T) {
	db := dbtest.New(t)
	uid := entityTrashUser(t, db, "a")
	survivor := models.Entity{UserID: uid, Kind: models.EntityKindOrganisation, Name: "Acme"}
	loser := models.Entity{UserID: uid, Kind: models.EntityKindOrganisation, Name: "Acme Corp"}
	require.NoError(t, db.Create(&survivor).Error)
	require.NoError(t, db.Create(&loser).Error)
	require.NoError(t, deleteEntity(db, uid, loser.ID, time.Now(), survivor.ID))

	got, _, err := ListTrashedEntities(db, uid, 0, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, survivor.ID, got[0].MergedInto)
	assert.Equal(t, "Acme", got[0].MergedIntoName)

	// A survivor that is itself gone (purged) leaves the id but no name.
	require.NoError(t, db.Unscoped().Delete(&survivor).Error)
	got, _, err = ListTrashedEntities(db, uid, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, survivor.ID, got[0].MergedInto)
	assert.Empty(t, got[0].MergedIntoName)
}

func TestListTrashedEntities_UnsupportedOrCorruptPayloadStillListsWithoutMergeInfo(t *testing.T) {
	db := dbtest.New(t)
	uid := entityTrashUser(t, db, "a")
	newer := trashedEntity(t, db, uid, "Newer")
	corrupt := trashedEntity(t, db, uid, "Corrupt")
	require.NoError(t, db.Model(&models.EntityDeletionSnapshot{}).Where("entity_id = ?", newer.ID).Update("payload_version", 99).Error)
	require.NoError(t, db.Model(&models.EntityDeletionSnapshot{}).Where("entity_id = ?", corrupt.ID).Update("payload", "not json").Error)

	got, total, err := ListTrashedEntities(db, uid, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total, "restore reports the unsupported version; the row is still visible")
	for _, g := range got {
		assert.Empty(t, g.MergedInto)
	}
}

func TestListTrashedEntities_QueryFailuresSurface(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table string
		nth   int32
	}{
		{"count", "entities", 1},
		{"page", "entities", 2},
		{"snapshot", "entity_deletion_snapshots", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.New(t)
			uid := entityTrashUser(t, db, "a")
			trashedEntity(t, db, uid, "Acme")
			failNthQuery(t, db, tc.table, tc.nth)
			_, _, err := ListTrashedEntities(db, uid, 0, 10)
			assert.Error(t, err)
		})
	}
}
