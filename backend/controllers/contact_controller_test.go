package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

func TestGetContacts(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	// Create some contacts, then force EVERY row to share the same
	// updated_at timestamp — the exact condition where a bare `updated_at`
	// cursor would drop or duplicate rows at page boundaries. Only the id
	// tiebreak keeps the (updated_at, id) order total.
	contacts := []models.Contact{
		{UserID: user.ID, Firstname: "Alice", Lastname: "Johnson"},
		{UserID: user.ID, Firstname: "Bob", Lastname: "Smith"},
		{UserID: user.ID, Firstname: "Carol", Lastname: "Williams"},
		{UserID: user.ID, Firstname: "David", Lastname: "Brown"},
		{UserID: user.ID, Firstname: "Eve", Lastname: "Davis"},
	}
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	for i, contact := range contacts {
		require.NoError(t, db.Create(&contact).Error)
		require.NoError(t, db.Model(&contact).UpdateColumn("updated_at", fixed).Error)
		contacts[i] = contact
	}

	// T17: cursor pagination. Page through with limit=2 following next_cursor
	// until it runs out, and assert every one of the 5 contacts is returned
	// exactly once — no drops, no duplicates — despite every row sharing the
	// same updated_at (the id tiebreak is what makes the order total).
	fetch := func(query string) ([]any, string) {
		req, _ := http.NewRequest("GET", "/contacts?limit=2&"+query, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var responseBody map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &responseBody))

		// New envelope: no total/page; next_cursor + limit + sync.
		assert.NotContains(t, responseBody, "total")
		assert.NotContains(t, responseBody, "page")
		assert.Equal(t, float64(2), responseBody["limit"])
		sync := responseBody["sync"].(map[string]any)
		assert.Equal(t, "incremental", sync["mode"])

		items := responseBody["contacts"].([]any)
		next, _ := responseBody["next_cursor"].(string)
		return items, next
	}

	var seen []float64
	next := ""
	page := 0
	for {
		items, n := fetch("cursor=" + next)
		for _, raw := range items {
			seen = append(seen, raw.(map[string]any)["id"].(float64))
		}
		if n == "" {
			break
		}
		next = n
		page++
		if page > 10 {
			t.Fatal("cursor pagination did not terminate")
		}
	}

	assert.Len(t, seen, 5, "all contacts should be returned across pages")
	assert.ElementsMatch(t, []float64{1, 2, 3, 4, 5}, seen, "no dropped or duplicated contacts")
}

// TestGetContacts_SummaryHasNicknameNoCircles is T108's regression test
// (docs/fork-plan/tickets/152-T108-contact-summary-missing-columns.md):
// contactSummaryColumns (the fixed Select() list GetContacts actually runs)
// never included "nickname", so every ContactSummary shipped an empty
// nickname despite the DTO and NewContactSummary both carrying it correctly
// -- a bug no test decoding into models.ContactSummary could have caught,
// since NewContactSummary's own unit test builds a Contact by hand and never
// goes through the controller's query at all (see
// contact_summary_test.go's TestNewContactSummary_IncludesNickname). This
// asserts against the raw JSON map instead of a decoded struct for the same
// reason /CLAUDE.md frontend trap #8 requires it for a Go response DTO: a
// decoded struct can't distinguish "key present with the zero value" from
// "key absent" -- both matter here, nickname for the former, circles (
// removed from the DTO entirely, not populated) for the latter.
func TestGetContacts_SummaryHasNicknameNoCircles(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	contact := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace", Nickname: "Countess"}
	require.NoError(t, db.Create(&contact).Error)

	req, _ := http.NewRequest("GET", "/contacts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var responseBody map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &responseBody))
	items := responseBody["contacts"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)

	assert.Equal(t, "Countess", item["nickname"], "the list query must actually select the nickname column")
	assert.NotContains(t, item, "circles", "circles must be gone from the DTO entirely, not present-and-empty")
}

// TestGetContacts_FiltersByVCardUID pins down §3d WP0
// (docs/fork-plan/95-backlog-and-priorities.md): the RelationshipEdge
// frontend needs to resolve a batch of Contact.VCardUID values (edge
// SourceID/TargetID, which carry no nested contact data) back into
// displayable Contacts. Proves the ?vcard_uid= filter matches multiple
// requested UIDs, ignores an unrequested one, and stays scoped to the
// requesting user.
func TestGetContacts_FiltersByVCardUID(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts", GetContacts)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "vuid-other", Password: "x", Email: "vuid-other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)

	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	carol := models.Contact{UserID: user.ID, Firstname: "Carol"}
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)
	require.NoError(t, db.Create(&carol).Error)
	require.NoError(t, db.Create(&othersContact).Error)

	req, _ := http.NewRequest("GET", "/contacts?vcard_uid="+alice.VCardUID+"&vcard_uid="+bob.VCardUID+"&vcard_uid="+othersContact.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Contacts []models.ContactSummary `json:"contacts"`
		Total    int64                   `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Carol (not requested) and othersContact (requested but not this
	// user's) must both be absent; only Alice and Bob match.
	require.Len(t, resp.Contacts, 2)
	assert.EqualValues(t, 2, resp.Total)
	gotUIDs := map[string]bool{}
	for _, c := range resp.Contacts {
		gotUIDs[c.UID] = true
	}
	assert.True(t, gotUIDs[alice.VCardUID])
	assert.True(t, gotUIDs[bob.VCardUID])
}

// TestGetContacts_VCardUIDFilter_RealMigratedSchema is the real-DB check for
// §3d WP0: unlike the AutoMigrate-backed test above, this runs against a
// database.InitDB-migrated file DB, confirming the `vcard_uid`/`archived`
// columns the new filter queries actually exist with those exact names in
// the real migration SQL (this fork's own recurring bug class).
func TestGetContacts_VCardUIDFilter_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "vcard-uid-filter-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "realdb-vuid", Password: "password123!A", Email: "realdb-vuid@example.com"}
	require.NoError(t, db.Create(&user).Error)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.GET("/contacts", GetContacts)

	req, _ := http.NewRequest("GET", "/contacts?vcard_uid="+alice.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Contacts []models.ContactSummary `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Contacts, 1)
	assert.Equal(t, alice.VCardUID, resp.Contacts[0].UID)
}

