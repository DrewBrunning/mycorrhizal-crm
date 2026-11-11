package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GET /trash/entities (issue #2446, ADR 0043 decision 6): the owner's
// soft-deleted entities that can still be restored, restored through the
// existing POST /entities/:id/restore.

func newEntityTrashEnv(t *testing.T) *mergeEnv {
	t.Helper()
	e := newMergeEnv(t)
	e.router.GET("/trash/entities", ListTrashedEntities)
	e.router.GET("/trash/contacts", ListTrashedContacts)
	return e
}

func (e *mergeEnv) trashEntities(t *testing.T, query string) models.TrashEntityListResponse {
	t.Helper()
	w := e.do(http.MethodGet, "/trash/entities"+query, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list models.TrashEntityListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	return list
}

func TestTrashEntities_ListsDeletedEntityAndMergeLoser(t *testing.T) {
	e := newEntityTrashEnv(t)
	gone := e.org(t, "Gone Inc")
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	live := e.org(t, "Still Here")
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+gone.ID, nil).Code)
	e.mergeOK(t, loser.ID, survivor.ID, nil)

	list := e.trashEntities(t, "")
	require.EqualValues(t, 2, list.Total)
	byID := map[string]models.TrashedEntity{}
	for _, it := range list.Entities {
		byID[it.Entity.ID] = it
	}
	require.Contains(t, byID, gone.ID)
	require.Contains(t, byID, loser.ID)
	assert.NotContains(t, byID, live.ID, "live entity is not trashed")
	assert.NotContains(t, byID, survivor.ID)
	assert.Equal(t, "organisation", byID[gone.ID].Entity.Kind)
	assert.Equal(t, "Gone Inc", byID[gone.ID].Entity.Name)
	assert.Empty(t, byID[gone.ID].MergedInto)
	assert.Equal(t, survivor.ID, byID[loser.ID].MergedInto)
	assert.Equal(t, "Acme", byID[loser.ID].MergedIntoName, "survivor name shown without a second fetch")
	assert.False(t, byID[gone.ID].DeletedAt.IsZero())
}

func TestTrashEntities_EmptyIsArrayAndRetentionReported(t *testing.T) {
	e := newEntityTrashEnv(t)
	w := e.do(http.MethodGet, "/trash/entities", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["entities"]), "empty collection is [] not null (frontend trap #8)")
	assert.Contains(t, raw, "retention_days")
}

func TestTrashEntities_PurgeAtFollowsRetention(t *testing.T) {
	for _, tc := range []struct {
		days    int
		wantNil bool
	}{{30, false}, {0, true}} {
		db, router := setupRouterWithRetention(t, tc.days)
		var user models.User
		require.NoError(t, db.First(&user).Error)
		router.GET("/trash/entities", ListTrashedEntities)
		ent := models.Entity{UserID: user.ID, Kind: models.EntityKindOrganisation, Name: "Acme"}
		require.NoError(t, db.Create(&ent).Error)
		require.NoError(t, services.DeleteEntity(db, user.ID, ent.ID, time.Now()))

		w := trashDo(router, http.MethodGet, "/trash/entities")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var list models.TrashEntityListResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		require.Len(t, list.Entities, 1)
		assert.Equal(t, tc.days, list.RetentionDays)
		if tc.wantNil {
			assert.Nil(t, list.Entities[0].PurgeAt, "retention 0 keeps forever")
			continue
		}
		require.NotNil(t, list.Entities[0].PurgeAt)
		assert.True(t, list.Entities[0].PurgeAt.After(list.Entities[0].DeletedAt))
	}
}

func TestTrashEntities_PaginatesNewestFirst(t *testing.T) {
	e := newEntityTrashEnv(t)
	var ids []string
	for i := 0; i < 3; i++ {
		o := e.org(t, fmt.Sprintf("Org %d", i))
		require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+o.ID, nil).Code)
		ids = append(ids, o.ID)
	}
	p1 := e.trashEntities(t, "?page=1&limit=2")
	p2 := e.trashEntities(t, "?page=2&limit=2")
	assert.EqualValues(t, 3, p1.Total)
	assert.Len(t, p1.Entities, 2)
	assert.Len(t, p2.Entities, 1)
	assert.Equal(t, 2, p2.Page)
	seen := map[string]bool{}
	for _, it := range append(p1.Entities, p2.Entities...) {
		seen[it.Entity.ID] = true
	}
	assert.Len(t, seen, 3, "every trashed entity appears exactly once across pages")
	for _, id := range ids {
		assert.True(t, seen[id])
	}
}

