package controllers

import (
	"encoding/json"
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

func TestEntityMerge_FillingFromAMoreSensitiveLoserRaisesTheSurvivorsSensitivity(t *testing.T) {
	e := newMergeEnv(t)
	surv := e.create(t, map[string]any{"kind": "place", "name": "A"})
	lose := e.create(t, map[string]any{"kind": "place", "name": "B", "sensitivity": "secret", "core": map[string]any{"notes": "hidden"}})
	plain := e.create(t, map[string]any{"kind": "place", "name": "C"})
	// no resolution: nothing copied, sensitivity unchanged
	res := e.mergeOK(t, plain.ID, surv.ID, nil)
	assert.Equal(t, "normal", res.Survivor.Sensitivity)
	res = e.mergeOK(t, lose.ID, surv.ID, map[string]string{"notes": "loser"})
	assert.Equal(t, "secret", res.Survivor.Sensitivity)
}

func TestEntityMerge_TransactionFailuresRollBackEveryStep(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	triggers := map[string]string{
		"repoint link":      "CREATE TRIGGER f BEFORE UPDATE ON contact_entity_links WHEN NEW.entity_id <> OLD.entity_id BEGIN SELECT RAISE(ABORT,'boom'); END",
		"collapse link":     "CREATE TRIGGER f BEFORE UPDATE ON contact_entity_links WHEN NEW.deleted_at IS NOT NULL BEGIN SELECT RAISE(ABORT,'boom'); END",
		"promote primary":   "CREATE TRIGGER f BEFORE UPDATE ON contact_entity_links WHEN NEW.is_primary = 1 AND OLD.is_primary = 0 BEGIN SELECT RAISE(ABORT,'boom'); END",
		"rekey field value": "CREATE TRIGGER f BEFORE UPDATE ON field_values BEGIN SELECT RAISE(ABORT,'boom'); END",
		"save survivor":     "CREATE TRIGGER f BEFORE UPDATE ON entities WHEN NEW.deleted_at IS NULL BEGIN SELECT RAISE(ABORT,'boom'); END",
	}
	for name, trig := range triggers {
		e := newMergeEnv(t)
		survivor := e.org(t, "Acme")
		loser := e.org(t, "Acme Corp")
		def := models.FieldDefinition{UserID: e.userID, Label: "I", Key: "i", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
		require.NoError(t, e.db.Create(&def).Error)
		w := e.do(http.MethodPut, "/entities/"+loser.ID+"/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": def.ID, "value": "x"}}})
		require.Equal(t, http.StatusOK, w.Code)
		c := e.contact(t, nil)
		c2 := e.contact(t, nil)
		e.addLink(t, c, map[string]any{"entity_id": survivor.ID, "role": "employee"})
		e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "employee", "is_primary": true}) // dup + primary promotion
		e.addLink(t, c2, map[string]any{"entity_id": loser.ID, "role": "owner"})                       // plain re-point
		require.NoError(t, e.db.Exec(trig).Error)
		w = e.merge(loser.ID, survivor.ID, nil)
		assert.Equal(t, http.StatusInternalServerError, w.Code, name+": "+w.Body.String())
		var live int64
		require.NoError(t, e.db.Model(&models.Entity{}).Where("id = ?", loser.ID).Count(&live).Error)
		assert.EqualValues(t, 1, live, name+": the loser is still live")
		var moved int64
		require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Where("entity_id = ?", survivor.ID).Count(&moved).Error)
		assert.EqualValues(t, 1, moved, name+": nothing was re-pointed")
	}
}

