package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/contactmodel"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Entity merge, suggestions, dismissal and the create-from-contacts helper
// (issue #2299, ADR 0043 decisions 8, 9, 10) against the real migrated schema.

type mergeEnv struct{ *linkEnv }

func newMergeEnv(t *testing.T) *mergeEnv {
	t.Helper()
	e := newLinkEnv(t)
	r := e.router
	r.GET("/entities/:id", GetEntity)
	r.PUT("/entities/:id", middleware.ValidateJSONMiddleware(&models.EntityInput{}), UpdateEntity)
	r.POST("/entities/:id/restore", RestoreEntity)
	r.PUT("/entities/:id/field-values", middleware.ValidateJSONMiddleware(&models.ContactFieldValuesInput{}), ReplaceEntityFieldValues)
	r.POST("/entities/:id/merge", middleware.ValidateJSONMiddleware(&models.EntityMergeInput{}), MergeEntity)
	r.GET("/entities/suggestions", ListEntitySuggestions)
	r.POST("/entities/suggestions/dismiss", middleware.ValidateJSONMiddleware(&models.EntityDismissInput{}), DismissEntitySuggestion)
	r.POST("/entities/from-contacts", middleware.ValidateJSONMiddleware(&models.EntityFromContactsInput{}), CreateEntitiesFromContacts)
	return &mergeEnv{e}
}

func (e *mergeEnv) merge(loser, into string, resolutions map[string]string) *httptest.ResponseRecorder {
	body := map[string]any{"into": into}
	if resolutions != nil {
		body["resolutions"] = resolutions
	}
	return e.do(http.MethodPost, "/entities/"+loser+"/merge", body)
}

func (e *mergeEnv) mergeOK(t *testing.T, loser, into string, resolutions map[string]string) models.EntityMergeResponse {
	t.Helper()
	w := e.merge(loser, into, resolutions)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var r models.EntityMergeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	return r
}

func (e *mergeEnv) orgWithCore(t *testing.T, name string, core map[string]any) models.EntityResponse {
	t.Helper()
	return e.create(t, map[string]any{"kind": "organisation", "name": name, "core": core})
}

func (e *mergeEnv) link(t *testing.T, id string) models.ContactEntityLink {
	t.Helper()
	var l models.ContactEntityLink
	require.NoError(t, e.db.Unscoped().First(&l, "id = ?", id).Error)
	return l
}

func (e *mergeEnv) liveLinks(t *testing.T, entityID string) []models.ContactEntityLink {
	t.Helper()
	var ls []models.ContactEntityLink
	require.NoError(t, e.db.Where("entity_id = ?", entityID).Order("created_at, id").Find(&ls).Error)
	return ls
}

func (e *mergeEnv) suggestions(t *testing.T, query string) models.EntitySuggestionsResponse {
	t.Helper()
	w := e.do(http.MethodGet, "/entities/suggestions?"+query, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var r models.EntitySuggestionsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	return r
}

func yearPeriod(y int) map[string]any {
	return map[string]any{"start": map[string]any{"year": y}}
}

// --- merge: re-pointing, duplicates, primaries -----------------------------

func TestEntityMerge_RepointsLinksSoftDeletesLoserAndBumpsRevisions(t *testing.T) {
	e := newMergeEnv(t)
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	c1 := e.contact(t, nil)
	c2 := e.contact(t, func(c *models.Contact) { c.Firstname = "Grace" })
	l1 := e.addLink(t, c1, map[string]any{"entity_id": loser.ID, "role": "employee", "is_primary": true})
	l2 := e.addLink(t, c2, map[string]any{"entity_id": survivor.ID, "role": "owner"})

	res := e.mergeOK(t, loser.ID, survivor.ID, nil)
	assert.Equal(t, loser.ID, res.LoserID)
	assert.Equal(t, 1, res.LinksRepointed)
	assert.Equal(t, 0, res.LinksCollapsed)
	assert.Equal(t, models.EntityRestoreScopeEntityOnly, res.RestoreScope)
	assert.Greater(t, res.Survivor.Revision, survivor.Revision, "the survivor's revision advances")
	assert.Equal(t, "Acme", res.Survivor.Name, "the survivor keeps its own name")
	assert.NotNil(t, res.PrimariesDemoted)
	assert.NotNil(t, res.FieldValuesDropped)
	assert.NotNil(t, res.ResolvedFields)

	moved := e.link(t, l1.ID)
	assert.Equal(t, survivor.ID, moved.EntityID)
	assert.True(t, moved.IsPrimary, "a lone primary stays primary")
	assert.False(t, moved.DeletedAt.Valid)
	assert.Greater(t, moved.Revision, l1.Revision, "a re-pointed link's revision advances")
	untouched := e.link(t, l2.ID)
	assert.Equal(t, l2.Revision, untouched.Revision, "a link that did not move is not bumped")

	// the loser is in the trash with a snapshot recording the survivor
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/entities/"+loser.ID, nil).Code)
	var tomb models.Entity
	require.NoError(t, e.db.Unscoped().First(&tomb, "id = ?", loser.ID).Error)
	assert.True(t, tomb.DeletedAt.Valid)
	var snap models.EntityDeletionSnapshot
	require.NoError(t, e.db.First(&snap, "entity_id = ?", loser.ID).Error)
	assert.Contains(t, snap.Payload, survivor.ID)

	// the survivor now lists both contacts
	w := e.do(http.MethodGet, "/entities/"+survivor.ID+"/contacts", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var cs models.EntityContactsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cs))
	assert.Len(t, cs.Contacts, 2)
}

