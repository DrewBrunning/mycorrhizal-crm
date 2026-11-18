package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2743: GET /entities items and GET /entities/suggestions pair sides
// carry link_count (distinct live contacts linked, the "Linked to N contacts"
// definition) and updated_at, computed in one extra query per response.

func newCountEnv(t *testing.T) *mergeEnv {
	t.Helper()
	e := newMergeEnv(t)
	e.router.GET("/entities", ListEntities)
	return e
}

type countedEntity struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
	LinkCount *int      `json:"link_count"`
}

func (e *mergeEnv) listEntities(t *testing.T, query string) []countedEntity {
	t.Helper()
	w := e.do(http.MethodGet, "/entities?"+query, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Entities []countedEntity `json:"entities"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.Entities
}

// twoAcmes builds two same-named organisations: the first has three link rows
// but only two distinct contacts (one contact holds two roles), the second one.
func twoAcmes(t *testing.T, e *mergeEnv) (a, b models.EntityResponse) {
	t.Helper()
	a, b = e.org(t, "Acme"), e.org(t, "Acme")
	c1, c2, c3 := e.contact(t, nil), e.contact(t, func(c *models.Contact) { c.Firstname = "Grace" }), e.contact(t, func(c *models.Contact) { c.Firstname = "Alan" })
	e.addLink(t, c1, map[string]any{"entity_id": a.ID, "role": "employee"})
	e.addLink(t, c1, map[string]any{"entity_id": a.ID, "role": "owner"})
	e.addLink(t, c2, map[string]any{"entity_id": a.ID, "role": "employee"})
	e.addLink(t, c3, map[string]any{"entity_id": b.ID, "role": "employee"})
	return a, b
}

func TestEntitySuggestions_SidesCarryLinkCountAndUpdatedAt(t *testing.T) {
	e := newCountEnv(t)
	a, b := twoAcmes(t, e)
	lonely := e.org(t, "Acme")
	_ = lonely

	res := e.suggestions(t, "kind=organisation")
	require.NotEmpty(t, res.Pairs)
	want := map[string]int{a.ID: 2, b.ID: 1, lonely.ID: 0}
	for _, p := range res.Pairs {
		for _, side := range []models.EntitySuggestionEntity{p.A, p.B} {
			assert.Equal(t, want[side.ID], side.LinkCount, "link_count of %s", side.ID)
			assert.False(t, side.UpdatedAt.IsZero(), "updated_at of %s", side.ID)
		}
	}
	// Same-named duplicates are now distinguishable by their counts.
	var counts []int
	for _, p := range res.Pairs {
		if (p.A.ID == a.ID && p.B.ID == b.ID) || (p.A.ID == b.ID && p.B.ID == a.ID) {
			counts = []int{p.A.LinkCount, p.B.LinkCount}
		}
	}
	assert.ElementsMatch(t, []int{2, 1}, counts)

	// Raw JSON: a zero count is present, not omitted.
	w := e.do(http.MethodGet, "/entities/suggestions?kind=organisation", nil)
	assert.Contains(t, w.Body.String(), `"link_count":0`)
	assert.Contains(t, w.Body.String(), `"updated_at":"`)
}

func TestEntityList_ItemsCarryLinkCountAndUpdatedAt(t *testing.T) {
	e := newCountEnv(t)
	a, b := twoAcmes(t, e)
	lonely := e.org(t, "Zed")

	items := e.listEntities(t, "")
	byID := map[string]countedEntity{}
	for _, it := range items {
		byID[it.ID] = it
	}
	require.Len(t, byID, 3)
	for id, want := range map[string]int{a.ID: 2, b.ID: 1, lonely.ID: 0} {
		it := byID[id]
		require.NotNil(t, it.LinkCount, "link_count must be present on every list item")
		assert.Equal(t, want, *it.LinkCount, id)
		assert.False(t, it.UpdatedAt.IsZero())
	}
	assert.Equal(t, a.UpdatedAt.UTC(), byID[a.ID].UpdatedAt.UTC(), "updated_at is the entity's own")
}

func TestEntityCounts_ExcludeDeletedContactsAndOtherUsersLinks(t *testing.T) {
	e := newCountEnv(t)
	org := e.org(t, "Acme")
	live := e.contact(t, nil)
	gone := e.contact(t, func(c *models.Contact) { c.Firstname = "Gone" })
	e.addLink(t, live, map[string]any{"entity_id": org.ID, "role": "employee"})
	e.addLink(t, gone, map[string]any{"entity_id": org.ID, "role": "employee"})
	// A soft-deleted contact no longer counts (its link row may linger).
	require.NoError(t, e.db.Delete(&models.Contact{}, gone.ID).Error)
	// A link row from another user's contact naming this entity id (never
	// creatable through the API) must not be counted: counts are owner scoped.
	other, otherContact, _, _ := e.foreign(t)
	require.NoError(t, e.db.Create(&models.ContactEntityLink{
		ID: "11111111-1111-4111-8111-111111111111", UserID: other.ID, ContactVCardUID: otherContact.VCardUID,
		EntityID: org.ID, Kind: models.EntityKindOrganisation, Role: "employee", Sensitivity: "normal",
	}).Error)

	items := e.listEntities(t, "")
	require.Len(t, items, 1)
	require.NotNil(t, items[0].LinkCount)
	assert.Equal(t, 1, *items[0].LinkCount)
}

func TestEntityCounts_AreOneQueryNotPerEntity(t *testing.T) {
	// Issue #2743: no N+1. The list and the suggestions page issue the same
	// number of queries for 4 entities as for 40.
	dbPath := filepath.Join(t.TempDir(), "entity-counts.db")
	db := dbtest.NewAt(t, dbPath)
	user := models.User{Username: "counts", Password: "password123!A", Email: "counts@example.com"}
	require.NoError(t, db.Create(&user).Error)
	counter := database.NewQueryCounter()
	countDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: counter})
	require.NoError(t, err)
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", countDB)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{ProfilePhotoDir: t.TempDir()})
		c.Next()
	})
	r.GET("/entities", ListEntities)
	r.GET("/entities/suggestions", ListEntitySuggestions)

	seed := func(from, to int) {
		for i := from; i < to; i++ {
			ent := models.Entity{UserID: user.ID, Kind: models.EntityKindOrganisation, Name: "Acme"}
			require.NoError(t, db.Create(&ent).Error)
			c := models.Contact{UserID: user.ID, Firstname: fmt.Sprintf("C%d", i), Lastname: "X"}
			require.NoError(t, db.Create(&c).Error)
			require.NoError(t, db.Create(&models.ContactEntityLink{
				UserID: user.ID, ContactVCardUID: c.VCardUID, EntityID: ent.ID,
				Kind: models.EntityKindOrganisation, Role: "employee", Sensitivity: "normal",
			}).Error)
		}
	}
	measure := func(path string) int64 {
		counter.Reset()
		w := doPerfGet(t, r, path)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return counter.Count()
	}

	seed(0, 4)
	listSmall, sugSmall := measure("/entities"), measure("/entities/suggestions?kind=organisation&limit=100")
	seed(4, 40)
	listBig, sugBig := measure("/entities"), measure("/entities/suggestions?kind=organisation&limit=100")
	assert.Equal(t, listSmall, listBig, "GET /entities query count must not grow with the number of entities")
	assert.Equal(t, sugSmall, sugBig, "GET /entities/suggestions query count must not grow with the number of pairs")
	assert.LessOrEqual(t, listBig, int64(3))
	assert.LessOrEqual(t, sugBig, int64(5))
}

func TestEntityCounts_DatabaseFailureIsAServerError(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	e := newCountEnv(t)
	twoAcmes(t, e)
	dbtest.HideTable(t, e.db, "contact_entity_links")
	assert.Equal(t, http.StatusInternalServerError, e.do(http.MethodGet, "/entities", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, e.do(http.MethodGet, "/entities/suggestions?kind=organisation", nil).Code)
}