// Ownership: another user's deleted entity never appears, never counts, and
// cannot be restored by the caller.
func TestTrashEntities_OwnershipIsolation(t *testing.T) {
	e := newEntityTrashEnv(t)
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	foreign := models.Entity{UserID: other.ID, Kind: models.EntityKindOrganisation, Name: "Theirs"}
	require.NoError(t, e.db.Create(&foreign).Error)
	require.NoError(t, e.db.Delete(&foreign).Error)
	require.NoError(t, e.db.Create(&models.EntityDeletionSnapshot{UserID: other.ID, EntityID: foreign.ID, DeletionBatchID: "b", PayloadVersion: models.EntityDeletionSnapshotVersion, Payload: `{"field_values":[]}`}).Error)
	mine := e.org(t, "Mine")
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+mine.ID, nil).Code)

	list := e.trashEntities(t, "")
	require.Len(t, list.Entities, 1)
	assert.EqualValues(t, 1, list.Total)
	assert.Equal(t, mine.ID, list.Entities[0].Entity.ID)

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/entities/"+foreign.ID+"/restore", nil).Code)
	var still models.Entity
	require.NoError(t, e.db.Unscoped().First(&still, "id = ?", foreign.ID).Error)
	assert.True(t, still.DeletedAt.Valid, "foreign tombstone untouched")
}

func TestTrashEntities_OmitsTombstoneWithoutSnapshot(t *testing.T) {
	e := newEntityTrashEnv(t)
	org := e.org(t, "Legacy")
	require.NoError(t, e.db.Delete(&models.Entity{}, "id = ?", org.ID).Error)
	assert.Empty(t, e.trashEntities(t, "").Entities, "not restorable, so not listed (same rule as contacts)")
}

// Restoring from Trash returns the entity, its links and its field values to
// their previous state and removes it from the list.
func TestTrashEntities_RestoreFromTrashReturnsLinksAndValues(t *testing.T) {
	e := newEntityTrashEnv(t)
	org := e.org(t, "Acme")
	c := e.contact(t, nil)
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": true})
	def := models.FieldDefinition{UserID: e.userID, Label: "Size", Key: "size", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&def).Error)
	require.Equal(t, http.StatusOK, e.do(http.MethodPut, "/entities/"+org.ID+"/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": def.ID, "value": "50"}}}).Code)
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+org.ID, nil).Code)
	require.Len(t, e.trashEntities(t, "").Entities, 1)

	w := e.do(http.MethodPost, "/entities/"+org.ID+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Empty(t, e.trashEntities(t, "").Entities)
	assert.Zero(t, e.trashEntities(t, "").Total)
	live := e.liveLinks(t, org.ID)
	require.Len(t, live, 1)
	assert.Equal(t, l.ID, live[0].ID)
	assert.True(t, live[0].IsPrimary)
	var vals []models.FieldValue
	require.NoError(t, e.db.Where("entity_id = ?", org.ID).Find(&vals).Error)
	require.Len(t, vals, 1)
	assert.JSONEq(t, `"50"`, string(vals[0].Value))
}

// A merge loser restored from Trash behaves as the in-page restore: only the
// entity returns, its re-pointed links stay with the survivor.
func TestTrashEntities_RestoreMergeLoserKeepsLinksWithSurvivor(t *testing.T) {
	e := newEntityTrashEnv(t)
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	c := e.contact(t, nil)
	l := e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "employee"})
	e.mergeOK(t, loser.ID, survivor.ID, nil)
	require.Len(t, e.trashEntities(t, "").Entities, 1)

	w := e.do(http.MethodPost, "/entities/"+loser.ID+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var r models.EntityRestoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	assert.Equal(t, survivor.ID, r.MergedInto)
	assert.Equal(t, survivor.ID, e.link(t, l.ID).EntityID)
	assert.Empty(t, e.trashEntities(t, "").Entities)
}

// Regression: entities never leak into the contact trash and the contact list
// keeps its shape.
func TestTrashEntities_ContactTrashUnchanged(t *testing.T) {
	e := newEntityTrashEnv(t)
	org := e.org(t, "Acme")
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+org.ID, nil).Code)
	w := e.do(http.MethodGet, "/trash/contacts", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.JSONEq(t, `[]`, string(raw["contacts"]))
	assert.NotContains(t, raw, "entities")
	for _, k := range []string{"contacts", "total", "page", "limit", "retention_days"} {
		assert.Contains(t, raw, k)
	}
}

func TestTrashEntities_DatabaseFailureIs500(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs: Request error")
	e := newEntityTrashEnv(t)
	dbtest.HideTable(t, e.db, "entity_deletion_snapshots")
	w := e.do(http.MethodGet, "/trash/entities", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestTrashEntities_RequiresAuthenticatedUser(t *testing.T) {
	db := dbtest.New(t)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	router.GET("/trash/entities", ListTrashedEntities)
	assert.Equal(t, http.StatusUnauthorized, trashDo(router, http.MethodGet, "/trash/entities").Code)
}