func TestEntityMerge_CollapsesDuplicateLinksOnlyWhenRoleAndPeriodMatch(t *testing.T) {
	e := newMergeEnv(t)
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	c := e.contact(t, nil)
	e.addLink(t, c, map[string]any{"entity_id": survivor.ID, "role": "employee"})
	dup := e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "employee"})
	otherRole := e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "owner"})
	otherPeriod := e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "employee", "period": yearPeriod(2020)})

	res := e.mergeOK(t, loser.ID, survivor.ID, nil)
	assert.Equal(t, 1, res.LinksCollapsed)
	assert.Equal(t, 2, res.LinksRepointed)

	d := e.link(t, dup.ID)
	assert.True(t, d.DeletedAt.Valid, "the identical duplicate is soft-deleted")
	assert.Nil(t, d.DeletionBatchID, "a collapsed duplicate is not part of the loser's delete batch")
	assert.Equal(t, survivor.ID, e.link(t, otherRole.ID).EntityID)
	assert.Equal(t, survivor.ID, e.link(t, otherPeriod.ID).EntityID)
	assert.False(t, e.link(t, otherRole.ID).DeletedAt.Valid)
	assert.Len(t, e.liveLinks(t, survivor.ID), 3)
}

func TestEntityMerge_CollapsingAPrimaryDuplicateKeepsThePrimaryFlag(t *testing.T) {
	e := newMergeEnv(t)
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	c := e.contact(t, nil)
	keep := e.addLink(t, c, map[string]any{"entity_id": survivor.ID, "role": "employee"})
	e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "employee", "is_primary": true})

	e.mergeOK(t, loser.ID, survivor.ID, nil)
	assert.True(t, e.link(t, keep.ID).IsPrimary, "the collapsed link's primary flag moves to the kept duplicate")
	assert.Equal(t, 1, countPrimaries(t, e.linkEnv, c, models.EntityKindOrganisation))
}

func TestEntityMerge_RepointedPrimaryIsDemotedAndReportedWhenContactAlreadyHasOne(t *testing.T) {
	e := newMergeEnv(t)
	// The partial unique index makes two primaries impossible through the API;
	// drop it to exercise the defensive demotion the ADR requires.
	require.NoError(t, e.db.Exec("DROP INDEX idx_contact_entity_links_primary").Error)
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	c := e.contact(t, nil)
	keep := e.addLink(t, c, map[string]any{"entity_id": survivor.ID, "role": "employee", "is_primary": true})
	moved := e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "owner"})
	// the API demotes the previous primary, so force the second flag directly
	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Where("id = ?", moved.ID).UpdateColumn("is_primary", true).Error)

	res := e.mergeOK(t, loser.ID, survivor.ID, nil)
	require.Len(t, res.PrimariesDemoted, 1)
	assert.Equal(t, moved.ID, res.PrimariesDemoted[0].LinkID)
	assert.Equal(t, c.VCardUID, res.PrimariesDemoted[0].ContactVCardUID)
	assert.True(t, e.link(t, keep.ID).IsPrimary, "the survivor's existing primary keeps the flag")
	assert.False(t, e.link(t, moved.ID).IsPrimary)
	assert.Equal(t, survivor.ID, e.link(t, moved.ID).EntityID)
}

// --- merge: core resolutions and custom values -----------------------------

