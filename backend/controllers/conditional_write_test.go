package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestIfMatchSatisfied is the exhaustive unit table for the pure predicate
// behind checkIfMatch (CON-01, issue #456, ADR 0008): what an If-Match header
// value permits against a row currently at a given revision.
func TestIfMatchSatisfied(t *testing.T) {
	cases := []struct {
		name   string
		header string
		rev    int64
		want   bool
	}{
		{"absent header permits (opt-in)", "", 4, true},
		{"whitespace-only header permits", "   ", 4, true},
		{"star matches any existing row", "*", 4, true},
		{"quoted star matches", `"*"`, 4, true},
		{"exact bare revision matches", "4", 4, true},
		{"exact quoted revision matches", `"4"`, 4, true},
		{"weak-prefixed quoted revision matches", `W/"4"`, 4, true},
		{"surrounding whitespace tolerated", `  "4" `, 4, true},
		{"list with a matching member matches", `"1", "4", "9"`, 4, true},
		{"stale revision does not match", `"3"`, 4, false},
		{"future revision does not match", `"5"`, 4, false},
		{"non-numeric garbage does not match", `"nonsense"`, 4, false},
		{"empty quotes do not match a real revision", `""`, 4, false},
		{"list with no matching member does not match", `"1", "2"`, 4, false},
		{"revision 0 matches its own token", "0", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ifMatchSatisfied(tc.header, tc.rev))
		})
	}
}

// cwEnv is the shared real-migrated-schema fixture for the end-to-end
// conditional-write tests. Every handler runs against a database.InitDB schema
// (via dbtest), never AutoMigrate — CLAUDE.md backend trap #1, and the ticket's
// explicit "verify against a real migrated schema" requirement.
type cwEnv struct {
	db    *gorm.DB
	user  models.User
	alice models.Contact
	do    func(method, path, ifMatch string, body any) *httptest.ResponseRecorder
}

func newCWEnv(t *testing.T) *cwEnv {
	t.Helper()
	db := dbtest.New(t)

	user := models.User{Username: "cwtester", Password: "password123!A", Email: "cw@example.com"}
	require.NoError(t, db.Create(&user).Error)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
	router.DELETE("/contacts/:id", DeleteContact)
	router.PUT("/notes/:id", withValidated(func() any { return &models.NoteInput{} }), UpdateNote)
	router.DELETE("/notes/:id", DeleteNote)
	router.PUT("/activities/:id", withValidated(func() any { return &models.ActivityInput{} }), UpdateActivity)
	router.DELETE("/activities/:id", DeleteActivity)
	router.PUT("/life-events/:id", withValidated(func() any { return &models.LifeEventInput{} }), UpdateLifeEvent)
	router.DELETE("/life-events/:id", DeleteLifeEvent)
	router.PUT("/reminders/:id", withValidated(func() any { return &models.Reminder{} }), UpdateReminder)
	router.DELETE("/reminders/:id", DeleteReminder)

	do := func(method, path, ifMatch string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	return &cwEnv{db: db, user: user, alice: alice, do: do}
}

// assert412 checks the rejection is a clean 412 with the documented error code.
func assert412(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusPreconditionFailed, w.Code, w.Body.String())
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "PRECONDITION_FAILED", env.Error.Code)
}