func TestGetContacts_VCardUIDFilterExcludesArchivedByDefault(t *testing.T) {
	db, router := setupRouter()
	router.GET("/contacts", GetContacts)

	var user models.User
	db.First(&user)
	archived := models.Contact{UserID: user.ID, Firstname: "Gone", Archived: true}
	require.NoError(t, db.Create(&archived).Error)

	req, _ := http.NewRequest("GET", "/contacts?vcard_uid="+archived.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Contacts []models.ContactSummary `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Contacts)

	// include_archived=true must surface it -- an edge can point at an
	// archived contact and the caller may still want to resolve the name.
	req2, _ := http.NewRequest("GET", "/contacts?vcard_uid="+archived.VCardUID+"&include_archived=true", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
	var resp2 struct {
		Contacts []models.ContactSummary `json:"contacts"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.Len(t, resp2.Contacts, 1)
	assert.Equal(t, archived.VCardUID, resp2.Contacts[0].UID)
}

// TestGetContactsSearchMultiValue verifies search matches values in the emails/phones
// JSON arrays (including secondary entries), not just the denormalized primary scalar.
func TestGetContactsSearchMultiValue(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	c := models.Contact{
		UserID:    user.ID,
		Firstname: "Grace",
		Lastname:  "Hopper",
		Emails: []models.ContactEmail{
			{Type: "home", Value: "grace@home.example"},
			{Type: "work", Value: "grace@navy.example"},
		},
		Phones: []models.ContactPhone{{Type: "cell", Value: "+15551112222"}},
	}
	db.Create(&c)
	// A second contact that must NOT match the searches below.
	db.Create(&models.Contact{UserID: user.ID, Firstname: "Alan", Lastname: "Turing"})

	search := func(term string) int {
		req, _ := http.NewRequest("GET", "/contacts?search="+term, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		return len(body["contacts"].([]any))
	}

	assert.Equal(t, 1, search("navy"), "secondary work email should be searchable")
	assert.Equal(t, 1, search("grace@home"), "primary email should be searchable")
	assert.Equal(t, 1, search("5551112"), "phone array value should be searchable")
	assert.Equal(t, 1, search("Hopper"), "name should still be searchable")
	assert.Equal(t, 0, search("nomatch"), "unrelated term should match nothing")
}

// TestGetContactsSearchMatchesAddresses pins T38's other half: the legacy
// LIKE-based contacts search (applyContactSearch) must match address text the
// same way it matches names/emails/phones, via the denormalized
// addresses_flat column. The tokens are absent from every other field so the
// match can only come from the address.
func TestGetContactsSearchMatchesAddresses(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Grace", Lastname: "Hopper",
		Addresses: []models.ContactAddress{
			{Type: "home", Street: "Basil Grove", City: "Austin", Region: "TX"},
			{Type: "work", Street: "12 Sorrel Court"},
		},
	}).Error)
	// A second contact that must NOT match the searches below.
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Alan", Lastname: "Turing"}).Error)

	search := func(term string) int {
		req, _ := http.NewRequest("GET", "/contacts?search="+term, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		return len(body["contacts"].([]any))
	}

	assert.Equal(t, 1, search("Basil"), "street should be searchable")
	assert.Equal(t, 1, search("Austin"), "city should be searchable")
	assert.Equal(t, 1, search("Sorrel"), "a second address's street should be searchable")
	assert.Equal(t, 0, search("nomatch"), "unrelated term should match nothing")
}