func TestEntitySuggestions_EdgeCasesInKeysGroupsAndPaging(t *testing.T) {
	e := newMergeEnv(t)
	// two places with the same name AND the same address: matched by two tiers, the strongest reason wins
	mk := func(name string) {
		e.create(t, map[string]any{"kind": "place", "name": name, "core": map[string]any{"address": map[string]any{"full": "1 Main St, Springfield"}}})
	}
	mk("Home")
	mk("Home")
	pairs := e.suggestions(t, "kind=place").Pairs
	require.Len(t, pairs, 1)
	assert.Equal(t, models.EntitySuggestionReasonExactName, pairs[0].Reason)
	// a place with no address and no name key never matches on the address tier
	e.create(t, map[string]any{"kind": "place", "name": "Nowhere"})

	// contact values: duplicates per contact, punctuation-only names, no-key addresses, tied variants
	c1 := e.contact(t, func(c *models.Contact) {
		c.Card.Organizations = []contactmodel.Organization{{ID: "a", Name: "Acme"}, {ID: "b", Name: "ACME"}, {ID: "c", Name: "!!!"}, {ID: "d", Name: "  "}}
		c.Card.Addresses = []contactmodel.Address{{ID: "x"}, {ID: "y", Full: "9 Elm Rd"}}
	})
	e.contact(t, func(c *models.Contact) {
		c.Card.Organizations = []contactmodel.Organization{{ID: "a", Name: "ACME"}, {ID: "z", Name: "Zeta"}}
	})
	g := e.suggestions(t, "mode=contact_values&kind=organisation").Groups
	require.Len(t, g, 2)
	var acme models.EntityValueGroup
	for _, x := range g {
		if x.Contacts[0].ContactID == c1.ID {
			acme = x
		}
	}
	assert.Len(t, acme.Contacts, 2, "one value per contact per group")
	assert.Equal(t, []string{"ACME", "Acme"}, acme.Variants, "tied spellings sort alphabetically")
	assert.Len(t, e.suggestions(t, "mode=contact_values&kind=place").Groups, 1)

	// paging over groups
	first := e.suggestions(t, "mode=contact_values&kind=organisation&limit=1")
	require.Len(t, first.Groups, 1)
	require.NotEmpty(t, first.NextCursor)
	second := e.suggestions(t, "mode=contact_values&kind=organisation&limit=1&cursor="+first.NextCursor)
	require.Len(t, second.Groups, 1)
	assert.NotEqual(t, first.Groups[0].Key, second.Groups[0].Key)
	assert.Empty(t, second.NextCursor)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities/suggestions?mode=contact_values&kind=organisation&cursor=***", nil).Code)
}

func TestEntityHelper_MatchedByAnAddresslessPlaceAndRepeatedKeys(t *testing.T) {
	e := newMergeEnv(t)
	c := e.contact(t, func(c *models.Contact) {
		c.Card.Addresses = []contactmodel.Address{{ID: "a", Full: "9 Elm Rd"}}
		c.Card.Organizations = []contactmodel.Organization{{ID: "o", Name: "Acme"}}
	})
	// a linked place without an address has no key and matches nothing
	blank := e.create(t, map[string]any{"kind": "place", "name": "Blank"})
	e.addLink(t, c, map[string]any{"entity_id": blank.ID, "role": "resident"})
	groups := e.suggestions(t, "mode=contact_values&kind=place").Groups
	require.Len(t, groups, 1)

	w := e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "place", "groups": []map[string]any{
		{"key": groups[0].Key, "role": "resident", "contact_ids": []uint{c.ID, c.ID}},
		{"key": groups[0].Key, "role": "resident", "contact_ids": []uint{c.ID}}}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var out models.EntityFromContactsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Len(t, out.Created, 1, "the same key twice in one request creates one entity")
	assert.Len(t, out.Created[0].Links, 1, "a repeated contact id links once")
	require.Len(t, out.Skipped, 1)
	assert.Equal(t, "group_not_found", out.Skipped[0].Reason)
}

func TestEntityHelper_AndDismissDatabaseFailures(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test legitimately logs request errors for injected database failures")
	e := newMergeEnv(t)
	a, b := e.org(t, "A"), e.org(t, "B")
	c := e.contact(t, func(c *models.Contact) {
		c.Card.Organizations = []contactmodel.Organization{{ID: "o", Name: "Acme"}}
	})
	key := e.suggestions(t, "mode=contact_values&kind=organisation").Groups[0].Key
	post := func() int {
		return e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "organisation", "groups": []map[string]any{
			{"key": key, "role": "employee", "set_primary": true, "contact_ids": []uint{c.ID}}}}).Code
	}
	require.NoError(t, e.db.Exec("CREATE TRIGGER fe BEFORE INSERT ON entities BEGIN SELECT RAISE(ABORT,'boom'); END").Error)
	assert.Equal(t, http.StatusInternalServerError, post(), "entity insert fails")
	require.NoError(t, e.db.Exec("DROP TRIGGER fe").Error)

	dbtest.HideTable(t, e.db, "contact_entity_links")
	assert.Equal(t, http.StatusInternalServerError, post(), "helper cannot read links")

	e2 := newMergeEnv(t)
	a2, b2 := e2.org(t, "A"), e2.org(t, "B")
	dbtest.HideTable(t, e2.db, "dismissed_entity_pairs")
	assert.Equal(t, http.StatusInternalServerError, e2.do(http.MethodPost, "/entities/suggestions/dismiss", map[string]any{"entity_a": a2.ID, "entity_b": b2.ID}).Code)
	_, _ = a, b
}