func TestConditionalWrite_Contact(t *testing.T) {
	env := newCWEnv(t)
	id := strconv.Itoa(int(env.alice.ID))
	body := func(given string) models.ContactRecordInput {
		return models.ContactRecordInput{Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: given}}},
		}}
	}
	givenOf := func(resp models.ContactRecordResponse) string {
		if resp.Card.Name == nil {
			return ""
		}
		for _, comp := range resp.Card.Name.Components {
			if comp.Kind == "given" {
				return comp.Value
			}
		}
		return ""
	}

	// Contact starts at revision 1 (migration 000044 default + AfterCreate). A
	// conditional PUT carrying the current revision succeeds and bumps it.
	w := env.do("PUT", "/contacts/"+id, `"1"`, body("Alice-A"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.ContactRecordResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 2, resp.Revision)

	// Replaying the now-stale revision — in the same wall-clock second as the
	// write above, the exact case the old UpdatedAt.Unix() etag could not
	// catch (issue #591) — is rejected and does NOT touch the row.
	w = env.do("PUT", "/contacts/"+id, `"1"`, body("Alice-STALE"))
	assert412(t, w)

	// The row is exactly as the accepted write left it: revision 2, "Alice-A".
	w = env.do("PUT", "/contacts/"+id, `"2"`, body("Alice-B"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 3, resp.Revision)
	require.Equal(t, "Alice-B", givenOf(resp), "the STALE write between revisions 2 and 3 must have been discarded")

	// No If-Match header: the write is unconditional (opt-in enforcement).
	w = env.do("PUT", "/contacts/"+id, "", body("Alice-C"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// If-Match: * succeeds against an existing row.
	w = env.do("PUT", "/contacts/"+id, "*", body("Alice-D"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// A garbage token is a failed precondition.
	w = env.do("PUT", "/contacts/"+id, `"not-a-number"`, body("Alice-E"))
	assert412(t, w)

	// Conditional DELETE: a stale token is rejected and the contact survives.
	w = env.do("DELETE", "/contacts/"+id, `"1"`, nil)
	assert412(t, w)
	var stillThere models.Contact
	require.NoError(t, env.db.First(&stillThere, env.alice.ID).Error)

	// The current token deletes it.
	w = env.do("DELETE", "/contacts/"+id, "*", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.ErrorIs(t, env.db.First(&models.Contact{}, env.alice.ID).Error, gorm.ErrRecordNotFound)
}

func TestConditionalWrite_Note(t *testing.T) {
	env := newCWEnv(t)
	note := models.Note{UserID: env.user.ID, Content: "original", Date: time.Now(), ContactID: &env.alice.ID}
	require.NoError(t, env.db.Create(&note).Error)
	require.EqualValues(t, 1, note.Revision)
	id := strconv.Itoa(int(note.ID))

	upd := models.NoteInput{Content: "edited-A", Date: time.Now(), ContactID: &env.alice.ID}
	w := env.do("PUT", "/notes/"+id, `"1"`, upd)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var respBody struct {
		Note models.Note `json:"note"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &respBody))
	require.EqualValues(t, 2, respBody.Note.Revision)

	// Stale write rejected, row untouched (byte-identical to the accepted write).
	upd.Content = "edited-STALE"
	w = env.do("PUT", "/notes/"+id, `"1"`, upd)
	assert412(t, w)
	var after models.Note
	require.NoError(t, env.db.First(&after, note.ID).Error)
	assert.Equal(t, "edited-A", after.Content)
	assert.EqualValues(t, 2, after.Revision)

	// Unconditional write still works.
	upd.Content = "edited-C"
	w = env.do("PUT", "/notes/"+id, "", upd)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Stale conditional DELETE rejected; current one deletes.
	w = env.do("DELETE", "/notes/"+id, `"1"`, nil)
	assert412(t, w)
	w = env.do("DELETE", "/notes/"+id, "*", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestConditionalWrite_Activity(t *testing.T) {
	env := newCWEnv(t)
	act := models.Activity{UserID: env.user.ID, Title: "original", Date: time.Now()}
	require.NoError(t, env.db.Create(&act).Error)
	require.EqualValues(t, 1, act.Revision)
	id := strconv.Itoa(int(act.ID))

	upd := models.ActivityInput{Title: "edited-A", Date: time.Now()}
	w := env.do("PUT", "/activities/"+id, `"1"`, upd)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.Activity
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.EqualValues(t, 2, got.Revision)

	upd.Title = "edited-STALE"
	w = env.do("PUT", "/activities/"+id, `"1"`, upd)
	assert412(t, w)
	var after models.Activity
	require.NoError(t, env.db.First(&after, act.ID).Error)
	assert.Equal(t, "edited-A", after.Title)
	assert.EqualValues(t, 2, after.Revision)

	w = env.do("DELETE", "/activities/"+id, `"99"`, nil)
	assert412(t, w)
	w = env.do("DELETE", "/activities/"+id, `"2"`, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestConditionalWrite_LifeEvent(t *testing.T) {
	env := newCWEnv(t)
	year := 2024
	le := models.LifeEvent{
		UserID:   env.user.ID,
		EntityID: env.alice.VCardUID,
		Type:     models.LifeEventTypeGraduated,
		Date:     &contactmodel.PartialDate{Year: &year},
		Source:   models.LifeEventSourceUser,
	}
	require.NoError(t, env.db.Create(&le).Error)
	require.EqualValues(t, 1, le.Revision)

	upd := models.LifeEventInput{
		EntityID:    env.alice.VCardUID,
		Type:        models.LifeEventTypeGraduated,
		Date:        &contactmodel.PartialDate{Year: &year},
		Source:      models.LifeEventSourceUser,
		Description: "edited-A",
	}
	w := env.do("PUT", "/life-events/"+le.ID, `"1"`, upd)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.LifeEvent
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.EqualValues(t, 2, got.Revision)

	upd.Description = "edited-STALE"
	w = env.do("PUT", "/life-events/"+le.ID, `"1"`, upd)
	assert412(t, w)
	var after models.LifeEvent
	require.NoError(t, env.db.Where("id = ?", le.ID).First(&after).Error)
	assert.Equal(t, "edited-A", after.Description)
	assert.EqualValues(t, 2, after.Revision)

	w = env.do("DELETE", "/life-events/"+le.ID, `"1"`, nil)
	assert412(t, w)
	w = env.do("DELETE", "/life-events/"+le.ID, "*", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestConditionalWrite_Reminder(t *testing.T) {
	env := newCWEnv(t)
	byMail := false
	reoccur := true
	rem := models.Reminder{
		UserID:                env.user.ID,
		Message:               "original",
		ByMail:                &byMail,
		RemindAt:              time.Now().Add(48 * time.Hour),
		Recurrence:            "once",
		ReoccurFromCompletion: &reoccur,
		ContactID:             &env.alice.ID,
	}
	require.NoError(t, env.db.Create(&rem).Error)
	require.EqualValues(t, 1, rem.Revision)
	id := strconv.Itoa(int(rem.ID))

	upd := rem
	upd.Message = "edited-A"
	w := env.do("PUT", "/reminders/"+id, `"1"`, upd)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var respBody struct {
		Reminder models.Reminder `json:"reminder"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &respBody))
	require.EqualValues(t, 2, respBody.Reminder.Revision)

	upd.Message = "edited-STALE"
	w = env.do("PUT", "/reminders/"+id, `"1"`, upd)
	assert412(t, w)
	var after models.Reminder
	require.NoError(t, env.db.First(&after, rem.ID).Error)
	assert.Equal(t, "edited-A", after.Message)
	assert.EqualValues(t, 2, after.Revision)

	w = env.do("DELETE", "/reminders/"+id, `"1"`, nil)
	assert412(t, w)
	w = env.do("DELETE", "/reminders/"+id, `"2"`, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
