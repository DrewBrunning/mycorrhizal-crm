package controllers

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/clock"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contact-entity link CRUD, primary/period rules, adopt and link-from-existing
// (issue #2297, ADR 0043 decisions 2, 3, 6 and 10) against the real migrated
// schema with the real validation middleware.

type linkEnv struct {
	*entityEnv
	clk *clock.Fake
}

func newLinkEnv(t *testing.T) *linkEnv {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	clk := clock.NewFake(time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC))
	router.Use(func(c *gin.Context) { clock.Install(c, clk); c.Next() })
	// The clock middleware must precede the routes; gin applies Use only to
	// routes registered afterwards.
	router.POST("/entities", middleware.ValidateJSONMiddleware(&models.EntityInput{}), CreateEntity)
	router.GET("/entities/:id/contacts", ListEntityContacts)
	router.DELETE("/entities/:id", DeleteEntity)
	router.GET("/contacts", GetContacts)
	router.GET("/contacts/:id/entity-links", ListContactEntityLinks)
	router.POST("/contacts/:id/entity-links", middleware.ValidateJSONMiddleware(&models.ContactEntityLinkInput{}), CreateContactEntityLink)
	router.POST("/contacts/:id/entity-links/from-existing", middleware.ValidateJSONMiddleware(&models.ContactEntityLinkFromExistingInput{}), LinkContactEntityFromExisting)
	router.PUT("/contacts/:id/entity-links/:linkId", middleware.ValidateJSONMiddleware(&models.ContactEntityLinkInput{}), UpdateContactEntityLink)
	router.DELETE("/contacts/:id/entity-links/:linkId", DeleteContactEntityLink)
	router.POST("/contacts/:id/entity-links/:linkId/adopt", AdoptContactEntityLink)
	return &linkEnv{entityEnv: &entityEnv{db: db, router: router, userID: user.ID}, clk: clk}
}

func (e *linkEnv) contact(t *testing.T, mutate func(c *models.Contact)) models.Contact {
	t.Helper()
	c := models.Contact{UserID: e.userID, Firstname: "Ada", Lastname: "Lovelace"}
	if mutate != nil {
		mutate(&c)
	}
	if !reflect.DeepEqual(c.Card, contactmodel.Card{}) {
		// Trap #2: a Card set by direct field mutation does not persist.
		first, last := c.Firstname, c.Lastname
		models.ApplyRecordToContact(&c, &contactmodel.Record{Card: c.Card}, "")
		c.Firstname, c.Lastname = first, last
	}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *linkEnv) org(t *testing.T, name string) models.EntityResponse {
	t.Helper()
	return e.create(t, map[string]any{"kind": "organisation", "name": name,
		"core": map[string]any{"organization": map[string]any{"name": name, "units": []any{map[string]any{"name": "R&D"}}}}})
}

func (e *linkEnv) place(t *testing.T, name string) models.EntityResponse {
	t.Helper()
	return e.create(t, map[string]any{"kind": "place", "name": name,
		"core": map[string]any{"address": map[string]any{"components": []any{
			map[string]any{"kind": "name", "value": "1 Main St"}, map[string]any{"kind": "locality", "value": "Springfield"}}}}})
}

func linkPath(c models.Contact, rest ...string) string {
	p := fmt.Sprintf("/contacts/%d/entity-links", c.ID)
	for _, r := range rest {
		p += "/" + r
	}
	return p
}

func (e *linkEnv) addLink(t *testing.T, c models.Contact, body map[string]any) models.ContactEntityLinkResponse {
	t.Helper()
	w := e.do(http.MethodPost, linkPath(c), body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var r models.ContactEntityLinkResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	return r
}

func (e *linkEnv) listLinks(t *testing.T, c models.Contact) []models.ContactEntityLinkResponse {
	t.Helper()
	w := e.do(http.MethodGet, linkPath(c), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var r models.ContactEntityLinksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	return r.Links
}

func (e *linkEnv) foreign(t *testing.T) (models.User, models.Contact, models.Entity, models.ContactEntityLink) {
	t.Helper()
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, e.db.Create(&other).Error)
	c := models.Contact{UserID: other.ID, Firstname: "Eve"}
	require.NoError(t, e.db.Create(&c).Error)
	ent := models.Entity{UserID: other.ID, Kind: models.EntityKindOrganisation, Name: "Foreign Org"}
	require.NoError(t, e.db.Create(&ent).Error)
	l := models.ContactEntityLink{UserID: other.ID, ContactVCardUID: c.VCardUID, EntityID: ent.ID, Kind: ent.Kind, Role: "employee", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&l).Error)
	return other, c, ent, l
}

