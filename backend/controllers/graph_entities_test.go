package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type graphEntityEnv struct {
	db     *gorm.DB
	router http.Handler
	user   models.User
}

func newGraphEntityEnv(t *testing.T) *graphEntityEnv {
	t.Helper()
	db, router := setupRouter(t)
	router.GET("/graph", GetGraph)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	return &graphEntityEnv{db: db, router: router, user: user}
}

func (g *graphEntityEnv) contact(t *testing.T, name string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: g.user.ID, Firstname: name}
	require.NoError(t, g.db.Create(&c).Error)
	return c
}

func (g *graphEntityEnv) entity(t *testing.T, kind, name, sens string) models.Entity {
	t.Helper()
	e := models.Entity{UserID: g.user.ID, Kind: kind, Name: name, Sensitivity: sens}
	require.NoError(t, g.db.Create(&e).Error)
	return e
}

func (g *graphEntityEnv) link(t *testing.T, c models.Contact, e models.Entity, sens string) models.ContactEntityLink {
	t.Helper()
	role := map[string]string{"organisation": "employee", "place": "resident", "pet": "owner"}[e.Kind]
	l := models.ContactEntityLink{UserID: g.user.ID, ContactVCardUID: c.VCardUID, EntityID: e.ID, Kind: e.Kind, Role: role, Sensitivity: sens}
	require.NoError(t, g.db.Create(&l).Error)
	return l
}

func (g *graphEntityEnv) raw(t *testing.T, path string) (string, map[string]json.RawMessage) {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	g.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
	return w.Body.String(), m
}

func TestGetGraph_IncludeEntities_DefaultResponseIsByteIdentical(t *testing.T) {
	g := newGraphEntityEnv(t)
	a, b := g.contact(t, "A"), g.contact(t, "B")
	seedFriendEdge(t, g.db, g.user.ID, a, b)
	acme := g.entity(t, "organisation", "Acme", "normal")
	g.link(t, a, acme, "normal")

	before, _ := g.raw(t, "/graph")
	_, withKeys := g.raw(t, "/graph")
	assert.NotContains(t, withKeys, "entity_nodes")
	assert.NotContains(t, withKeys, "entity_edges")
	for _, q := range []string{"/graph?include_entities=false", "/graph?include_entities=", "/graph?include_entities=bogus"} {
		got, _ := g.raw(t, q)
		assert.Equal(t, before, got, q)
	}
}

func TestGetGraph_IncludeEntities_SeparateKeysAndLiveNormalOnly(t *testing.T) {
	g := newGraphEntityEnv(t)
	a, b := g.contact(t, "A"), g.contact(t, "B")
	acme := g.entity(t, "organisation", "Acme", "normal")
	home := g.entity(t, "place", "Home", "normal")
	rex := g.entity(t, "pet", "Rex", "normal")
	secretCo := g.entity(t, "organisation", "SecretCo", "private")
	unlinked := g.entity(t, "organisation", "Nobody", "normal")
	_ = unlinked
	g.link(t, a, acme, "normal")
	g.link(t, b, acme, "normal")
	g.link(t, a, home, "normal")
	g.link(t, a, rex, "normal")
	g.link(t, a, secretCo, "normal")                                 // entity above normal
	g.link(t, b, g.entity(t, "place", "Cabin", "normal"), "private") // link above normal
	gone := g.entity(t, "organisation", "Gone", "normal")
	dead := g.link(t, b, gone, "normal")
	require.NoError(t, g.db.Delete(&dead).Error) // soft-deleted link
	trashed := g.entity(t, "organisation", "Trashed", "normal")
	g.link(t, b, trashed, "normal")
	require.NoError(t, g.db.Delete(&trashed).Error) // soft-deleted entity

	_, m := g.raw(t, "/graph?include_entities=true")
	var nodes []models.GraphEntityNode
	var edges []models.GraphEntityEdge
	require.NoError(t, json.Unmarshal(m["entity_nodes"], &nodes))
	require.NoError(t, json.Unmarshal(m["entity_edges"], &edges))

	labels := map[string]string{}
	for _, n := range nodes {
		labels[n.Label] = n.Kind
	}
	assert.Equal(t, map[string]string{"Acme": "organisation", "Home": "place", "Rex": "pet"}, labels)
	assert.Len(t, edges, 4, "a-acme, b-acme, a-home, a-rex")
	for _, e := range edges {
		assert.Regexp(t, `^c-\d+$`, e.Source)
		assert.Regexp(t, `^e-`, e.Target)
	}

	// Entity data never enters the regular arrays and carries no extra fields.
	var plain []models.GraphNode
	require.NoError(t, json.Unmarshal(m["nodes"], &plain))
	assert.Len(t, plain, 2)
	var rawNodes []map[string]any
	require.NoError(t, json.Unmarshal(m["entity_nodes"], &rawNodes))
	for _, n := range rawNodes {
		assert.ElementsMatch(t, []string{"id", "kind", "label"}, keysOf(n))
	}
	var rawEdges []map[string]any
	require.NoError(t, json.Unmarshal(m["entity_edges"], &rawEdges))
	for _, e := range rawEdges {
		assert.ElementsMatch(t, []string{"id", "source", "target"}, keysOf(e), "no type, role, dates or status")
	}
}