func TestEntityMerge_SurvivorKeepsCoreUnlessAResolutionFillsAnEmptyField(t *testing.T) {
	e := newMergeEnv(t)
	survivor := e.orgWithCore(t, "Acme", map[string]any{"organization": map[string]any{"name": "Acme"}, "notes": "mine"})
	loser := e.orgWithCore(t, "Acme Corp", map[string]any{"organization": map[string]any{"name": "Acme Corp"}, "url": "https://acme.example", "notes": "theirs"})

	res := e.mergeOK(t, loser.ID, survivor.ID, map[string]string{"url": "loser", "notes": "survivor"})
	assert.Equal(t, []string{"url"}, res.ResolvedFields)
	assert.JSONEq(t, `{"organization":{"name":"Acme"},"url":"https://acme.example","notes":"mine"}`, string(res.Survivor.Core))

	// no resolution: nothing is silently unioned
	s2 := e.orgWithCore(t, "Beta", map[string]any{"organization": map[string]any{"name": "Beta"}})
	l2 := e.orgWithCore(t, "Beta Inc", map[string]any{"organization": map[string]any{"name": "Beta Inc"}, "url": "https://beta.example"})
	res = e.mergeOK(t, l2.ID, s2.ID, nil)
	assert.JSONEq(t, `{"organization":{"name":"Beta"}}`, string(res.Survivor.Core))
}

func TestEntityMerge_RejectsBadResolutionsAndWritesNothing(t *testing.T) {
	e := newMergeEnv(t)
	survivor := e.orgWithCore(t, "Acme", map[string]any{"organization": map[string]any{"name": "Acme"}, "notes": "mine"})
	loser := e.orgWithCore(t, "Acme Corp", map[string]any{"organization": map[string]any{"name": "Acme Corp"}, "url": "https://acme.example", "notes": "theirs"})
	c := e.contact(t, nil)
	l := e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "employee"})

	for name, res := range map[string]map[string]string{
		"survivor value would be overwritten": {"notes": "loser"},
		"unknown field":                       {"nope": "loser"},
		"field of another kind":               {"species": "loser"},
		"bad choice":                          {"url": "both"},
		"loser has nothing to give":           {"organization.sortAs": "loser"},
		"valid then invalid":                  {"url": "loser", "notes": "loser"},
	} {
		w := e.merge(loser.ID, survivor.ID, res)
		assert.Equal(t, http.StatusUnprocessableEntity, w.Code, "%s: %s", name, w.Body.String())
		assert.Equal(t, string(apperrors.ReasonEntityMergeResolutionInvalid), entityReason(t, w), name)
	}
	// transactional: nothing changed
	var s, lo models.Entity
	require.NoError(t, e.db.First(&s, "id = ?", survivor.ID).Error)
	require.NoError(t, e.db.First(&lo, "id = ?", loser.ID).Error)
	assert.Equal(t, survivor.Revision, s.Revision)
	assert.JSONEq(t, `{"organization":{"name":"Acme"},"notes":"mine"}`, s.Core)
	assert.Equal(t, loser.ID, e.link(t, l.ID).EntityID)
}