// TestGetContactsSearchDoesNotLeakAcrossUsers pins the ownership-scoping rule
// for the legacy contacts-list search (the same highest-risk rule the FTS path
// pins with TestSearch_CrossUserReturnsNothing): applyContactSearch's OR-clause
// must compose with the user_id scope under AND, never OR, so a search term
// can never surface a contact owned by another user. (GORM parenthesizes the
// Where clause, but this test guards against any regression that would let the
// unparenthesized OR escape the user scope.)
func TestGetContactsSearchDoesNotLeakAcrossUsers(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	// A second user owns a contact with a distinctive lastname.
	other := models.User{Username: "search-other", Password: "password123", Email: "search-other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Contact{
		UserID: other.ID, Firstname: "Sneaky", Lastname: "ZzyzxCrossUser",
	}).Error)

	// The first user searches for that distinctive lastname.
	req, _ := http.NewRequest("GET", "/contacts?search=ZzyzxCrossUser", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	contacts := body["contacts"].([]any)
	assert.Empty(t, contacts, "a contact owned by another user must never be returned via the contacts search")
}

// TestGetContactsSearchMatchesPhonesNormalized pins T69's other half: the
// legacy LIKE-based contacts search (applyContactSearch) must match a phone
// regardless of punctuation/grouping/country-code differences between the
// query and the stored value, via the denormalized phones_normalized column.
// The phone tokens are absent from every name/email/address field so the
// match can only come from the phone.
func TestGetContactsSearchMatchesPhonesNormalized(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Grace", Lastname: "Hopper",
		Phones: []models.ContactPhone{{Type: "cell", Value: "(800) 555-1234"}},
	}).Error)
	// A second contact with an 11-digit (country-code) number and a distinct
	// non-primary number.
	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Alan", Lastname: "Turing",
		Phones: []models.ContactPhone{
			{Type: "cell", Value: "+18005551234"},
			{Type: "work", Value: "800-555-2222"},
		},
	}).Error)
	// A third contact that must NOT match any of the searches below.
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Zoe", Lastname: "None"}).Error)

	search := func(term string) int {
		req, _ := http.NewRequest("GET", "/contacts?search="+term, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		return len(body["contacts"].([]any))
	}

	// Unpunctuated 10-digit query finds both stored forms.
	assert.Equal(t, 2, search("8005551234"), "the 10-digit query must find the punctuated and country-code stored numbers")
	// Punctuated query finds both too.
	assert.Equal(t, 2, search("(800) 555-1234"), "the punctuated query must find both stored forms")
	// The full international form matches the same number in either format.
	assert.Equal(t, 2, search("18005551234"), "the full 11-digit query must match both stored forms")
	// A non-primary number is findable.
	assert.Equal(t, 1, search("8005552222"), "a non-primary phone must be findable through the contacts search")
	// A different number matches nothing.
	assert.Equal(t, 0, search("8005559999"), "unrelated number should match nothing")
}

