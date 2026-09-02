// SEARCH-03 (issue #463): mutation-path search tests, handler level. The
// companion services/search_mutation_path_test.go covers the trigger
// state-transition matrix, CardDAV reconcile, bulk import and the structural
// guard; this file covers the paths that only exist as HTTP handlers:
//
//   - REST create / update / delete of a contact — asserting the
//     ApplyRecordToContact + db.Save handler path (CLAUDE.md backend trap #2:
//     the flat, trigger-indexed columns are derived by BeforeSave, never set
//     directly) keeps the index correct;
//   - archive / unarchive — an archived contact stays in the index but must
//     not be returned by search; unarchiving restores it;
//   - contact merge (POST /contacts/merge) — the loser's unique searchable
//     content disappears, the keeper absorbs it, no orphan index row;
//   - the audit Undo path (POST /audit/:id/undo) — reverting an update also
//     reverts the index.
//
// After every mutation the index is checked against a fresh
// services.RebuildSearchIndex and services.CheckSearchIndexConsistency (issue
// #463 action 2), never by asserting a trigger fired.
//
// Real migrated schema via dbtest.New (CLAUDE.md trap #1): contacts_fts and
// its triggers exist only in the hand-written migration SQL.
package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// smpRouter builds a real-migrated-schema router with every route the
// mutation-path tests exercise, scoped to one user.
func smpRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() {
		models.AuditFlush()
		models.RegisterAuditDB(nil)
	})

	user := models.User{Username: "smp-user", Password: "password123!A", Email: "smp@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{AuditRetentionDays: 90})
		c.Next()
	})

	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
	router.DELETE("/contacts/:id", DeleteContact)
	router.POST("/contacts/:id/archive", ArchiveContact)
	router.POST("/contacts/:id/unarchive", UnarchiveContact)
	router.POST("/contacts/merge", withValidated(func() any { return &models.ContactMergeRequest{} }), CommitContactMerge)
	router.GET("/contacts", GetContacts)
	router.GET("/search", SearchAll)
	router.POST("/notes", withValidated(func() any { return &models.NoteInput{} }), CreateUnassignedNote)
	router.PUT("/notes/:id", withValidated(func() any { return &models.NoteInput{} }), UpdateNote)
	router.DELETE("/notes/:id", DeleteNote)
	router.POST("/activities", withValidated(func() any { return &models.ActivityInput{} }), CreateActivity)
	router.PUT("/activities/:id", withValidated(func() any { return &models.ActivityInput{} }), UpdateActivity)
	router.DELETE("/activities/:id", DeleteActivity)

	return db, router, user
}