func TestEntityMerge_PlaceAddressFillsOnlyAnEmptyAddress(t *testing.T) {
	e := newMergeEnv(t)
	empty := e.create(t, map[string]any{"kind": "place", "name": "Home"})
	full := e.place(t, "12 High St")
	res := e.mergeOK(t, full.ID, empty.ID, map[string]string{"address": "loser"})
	assert.Equal(t, []string{"address"}, res.ResolvedFields)
	assert.Contains(t, string(res.Survivor.Core), "Springfield")

	p2 := e.place(t, "Other")
	p3 := e.place(t, "Third")
	w := e.merge(p3.ID, p2.ID, map[string]string{"address": "loser"})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestEntityMerge_ReKeysFieldValuesOnlyWhereSurvivorHasNone(t *testing.T) {
	e := newMergeEnv(t)
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	onlyLoser := models.FieldDefinition{UserID: e.userID, Label: "Industry", Key: "industry", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	both := models.FieldDefinition{UserID: e.userID, Label: "Size", Key: "size", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&onlyLoser).Error)
	require.NoError(t, e.db.Create(&both).Error)
	put := func(id, def, v string) {
		w := e.do(http.MethodPut, "/entities/"+id+"/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": def, "value": v}}})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	put(loser.ID, onlyLoser.ID, "software")
	// ReplaceEntityFieldValues replaces the whole set, so write both in one call for the loser.
	w := e.do(http.MethodPut, "/entities/"+loser.ID+"/field-values", map[string]any{"field_values": []map[string]any{
		{"field_definition_id": onlyLoser.ID, "value": "software"}, {"field_definition_id": both.ID, "value": "loser-size"}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	put(survivor.ID, both.ID, "survivor-size")

	res := e.mergeOK(t, loser.ID, survivor.ID, nil)
	assert.Equal(t, 1, res.FieldValuesMoved)
	assert.Equal(t, []string{both.ID}, res.FieldValuesDropped)

	var vals []models.FieldValue
	require.NoError(t, e.db.Where("entity_id = ?", survivor.ID).Find(&vals).Error)
	got := map[string]string{}
	for _, v := range vals {
		got[v.FieldDefinitionID] = string(v.Value)
	}
	assert.Equal(t, `"software"`, got[onlyLoser.ID])
	assert.Equal(t, `"survivor-size"`, got[both.ID], "the survivor's value wins")
	var left int64
	require.NoError(t, e.db.Model(&models.FieldValue{}).Where("entity_id = ?", loser.ID).Count(&left).Error)
	assert.EqualValues(t, 0, left, "the loser keeps no live values; the dropped one is in its snapshot")
}

// --- merge: restore round trip ---------------------------------------------

func TestEntityMerge_RestoringTheLoserRevivesOnlyTheEntity(t *testing.T) {
	e := newMergeEnv(t)
	survivor := e.org(t, "Acme")
	loser := e.org(t, "Acme Corp")
	both := models.FieldDefinition{UserID: e.userID, Label: "Size", Key: "size", Target: "organisation", Type: "string", Projection: "internal-only", Sensitivity: "normal"}
	require.NoError(t, e.db.Create(&both).Error)
	for _, id := range []string{loser.ID, survivor.ID} {
		w := e.do(http.MethodPut, "/entities/"+id+"/field-values", map[string]any{"field_values": []map[string]any{{"field_definition_id": both.ID, "value": "v-" + id[:4]}}})
		require.Equal(t, http.StatusOK, w.Code)
	}
	c := e.contact(t, nil)
	l := e.addLink(t, c, map[string]any{"entity_id": loser.ID, "role": "employee", "is_primary": true})
	e.mergeOK(t, loser.ID, survivor.ID, nil)

	w := e.do(http.MethodPost, "/entities/"+loser.ID+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var r models.EntityRestoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	assert.Equal(t, survivor.ID, r.MergedInto, "the response says the entity was merged away")
	assert.Equal(t, 0, r.LinksRevived, "no link comes back with a merge loser")

	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, "/entities/"+loser.ID, nil).Code)
	assert.Empty(t, e.liveLinks(t, loser.ID))
	assert.Equal(t, survivor.ID, e.link(t, l.ID).EntityID, "the re-pointed link stays with the survivor")
	assert.True(t, e.link(t, l.ID).IsPrimary)
	var vals []models.FieldValue
	require.NoError(t, e.db.Where("entity_id = ?", loser.ID).Find(&vals).Error)
	assert.Len(t, vals, 1, "the loser's own value is re-created from its snapshot")
	assert.Equal(t, 1, countPrimaries(t, e.linkEnv, c, models.EntityKindOrganisation))
}

func TestEntityRestore_PlainDeleteReportsLinksRevivedAndNoMerge(t *testing.T) {
	e := newMergeEnv(t)
	org := e.org(t, "Acme")
	c := e.contact(t, nil)
	e.addLink(t, c, map[string]any{"entity_id": org.ID, "role": "employee"})
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+org.ID, nil).Code)
	w := e.do(http.MethodPost, "/entities/"+org.ID+"/restore", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var r models.EntityRestoreResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
	assert.Equal(t, 1, r.LinksRevived)
	assert.Empty(t, r.MergedInto)
}

// --- merge: validation and ownership ---------------------------------------

func TestEntityMerge_RejectsSameEntityAndKindMismatch(t *testing.T) {
	e := newMergeEnv(t)
	org := e.org(t, "Acme")
	place := e.place(t, "Home")
	w := e.merge(org.ID, org.ID, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, string(apperrors.ReasonEntityMergeSameEntity), entityReason(t, w))
	w = e.merge(org.ID, place.ID, nil)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, string(apperrors.ReasonEntityMergeKindMismatch), entityReason(t, w))
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/entities/"+org.ID+"/merge", map[string]any{"into": "not-a-uuid"}).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/entities/"+org.ID+"/merge", map[string]any{}).Code)
	// still both live
	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, "/entities/"+org.ID, nil).Code)
}

func TestEntityMerge_ForeignAndTrashedEntitiesAre404AndWriteNothing(t *testing.T) {
	e := newMergeEnv(t)
	mine := e.org(t, "Mine")
	trashed := e.org(t, "Trashed")
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, "/entities/"+trashed.ID, nil).Code)
	_, fc, fent, fl := e.foreign(t)
	_ = fc
	c := e.contact(t, nil)
	myLink := e.addLink(t, c, map[string]any{"entity_id": mine.ID, "role": "employee"})

	for name, call := range map[string]*httptest.ResponseRecorder{
		"foreign loser":     e.merge(fent.ID, mine.ID, nil),
		"foreign survivor":  e.merge(mine.ID, fent.ID, nil),
		"trashed survivor":  e.merge(mine.ID, trashed.ID, nil),
		"trashed loser":     e.merge(trashed.ID, mine.ID, nil),
		"unknown loser":     e.merge("00000000-0000-4000-8000-000000000000", mine.ID, nil),
		"unknown survivor":  e.merge(mine.ID, "00000000-0000-4000-8000-000000000000", nil),
		"both foreign-ish":  e.merge(fent.ID, fent.ID, nil),
		"foreign with res.": e.merge(fent.ID, mine.ID, map[string]string{"url": "loser"}),
	} {
		assert.Equal(t, http.StatusNotFound, call.Code, "%s: %s", name, call.Body.String())
		assert.Equal(t, string(apperrors.ReasonEntityNotFound), entityReason(t, call), name)
	}
	var fresh models.Entity
	require.NoError(t, e.db.First(&fresh, "id = ?", mine.ID).Error)
	assert.Equal(t, mine.Revision, fresh.Revision)
	assert.Equal(t, mine.ID, e.link(t, myLink.ID).EntityID)
	var f models.ContactEntityLink
	require.NoError(t, e.db.First(&f, "id = ?", fl.ID).Error)
	assert.Equal(t, fent.ID, f.EntityID)
	var fe models.Entity
	require.NoError(t, e.db.First(&fe, "id = ?", fent.ID).Error, "the foreign entity is still live")
}