func TestGetContact(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts/:id", GetContact)

	// Create a contact
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Doe",
	}
	db.Create(&contact)

	// Make the request to get the contact by ID
	req, _ := http.NewRequest("GET", "/contacts/"+strconv.Itoa(int(contact.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// GET /contacts/:id now returns the full neutral ContactRecordResponse
	// (Card/CRM/Passthrough nested), not a flat models.Contact (Gap 3/item 2).
	var responseBody models.ContactRecordResponse
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, contact.ID, responseBody.ID)
	if assert.NotNil(t, responseBody.Card.Name) {
		var gotFirst string
		for _, comp := range responseBody.Card.Name.Components {
			if comp.Kind == "given" {
				gotFirst = comp.Value
			}
		}
		assert.Equal(t, contact.Firstname, gotFirst)
	}
}

// TestGetContactsFieldsParamIgnored asserts fields= is gone (Gap 3): passing
// it no longer restricts or alters the response shape, which is always the
// fixed ContactSummary regardless of what (if anything) fields= requests.
func TestGetContactsFieldsParamIgnored(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	contact := models.Contact{
		UserID:          user.ID,
		Firstname:       "Jane",
		Lastname:        "Doe",
		Email:           "jane@example.com",
		Phone:           "1234567890",
		Address:         "123 Main St",
		WorkInformation: "Software Engineer at TechCorp",
	}
	db.Create(&contact)

	// fields=firstname,email would once have restricted the response to just
	// those columns; it must now have no effect at all.
	req, _ := http.NewRequest("GET", "/contacts?fields=firstname,email", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)

	contacts := responseBody["contacts"].([]any)
	assert.Len(t, contacts, 1)

	contactData := contacts[0].(map[string]any)
	assert.Equal(t, "Jane", contactData["firstname"])
	assert.Equal(t, "jane@example.com", contactData["primary_email"])
	// lastname was NOT in fields=, but the slim ContactSummary shape is
	// returned unconditionally now, so it's still present.
	assert.Equal(t, "Doe", contactData["lastname"])
}

func TestGetContactWithRelationships(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	// Create a contact
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Doe",
	}
	db.Create(&contact)

	// Create a note for the contact
	note := models.Note{
		UserID:    user.ID,
		ContactID: &contact.ID,
		Content:   "Test note content",
	}
	db.Create(&note)

	// Create a reminder for the contact
	byMail := false
	reminder := models.Reminder{
		UserID:     user.ID,
		ContactID:  &contact.ID,
		Message:    "Follow up with Jane",
		RemindAt:   time.Date(2025, 12, 31, 10, 0, 0, 0, time.UTC),
		Recurrence: "once",
		ByMail:     &byMail,
	}
	db.Create(&reminder)

	// Request contacts with notes included
	req, _ := http.NewRequest("GET", "/contacts?includes=notes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)

	contacts := responseBody["contacts"].([]any)
	assert.Len(t, contacts, 1)

	contactData := contacts[0].(map[string]any)
	notes := contactData["notes"].([]any)
	assert.Len(t, notes, 1)
	assert.Equal(t, "Test note content", notes[0].(map[string]any)["content"])

	// Request contacts with reminders included
	req, _ = http.NewRequest("GET", "/contacts?includes=reminders", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	json.Unmarshal(w.Body.Bytes(), &responseBody)

	contacts = responseBody["contacts"].([]any)
	contactData = contacts[0].(map[string]any)
	reminders := contactData["reminders"].([]any)
	assert.Len(t, reminders, 1)
	assert.Equal(t, "Follow up with Jane", reminders[0].(map[string]any)["message"])

	// Request contacts with multiple relationships included
	req, _ = http.NewRequest("GET", "/contacts?includes=notes,reminders", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	json.Unmarshal(w.Body.Bytes(), &responseBody)

	contacts = responseBody["contacts"].([]any)
	contactData = contacts[0].(map[string]any)
	notes = contactData["notes"].([]any)
	reminders = contactData["reminders"].([]any)
	assert.Len(t, notes, 1)
	assert.Len(t, reminders, 1)
}

// TestGetContactsArchiveAndCircleFiltering asserts Gap 2's binding
// preservation of the archive-filtering and circle-filtering mechanics
// against the new ContactSummary item shape.
func TestGetContactsArchiveAndCircleFiltering(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	active := models.Contact{UserID: user.ID, Firstname: "Active", Lastname: "One"}
	archived := models.Contact{UserID: user.ID, Firstname: "Archived", Lastname: "One", Archived: true}
	db.Create(&active)
	db.Create(&archived)

	// Create a Circle + membership so the circle filter works against
	// the new circle_members join (T3).
	friendsCircle := models.Circle{UserID: user.ID, Name: "friends"}
	db.Create(&friendsCircle)
	db.Create(&models.CircleMember{
		CircleID:       friendsCircle.ID,
		UserID:         user.ID,
		MemberVCardUID: active.VCardUID,
	})

	// Default: archived excluded.
	req, _ := http.NewRequest("GET", "/contacts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Len(t, body["contacts"].([]any), 1)

	// include_archived=true: both returned.
	req, _ = http.NewRequest("GET", "/contacts?include_archived=true", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Len(t, body["contacts"].([]any), 2)

	// archived=true (without include_archived): only the archived one.
	req, _ = http.NewRequest("GET", "/contacts?archived=true", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	json.Unmarshal(w.Body.Bytes(), &body)
	contacts := body["contacts"].([]any)
	assert.Len(t, contacts, 1)
	assert.Equal(t, "Archived", contacts[0].(map[string]any)["firstname"])

	// circle= filters by circle membership.
	req, _ = http.NewRequest("GET", "/contacts?circle=friends", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	json.Unmarshal(w.Body.Bytes(), &body)
	contacts = body["contacts"].([]any)
	assert.Len(t, contacts, 1)
	assert.Equal(t, "Active", contacts[0].(map[string]any)["firstname"])
}

func TestGetContactsWithSearchCriteria(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts", GetContacts)

	// Create multiple contacts
	contacts := []models.Contact{
		{UserID: user.ID, Firstname: "Alice", Lastname: "Johnson", Nickname: "Ali"},
		{UserID: user.ID, Firstname: "Bob", Lastname: "Smith", Nickname: "Bobby"},
		{UserID: user.ID, Firstname: "Carol", Lastname: "Williams", Nickname: ""},
	}

	for _, c := range contacts {
		db.Create(&c)
	}

	// Search by firstname
	req, _ := http.NewRequest("GET", "/contacts?search=Alice", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)

	returnedContacts := responseBody["contacts"].([]any)
	assert.Len(t, returnedContacts, 1)
	assert.Equal(t, "Alice", returnedContacts[0].(map[string]any)["firstname"])

	// Search by lastname
	req, _ = http.NewRequest("GET", "/contacts?search=Smith", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	json.Unmarshal(w.Body.Bytes(), &responseBody)

	returnedContacts = responseBody["contacts"].([]any)
	assert.Len(t, returnedContacts, 1)
	assert.Equal(t, "Bob", returnedContacts[0].(map[string]any)["firstname"])

	// Search by nickname
	req, _ = http.NewRequest("GET", "/contacts?search=Bobby", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	json.Unmarshal(w.Body.Bytes(), &responseBody)

	returnedContacts = responseBody["contacts"].([]any)
	assert.Len(t, returnedContacts, 1)
	assert.Equal(t, "Bob", returnedContacts[0].(map[string]any)["firstname"])

	// Search with no results
	req, _ = http.NewRequest("GET", "/contacts?search=NonExistent", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	json.Unmarshal(w.Body.Bytes(), &responseBody)

	returnedContacts = responseBody["contacts"].([]any)
	assert.Len(t, returnedContacts, 0)
}

func TestCreateContact(t *testing.T) {
	_, router := setupRouter()

	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	// Create a contact with basic fields, using the new nested Card shape.
	newContact := models.ContactRecordInput{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
				{Kind: "given", Value: "Alice"},
				{Kind: "surname", Value: "Johnson"},
			}},
			Emails: []contactmodel.Email{{Address: "alice@example.com"}},
			Phones: []contactmodel.Phone{{Number: "1234567890"}},
		},
	}

	jsonValue, _ := json.Marshal(newContact)

	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "Contact created successfully", responseBody["message"])
}

func TestCreateContactWithAllFields(t *testing.T) {
	_, router := setupRouter()

	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	// Create a contact with all fields filled out, via the nested Card/CRM
	// shape (Gender rides alongside, per ContactRecordInput's doc comment).
	fullContact := models.ContactRecordInput{
		Gender: "male",
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
				{Kind: "given", Value: "Robert"},
				{Kind: "surname", Value: "Anderson"},
			}},
			Nicknames: []contactmodel.Nickname{{Name: "Bob"}},
			Emails:    []contactmodel.Email{{Address: "robert.anderson@example.com"}},
			Phones:    []contactmodel.Phone{{Number: "+1-555-123-4567"}},
			Anniversaries: []contactmodel.Anniversary{
				{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: &contactmodel.PartialDate{
					Year: intPtr(1985), Month: intPtr(3), Day: intPtr(15),
				}}},
			},
			Addresses: []contactmodel.Address{{Full: "456 Oak Avenue, Springfield, IL 62701"}},
		},
		CRM: contactmodel.CRMEnvelope{
			HowWeMet:           "Met at a tech conference in 2020",
			WorkInformation:    "Senior Software Engineer at TechCorp Inc.",
			ContactInformation: "Prefers email, available weekdays 9-5",
			Circles:            []string{"Friends", "Work", "Tech Community"},
		},
	}

	jsonValue, _ := json.Marshal(fullContact)

	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "Contact created successfully", responseBody["message"])

	// Verify all fields are returned correctly in the nested
	// ContactRecordResponse shape.
	contact := responseBody["contact"].(map[string]any)
	assert.Equal(t, "male", contact["gender"])

	crm := contact["crm"].(map[string]any)
	assert.Equal(t, "Met at a tech conference in 2020", crm["how_we_met"])
	assert.Equal(t, "Senior Software Engineer at TechCorp Inc.", crm["work_information"])
	assert.Equal(t, "Prefers email, available weekdays 9-5", crm["contact_information"])
	circles := crm["circles"].([]any)
	assert.Len(t, circles, 3)

	card := contact["card"].(map[string]any)
	nicknames := card["nicknames"].([]any)
	assert.Equal(t, "Bob", nicknames[0].(map[string]any)["name"])
	emails := card["emails"].([]any)
	assert.Equal(t, "robert.anderson@example.com", emails[0].(map[string]any)["address"])
}

// TestCreateContactBirthdayPartialDates replaces the old flat-scalar
// birthday-format test: the new nested input carries a birthday as a
// structured Card.Anniversaries[kind=birth].Date.Partial (year/month/day),
// not a raw formatted string, so this exercises the equivalent
// full-date/year-less/absent variations against that shape instead.
func TestCreateContactBirthdayPartialDates(t *testing.T) {
	_, router := setupRouter()

	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	tests := []struct {
		name  string
		date  *contactmodel.PartialDate
		count int // expected len(card.anniversaries)
	}{
		{"Fully known birthday", &contactmodel.PartialDate{Year: intPtr(1990), Month: intPtr(12), Day: intPtr(25)}, 1},
		{"Birthday without year", &contactmodel.PartialDate{Month: intPtr(12), Day: intPtr(25)}, 1},
		{"Unknown birthday", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := models.ContactRecordInput{
				Card: contactmodel.Card{
					Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Test"}}},
				},
			}
			if tt.date != nil {
				input.Card.Anniversaries = []contactmodel.Anniversary{
					{Kind: "birth", Date: contactmodel.AnniversaryDate{Partial: tt.date}},
				}
			}

			jsonValue, _ := json.Marshal(input)

			req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusCreated, w.Code)

			var responseBody map[string]any
			json.Unmarshal(w.Body.Bytes(), &responseBody)
			assert.Equal(t, "Contact created successfully", responseBody["message"])

			contactResp := responseBody["contact"].(map[string]any)
			card := contactResp["card"].(map[string]any)
			anniversaries, _ := card["anniversaries"].([]any)
			assert.Len(t, anniversaries, tt.count)
		})
	}
}