// smpDo issues one JSON request and returns the recorder.
func smpDo(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// smpSearchHits returns the ids from GET /search for a term (contacts unless
// kind says otherwise).
func smpSearchIDs(t *testing.T, router *gin.Engine, term, kind string) []float64 {
	t.Helper()
	w := smpDo(t, router, "GET", "/search?q="+url.QueryEscape(term), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	raw, _ := body[kind].([]any)
	var ids []float64
	for _, r := range raw {
		m := r.(map[string]any)
		// contacts serialise the PK as "id" (explicit json tag); notes and
		// activities embed gorm.Model, whose ID has no tag → "ID".
		id, ok := m["id"].(float64)
		if !ok {
			id, ok = m["ID"].(float64)
		}
		if ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func smpContains(ids []float64, want uint) bool {
	for _, id := range ids {
		if uint(id) == want {
			return true
		}
	}
	return false
}

// smpListSearchIDs runs GET /contacts?search= (applyContactSearch — the path
// with the archived filter) and returns the contact ids.
func smpListSearchIDs(t *testing.T, router *gin.Engine, term string) []uint {
	t.Helper()
	w := smpDo(t, router, "GET", "/contacts?search="+url.QueryEscape(term), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	raw, _ := body["contacts"].([]any)
	var ids []uint
	for _, r := range raw {
		m := r.(map[string]any)
		ids = append(ids, uint(m["id"].(float64)))
	}
	return ids
}

// smpAssertIndexCanonical asserts the trigger-maintained index matches a
// fresh rebuild and passes the consistency check.
func smpAssertIndexCanonical(t *testing.T, db *gorm.DB) {
	t.Helper()

	cons, err := services.CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.Truef(t, cons.Clean(), "consistency check found divergence: %s", cons.Summary())

	before := smpDumpFTS(t, db)
	require.NoError(t, services.RebuildSearchIndex(db))
	after := smpDumpFTS(t, db)
	assert.Equal(t, before, after, "index state after the mutation path must equal a fresh rebuild")
}

// smpDumpFTS renders the live-row contents of all three FTS tables as a
// deterministic string keyed by table.
func smpDumpFTS(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tbl := range []struct{ fts, base string }{
		{"contacts_fts", "contacts"}, {"notes_fts", "notes"}, {"activities_fts", "activities"},
	} {
		rows, err := db.Raw(fmt.Sprintf(
			`SELECT f.rowid, f.* FROM %s f JOIN %s b ON b.id = f.rowid AND b.deleted_at IS NULL ORDER BY f.rowid`,
			tbl.fts, tbl.base)).Rows()
		require.NoError(t, err)
		cols, err := rows.Columns()
		require.NoError(t, err)
		var s string
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			require.NoError(t, rows.Scan(ptrs...))
			s += fmt.Sprint(vals...) + "\n"
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		out[tbl.fts] = s
	}
	return out
}

// smpCreateContact POSTs /contacts and returns the new contact's id.
func smpCreateContact(t *testing.T, router *gin.Engine, in models.ContactRecordInput) uint {
	t.Helper()
	w := smpDo(t, router, "POST", "/contacts", in)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var body struct {
		Contact models.ContactRecordResponse `json:"contact"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotZero(t, body.Contact.ID)
	return body.Contact.ID
}

func smpContactInput(given, surname, org string) models.ContactRecordInput {
	return models.ContactRecordInput{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
				{Kind: "given", Value: given},
				{Kind: "surname", Value: surname},
			}},
			Organizations: []contactmodel.Organization{{Name: org}},
		},
	}
}

// ---------------------------------------------------------------------------
// REST create / update / delete
// ---------------------------------------------------------------------------

func TestSearchMutationREST_ContactCreateUpdateDelete(t *testing.T) {
	db, router, user := smpRouter(t)

	// create
	id := smpCreateContact(t, router, smpContactInput("Arthur", "Dent", "Sirius Cybernetics"))

	assert.True(t, smpContains(smpSearchIDs(t, router, "Arthur", "contacts"), id), "a REST-created contact is findable")
	assert.True(t, smpContains(smpSearchIDs(t, router, "Sirius", "contacts"), id), "on its organization (a BeforeSave-derived indexed column)")
	smpAssertIndexCanonical(t, db)

	// update a searchable field (organization → org column, derived by BeforeSave)
	w := smpDo(t, router, "PUT", "/contacts/"+strconv.Itoa(int(id)), smpContactInput("Arthur", "Dent", "Megadodo Publications"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, smpContains(smpSearchIDs(t, router, "Megadodo", "contacts"), id), "the updated organization is findable")
	assert.False(t, smpContains(smpSearchIDs(t, router, "Sirius", "contacts"), id), "the replaced organization is gone from the index")
	smpAssertIndexCanonical(t, db)

	// update a non-searchable field (gender): the index must not change
	before := smpDumpFTS(t, db)
	in := smpContactInput("Arthur", "Dent", "Megadodo Publications")
	in.Gender = "male"
	w = smpDo(t, router, "PUT", "/contacts/"+strconv.Itoa(int(id)), in)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, before["contacts_fts"], smpDumpFTS(t, db)["contacts_fts"], "updating a non-indexed field must not change the FTS row")
	smpAssertIndexCanonical(t, db)

	// delete (soft): no longer findable
	w = smpDo(t, router, "DELETE", "/contacts/"+strconv.Itoa(int(id)), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, smpContains(smpSearchIDs(t, router, "Arthur", "contacts"), id), "a deleted contact is not findable")
	var inIndex int64
	require.NoError(t, db.Raw("SELECT count(*) FROM contacts_fts WHERE rowid = ?", id).Scan(&inIndex).Error)
	assert.Equal(t, int64(0), inIndex, "the deleted contact's index row is gone")
	smpAssertIndexCanonical(t, db)

	_ = user
}

// ---------------------------------------------------------------------------
// Archive / unarchive
// ---------------------------------------------------------------------------

func TestSearchMutationREST_ArchiveUnarchive(t *testing.T) {
	db, router, _ := smpRouter(t)

	id := smpCreateContact(t, router, smpContactInput("Marvin", "Android", "Paranoid Robotics"))

	require.Contains(t, smpListSearchIDs(t, router, "Paranoid"), id, "findable before archiving")
	smpAssertIndexCanonical(t, db)

	// archive: still in the index (a live row), but the list-search's archived
	// filter must hide it.
	w := smpDo(t, router, "POST", "/contacts/"+strconv.Itoa(int(id))+"/archive", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stillIndexed int64
	require.NoError(t, db.Raw("SELECT count(*) FROM contacts_fts WHERE rowid = ?", id).Scan(&stillIndexed).Error)
	assert.Equal(t, int64(1), stillIndexed, "an archived (still-live) contact stays in the index by contract")
	assert.NotContains(t, smpListSearchIDs(t, router, "Paranoid"), id, "an archived contact must not be returned by search")
	smpAssertIndexCanonical(t, db)

	// unarchive: returned again.
	w = smpDo(t, router, "POST", "/contacts/"+strconv.Itoa(int(id))+"/unarchive", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, smpListSearchIDs(t, router, "Paranoid"), id, "unarchiving brings the contact back into search")
	smpAssertIndexCanonical(t, db)
}

// ---------------------------------------------------------------------------
// Contact merge
// ---------------------------------------------------------------------------

func TestSearchMutationREST_Merge(t *testing.T) {
	db, router, user := smpRouter(t)

	keeper := models.Contact{UserID: user.ID, Firstname: "Zaphod", Lastname: "Beeblebrox", Organization: "Presidency"}
	loser := models.Contact{
		UserID: user.ID, Firstname: "Zaphod", Lastname: "Beeblebrox", Organization: "Presidency",
		Emails: []models.ContactEmail{{Type: "home", Value: "zaphod@twoheads.example"}},
	}
	require.NoError(t, db.Create(&keeper).Error)
	require.NoError(t, db.Create(&loser).Error)

	// The loser is findable on its unique email token before the merge.
	assert.True(t, smpContains(smpSearchIDs(t, router, "twoheads", "contacts"), loser.ID), "loser findable pre-merge")
	smpAssertIndexCanonical(t, db)

	w := smpDo(t, router, "POST", "/contacts/merge", models.ContactMergeRequest{
		KeepID:  keeper.ID,
		MergeID: loser.ID,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The loser's row is gone from the index (soft-deleted), and nothing in
	// the index points at it.
	var loserRows int64
	require.NoError(t, db.Raw("SELECT count(*) FROM contacts_fts WHERE rowid = ?", loser.ID).Scan(&loserRows).Error)
	assert.Equal(t, int64(0), loserRows, "the merged-away contact leaves no index row")

	// The keeper absorbed the loser's unique email — searching that token now
	// returns the keeper.
	hits := smpSearchIDs(t, router, "twoheads", "contacts")
	assert.True(t, smpContains(hits, keeper.ID), "the keeper absorbed the loser's searchable email")
	assert.False(t, smpContains(hits, loser.ID), "the loser is no longer a hit")

	smpAssertIndexCanonical(t, db)
}

// ---------------------------------------------------------------------------
// Audit Undo
// ---------------------------------------------------------------------------

func TestSearchMutationREST_AuditUndo(t *testing.T) {
	db, router, user := smpRouter(t)
	router.POST("/audit/:id/undo", UndoAuditEvent)
	models.AuditFlush()

	contact := models.Contact{UserID: user.ID, Firstname: "Fenchurch", Organization: "Islington"}
	require.NoError(t, db.Create(&contact).Error)
	models.AuditFlush()

	// A searchable-field edit that produces an update audit event.
	contact.Organization = "Hampstead"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()
	require.True(t, smpContains(smpSearchIDs(t, router, "Hampstead", "contacts"), contact.ID), "the edited org is findable")
	require.False(t, smpContains(smpSearchIDs(t, router, "Islington", "contacts"), contact.ID))

	var event models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpUpdate).Order("id desc").First(&event).Error)

	w := smpDo(t, router, "POST", "/audit/"+strconv.FormatUint(uint64(event.ID), 10)+"/undo", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Undo reverted the row — and the index with it.
	assert.True(t, smpContains(smpSearchIDs(t, router, "Islington", "contacts"), contact.ID), "undo restored the pre-edit org into the index")
	assert.False(t, smpContains(smpSearchIDs(t, router, "Hampstead", "contacts"), contact.ID), "the undone value is gone from the index")
	smpAssertIndexCanonical(t, db)
}

// ---------------------------------------------------------------------------
// REST create / update / delete — notes_fts and activities_fts
// ---------------------------------------------------------------------------

func TestSearchMutationREST_NotesAndActivities(t *testing.T) {
	db, router, user := smpRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Eddie"}
	require.NoError(t, db.Create(&contact).Error)

	// --- note: create / update / delete ---
	w := smpDo(t, router, "POST", "/notes", models.NoteInput{Content: "shipboard computer diagnostics", Date: time.Now()})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var noteBody struct {
		Note models.Note `json:"note"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &noteBody))
	note := noteBody.Note
	require.NotZero(t, note.ID)
	assert.True(t, smpContains(smpSearchIDs(t, router, "diagnostics", "notes"), note.ID), "a REST-created note is findable")
	smpAssertIndexCanonical(t, db)

	w = smpDo(t, router, "PUT", "/notes/"+strconv.FormatUint(uint64(note.ID), 10),
		models.NoteInput{Content: "tea synthesis subroutine failure", Date: time.Now()})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, smpContains(smpSearchIDs(t, router, "synthesis", "notes"), note.ID), "the updated content is findable")
	assert.False(t, smpContains(smpSearchIDs(t, router, "diagnostics", "notes"), note.ID), "the replaced content is gone")
	smpAssertIndexCanonical(t, db)

	w = smpDo(t, router, "DELETE", "/notes/"+strconv.FormatUint(uint64(note.ID), 10), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, smpContains(smpSearchIDs(t, router, "synthesis", "notes"), note.ID), "a deleted note is not findable")
	smpAssertIndexCanonical(t, db)

	// --- activity: create / update / delete ---
	w = smpDo(t, router, "POST", "/activities", models.ActivityInput{
		Title: "Nutrimatic demonstration", Description: "almost but not quite entirely unlike tea",
		Location: "galley", Date: time.Now(),
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var actBody struct {
		Activity models.Activity `json:"activity"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &actBody))
	act := actBody.Activity
	require.NotZero(t, act.ID)
	assert.True(t, smpContains(smpSearchIDs(t, router, "Nutrimatic", "activities"), act.ID), "matched on title")
	assert.True(t, smpContains(smpSearchIDs(t, router, "galley", "activities"), act.ID), "matched on location")
	smpAssertIndexCanonical(t, db)

	w = smpDo(t, router, "PUT", "/activities/"+strconv.FormatUint(uint64(act.ID), 10), models.ActivityInput{
		Title: "Nutrimatic demonstration", Description: "produced a cup of liquid that was almost, but not quite, entirely unlike tea",
		Location: "bridge", Date: time.Now(),
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, smpContains(smpSearchIDs(t, router, "bridge", "activities"), act.ID), "the updated location is findable")
	assert.False(t, smpContains(smpSearchIDs(t, router, "galley", "activities"), act.ID), "the replaced location is gone")
	smpAssertIndexCanonical(t, db)

	w = smpDo(t, router, "DELETE", "/activities/"+strconv.FormatUint(uint64(act.ID), 10), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, smpContains(smpSearchIDs(t, router, "Nutrimatic", "activities"), act.ID), "a deleted activity is not findable")
	smpAssertIndexCanonical(t, db)
}