// --- suggestions -----------------------------------------------------------

func pairKey(p models.EntitySuggestionPair) string { return p.A.Name + "|" + p.B.Name + "|" + p.Reason }

func TestEntitySuggestions_MatchADRNormalizationTiers(t *testing.T) {
	e := newMergeEnv(t)
	e.org(t, "Acme Corp")
	e.org(t, "  acme   corp. ") // same name_key: exact
	e.org(t, "Acme, Inc")       // legal suffix tier only: normalized
	e.org(t, "Café Noir")
	e.org(t, "Cafe Noir") // no diacritic folding: no match
	e.org(t, "Globex")

	got := e.suggestions(t, "kind=organisation&limit=100")
	assert.Equal(t, "pairs", got.Mode)
	reasons := map[string]int{}
	for _, p := range got.Pairs {
		reasons[p.Reason]++
		assert.Less(t, p.A.ID, p.B.ID)
		assert.NotContains(t, pairKey(p), "Caf", "no diacritic folding")
	}
	assert.Equal(t, 1, reasons[models.EntitySuggestionReasonExactName])
	assert.Equal(t, 2, reasons[models.EntitySuggestionReasonNormalName], "Acme Inc pairs with each of the other two")
	assert.Len(t, got.Pairs, 3)
}

func TestEntitySuggestions_PlacesByAddressComponentsAndPetsByNameAndSpecies(t *testing.T) {
	e := newMergeEnv(t)
	mk := func(name, number, street string) {
		e.create(t, map[string]any{"kind": "place", "name": name, "core": map[string]any{"address": map[string]any{"components": []any{
			map[string]any{"kind": "number", "value": number}, map[string]any{"kind": "name", "value": street},
			map[string]any{"kind": "locality", "value": "Springfield"}}}}})
	}
	mk("Home", "12", "High St.")
	mk("Nana's", "12", "High  St")
	mk("Elsewhere", "13", "High St")
	pet := func(name, species string) {
		e.create(t, map[string]any{"kind": "pet", "name": name, "core": map[string]any{"species": species}})
	}
	pet("Rex", "dog")
	pet("REX", "Dog")
	pet("Rex", "cat")

	places := e.suggestions(t, "kind=place")
	require.Len(t, places.Pairs, 1)
	assert.Equal(t, models.EntitySuggestionReasonAddressParts, places.Pairs[0].Reason)
	pets := e.suggestions(t, "kind=pet")
	require.Len(t, pets.Pairs, 1, "same name but another species is not a candidate")
	assert.Equal(t, models.EntitySuggestionReasonExactName, pets.Pairs[0].Reason)
	assert.Len(t, e.suggestions(t, "").Pairs, 2, "no kind filter covers every kind")
}