func yearOnly(y int) *contactmodel.PartialDate { return &contactmodel.PartialDate{Year: &y} }

func TestContactEntityLink_CreateListUpdateDeleteRoundTrip(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")

	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "role_label": "Chief Analyst"})
	assert.Equal(t, org.ID, l.EntityID)
	assert.Equal(t, "Acme", l.EntityName)
	assert.Equal(t, "organisation", l.Kind)
	assert.Equal(t, "employee", l.Role)
	assert.Equal(t, "Chief Analyst", l.RoleLabel)
	assert.False(t, l.IsPrimary, "the first link is NOT automatically primary")
	assert.Equal(t, "normal", l.Sensitivity)
	assert.EqualValues(t, 1, l.Revision)
	assert.Equal(t, c.VCardUID, l.ContactVCardUID)

	links := e.listLinks(t, c)
	require.Len(t, links, 1)
	assert.Equal(t, l.ID, links[0].ID)

	// PUT replaces role/role_label/period; is_primary and sensitivity are kept when omitted.
	w := e.do(http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "owner", "period": map[string]any{"start": map[string]any{"year": 2020}}}, "If-Match", "1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var upd models.ContactEntityLinkResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &upd))
	assert.Equal(t, "owner", upd.Role)
	assert.Equal(t, "", upd.RoleLabel)
	require.NotNil(t, upd.Period)
	assert.Equal(t, 2020, *upd.Period.Start.Year)
	assert.EqualValues(t, 2, upd.Revision)

	// stale If-Match is a 412
	w = e.do(http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "member"}, "If-Match", "1")
	assert.Equal(t, http.StatusPreconditionFailed, w.Code, w.Body.String())

	w = e.do(http.MethodDelete, linkPath(c, l.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, e.listLinks(t, c))

	// soft delete: the row remains with deleted_at set and no batch id
	var row models.ContactEntityLink
	require.NoError(t, e.db.Unscoped().First(&row, "id = ?", l.ID).Error)
	assert.True(t, row.DeletedAt.Valid)
	assert.Nil(t, row.DeletionBatchID, "a direct link delete is not part of a contact/entity delete batch")

	w = e.do(http.MethodDelete, linkPath(c, l.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "entity_link_not_found", entityReason(t, w))
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "owner"}).Code)
}

func TestContactEntityLink_RoleVocabularyIsClosedPerKind(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	place := e.place(t, "Cabin")
	pet := e.create(t, map[string]any{"kind": "pet", "name": "Rex"})

	for _, tc := range []struct {
		entity models.EntityResponse
		role   string
		ok     bool
	}{
		{org, "employee", true}, {org, "volunteer", true}, {org, "resident", false}, {org, "carer", false},
		{place, "resident", true}, {place, "employee", false},
		{pet, "carer", true}, {pet, "owner", true}, {pet, "member", false},
		{org, "", false}, {org, "Employee", false},
	} {
		w := e.do(http.MethodPost, linkPath(c), map[string]any{"entity_id": tc.entity.ID, "role": tc.role})
		if tc.ok {
			assert.Equal(t, http.StatusCreated, w.Code, "%s/%s: %s", tc.entity.Kind, tc.role, w.Body.String())
		} else {
			assert.Equal(t, http.StatusBadRequest, w.Code, "%s/%s: %s", tc.entity.Kind, tc.role, w.Body.String())
		}
	}

	// role_label is free text, capped at 100, and is never matched.
	w := e.do(http.MethodPost, linkPath(c), map[string]any{"entity_id": org.ID, "role": "other", "role_label": string(make([]byte, 101))})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// A role invalid for the kind on update is rejected the same way.
	l := e.addLink(t, c, map[string]any{"entity_id": place.ID, "role": "owner"})
	w = e.do(http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "employee"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "entity_link_role_invalid", entityReason(t, w))
}

