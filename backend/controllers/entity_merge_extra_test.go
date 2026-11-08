package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Remaining resolution fields per kind and database-failure behaviour of the
// entity merge / suggestions / helper routes (issue #2299).

func TestEntityMerge_ResolutionsFillEveryKindsEmptyFields(t *testing.T) {
	e := newMergeEnv(t)

	// pet
	surv := e.create(t, map[string]any{"kind": "pet", "name": "Rex"})
	lose := e.create(t, map[string]any{"kind": "pet", "name": "Rexy", "core": map[string]any{
		"species": "dog", "breed": "lab", "birth": map[string]any{"year": 2020}, "death": map[string]any{"year": 2030}, "notes": "good boy"}})
	res := e.mergeOK(t, lose.ID, surv.ID, map[string]string{"species": "loser", "breed": "loser", "birth": "loser", "death": "loser", "notes": "loser"})
	assert.Len(t, res.ResolvedFields, 5)
	assert.JSONEq(t, `{"species":"dog","breed":"lab","birth":{"year":2020},"death":{"year":2030},"notes":"good boy"}`, string(res.Survivor.Core))

	// organisation units, sortAs, notes; place notes
	os := e.org(t, "Acme")
	ol := e.orgWithCore(t, "Acme Corp", map[string]any{"organization": map[string]any{"name": "Acme Corp", "sortAs": "Acme", "units": []any{map[string]any{"name": "R&D"}}}, "notes": "n"})
	// the survivor from e.org already has units, so units is rejected and sortAs/notes fill
	w := e.merge(ol.ID, os.ID, map[string]string{"organization.units": "loser"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	res = e.mergeOK(t, ol.ID, os.ID, map[string]string{"organization.sortAs": "loser", "notes": "loser"})
	assert.Equal(t, []string{"notes", "organization.sortAs"}, res.ResolvedFields)

	ps := e.create(t, map[string]any{"kind": "place", "name": "A"})
	pl := e.create(t, map[string]any{"kind": "place", "name": "B", "core": map[string]any{"notes": "keys under the mat"}})
	res = e.mergeOK(t, pl.ID, ps.ID, map[string]string{"notes": "loser", "address": "survivor"})
	assert.Equal(t, []string{"notes"}, res.ResolvedFields)

	us := e.create(t, map[string]any{"kind": "organisation", "name": "NoUnits", "core": map[string]any{"organization": map[string]any{"name": "NoUnits"}}})
	res = e.mergeOK(t, e.org(t, "WithUnits").ID, us.ID, map[string]string{"organization.units": "loser"})
	assert.Equal(t, []string{"organization.units"}, res.ResolvedFields)
}

func TestEntityMerge_DatabaseFailuresAre500(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	for _, table := range []string{"contact_entity_links", "field_values", "entity_deletion_snapshots", "dismissed_entity_pairs"} {
		e := newMergeEnv(t)
		a, b := e.org(t, "A"), e.org(t, "B")
		dbtest.HideTable(t, e.db, table)
		w := e.merge(b.ID, a.ID, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code, table+": "+w.Body.String())
		var live int64
		require.NoError(t, e.db.Model(&models.Entity{}).Where("id = ?", b.ID).Count(&live).Error)
		assert.EqualValues(t, 1, live, table+": a failed merge leaves the loser live")
	}

	e := newMergeEnv(t)
	a, b := e.org(t, "A"), e.org(t, "B")
	dbtest.HideTable(t, e.db, "entities")
	assert.Equal(t, http.StatusInternalServerError, e.do(http.MethodGet, "/entities/suggestions", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, e.do(http.MethodPost, "/entities/suggestions/dismiss", map[string]any{"entity_a": a.ID, "entity_b": b.ID}).Code)
}

func TestEntitySuggestions_DatabaseFailuresAre500(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	for _, table := range []string{"dismissed_entity_pairs", "contact_entity_links", "contacts"} {
		e := newMergeEnv(t)
		e.org(t, "A")
		e.org(t, "A")
		dbtest.HideTable(t, e.db, table)
		pairs := e.do(http.MethodGet, "/entities/suggestions", nil).Code
		groups := e.do(http.MethodGet, "/entities/suggestions?mode=contact_values&kind=organisation", nil).Code
		switch table {
		case "dismissed_entity_pairs":
			assert.Equal(t, http.StatusInternalServerError, pairs)
		case "contact_entity_links", "contacts":
			assert.Equal(t, http.StatusInternalServerError, groups)
		}
	}
}

func TestEntityHelper_DatabaseFailureRollsBackEverything(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	e := newMergeEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card.Organizations = []contactmodel.Organization{{ID: "o", Name: "Acme"}}
	})
	key := e.suggestions(t, "mode=contact_values&kind=organisation").Groups[0].Key
	// the group read succeeds, then the link insert fails: hide the table after
	// the entity insert would have happened by hiding it up front for writes only
	// is not possible, so assert the all-or-nothing contract via a role the link
	// table rejects.
	require.NoError(t, e.db.Exec("CREATE TRIGGER fail_link BEFORE INSERT ON contact_entity_links BEGIN SELECT RAISE(ABORT, 'boom'); END").Error)
	w := e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "organisation", "groups": []map[string]any{
		{"key": key, "role": "employee", "contact_ids": []uint{c.ID}}}})
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	var n int64
	require.NoError(t, e.db.Unscoped().Model(&models.Entity{}).Count(&n).Error)
	assert.EqualValues(t, 0, n, "the entity insert rolled back with the failed link")
}

func TestEntitySuggestions_LongNamesAndNameInvalid(t *testing.T) {
	e := newMergeEnv(t)
	long := ""
	for i := 0; i < 201; i++ {
		long += "x"
	}
	c := e.contact(t, func(c *models.Contact) {
		c.Card.Organizations = []contactmodel.Organization{{ID: "o", Name: long}}
	})
	g := e.suggestions(t, "mode=contact_values&kind=organisation").Groups
	require.Len(t, g, 1)
	w := e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "organisation", "groups": []map[string]any{
		{"key": g[0].Key, "role": "employee", "contact_ids": []uint{c.ID}}}})
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "name_invalid")
}
