package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issues #2202/#2203: every ?entity_id= list filter must (a) return exactly
// the narrowed rows (by id, not just count) and (b) return nothing when the
// uid belongs to another user, even though that user has rows on it.

// idsFromList GETs path and returns the "id" of every element under key.
func idsFromList(t *testing.T, router *gin.Engine, path, key string) []string {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	raw, ok := body[key]
	require.True(t, ok, "response missing %q: %s", key, w.Body.String())
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(raw, &rows))
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		switch v := r["id"].(type) {
		case string:
			ids = append(ids, v)
		case float64:
			ids = append(ids, strconv.FormatUint(uint64(v), 10))
		default:
			t.Fatalf("row without id: %v", r)
		}
	}
	return ids
}

type scopeFixture struct {
	me, other       models.User
	alice, bob      models.Contact // owned by me
	theirs, theirs2 models.Contact // owned by other
}

func newScopeFixture(t *testing.T, db *gorm.DB) *scopeFixture {
	t.Helper()
	f := &scopeFixture{}
	require.NoError(t, db.First(&f.me).Error)
	f.other = models.User{Username: "scope-other", Password: "x", Email: "scope-other@example.com"}
	require.NoError(t, db.Create(&f.other).Error)
	f.alice = models.Contact{UserID: f.me.ID, Firstname: "Alice", HowWeMet: "met Alice"}
	f.bob = models.Contact{UserID: f.me.ID, Firstname: "Bob", HowWeMet: "met Bob"}
	f.theirs = models.Contact{UserID: f.other.ID, Firstname: "Theirs", HowWeMet: "met Theirs"}
	f.theirs2 = models.Contact{UserID: f.other.ID, Firstname: "Theirs2"}
	for _, c := range []*models.Contact{&f.alice, &f.bob, &f.theirs, &f.theirs2} {
		require.NoError(t, db.Create(c).Error)
	}
	return f
}

func TestEntityIDListFilters_ExactIDsAndForeignUIDEmpty(t *testing.T) {
	cases := []struct {
		name, route, key string
		register         func(*gin.Engine)
		// seed creates one row for (userID, uid) and returns its id.
		seed func(t *testing.T, db *gorm.DB, userID uint, uid string) string
	}{
		{
			name: "gifts", route: "/gifts", key: "gifts",
			register: func(r *gin.Engine) { r.GET("/gifts", ListGifts) },
			seed: func(t *testing.T, db *gorm.DB, u uint, uid string) string {
				m := models.Gift{UserID: u, EntityID: uid, Description: "g"}
				require.NoError(t, db.Create(&m).Error)
				return m.ID
			},
		},
		{
			name: "life-events", route: "/life-events", key: "life_events",
			register: func(r *gin.Engine) { r.GET("/life-events", ListLifeEvents) },
			seed: func(t *testing.T, db *gorm.DB, u uint, uid string) string {
				m := models.LifeEvent{UserID: u, EntityID: uid, Type: models.LifeEventTypeMoved}
				require.NoError(t, db.Create(&m).Error)
				return m.ID
			},
		},
		{
			name: "preferences", route: "/preferences", key: "preferences",
			register: func(r *gin.Engine) { r.GET("/preferences", ListPreferences) },
			seed: func(t *testing.T, db *gorm.DB, u uint, uid string) string {
				m := models.Preference{UserID: u, EntityID: uid, Category: models.PreferenceCategoryHobby, Value: "v", Sensitivity: models.RelationshipSensitivityNormal}
				require.NoError(t, db.Create(&m).Error)
				return m.ID
			},
		},
		{
			name: "cadence-policies", route: "/cadence-policies", key: "cadence_policies",
			register: func(r *gin.Engine) { r.GET("/cadence-policies", ListCadencePolicies) },
			seed: func(t *testing.T, db *gorm.DB, u uint, uid string) string {
				m := models.CadencePolicy{UserID: u, EntityID: uid, TargetIntervalDays: 30}
				require.NoError(t, db.Create(&m).Error)
				return m.ID
			},
		},
		{
			name: "data-decay-policies", route: "/data-decay-policies", key: "data_decay_policies",
			register: func(r *gin.Engine) { r.GET("/data-decay-policies", ListDataDecayPolicies) },
			seed: func(t *testing.T, db *gorm.DB, u uint, uid string) string {
				m := models.DataDecayPolicy{UserID: u, EntityID: uid, IntervalDays: 90}
				require.NoError(t, db.Create(&m).Error)
				return m.ID
			},
		},
		{
			name: "conversation-agenda", route: "/conversation-agenda", key: "conversation_agenda",
			register: func(r *gin.Engine) { r.GET("/conversation-agenda", ListConversationAgenda) },
			seed: func(t *testing.T, db *gorm.DB, u uint, uid string) string {
				m := models.ConversationAgenda{UserID: u, EntityID: uid, Content: "ask"}
				require.NoError(t, db.Create(&m).Error)
				return m.ID
			},
		},
		{
			name: "occasion-obligations", route: "/occasion-obligations", key: "occasion_obligations",
			register: func(r *gin.Engine) { r.GET("/occasion-obligations", ListOccasionObligations) },
			seed: func(t *testing.T, db *gorm.DB, u uint, uid string) string {
				m := models.OccasionObligation{UserID: u, EntityID: uid, Kind: "birthday", Label: "Birthday"}
				require.NoError(t, db.Create(&m).Error)
				return m.ID
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, router := setupRouter(t)
			tc.register(router)
			f := newScopeFixture(t, db)
			aliceID := tc.seed(t, db, f.me.ID, f.alice.VCardUID)
			bobID := tc.seed(t, db, f.me.ID, f.bob.VCardUID)
			theirID := tc.seed(t, db, f.other.ID, f.theirs.VCardUID)

			assert.Equal(t, []string{aliceID}, idsFromList(t, router, tc.route+"?entity_id="+f.alice.VCardUID, tc.key),
				"narrowed query must return exactly alice's row")
			assert.Equal(t, []string{bobID}, idsFromList(t, router, tc.route+"?entity_id="+f.bob.VCardUID, tc.key))

			assert.Empty(t, idsFromList(t, router, tc.route+"?entity_id="+f.theirs.VCardUID, tc.key),
				"another user's uid must yield an empty list, not their rows")

			all := idsFromList(t, router, tc.route, tc.key)
			assert.ElementsMatch(t, []string{aliceID, bobID}, all)
			assert.NotContains(t, all, theirID, "unfiltered list must not include another user's row")
		})
	}
}