func TestContactEntityLink_PrimaryIsDeliberateAndDemotesPrevious(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	a, b := e.org(t, "Acme"), e.org(t, "Beta")
	place := e.place(t, "Cabin")

	la := e.addLink(t, c, map[string]any{"entity_id": a.ID, "role": "employee", "is_primary": true})
	assert.True(t, la.IsPrimary)
	lb := e.addLink(t, c, map[string]any{"entity_id": b.ID, "role": "employee"})
	assert.False(t, lb.IsPrimary)
	lp := e.addLink(t, c, map[string]any{"entity_id": place.ID, "role": "resident", "is_primary": true})
	assert.True(t, lp.IsPrimary, "primary is per kind: an organisation primary and a place primary coexist")

	// Making B primary through PUT demotes A in the same transaction and bumps A's revision.
	w := e.do(http.MethodPut, linkPath(c, lb.ID), map[string]any{"role": "employee", "is_primary": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	byID := map[string]models.ContactEntityLinkResponse{}
	for _, l := range e.listLinks(t, c) {
		byID[l.ID] = l
	}
	assert.True(t, byID[lb.ID].IsPrimary)
	assert.False(t, byID[la.ID].IsPrimary)
	assert.EqualValues(t, 2, byID[la.ID].Revision, "the demoted link is saved through the revision path")
	assert.True(t, byID[lp.ID].IsPrimary, "the place primary is untouched")

	// Creating with is_primary also demotes.
	c2 := e.org(t, "Gamma")
	lc := e.addLink(t, c, map[string]any{"entity_id": c2.ID, "role": "member", "is_primary": true})
	assert.True(t, lc.IsPrimary)
	assert.Equal(t, 1, countPrimaries(t, e, c, "organisation"))

	// Omitting is_primary on PUT keeps the flag; explicit false clears it.
	w = e.do(http.MethodPut, linkPath(c, lc.ID), map[string]any{"role": "member"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, countPrimaries(t, e, c, "organisation"))
	w = e.do(http.MethodPut, linkPath(c, lc.ID), map[string]any{"role": "member", "is_primary": false})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, countPrimaries(t, e, c, "organisation"))
}

func countPrimaries(t *testing.T, e *linkEnv, c models.Contact, kind string) int {
	t.Helper()
	var n int64
	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).
		Where("user_id = ? AND contact_vcard_uid = ? AND kind = ? AND is_primary = ?", e.userID, c.VCardUID, kind, true).Count(&n).Error)
	return int(n)
}

func TestContactEntityLink_ConcurrentSetPrimaryLeavesExactlyOne(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	var ids []string
	for i := 0; i < 6; i++ {
		ent := e.org(t, fmt.Sprintf("Org %d", i))
		ids = append(ids, e.addLink(t, c, map[string]any{"entity_id": ent.ID, "role": "employee"}).ID)
	}

	var wg sync.WaitGroup
	codes := make([]int, len(ids))
	start := make(chan struct{})
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			w := e.do(http.MethodPut, linkPath(c, id), map[string]any{"role": "employee", "is_primary": true})
			codes[i] = w.Code
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, 1, countPrimaries(t, e, c, "organisation"), "concurrent set-primary must never leave two primaries (codes %v)", codes)
	for _, code := range codes {
		assert.Contains(t, []int{http.StatusOK, http.StatusConflict}, code, "a loser may only see a clean 409, never a 500")
	}
}

func TestContactEntityLink_PeriodValidationAndTodayProjection(t *testing.T) {
	e := newLinkEnv(t) // fake clock: 2026-06-15
	c := e.contact(t, nil)
	org := e.org(t, "Acme")

	// end before start is rejected
	w := e.do(http.MethodPost, linkPath(c), map[string]any{"entity_id": org.ID, "role": "employee",
		"period": map[string]any{"start": map[string]any{"year": 2024}, "end": map[string]any{"year": 2020}}})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// an all-empty period stores NULL
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "period": map[string]any{}})
	assert.Nil(t, l.Period)
	var row models.ContactEntityLink
	require.NoError(t, e.db.First(&row, "id = ?", l.ID).Error)
	assert.Nil(t, row.Period)
	var raw *string
	require.NoError(t, e.db.Raw("SELECT period FROM contact_entity_links WHERE id = ?", l.ID).Scan(&raw).Error)
	assert.Nil(t, raw, "an all-empty range is NULL, not '{}'")

	type tc struct {
		name    string
		period  map[string]any
		current bool
	}
	for _, c2 := range []tc{
		{"open range", nil, true},
		{"started before today, no end", map[string]any{"start": map[string]any{"year": 2020}}, true},
		{"ended last year", map[string]any{"end": map[string]any{"year": 2025}}, false},
		{"starts next year", map[string]any{"start": map[string]any{"year": 2027}}, false},
		{"includes this month", map[string]any{"start": map[string]any{"year": 2026, "month": 6}, "end": map[string]any{"year": 2026, "month": 6}}, true},
		{"ended last month", map[string]any{"end": map[string]any{"year": 2026, "month": 5}}, false},
		{"yearless endpoint is non-comparable, counts as current", map[string]any{"end": map[string]any{"month": 1}}, true},
	} {
		body := map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": true}
		if c2.period != nil {
			body["period"] = c2.period
		}
		got := e.addLink(t, c, body)
		assert.Equal(t, c2.current, got.Current, c2.name)
		assert.True(t, got.IsPrimary, "%s: is_primary is retained regardless of the dates", c2.name)
		if c2.current {
			assert.Equal(t, "ORG", got.ProjectsAs, c2.name)
		} else {
			assert.Equal(t, "", got.ProjectsAs, c2.name)
		}
	}

	// The clock is injected and moving it changes the answer; the owner's
	// calendar day (reminder zone) decides, not UTC.
	got := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "period": map[string]any{"end": map[string]any{"year": 2026, "month": 6, "day": 15}}})
	assert.True(t, got.Current, "an end date of today still includes today")
	e.clk.Set(time.Date(2026, time.June, 16, 12, 0, 0, 0, time.UTC))
	for _, l := range e.listLinks(t, c) {
		if l.ID == got.ID {
			assert.False(t, l.Current, "the day after the end date, the link no longer projects")
		}
	}
}

