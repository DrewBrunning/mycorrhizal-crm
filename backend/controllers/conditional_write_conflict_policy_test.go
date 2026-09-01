package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are CON-03 (issue #458) tests: they pin the REST write-conflict policy
// recorded in docs/adrs/0009-rest-conflict-policy.md — reject-and-return, no
// silent merge, no automatic merge — per entity shape. Real migrated schema
// via dbtest (CLAUDE.md backend trap #1).

func cwPhones(resp models.ContactRecordResponse) []string {
	out := make([]string, 0, len(resp.Card.Phones))
	for _, p := range resp.Card.Phones {
		out = append(out, p.Number)
	}
	return out
}

func cwContactByCard(phones ...string) models.ContactRecordInput {
	ph := make([]contactmodel.Phone, 0, len(phones))
	for _, n := range phones {
		ph = append(ph, contactmodel.Phone{Number: n})
	}
	return models.ContactRecordInput{Card: contactmodel.Card{
		Name:   &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Alice"}}},
		Phones: ph,
	}}
}

// TestConflictPolicy_DisjointScalarFieldsSequenceToBothChanges is the ADR 0009
// "disjoint fields" consequence: a concurrent edit of a *different* scalar
// field is still a 412 (the model is row-level, not field-level), the loser
// re-reads and re-applies, and the net effect is both changes — applied in
// sequence by clients that each saw the other's.
func TestConflictPolicy_DisjointScalarFieldsSequenceToBothChanges(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))

	body := func(given, nickname string) models.ContactRecordInput {
		in := models.ContactRecordInput{Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: given}}},
		}}
		if nickname != "" {
			in.Card.Nicknames = []contactmodel.Nickname{{Name: nickname}}
		}
		return in
	}

	// Client A renames (rev 1 -> 2), no nickname.
	w := env.do("PUT", "/contacts/"+id, `"1"`, body("Alice-Renamed", ""))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Client B, still on revision 1, edits a *disjoint* field (nickname). Still 412.
	w = env.do("PUT", "/contacts/"+id, `"1"`, body("Alice", "Al"))
	assert412(t, w)

	// B re-reads (revision 2, sees A's rename) and re-applies its nickname edit.
	w = env.do("PUT", "/contacts/"+id, `"2"`, body("Alice-Renamed", "Al"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Net effect: both changes present.
	var got models.Contact
	require.NoError(t, env.db.First(&got, env.alice.ID).Error)
	assert.Equal(t, "Alice-Renamed", got.Firstname, "A's rename survived")
	require.Len(t, resp.Card.Nicknames, 1, "B's nickname edit landed")
	assert.Equal(t, "Al", resp.Card.Nicknames[0].Name)
	assert.EqualValues(t, 3, resp.Revision)
}

// TestConflictPolicy_SameScalarFieldConcurrentEdit: the classic conflict — two
// clients edit the same field. The stale one is rejected; the server does not
// merge or pick a winner by content, it just keeps what is stored.
func TestConflictPolicy_SameScalarFieldConcurrentEdit(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))
	body := func(given string) models.ContactRecordInput {
		return models.ContactRecordInput{Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: given}}},
		}}
	}

	w := env.do("PUT", "/contacts/"+id, `"1"`, body("A-wins"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = env.do("PUT", "/contacts/"+id, `"1"`, body("B-loses"))
	assert412(t, w)

	var got models.Contact
	require.NoError(t, env.db.First(&got, env.alice.ID).Error)
	assert.Equal(t, "A-wins", got.Firstname)
	assert.EqualValues(t, 2, got.Revision)
}

// TestConflictPolicy_RepeatableFieldConcurrentAdd_NoSilentDrop is the ADR 0009
// repeatable-field rule and the ticket's hand-verify anchor: PUT replaces the
// list wholesale, so without the conditional-write guard client B's
// single-phone PUT would silently drop client A's phone. The guard makes the
// losing write a clean 412 with A's entry intact; the union is the client's
// job after a re-read.
//
// Hand-verify (CLAUDE.md): replace checkIfMatch in UpdateContact with a merge
// that keeps only the incoming list, and this test fails naming "phones".
func TestConflictPolicy_RepeatableFieldConcurrentAdd_NoSilentDrop(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))

	// Client A adds a phone (rev 1 -> 2).
	w := env.do("PUT", "/contacts/"+id, `"1"`, cwContactByCard("111-A"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var aResp models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &aResp))
	require.Equal(t, []string{"111-A"}, cwPhones(aResp))

	// Client B, still on revision 1, adds a *different* phone. Its PUT carries
	// only its own phone (it never saw A's). Rejected — not merged, not applied.
	w = env.do("PUT", "/contacts/"+id, `"1"`, cwContactByCard("222-B"))
	assert412(t, w)

	// A's phone was not dropped, and B's was not silently merged in.
	var stored models.Contact
	require.NoError(t, env.db.First(&stored, env.alice.ID).Error)
	getResp := env.do("PUT", "/contacts/"+id, `"2"`, cwContactByCard("111-A"))
	require.Equal(t, http.StatusOK, getResp.Code)
	var afterReject models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &afterReject))
	assert.Equal(t, []string{"111-A"}, cwPhones(afterReject),
		"the rejected write must not have dropped A's phone or merged in B's")
	_ = stored

	// B re-reads (now at revision 3), performs the union client-side, re-PUTs.
	w = env.do("PUT", "/contacts/"+id, `"3"`, cwContactByCard("111-A", "222-B"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var union models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &union))
	assert.ElementsMatch(t, []string{"111-A", "222-B"}, cwPhones(union),
		"once the client does the union explicitly, both entries persist")
}