func TestListRelationshipEdgesContactID_ExactIDsAndForeignUIDEmpty(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/relationship-edges", ListRelationshipEdges)
	f := newScopeFixture(t, db)
	carol := models.Contact{UserID: f.me.ID, Firstname: "Carol"}
	require.NoError(t, db.Create(&carol).Error)

	mk := func(uid uint, src, tgt string) string {
		e := models.RelationshipEdge{
			UserID: uid, SourceID: src, TargetID: tgt, Type: "friend_of",
			Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0, Status: models.RelationshipStatusConfirmed,
		}
		require.NoError(t, db.Create(&e).Error)
		return e.ID
	}
	asSource := mk(f.me.ID, f.alice.VCardUID, f.bob.VCardUID)
	asTarget := mk(f.me.ID, carol.VCardUID, f.alice.VCardUID)
	unrelated := mk(f.me.ID, f.bob.VCardUID, carol.VCardUID)
	theirEdge := mk(f.other.ID, f.theirs.VCardUID, f.theirs2.VCardUID)

	got := idsFromList(t, router, "/relationship-edges?contact_id="+f.alice.VCardUID, "relationship_edges")
	assert.ElementsMatch(t, []string{asSource, asTarget}, got)
	assert.NotContains(t, got, unrelated)

	assert.Empty(t, idsFromList(t, router, "/relationship-edges?contact_id="+f.theirs.VCardUID, "relationship_edges"),
		"another user's contact uid must yield no edges")
	all := idsFromList(t, router, "/relationship-edges", "relationship_edges")
	assert.ElementsMatch(t, []string{asSource, asTarget, unrelated}, all)
	assert.NotContains(t, all, theirEdge)
}

func TestListAuditEventsEntityID_ExactIDsAndForeignUIDEmpty(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/audit", ListAuditEvents)
	f := newScopeFixture(t, db)

	mk := func(uid uint, entityID, op string) string {
		e := models.AuditEvent{UserID: uid, EntityType: models.AuditEntityContact, EntityID: entityID, Operation: op}
		require.NoError(t, db.Create(&e).Error)
		return strconv.FormatUint(uint64(e.ID), 10)
	}
	aliceEv := mk(f.me.ID, f.alice.VCardUID, models.AuditOpUpdate)
	bobEv := mk(f.me.ID, f.bob.VCardUID, models.AuditOpUpdate)
	theirEv := mk(f.other.ID, f.theirs.VCardUID, models.AuditOpUpdate)

	got := idsFromList(t, router, "/audit?entity_id="+f.alice.VCardUID, "audit_events")
	assert.Contains(t, got, aliceEv)
	assert.NotContains(t, got, bobEv)
	assert.NotContains(t, got, theirEv)
	// Contact-creation hooks may add their own events for alice; the narrowed
	// list must equal exactly the caller's stored events for that uid.
	var stored []models.AuditEvent
	require.NoError(t, db.Where("user_id = ? AND entity_id = ?", f.me.ID, f.alice.VCardUID).Find(&stored).Error)
	assert.Len(t, got, len(stored))

	// The other user has an event on this uid; the caller must not see it.
	assert.Empty(t, idsFromList(t, router, "/audit?entity_id="+f.theirs.VCardUID, "audit_events"),
		"another user's entity uid must yield no audit events")
}

func TestExportPreflightVCardUID_NarrowsAndForeignUIDEmpty(t *testing.T) {
	db, router := setupRouter(t)
	registerPreflightRoute(router)
	f := newScopeFixture(t, db)

	resp := preflightReports(t, router, "?format=vcard4&vcard_uid="+f.alice.VCardUID)
	assert.Equal(t, 1, resp.ContactCount)
	require.NotEmpty(t, resp.Diagnostics)
	for _, d := range resp.Diagnostics {
		assert.Equal(t, "Alice", d.ContactName, "only the narrowed contact's loss report may appear")
	}

	foreign := preflightReports(t, router, "?format=vcard4&vcard_uid="+f.theirs.VCardUID)
	assert.Equal(t, 0, foreign.ContactCount)
	assert.Empty(t, foreign.Diagnostics, "another user's contact must produce no loss report")

	all := preflightReports(t, router, "?format=vcard4")
	assert.Equal(t, 2, all.ContactCount)
	for _, d := range all.Diagnostics {
		assert.NotEqual(t, "Theirs", d.ContactName)
	}
}