func TestContactEntityLink_PetsNeverProject(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	pet := e.create(t, map[string]any{"kind": "pet", "name": "Rex"})
	l := e.addLink(t, c, map[string]any{"entity_id": pet.ID, "role": "owner", "is_primary": true})
	assert.True(t, l.IsPrimary)
	assert.Equal(t, "", l.ProjectsAs)
}

func TestContactEntityLink_ReLinkSameEntityIsTwoLinks(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "period": map[string]any{"end": map[string]any{"year": 2020}}})
	e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "period": map[string]any{"start": map[string]any{"year": 2022}}})
	assert.Len(t, e.listLinks(t, c), 2, "re-hire: no natural-key uniqueness")
}

func TestContactEntityLink_ListIsScopedAndShowsSensitivityToOwner(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "sensitivity": "secret"})
	assert.Equal(t, "secret", l.Sensitivity)
	links := e.listLinks(t, c)
	require.Len(t, links, 1)
	assert.Equal(t, "secret", links[0].Sensitivity, "an owner surface shows the owner everything (ADR 0043 decision 4)")
	w := e.do(http.MethodGet, linkPath(c), nil)
	assert.Contains(t, w.Body.String(), `"links":[`, "collection key is present")
	c2 := e.contact(t, nil)
	assert.Contains(t, e.do(http.MethodGet, linkPath(c2), nil).Body.String(), `"links":[]`, "an empty collection serialises as [], not null")
}

// Authorization matrix: every route, with a foreign contact, entity and link.
func TestContactEntityLink_ForeignResourcesAre404AndWriteNothing(t *testing.T) {
	e := newLinkEnv(t)
	mine := e.contact(t, func(c *models.Contact) { c.Firstname = "Mine" })
	myOrg := e.org(t, "Acme")
	myLink := e.addLink(t, mine, map[string]any{"entity_id": myOrg.ID, "role": "employee"})
	_, fc, fent, fl := e.foreign(t)
	sibling := e.contact(t, func(c *models.Contact) { c.Firstname = "Sibling" })

	var before int64
	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Count(&before).Error)

	type call struct {
		name, method, path string
		body               any
	}
	calls := []call{
		{"list on foreign contact", http.MethodGet, linkPath(fc), nil},
		{"create on foreign contact", http.MethodPost, linkPath(fc), map[string]any{"entity_id": myOrg.ID, "role": "employee"}},
		{"create with foreign entity", http.MethodPost, linkPath(mine), map[string]any{"entity_id": fent.ID, "role": "employee"}},
		{"create with unknown entity", http.MethodPost, linkPath(mine), map[string]any{"entity_id": "nope", "role": "employee"}},
		{"update foreign link via foreign contact", http.MethodPut, linkPath(fc, fl.ID), map[string]any{"role": "owner"}},
		{"update foreign link via own contact", http.MethodPut, linkPath(mine, fl.ID), map[string]any{"role": "owner"}},
		{"update own link via foreign contact", http.MethodPut, linkPath(fc, myLink.ID), map[string]any{"role": "owner"}},
		{"delete foreign link via foreign contact", http.MethodDelete, linkPath(fc, fl.ID), nil},
		{"delete foreign link via own contact", http.MethodDelete, linkPath(mine, fl.ID), nil},
		{"delete own link via foreign contact", http.MethodDelete, linkPath(fc, myLink.ID), nil},
		{"adopt foreign link via foreign contact", http.MethodPost, linkPath(fc, fl.ID, "adopt"), nil},
		{"adopt foreign link via own contact", http.MethodPost, linkPath(mine, fl.ID, "adopt"), nil},
		{"adopt own link via foreign contact", http.MethodPost, linkPath(fc, myLink.ID, "adopt"), nil},
		{"from-existing on foreign contact", http.MethodPost, linkPath(fc, "from-existing"), map[string]any{"source": map[string]any{"kind": "organization", "id": "x"}, "role": "employee"}},
		{"from-existing with foreign entity", http.MethodPost, linkPath(mine, "from-existing"), map[string]any{"source": map[string]any{"kind": "organization", "id": "x"}, "entity_id": fent.ID, "role": "employee"}},
		{"update own link via a sibling own contact", http.MethodPut, linkPath(sibling, myLink.ID), map[string]any{"role": "owner"}},
		{"delete own link via a sibling own contact", http.MethodDelete, linkPath(sibling, myLink.ID), nil},
		{"adopt own link via a sibling own contact", http.MethodPost, linkPath(sibling, myLink.ID, "adopt"), nil},
		{"unknown contact", http.MethodGet, "/contacts/999999/entity-links", nil},
		{"non-numeric contact", http.MethodGet, "/contacts/abc/entity-links", nil},
	}
	for _, tc := range calls {
		w := e.do(tc.method, tc.path, tc.body)
		want := http.StatusNotFound
		if tc.name == "non-numeric contact" {
			want = http.StatusBadRequest
		}
		assert.Equal(t, want, w.Code, "%s: %s", tc.name, w.Body.String())
	}

	var after int64
	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Count(&after).Error)
	assert.Equal(t, before, after)
	var fresh models.ContactEntityLink
	require.NoError(t, e.db.First(&fresh, "id = ?", fl.ID).Error)
	assert.Equal(t, "employee", fresh.Role, "the foreign link is untouched")
	var mine2 models.ContactEntityLink
	require.NoError(t, e.db.First(&mine2, "id = ?", myLink.ID).Error)
	assert.False(t, mine2.DeletedAt.Valid, "my link survived the cross-contact delete attempt")
	assert.Equal(t, "employee", mine2.Role, "my link was not edited through another contact's path")
	assert.Equal(t, mine.VCardUID, mine2.ContactVCardUID)

	// the filter by a foreign entity id returns nothing, never the foreign contact
	w := e.do(http.MethodGet, "/contacts?entity_id="+fent.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "Eve")
}