func TestEntitySuggestions_AreReadOnlyIdempotentAndStablyPaged(t *testing.T) {
	e := newMergeEnv(t)
	for i := 0; i < 4; i++ {
		e.org(t, "Acme")
	}
	var entsBefore, linksBefore, dismissedBefore int64
	count := func() {
		require.NoError(t, e.db.Unscoped().Model(&models.Entity{}).Count(&entsBefore).Error)
		require.NoError(t, e.db.Unscoped().Model(&models.ContactEntityLink{}).Count(&linksBefore).Error)
		require.NoError(t, e.db.Model(&models.DismissedEntityPair{}).Count(&dismissedBefore).Error)
	}
	count()
	var revs []int64
	require.NoError(t, e.db.Model(&models.Entity{}).Order("id").Pluck("revision", &revs).Error)

	full := e.suggestions(t, "kind=organisation&limit=100")
	require.Len(t, full.Pairs, 6)
	again := e.suggestions(t, "kind=organisation&limit=100")
	assert.Equal(t, full.Pairs, again.Pairs, "idempotent")

	var walked []models.EntitySuggestionPair
	cursor := ""
	for page := 0; page < 10; page++ {
		r := e.suggestions(t, "kind=organisation&limit=2&cursor="+cursor)
		walked = append(walked, r.Pairs...)
		if r.NextCursor == "" {
			break
		}
		cursor = r.NextCursor
	}
	assert.Equal(t, full.Pairs, walked, "paging yields every pair exactly once in the same order")

	var entsAfter, linksAfter, dismissedAfter int64
	require.NoError(t, e.db.Unscoped().Model(&models.Entity{}).Count(&entsAfter).Error)
	require.NoError(t, e.db.Unscoped().Model(&models.ContactEntityLink{}).Count(&linksAfter).Error)
	require.NoError(t, e.db.Model(&models.DismissedEntityPair{}).Count(&dismissedAfter).Error)
	assert.Equal(t, entsBefore, entsAfter)
	assert.Equal(t, linksBefore, linksAfter)
	assert.Equal(t, dismissedBefore, dismissedAfter)
	var revsAfter []int64
	require.NoError(t, e.db.Model(&models.Entity{}).Order("id").Pluck("revision", &revsAfter).Error)
	assert.Equal(t, revs, revsAfter, "no entity was touched")

	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities/suggestions?cursor=***", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities/suggestions?limit=0", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities/suggestions?kind=robot", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities/suggestions?mode=other", nil).Code)
}

func TestEntitySuggestions_DismissedPairIsNeverResuggestedAndDismissIsIdempotent(t *testing.T) {
	e := newMergeEnv(t)
	a := e.org(t, "Acme")
	b := e.org(t, "Acme")
	third := e.org(t, "Acme")
	require.Len(t, e.suggestions(t, "kind=organisation").Pairs, 3)

	// either argument order, twice
	for i := 0; i < 2; i++ {
		w := e.do(http.MethodPost, "/entities/suggestions/dismiss", map[string]any{"entity_a": b.ID, "entity_b": a.ID})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var r models.EntityDismissResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &r))
		assert.Less(t, r.IDLow, r.IDHigh)
	}
	var n int64
	require.NoError(t, e.db.Model(&models.DismissedEntityPair{}).Count(&n).Error)
	assert.EqualValues(t, 1, n)
	got := e.suggestions(t, "kind=organisation")
	require.Len(t, got.Pairs, 2)
	for _, p := range got.Pairs {
		assert.False(t, (p.A.ID == a.ID && p.B.ID == b.ID) || (p.A.ID == b.ID && p.B.ID == a.ID), "dismissed pair came back")
	}

	// a merge of a member removes the dismissal naming the loser
	e.mergeOK(t, a.ID, third.ID, nil)
	require.NoError(t, e.db.Model(&models.DismissedEntityPair{}).Count(&n).Error)
	assert.EqualValues(t, 0, n, "the loser's dismissals go with it (EntityCascadeRegistry)")
}

func TestEntitySuggestions_DismissValidationAndOwnership(t *testing.T) {
	e := newMergeEnv(t)
	org := e.org(t, "Acme")
	org2 := e.org(t, "Acme")
	place := e.place(t, "Home")
	_, _, fent, _ := e.foreign(t)
	post := func(a, b string) *httptest.ResponseRecorder {
		return e.do(http.MethodPost, "/entities/suggestions/dismiss", map[string]any{"entity_a": a, "entity_b": b})
	}
	assert.Equal(t, http.StatusUnprocessableEntity, post(org.ID, org.ID).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, post(org.ID, place.ID).Code)
	assert.Equal(t, http.StatusNotFound, post(org.ID, fent.ID).Code)
	assert.Equal(t, http.StatusNotFound, post(fent.ID, org.ID).Code)
	assert.Equal(t, http.StatusNotFound, post(org.ID, "00000000-0000-4000-8000-000000000000").Code)
	assert.Equal(t, http.StatusBadRequest, post("x", org2.ID).Code)
	var n int64
	require.NoError(t, e.db.Model(&models.DismissedEntityPair{}).Count(&n).Error)
	assert.EqualValues(t, 0, n, "no failed dismissal wrote a row")
}

func TestEntitySuggestions_NeverIncludeAnotherUsersEntities(t *testing.T) {
	e := newMergeEnv(t)
	e.org(t, "Foreign Org") // mine, same name as the foreign one below
	_, _, _, _ = e.foreign(t)
	got := e.suggestions(t, "kind=organisation")
	assert.Empty(t, got.Pairs, "a foreign entity with the same name is not a candidate")
	vals := e.suggestions(t, "mode=contact_values&kind=organisation")
	assert.Empty(t, vals.Groups, "a foreign user's contacts never feed the helper")
}

