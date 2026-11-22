package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type sgLabelRow struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (e *sgEnv) labels(u models.User, path string) map[string]string {
	e.t.Helper()
	w := e.do(u, http.MethodGet, path, nil)
	require.Equal(e.t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Grants []sgLabelRow `json:"share_grants"`
	}
	require.NoError(e.t, json.Unmarshal(w.Body.Bytes(), &out))
	m := map[string]string{}
	for _, g := range out.Grants {
		m[g.ID] = g.Label
	}
	return m
}

// Issue #2744 (item 4): a note grant's label is the first line of the note,
// capped, so share rows are not all just "Note".
func TestShareGrantNoteLabel_OwnerSeesFirstLine(t *testing.T) {
	e := newSGEnv(t)
	long := strings.Repeat("é", 120)
	cases := []struct {
		content, want string
	}{
		{"Gift ideas for Mum\nsecond line is body text", "Gift ideas for Mum"},
		{"\n\n   Leading blank lines skipped  \nbody", "Leading blank lines skipped"},
		{"Tabs\tand   runs of   space", "Tabs and runs of space"},
		{"Windows line ending\r\nbody", "Windows line ending"},
		{long, strings.Repeat("é", 79) + "…"},
		{"   \n  ", "Note"},
	}
	ids := map[string]string{}
	for _, tc := range cases {
		n := e.note(e.A, tc.content)
		ids[tc.content] = e.grant(e.A, e.B, "note", n.UUID, nil)
	}
	got := e.labels(e.A, "/share-grants/outgoing")
	for _, tc := range cases {
		assert.Equal(t, tc.want, got[ids[tc.content]], "content %q", tc.content)
		assert.LessOrEqual(t, len([]rune(got[ids[tc.content]])), 80)
	}

	// The single-grant owner view (create/patch/revoke responses) agrees.
	n := e.note(e.A, "Created now\nbody")
	w := e.do(e.A, http.MethodPost, "/share-grants", map[string]any{"to_user_id": e.C.ID, "entity_type": "note", "entity_id": n.UUID})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"label":"Created now"`)
}

// The recipient gets the note text in the label only once the grant is active,
// i.e. exactly when the content endpoint would already give them the text. A
// pending grant keeps the generic label (ADR 0042 decision 7).
func TestShareGrantNoteLabel_RecipientOnlyWhenActive(t *testing.T) {
	e := newSGEnv(t)
	n := e.note(e.A, "Private plans\nbody text")
	id := e.grant(e.A, e.B, "note", n.UUID, nil)

	assert.Equal(t, "Note", e.labels(e.B, "/share-grants/incoming")[id], "pending: generic label only")
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String(), "Private plans")

	require.Equal(t, http.StatusOK, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	assert.Equal(t, "Private plans", e.labels(e.B, "/share-grants/incoming")[id])
	// The accept response is a recipient view too.
	// (Already active at this point: re-accept is idempotent and returns it.)
	w := e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"label":"Private plans"`)
	// Only the first line, never the body.
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String(), "body text")

	// Once the grant stops being active the text stops being offered.
	require.NoError(t, e.db.Model(&models.ShareGrant{}).Where("id = ?", id).Update("expires_at", e.clock.Now().Add(-time.Minute)).Error)
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String(), "Private plans")
	// An unrelated user never sees it.
	assert.NotContains(t, e.do(e.C, http.MethodGet, "/share-grants/incoming", nil).Body.String(), "Private plans")
}

// The label lookup is scoped by the grant's owner: a foreign note with the same
// uuid (a forged or stale row) never supplies the label.
func TestShareGrantNoteLabel_CrossOwnerSameUUID(t *testing.T) {
	e := newSGEnv(t)
	const uid = "99999999-2222-4333-8444-555555555555"
	foreign := models.Note{UserID: e.C.ID, Content: "FOREIGN SECRET", Date: e.clock.Now(), UUID: uid}
	own := models.Note{UserID: e.A.ID, Content: "Owner side", Date: e.clock.Now(), UUID: uid}
	// Foreign row first so a query that lost its owner scope would return it.
	require.NoError(t, e.db.Create(&foreign).Error)
	require.NoError(t, e.db.Create(&own).Error)
	id := e.grant(e.A, e.B, "note", uid, nil)
	require.Equal(t, http.StatusOK, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)

	assert.Equal(t, "Owner side", e.labels(e.A, "/share-grants/outgoing")[id])
	assert.Equal(t, "Owner side", e.labels(e.B, "/share-grants/incoming")[id])
	assert.NotContains(t, e.do(e.A, http.MethodGet, "/share-grants/outgoing", nil).Body.String(), "FOREIGN")
	assert.NotContains(t, e.do(e.B, http.MethodGet, "/share-grants/incoming", nil).Body.String(), "FOREIGN")

	// A deleted note falls back to the generic label.
	require.NoError(t, e.db.Delete(&own).Error)
	assert.Equal(t, "Note", e.labels(e.A, "/share-grants/outgoing")[id])
}

// A list of note grants costs one notes query, not one per row.
func TestShareGrantNoteLabel_ListIsNotNPlusOne(t *testing.T) {
	e := newSGEnv(t)
	for i := 0; i < 6; i++ {
		n := e.note(e.A, "Title "+string(rune('A'+i)))
		id := e.grant(e.A, e.B, "note", n.UUID, nil)
		require.Equal(t, http.StatusOK, e.do(e.B, http.MethodPost, "/share-grants/"+id+"/accept", nil).Code)
	}
	var noteQueries int
	cbName := "count_note_queries"
	require.NoError(t, e.db.Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement.Table == "notes" {
			noteQueries++
		}
	}))
	t.Cleanup(func() { _ = e.db.Callback().Query().Remove(cbName) })

	for _, p := range []struct {
		u    models.User
		path string
	}{{e.A, "/share-grants/outgoing"}, {e.B, "/share-grants/incoming"}} {
		noteQueries = 0
		got := e.labels(p.u, p.path)
		require.Len(t, got, 6)
		assert.LessOrEqual(t, noteQueries, 1, "%s: one batched notes query for all rows", p.path)
		assert.Contains(t, got[firstKey(got)], "Title")
	}
}

func firstKey(m map[string]string) string {
	for k := range m {
		return k
	}
	return ""
}