func TestContactEntityLink_TombstonedEntityOrContactIs404(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})

	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+org.ID, nil).Code)
	w := e.do(http.MethodPost, linkPath(c), map[string]any{"entity_id": org.ID, "role": "employee"})
	assert.Equal(t, http.StatusNotFound, w.Code, "a trashed entity cannot be linked")
	assert.Equal(t, "entity_not_found", entityReason(t, w))
	assert.Empty(t, e.listLinks(t, c), "the entity delete tombstoned the link")
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPut, linkPath(c, l.ID), map[string]any{"role": "owner"}).Code)

	require.NoError(t, e.db.Delete(&c).Error)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, linkPath(c), nil).Code)
}

func TestContactEntityLink_ContactsFilterByEntity(t *testing.T) {
	e := newLinkEnv(t)
	a := e.contact(t, func(c *models.Contact) { c.Firstname = "Alice" })
	b := e.contact(t, func(c *models.Contact) { c.Firstname = "Bob" })
	_ = e.contact(t, func(c *models.Contact) { c.Firstname = "Carol" })
	org := e.org(t, "Acme")
	other := e.org(t, "Beta")
	e.addLink(t, a, map[string]any{"entity_id": org.ID, "role": "employee"})
	lb := e.addLink(t, b, map[string]any{"entity_id": org.ID, "role": "owner"})
	e.addLink(t, a, map[string]any{"entity_id": other.ID, "role": "employee"})

	names := func(path string) []string {
		w := e.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var r contactsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
		var out []string
		for _, c := range r.Contacts {
			out = append(out, c["firstname"].(string))
		}
		return out
	}
	assert.ElementsMatch(t, []string{"Alice", "Bob"}, names("/contacts?entity_id="+org.ID))
	assert.ElementsMatch(t, []string{"Alice"}, names("/contacts?entity_id="+other.ID))
	assert.Empty(t, names("/contacts?entity_id=does-not-exist"))

	// a tombstoned link no longer matches
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, linkPath(b, lb.ID), nil).Code)
	assert.ElementsMatch(t, []string{"Alice"}, names("/contacts?entity_id="+org.ID))
}