// TestConflictPolicy_RESTNeverSilentlyMerges: a rejected write leaves the
// repeatable field exactly as stored — never a union like [X, Y, Z] appearing
// on its own.
func TestConflictPolicy_RESTNeverSilentlyMerges(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))

	w := env.do("PUT", "/contacts/"+id, `"1"`, cwContactByCard("X", "Y"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = env.do("PUT", "/contacts/"+id, `"1"`, cwContactByCard("Z"))
	assert412(t, w)

	w = env.do("PUT", "/contacts/"+id, `"2"`, cwContactByCard("X", "Y"))
	require.Equal(t, http.StatusOK, w.Code)
	var resp models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []string{"X", "Y"}, cwPhones(resp), "no silent union with the rejected write's Z")
}

// TestConflictPolicy_RelationshipEdgeDirectionNeverInverted pins ADR 0009's
// "relationship direction is an invariant, not a policy": RelationshipEdge has
// no revision token (LWW), but an update writes exactly the (source, target,
// type) the client sent — the server never derives and persists the inverse,
// so concurrent updates can only replace the row with another correctly
// oriented row, never flip A parent_of B into B parent_of A.
func TestConflictPolicy_RelationshipEdgeDirectionNeverInverted(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "edgeconf", Password: "password123!A", Email: "edgeconf@example.com"}
	require.NoError(t, db.Create(&user).Error)
	parent := models.Contact{UserID: user.ID, Firstname: "Parent"}
	child := models.Contact{UserID: user.ID, Firstname: "Child"}
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&child).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.POST("/relationship-edges", withValidated(func() any { return &models.RelationshipEdgeInput{} }), CreateRelationshipEdge)
	router.PUT("/relationship-edges/:id", withValidated(func() any { return &models.RelationshipEdgeInput{} }), UpdateRelationshipEdge)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// A is B's parent: source=parent, target=child, type=parent_of.
	w := do("POST", "/relationship-edges", models.RelationshipEdgeInput{
		SourceID: parent.VCardUID, TargetID: child.VCardUID, Type: "parent_of",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		RelationshipEdge models.RelationshipEdge `json:"relationship_edge"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	edgeID := created.RelationshipEdge.ID
	require.NotEmpty(t, edgeID)

	// Two concurrent-shaped updates, both sending the correctly-oriented edge
	// (e.g. one tweaks metadata/sensitivity). LWW — the last one stored wins.
	w = do("PUT", "/relationship-edges/"+edgeID, models.RelationshipEdgeInput{
		SourceID: parent.VCardUID, TargetID: child.VCardUID, Type: "parent_of", Sensitivity: "private",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = do("PUT", "/relationship-edges/"+edgeID, models.RelationshipEdgeInput{
		SourceID: parent.VCardUID, TargetID: child.VCardUID, Type: "parent_of", Sensitivity: "normal",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The stored row is still oriented parent -> child, and there is exactly
	// one edge row for this pair — no derived `child_of` inverse was ever
	// persisted.
	var edges []models.RelationshipEdge
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&edges).Error)
	require.Len(t, edges, 1, "no phantom inverse edge row")
	assert.Equal(t, parent.VCardUID, edges[0].SourceID, "source is still the parent")
	assert.Equal(t, child.VCardUID, edges[0].TargetID, "target is still the child")
	assert.Equal(t, "parent_of", edges[0].Type, "type never flipped to child_of")

	var inverseCount int64
	require.NoError(t, db.Model(&models.RelationshipEdge{}).
		Where("source_id = ? AND target_id = ? AND type = ?", child.VCardUID, parent.VCardUID, "child_of").
		Count(&inverseCount).Error)
	assert.Zero(t, inverseCount, "the derived inverse is never persisted (ADR 0009 invariant)")
}