func TestGetGraph_IncludeEntities_EmptyIsArraysNotAbsent(t *testing.T) {
	g := newGraphEntityEnv(t)
	g.contact(t, "A")
	_, m := g.raw(t, "/graph?include_entities=true")
	assert.JSONEq(t, `[]`, string(m["entity_nodes"]))
	assert.JSONEq(t, `[]`, string(m["entity_edges"]))
}

func TestGetGraph_IncludeEntities_EgoScopeCountsEntityHopAsOne(t *testing.T) {
	g := newGraphEntityEnv(t)
	self, a, b := g.contact(t, "Self"), g.contact(t, "A"), g.contact(t, "B")
	require.NoError(t, g.db.Model(&g.user).Update("self_contact_vcard_uid", self.VCardUID).Error)
	seedFriendEdge(t, g.db, g.user.ID, self, a)
	seedFriendEdge(t, g.db, g.user.ID, a, b)
	g.link(t, self, g.entity(t, "organisation", "SelfCo", "normal"), "normal")
	g.link(t, a, g.entity(t, "organisation", "ACo", "normal"), "normal")
	g.link(t, b, g.entity(t, "organisation", "BCo", "normal"), "normal")

	labelsAt := func(path string) []string {
		_, m := g.raw(t, path)
		var nodes []models.GraphEntityNode
		require.NoError(t, json.Unmarshal(m["entity_nodes"], &nodes))
		var out []string
		for _, n := range nodes {
			out = append(out, n.Label)
		}
		return out
	}
	assert.ElementsMatch(t, []string{"SelfCo"}, labelsAt("/graph?full=false&depth=1&include_entities=true"))
	assert.ElementsMatch(t, []string{"SelfCo", "ACo"}, labelsAt("/graph?full=false&depth=2&include_entities=true"))
	assert.ElementsMatch(t, []string{"SelfCo", "ACo", "BCo"}, labelsAt("/graph?include_entities=true"))
}

func TestGetGraph_IncludeEntities_DoesNotAffectTraversalOrOtherUsers(t *testing.T) {
	g := newGraphEntityEnv(t)
	a, b := g.contact(t, "A"), g.contact(t, "B")
	acme := g.entity(t, "organisation", "Acme", "normal")
	g.link(t, a, acme, "normal")
	g.link(t, b, acme, "normal") // colleagues sharing an employer are NOT related

	// A foreign user's entity linked (by forged row) to our contact never shows.
	other := models.User{Username: "gother", Password: "x", Email: "gother@example.com"}
	require.NoError(t, g.db.Create(&other).Error)
	foreign := models.Entity{UserID: other.ID, Kind: "organisation", Name: "Foreign"}
	require.NoError(t, g.db.Create(&foreign).Error)
	require.NoError(t, g.db.Create(&models.ContactEntityLink{UserID: g.user.ID, ContactVCardUID: a.VCardUID, EntityID: foreign.ID, Kind: "organisation", Role: "employee"}).Error)

	_, m := g.raw(t, "/graph?include_entities=true")
	var nodes []models.GraphEntityNode
	require.NoError(t, json.Unmarshal(m["entity_nodes"], &nodes))
	require.Len(t, nodes, 1)
	assert.Equal(t, "Acme", nodes[0].Label)
	var edges []models.GraphEdge
	require.NoError(t, json.Unmarshal(m["edges"], &edges))
	assert.Empty(t, edges, "a shared employer creates no person-to-person edge")
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