func TestContactEntityLink_ContactDeleteTombstonesLinksWithBatch(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	own := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "owner"})
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, linkPath(c, own.ID), nil).Code)

	router := e.router
	router.DELETE("/contacts/:id", DeleteContact)
	w := e.do(http.MethodDelete, fmt.Sprintf("/contacts/%d", c.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row models.ContactEntityLink
	require.NoError(t, e.db.Unscoped().First(&row, "id = ?", l.ID).Error)
	assert.True(t, row.DeletedAt.Valid)
	require.NotNil(t, row.DeletionBatchID, "the contact delete batch marks the link so trash restore revives it")
	var directly models.ContactEntityLink
	require.NoError(t, e.db.Unscoped().First(&directly, "id = ?", own.ID).Error)
	assert.Nil(t, directly.DeletionBatchID, "a link deleted by hand earlier is not part of the contact's batch")
}

// --- adopt -----------------------------------------------------------------

func contactCardJSON(t *testing.T, e *linkEnv, id uint) contactmodel.Card {
	t.Helper()
	var c models.Contact
	require.NoError(t, e.db.First(&c, id).Error)
	return c.Card
}

func TestContactEntityLink_AdoptOrganisationCopiesIntoCardWithFreshID(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "own-1", Name: "Old Co"}}}
		c.Card.Emails = []contactmodel.Email{{Address: "ada@example.com", Label: "work"}}
	})
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": true})

	var beforeContact models.Contact
	require.NoError(t, e.db.First(&beforeContact, c.ID).Error)

	w := e.do(http.MethodPost, linkPath(c, l.ID, "adopt"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.ContactEntityLinkAdoptResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Adopted)
	assert.Equal(t, "organization", resp.ElementKind)
	assert.Equal(t, l.ID, resp.Link.ID)
	assert.True(t, resp.Link.IsPrimary)

	card := contactCardJSON(t, e, c.ID)
	require.Len(t, card.Organizations, 2, "the contact's own value stays and the entity's value is appended")
	assert.Equal(t, contactmodel.Organization{ID: "own-1", Name: "Old Co"}, card.Organizations[0], "own value untouched")
	adopted := card.Organizations[1]
	assert.Equal(t, "Acme", adopted.Name)
	require.Len(t, adopted.Units, 1)
	assert.Equal(t, "R&D", adopted.Units[0].Name)
	assert.NotEmpty(t, adopted.ID, "fresh element id")
	assert.NotContains(t, adopted.ID, "ent-", "the copy is an ordinary contact value, not a projection echo (reserved ent- prefix)")
	assert.NotEqual(t, org.ID, adopted.ID)
	assert.Equal(t, resp.ElementID, adopted.ID)

	var after models.Contact
	require.NoError(t, e.db.First(&after, c.ID).Error)
	assert.Equal(t, beforeContact.Revision+1, after.Revision, "saved through the contact write path (revision bump)")
	assert.Equal(t, "Old Co", after.Organization, "flat org still derives from the first (own) organisation")
	assert.Equal(t, beforeContact.Emails, after.Emails, "no other card data is lost")

	// link remains, edit audited
	assert.Len(t, e.listLinks(t, c), 1)
	var audits int64
	require.NoError(t, e.db.Model(&models.AuditEvent{}).Where("user_id = ? AND entity_type = ? AND entity_id = ? AND operation = ?",
		e.userID, models.AuditEntityContact, c.VCardUID, models.AuditOpUpdate).Count(&audits).Error)
	assert.GreaterOrEqual(t, audits, int64(1), "adopt is audited like any contact edit")

	// repeat adopt is value-equal and writes nothing
	w = e.do(http.MethodPost, linkPath(c, l.ID, "adopt"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var again models.ContactEntityLinkAdoptResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &again))
	assert.False(t, again.Adopted)
	assert.Len(t, contactCardJSON(t, e, c.ID).Organizations, 2)
	var final models.Contact
	require.NoError(t, e.db.First(&final, c.ID).Error)
	assert.Equal(t, after.Revision, final.Revision)
}

func TestContactEntityLink_AdoptPlaceCopiesAddressAndCarriesSensitivity(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	place := e.create(t, map[string]any{"kind": "place", "name": "Cabin", "sensitivity": "private",
		"core": map[string]any{"address": map[string]any{"components": []any{map[string]any{"kind": "name", "value": "1 Lake Rd"}}}}})
	l := e.addLink(t, c, map[string]any{"entity_id": place.ID, "role": "resident"})

	w := e.do(http.MethodPost, linkPath(c, l.ID, "adopt"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	card := contactCardJSON(t, e, c.ID)
	require.Len(t, card.Addresses, 1)
	assert.Equal(t, "1 Lake Rd", card.Addresses[0].Components[0].Value)
	assert.NotEmpty(t, card.Addresses[0].ID)
	assert.Equal(t, "private", card.Addresses[0].Sensitivity, "the entity's sensitivity is carried onto the copied address so it stays withheld from outward surfaces")
}

func TestContactEntityLink_AdoptPetIsRefused(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	pet := e.create(t, map[string]any{"kind": "pet", "name": "Rex"})
	l := e.addLink(t, c, map[string]any{"entity_id": pet.ID, "role": "owner"})
	w := e.do(http.MethodPost, linkPath(c, l.ID, "adopt"), nil)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Equal(t, "entity_link_kind_not_adoptable", entityReason(t, w))
}

func TestContactEntityLink_AdoptIsAvailableRegardlessOfPeriod(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "period": map[string]any{"end": map[string]any{"year": 2001}}})
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, linkPath(c, l.ID, "adopt"), nil).Code)
	assert.Len(t, contactCardJSON(t, e, c.ID).Organizations, 1)
}