// --- contact_values mode and the helper ------------------------------------

func orgContact(e *mergeEnv, t *testing.T, first string, names ...string) models.Contact {
	return e.contact(t, func(c *models.Contact) {
		c.Firstname = first
		for _, n := range names {
			c.Card.Organizations = append(c.Card.Organizations, contactmodel.Organization{ID: "o-" + n, Name: n})
		}
	})
}

func TestEntityHelper_GroupsContactValuesAndRequiresAKind(t *testing.T) {
	e := newMergeEnv(t)
	orgContact(e, t, "Ada", "Acme Corp")
	orgContact(e, t, "Bob", "Acme, Inc")
	orgContact(e, t, "Cy", "Acme Corp")
	orgContact(e, t, "Di", "Globex")
	e.contact(t, func(c *models.Contact) { c.Firstname = "NoOrg" })

	got := e.suggestions(t, "mode=contact_values&kind=organisation")
	assert.Equal(t, "contact_values", got.Mode)
	assert.Empty(t, got.Pairs)
	require.Len(t, got.Groups, 2)
	var acme models.EntityValueGroup
	for _, g := range got.Groups {
		if len(g.Contacts) == 3 {
			acme = g
		}
	}
	require.Len(t, acme.Contacts, 3)
	assert.Equal(t, "Acme Corp", acme.Name, "the most common spelling is proposed")
	assert.Equal(t, models.EntitySuggestionReasonNormalName, acme.Reason)
	assert.ElementsMatch(t, []string{"Acme Corp", "Acme, Inc"}, acme.Variants)

	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities/suggestions?mode=contact_values", nil).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodGet, "/entities/suggestions?mode=contact_values&kind=pet", nil).Code)

	// read-only: nothing was created
	var n int64
	require.NoError(t, e.db.Model(&models.Entity{}).Count(&n).Error)
	assert.EqualValues(t, 0, n)
}

func TestEntityHelper_CreatesEntitiesAndLinksWithoutTouchingContactValuesAndIsResumable(t *testing.T) {
	e := newMergeEnv(t)
	c1 := orgContact(e, t, "Ada", "Acme Corp")
	c2 := orgContact(e, t, "Bob", "Acme, Inc")
	c3 := orgContact(e, t, "Cy", "Acme Corp")
	cardBefore := map[uint]contactmodel.Card{}
	revBefore := map[uint]int64{}
	for _, c := range []models.Contact{c1, c2, c3} {
		cardBefore[c.ID] = contactCardJSON(t, e.linkEnv, c.ID)
		var fresh models.Contact
		require.NoError(t, e.db.First(&fresh, c.ID).Error)
		revBefore[c.ID] = fresh.Revision
	}
	// c2 already has an organisation primary elsewhere
	other := e.org(t, "Initech")
	e.addLink(t, c2, map[string]any{"entity_id": other.ID, "role": "employee", "is_primary": true})

	group := e.suggestions(t, "mode=contact_values&kind=organisation").Groups
	var acme models.EntityValueGroup
	for _, g := range group {
		if len(g.Contacts) == 3 {
			acme = g
		}
	}
	require.Len(t, acme.Contacts, 3)
	for _, c := range acme.Contacts {
		assert.Equal(t, c.ContactID == c2.ID, c.HasPrimary)
	}

	// confirm only c1 and c2, set primary
	w := e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "organisation", "groups": []map[string]any{
		{"key": acme.Key, "role": "employee", "set_primary": true, "contact_ids": []uint{c1.ID, c2.ID}}}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var out models.EntityFromContactsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Created, 1)
	assert.NotNil(t, out.Skipped)
	assert.Equal(t, "Acme Corp", out.Created[0].Entity.Name)
	assert.JSONEq(t, `{"organization":{"name":"Acme Corp"}}`, string(out.Created[0].Entity.Core))
	require.Len(t, out.Created[0].Links, 2)
	prim := map[string]bool{}
	for _, l := range out.Created[0].Links {
		prim[l.ContactVCardUID] = l.IsPrimary
	}
	assert.True(t, prim[c1.VCardUID], "primary when ticked and the contact has no other primary")
	assert.False(t, prim[c2.VCardUID], "a contact with another primary is never demoted by the helper")
	assert.Equal(t, 1, countPrimaries(t, e.linkEnv, c2, models.EntityKindOrganisation))

	// contact values untouched
	for _, c := range []models.Contact{c1, c2, c3} {
		assert.Equal(t, cardBefore[c.ID], contactCardJSON(t, e.linkEnv, c.ID))
		var fresh models.Contact
		require.NoError(t, e.db.First(&fresh, c.ID).Error)
		assert.Equal(t, revBefore[c.ID], fresh.Revision, "the helper never writes a contact")
	}

	// resumable: re-running proposes only the unmatched value (c3)
	again := e.suggestions(t, "mode=contact_values&kind=organisation").Groups
	var rest []models.EntityValueGroup
	for _, g := range again {
		if g.Name != "Initech" {
			rest = append(rest, g)
		}
	}
	require.Len(t, rest, 1)
	require.Len(t, rest[0].Contacts, 1)
	assert.Equal(t, c3.ID, rest[0].Contacts[0].ContactID)

	// a stale confirmation of the old key now links nothing new
	w = e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "organisation", "groups": []map[string]any{
		{"key": acme.Key, "role": "employee", "contact_ids": []uint{c1.ID}}}})
	require.Equal(t, http.StatusCreated, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Empty(t, out.Created)
	require.Len(t, out.Skipped, 1)
	assert.Equal(t, "contact_not_in_group", out.Skipped[0].Reason)
}