func TestUpdateContact(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)

	// Create a contact
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Alice",
		Lastname:  "Johnson",
	}
	db.Create(&contact)

	// Update the contact via the new nested shape
	updatedContact := models.ContactRecordInput{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
				{Kind: "given", Value: "Alice Updated"},
				{Kind: "surname", Value: "Johnson Updated"},
			}},
		},
	}
	jsonValue, _ := json.Marshal(updatedContact)

	req, _ := http.NewRequest("PUT", "/contacts/"+strconv.Itoa(int(contact.ID)), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody models.ContactRecordResponse
	json.Unmarshal(w.Body.Bytes(), &responseBody)

	var gotFirst string
	if responseBody.Card.Name != nil {
		for _, comp := range responseBody.Card.Name.Components {
			if comp.Kind == "given" {
				gotFirst = comp.Value
			}
		}
	}
	assert.Equal(t, "Alice Updated", gotFirst)
}

func TestDeleteContact(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.DELETE("/contacts/:id", DeleteContact)

	// Create a contact
	contact := models.Contact{
		UserID:    user.ID,
		Firstname: "Alice",
		Lastname:  "Johnson",
	}
	db.Create(&contact)

	// Make the request to delete the contact
	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.Itoa(int(contact.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, "Contact deleted", responseBody["message"])
}

// TestDeleteContact_CleansUpReferencingRows is the regression test for Tier
// 3c item 1 (docs/fork-plan/95-backlog-and-priorities.md): deleting a contact
// must remove every row that references it via Contact.VCardUID (or, for
// ContactSyncLink, Contact.ID), but must NOT delete the shared
// Household/Circle/Tag/FieldDefinition containers other contacts may still
// belong to.
func TestDeleteContact_CleansUpReferencingRows(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	contact := models.Contact{UserID: user.ID, Firstname: "Orphan", Lastname: "Check"}
	require.NoError(t, db.Create(&contact).Error)

	require.NoError(t, db.Create(&models.RelationshipEdge{UserID: user.ID, SourceID: contact.VCardUID, TargetID: contact.VCardUID, Type: "related_to"}).Error)
	require.NoError(t, db.Create(&models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "custom"}).Error)

	household := models.Household{UserID: user.ID, Name: "h", Type: "other"}
	require.NoError(t, db.Create(&household).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: contact.VCardUID, Role: "adult"}).Error)

	circle := models.Circle{UserID: user.ID, Name: "c"}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: contact.VCardUID}).Error)

	tag := models.Tag{UserID: user.ID, Name: "t"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: tag.ID, UserID: user.ID, ContactVCardUID: contact.VCardUID}).Error)

	fieldDef := models.FieldDefinition{UserID: user.ID, Label: "f", Key: "f", Target: "contact", Type: "text"}
	require.NoError(t, db.Create(&fieldDef).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: fieldDef.ID, UserID: user.ID, EntityID: contact.VCardUID, Value: json.RawMessage(`"v"`)}).Error)

	sub := models.ContactSubscription{UserID: user.ID, Name: "sub", URL: "https://example.com/dav"}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&models.ContactSyncLink{SubscriptionID: sub.ID, UserID: user.ID, ContactID: contact.ID, Href: "/dav/1.vcf"}).Error)

	require.NoError(t, db.Create(&models.ReminderCompletion{UserID: user.ID, ContactID: contact.ID, Message: "done", CompletedAt: time.Now()}).Error)

	require.NoError(t, db.Create(&models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "A gift idea"}).Error)

	require.NoError(t, db.Create(&models.ExternalIdentity{UserID: user.ID, EntityID: contact.VCardUID, System: "immich", ExternalID: "p-1"}).Error)
	require.NoError(t, db.Create(&models.ExternalActivity{UserID: user.ID, EntityID: contact.VCardUID, SourceSystem: "immich", ExternalID: "a-1", Type: "photo-appearance", OccurredAt: time.Now()}).Error)

	router.DELETE("/contacts/:id", DeleteContact)

	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.Itoa(int(contact.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assertCount := func(name string, model any, want int64, where string, args ...any) {
		t.Helper()
		var count int64
		require.NoError(t, db.Model(model).Where(where, args...).Count(&count).Error)
		assert.Equal(t, want, count, "%s row count mismatch after DeleteContact", name)
	}

	assertCount("RelationshipEdge", &models.RelationshipEdge{}, 0, "user_id = ?", user.ID)
	assertCount("LifeEvent", &models.LifeEvent{}, 0, "user_id = ?", user.ID)
	assertCount("HouseholdMember", &models.HouseholdMember{}, 0, "user_id = ?", user.ID)
	assertCount("CircleMember", &models.CircleMember{}, 0, "user_id = ?", user.ID)
	assertCount("ContactTag", &models.ContactTag{}, 0, "user_id = ?", user.ID)
	assertCount("FieldValue", &models.FieldValue{}, 0, "user_id = ?", user.ID)
	assertCount("Gift", &models.Gift{}, 0, "user_id = ?", user.ID)
	assertCount("ExternalIdentity", &models.ExternalIdentity{}, 0, "user_id = ?", user.ID)
	assertCount("ExternalActivity", &models.ExternalActivity{}, 0, "user_id = ?", user.ID)
	assertCount("ContactSyncLink", &models.ContactSyncLink{}, 0, "user_id = ?", user.ID)
	assertCount("ReminderCompletion", &models.ReminderCompletion{}, 0, "user_id = ?", user.ID)

	// The shared containers themselves must survive
	assertCount("Household", &models.Household{}, 1, "id = ?", household.ID)
	assertCount("Circle", &models.Circle{}, 1, "id = ?", circle.ID)
	assertCount("Tag", &models.Tag{}, 1, "id = ?", tag.ID)
	assertCount("FieldDefinition", &models.FieldDefinition{}, 1, "id = ?", fieldDef.ID)
}

func TestGetCircles(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.GET("/contacts/circles", GetCircles)

	// T4: GetCircles now reads from the real circles table, not the flat
	// Contact.Circles JSON column. Create Circle entities instead.
	db.Create(&models.Circle{UserID: user.ID, Name: "Friends"})
	db.Create(&models.Circle{UserID: user.ID, Name: "Family"})
	db.Create(&models.Circle{UserID: user.ID, Name: "Work"})

	// Make the request to get circles
	req, _ := http.NewRequest("GET", "/contacts/circles", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody []string
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, int(3), len(responseBody))
	assert.ElementsMatch(t, []string{"Friends", "Family", "Work"}, responseBody)
}

func TestDeleteContactCleansUpPhotos(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.DELETE("/contacts/:id", DeleteContact)

	// Create a temporary directory for test photos
	tempDir, err := os.MkdirTemp("", "photo_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Set the PROFILE_PHOTO_DIR environment variable
	originalDir := os.Getenv("PROFILE_PHOTO_DIR")
	os.Setenv("PROFILE_PHOTO_DIR", tempDir)
	defer os.Setenv("PROFILE_PHOTO_DIR", originalDir)

	// Create test photo files
	photoName := "test_photo.jpg"
	thumbnailName := "test_thumbnail.jpg"
	photoPath := filepath.Join(tempDir, photoName)
	thumbnailPath := filepath.Join(tempDir, thumbnailName)

	err = os.WriteFile(photoPath, []byte("fake photo data"), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(thumbnailPath, []byte("fake thumbnail data"), 0644)
	assert.NoError(t, err)

	// Verify files exist
	_, err = os.Stat(photoPath)
	assert.NoError(t, err, "Photo file should exist before deletion")
	_, err = os.Stat(thumbnailPath)
	assert.NoError(t, err, "Thumbnail file should exist before deletion")

	// Create a contact with photo references
	contact := models.Contact{
		UserID:         user.ID,
		Firstname:      "Alice",
		Lastname:       "Johnson",
		Photo:          photoName,
		PhotoThumbnail: thumbnailName,
	}
	db.Create(&contact)

	// Make the request to delete the contact
	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.Itoa(int(contact.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &respBody)
	assert.Equal(t, "Contact deleted", respBody["message"])

	// Verify photo files are deleted
	_, err = os.Stat(photoPath)
	assert.True(t, os.IsNotExist(err), "Photo file should be deleted")
	_, err = os.Stat(thumbnailPath)
	assert.True(t, os.IsNotExist(err), "Thumbnail file should be deleted")
}

func TestDeleteContactWithNoPhotos(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.DELETE("/contacts/:id", DeleteContact)

	// Create a contact without photos
	contact := models.Contact{
		UserID:         user.ID,
		Firstname:      "Bob",
		Lastname:       "Smith",
		Photo:          "",
		PhotoThumbnail: "",
	}
	db.Create(&contact)

	// Make the request to delete the contact (should not error even without photos)
	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.Itoa(int(contact.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]string
	json.Unmarshal(w.Body.Bytes(), &respBody)
	assert.Equal(t, "Contact deleted", respBody["message"])
}

// M3/T26: re-import with same vcard_uid after soft-delete must succeed.
// The partial unique index (migration 000039) only applies WHERE
// deleted_at IS NULL, so a soft-deleted contact no longer occupies the
// vcard_uid slot.
func TestRecreateContactAfterDeleteUsesSameVCardUID(t *testing.T) {
	db, router := setupRouter()

	var user models.User
	db.First(&user)

	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.DELETE("/contacts/:id", DeleteContact)

	first := models.Contact{UserID: user.ID, Firstname: "First"}
	db.Create(&first)
	vcard := first.VCardUID

	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.Itoa(int(first.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Attempt to create a second contact that would collide on vcard_uid
	// if the old soft-deleted row still occupied the unique index.
	secondJSON := `{"card":{"name":{"components":[{"kind":"given","value":"Second"}]}}}`
	req2, _ := http.NewRequest("POST", "/contacts", bytes.NewBufferString(secondJSON))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusCreated, w2.Code, "re-creating a contact after soft-delete must not collide on vcard_uid (T26)")

	// Verify the new contact was created successfully with a fresh vcard_uid.
	var resp struct {
		Contact struct {
			UID string `json:"uid"`
		} `json:"contact"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp.Contact.UID)
	assert.NotEqual(t, vcard, resp.Contact.UID, "new contact gets a fresh vcard_uid — the old one is still reserved by the old row")
}