// --- link from existing ----------------------------------------------------

func TestContactEntityLink_FromExistingOrganisationCreatesEntityAndKeepsOwnValueByteIdentical(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{Organizations: []contactmodel.Organization{
			{ID: "o1", Name: "Initech", Units: []contactmodel.OrgUnit{{Name: "Ops"}}, SortAs: "initech"}}}
	})
	var before models.Contact
	require.NoError(t, e.db.First(&before, c.ID).Error)
	rawBefore, err := json.Marshal(before.Card)
	require.NoError(t, err)

	w := e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{
		"source": map[string]any{"kind": "organization", "id": "o1"}, "role": "employee", "is_primary": true})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp models.ContactEntityLinkFromExistingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.EntityCreated)
	assert.Equal(t, "Initech", resp.Entity.Name)
	assert.Equal(t, "organisation", resp.Entity.Kind)
	assert.JSONEq(t, `{"organization":{"name":"Initech","units":[{"name":"Ops"}],"sortAs":"initech"}}`, string(resp.Entity.Core), "the entity core drops the contact-local element id")
	assert.Equal(t, resp.Entity.ID, resp.Link.EntityID)
	assert.True(t, resp.Link.IsPrimary)
	assert.Equal(t, "employee", resp.Link.Role)

	var after models.Contact
	require.NoError(t, e.db.First(&after, c.ID).Error)
	rawAfter, err := json.Marshal(after.Card)
	require.NoError(t, err)
	assert.Equal(t, string(rawBefore), string(rawAfter), "link-from-existing never rewrites the contact's own value (byte-identical Card)")
	assert.Equal(t, before.Revision, after.Revision, "the contact row is not even written")
	assert.Equal(t, before.Organization, after.Organization)
}

func TestContactEntityLink_FromExistingMatchesGivenEntityAndRejectsKindMismatch(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{
			Organizations: []contactmodel.Organization{{ID: "o1", Name: "Initech"}},
			Addresses:     []contactmodel.Address{{ID: "a1", Components: []contactmodel.AddressComponent{{Kind: "name", Value: "9 Elm"}}, Sensitivity: "secret"}},
		}
	})
	org := e.org(t, "Initech Inc")
	place := e.place(t, "Cabin")

	w := e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{
		"source": map[string]any{"kind": "organization", "id": "o1"}, "entity_id": org.ID, "role": "member"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp models.ContactEntityLinkFromExistingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.EntityCreated)
	assert.Equal(t, org.ID, resp.Link.EntityID)
	var n int64
	require.NoError(t, e.db.Model(&models.Entity{}).Where("user_id = ?", e.userID).Count(&n).Error)
	assert.EqualValues(t, 2, n, "no entity was created when one was matched")

	// organisation value against a place entity: kind mismatch
	w = e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{
		"source": map[string]any{"kind": "organization", "id": "o1"}, "entity_id": place.ID, "role": "resident"})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// address value creates a place that carries the address sensitivity in the entity column, not core
	w = e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{
		"source": map[string]any{"kind": "address", "id": "a1"}, "role": "resident"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.EntityCreated)
	assert.Equal(t, "place", resp.Entity.Kind)
	assert.Equal(t, "secret", resp.Entity.Sensitivity)
	assert.NotContains(t, string(resp.Entity.Core), "sensitivity")
	assert.Contains(t, string(resp.Entity.Core), "9 Elm")
	assert.NotEmpty(t, resp.Entity.Name)
}