func TestEntityHelper_SkipsUnknownGroupsForeignContactsAndRejectsBadRoles(t *testing.T) {
	e := newMergeEnv(t)
	c1 := orgContact(e, t, "Ada", "Acme Corp")
	fo, _, _, _ := e.foreign(t)
	foreignContact := models.Contact{UserID: fo.ID, Firstname: "Eve"}
	foreignContact.Card.Organizations = []contactmodel.Organization{{ID: "x", Name: "Acme Corp"}}
	models.ApplyRecordToContact(&foreignContact, &contactmodel.Record{Card: foreignContact.Card}, "")
	foreignContact.Firstname = "Eve"
	require.NoError(t, e.db.Create(&foreignContact).Error)

	key := e.suggestions(t, "mode=contact_values&kind=organisation").Groups[0].Key
	post := func(groups ...map[string]any) (*httptest.ResponseRecorder, models.EntityFromContactsResponse) {
		w := e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "organisation", "groups": groups})
		var r models.EntityFromContactsResponse
		_ = json.Unmarshal(w.Body.Bytes(), &r)
		return w, r
	}
	w, r := post(map[string]any{"key": "org:nope", "role": "employee", "contact_ids": []uint{c1.ID}})
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Empty(t, r.Created)
	assert.Equal(t, "group_not_found", r.Skipped[0].Reason)

	w, r = post(map[string]any{"key": key, "role": "employee", "contact_ids": []uint{foreignContact.ID, 999999}})
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Empty(t, r.Created, "nothing was created for contacts that are not mine")
	require.Len(t, r.Skipped, 2)
	var ents, links int64
	require.NoError(t, e.db.Model(&models.Entity{}).Where("user_id = ?", e.userID).Count(&ents).Error)
	require.NoError(t, e.db.Model(&models.ContactEntityLink{}).Where("contact_vcard_uid = ?", foreignContact.VCardUID).Count(&links).Error)
	assert.EqualValues(t, 0, ents)
	assert.EqualValues(t, 0, links, "no link was written to the foreign contact")

	w, _ = post(map[string]any{"key": key, "role": "wizard", "contact_ids": []uint{c1.ID}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "pet", "groups": []map[string]any{{"key": "k", "role": "owner", "contact_ids": []uint{1}}}}).Code)
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "organisation", "groups": []map[string]any{}}).Code)
}

func TestEntityHelper_PlacesCarryAddressSensitivityAndName(t *testing.T) {
	e := newMergeEnv(t)
	mk := func(first, sens string) models.Contact {
		return e.contact(t, func(c *models.Contact) {
			c.Firstname = first
			c.Card.Addresses = []contactmodel.Address{{ID: "a-" + first, Sensitivity: sens, Components: []contactmodel.AddressComponent{
				{Kind: "number", Value: "12"}, {Kind: "name", Value: "High St."}, {Kind: "locality", Value: "Springfield"}}}}
		})
	}
	a := mk("Ada", "")
	b := mk("Bob", "private")
	groups := e.suggestions(t, "mode=contact_values&kind=place").Groups
	require.Len(t, groups, 1)
	assert.Equal(t, models.EntitySuggestionReasonAddressParts, groups[0].Reason)
	w := e.do(http.MethodPost, "/entities/from-contacts", map[string]any{"kind": "place", "groups": []map[string]any{
		{"key": groups[0].Key, "name": "The Flat", "role": "resident", "contact_ids": []uint{a.ID, b.ID}}}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var out models.EntityFromContactsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Created, 1)
	assert.Equal(t, "The Flat", out.Created[0].Entity.Name)
	assert.Equal(t, "private", out.Created[0].Entity.Sensitivity, "the entity takes the strictest address sensitivity")
	assert.NotContains(t, string(out.Created[0].Entity.Core), "sensitivity")
	assert.Empty(t, e.suggestions(t, "mode=contact_values&kind=place").Groups)
}