func TestContactEntityLink_FromExistingRejectsMissingSourceAndBadRole(t *testing.T) {
	e := newLinkEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "o1", Name: "Initech"}, {ID: "o2"}}}
	})
	for _, tc := range []struct {
		name   string
		body   map[string]any
		status int
		reason string
	}{
		{"unknown element", map[string]any{"source": map[string]any{"kind": "organization", "id": "zzz"}, "role": "employee"}, 404, "entity_link_source_not_found"},
		{"unsupported source kind", map[string]any{"source": map[string]any{"kind": "title", "id": "o1"}, "role": "employee"}, 400, ""},
		{"nameless organisation cannot make an entity", map[string]any{"source": map[string]any{"kind": "organization", "id": "o2"}, "role": "employee"}, 400, ""},
		{"role invalid for kind", map[string]any{"source": map[string]any{"kind": "organization", "id": "o1"}, "role": "resident"}, 400, "entity_link_role_invalid"},
	} {
		w := e.do(http.MethodPost, linkPath(c, "from-existing"), tc.body)
		assert.Equal(t, tc.status, w.Code, "%s: %s", tc.name, w.Body.String())
		if tc.reason != "" {
			assert.Equal(t, tc.reason, entityReason(t, w), tc.name)
		}
	}
	var n int64
	require.NoError(t, e.db.Model(&models.Entity{}).Count(&n).Error)
	assert.Zero(t, n, "a rejected request creates no entity")
}

func TestContactEntityLink_FromExistingIsAtomic(t *testing.T) {
	// An entity created for the source value is rolled back when the link cannot be
	// created (invalid role is checked first, so force the failure on the primary
	// index path by hiding the link table).
	e := newLinkEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card = contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "o1", Name: "Initech"}}}
	})
	logtest.AllowWarnings(t, "the injected link-table failure is logged as a request error")
	dbtest.HideTable(t, e.db, "contact_entity_links")
	w := e.do(http.MethodPost, linkPath(c, "from-existing"), map[string]any{"source": map[string]any{"kind": "organization", "id": "o1"}, "role": "employee"})
	assert.GreaterOrEqual(t, w.Code, 500)
	var n int64
	require.NoError(t, e.db.Model(&models.Entity{}).Count(&n).Error)
	assert.Zero(t, n, "entity creation rolled back with the failed link")
}

// A router built with no clock still serves (System clock).
func TestContactEntityLink_WorksWithoutAnInstalledClock(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.GET("/contacts/:id/entity-links", ListContactEntityLinks)
	c := models.Contact{UserID: user.ID, Firstname: "A"}
	require.NoError(t, db.Create(&c).Error)
	req := httptest.NewRequest(http.MethodGet, linkPath(c), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// "Today" is the owner's calendar day (ADR 0015 rule 4), not UTC: at 23:30 UTC
// on 15 June it is already 16 June in Auckland.
func TestContactEntityLink_TodayIsTheOwnersCalendarDay(t *testing.T) {
	e := newLinkEnv(t)
	e.clk.Set(time.Date(2026, time.June, 15, 23, 30, 0, 0, time.UTC))
	c := e.contact(t, nil)
	org := e.org(t, "Acme")
	endsOn15th := map[string]any{"end": map[string]any{"year": 2026, "month": 6, "day": 15}}

	l := e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee", "is_primary": true, "period": endsOn15th})
	assert.True(t, l.Current, "UTC reminder zone: still the 15th")

	// Re-serve the listing with an Auckland reminder zone on the request.
	e.router.GET("/auckland/contacts/:id/entity-links", func(ctx *gin.Context) {
		cfg := currentConfig(ctx)
		cfg.ReminderTimezone = "Pacific/Auckland"
		ctx.Set("cfg", cfg)
		ListContactEntityLinks(ctx)
	})
	w := e.do(http.MethodGet, fmt.Sprintf("/auckland/contacts/%d/entity-links", c.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var r models.ContactEntityLinksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	require.Len(t, r.Links, 1)
	assert.False(t, r.Links[0].Current, "in the owner's zone it is the 16th: the link ended yesterday")
	assert.True(t, r.Links[0].IsPrimary, "is_primary is retained")
	assert.Equal(t, "", r.Links[0].ProjectsAs)
}

// Adopt and link-from-existing are explicit user actions (ADR 0043 decision 3):
// nothing but their own route registration may reach the handlers, so an import,
// a sync or a merge can never trigger them.
func TestContactEntityLink_AdoptAndFromExistingAreOnlyReachedByTheirRoutes(t *testing.T) {
	handlers := []string{"AdoptContactEntityLink", "LinkContactEntityFromExisting"}
	allowed := map[string]bool{
		"contact_entity_link_controller.go": true,
		"routes.go":                         true,
	}
	root := ".."
	var offenders []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == ".git" || n == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || allowed[d.Name()] {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, h := range handlers {
			if strings.Contains(string(src), h) {
				offenders = append(offenders, path+" references "+h)
			}
		}
		return nil
	}))
	assert.Empty(t, offenders, "adopt/from-existing must only be reachable from their routes")
}
