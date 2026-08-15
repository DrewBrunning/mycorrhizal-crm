package services

import (
	"mycorrhizal/database"
	"mycorrhizal/models"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newSearchDB migrates a REAL schema (database.InitDB) because the FTS5
// virtual tables + triggers live in migrations 000007 + 000010 and do not
// exist under AutoMigrate — a test that searched an AutoMigrate DB would
// silently return "no such table". The FTS5-vs-driver trap (the 'delete'
// special command failing on glebarez/sqlite) is exactly the kind of thing
// only a real-schema test can catch.
func newSearchDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "search-real.db"))
	require.NoError(t, err)
	return db
}

// AutoMigrate-based DB for service tests that don't touch FTS tables (e.g.
// synonym resolution) — keeps those fast.
func newSynonymDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Note{}, &models.Activity{}))
	return db
}

func seedSearchData(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	// Contact + note + activity that match "symphony".
	c := models.Contact{UserID: userID, Firstname: "Wolfgang", Lastname: "Symphony"}
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Create(&models.Note{UserID: userID, Content: "met Wolfgang about the symphony project", Date: time.Now(), ContactID: &c.ID}).Error)
	require.NoError(t, db.Create(&models.Activity{UserID: userID, Title: "Symphony rehearsal", Description: "second movement", Date: time.Now()}).Error)
}

func TestSearch_FindsContactNoteAndActivity(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-1", Password: "password123!A", Email: "search-1@example.com"}
	require.NoError(t, db.Create(&user).Error)
	seedSearchData(t, db, user.ID)

	result, err := Search(db, user.ID, "symphony", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 1, "the contact's lastname matches")
	assert.Equal(t, "Wolfgang", result.Contacts[0].Firstname)
	require.Len(t, result.Notes, 1, "the note's body matches")
	assert.Contains(t, result.Notes[0].Content, "symphony")
	assert.Equal(t, "Wolfgang Symphony", result.Notes[0].ContactName, "note carries its contact's name")
	require.Len(t, result.Activities, 1, "the activity title matches")
	assert.Equal(t, "Symphony rehearsal", result.Activities[0].Title)
}

func TestSearch_UnfiledNoteHasEmptyContactName(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-unf", Password: "password123!A", Email: "search-unf@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// A note without a contact_id (the dead-end journal's "unfiled" note).
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "unfiled thought about nothing", Date: time.Now()}).Error)

	result, err := Search(db, user.ID, "unfiled", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Notes, 1)
	assert.Equal(t, "", result.Notes[0].ContactName, "an unfiled note must have an empty contact_name")
	assert.Nil(t, result.Notes[0].ContactID, "contact_id must be nil for an unfiled note")
}

func TestSearch_SoftDeletedRowNotFindable(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-2", Password: "password123!A", Email: "search-2@example.com"}
	require.NoError(t, db.Create(&user).Error)

	note := models.Note{UserID: user.ID, Content: "needle in the haystack", Date: time.Now()}
	require.NoError(t, db.Create(&note).Error)

	activity := models.Activity{UserID: user.ID, Title: "findable activity title", Date: time.Now()}
	require.NoError(t, db.Create(&activity).Error)

	found, err := Search(db, user.ID, "haystack", 0, nil)
	require.NoError(t, err)
	require.Len(t, found.Notes, 1)

	foundAct, err := Search(db, user.ID, "findable", 0, nil)
	require.NoError(t, err)
	require.Len(t, foundAct.Activities, 1)

	// Soft delete → the FTS trigger removes it from the index.
	require.NoError(t, db.Delete(&note).Error)
	found2, err := Search(db, user.ID, "haystack", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, found2.Notes, "a soft-deleted note must not be findable")

	require.NoError(t, db.Delete(&activity).Error)
	foundAct2, err := Search(db, user.ID, "findable", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, foundAct2.Activities, "a soft-deleted activity must not be findable")
}

func TestSearch_CrossUserReturnsNothing(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-3", Password: "password123!A", Email: "search-3@example.com"}
	other := models.User{Username: "search-4", Password: "password123!A", Email: "search-4@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&other).Error)

	// Only the OTHER user has matching data.
	seedSearchData(t, db, other.ID)

	result, err := Search(db, user.ID, "symphony", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Contacts, "cross-user contacts must not leak")
	assert.Empty(t, result.Notes, "cross-user notes must not leak")
	assert.Empty(t, result.Activities, "cross-user activities must not leak")
}

func TestSearch_SynonymResolved(t *testing.T) {
	db := newSearchDB(t)

	// "brother" resolves through the registry to sibling_of.
	token, ok := ResolveSearchSynonym("brother")
	require.True(t, ok)
	assert.Equal(t, "sibling_of", token)

	// A whole-phrase synonym ("sister's") does not resolve.
	_, ok = ResolveSearchSynonym("sister's")
	assert.False(t, ok)

	result, err := Search(db, 1, "brother", 0, nil)
	require.NoError(t, err)
	assert.Equal(t, "sibling_of", result.ResolvedRelation, "the resolved synonym is echoed in the response")
}

// TestSearch_HouseholdScope pins T11's "everyone in the Smith household":
// with a householdID, contact hits are restricted to that household's
// members; notes/activities are unaffected.
func TestSearch_HouseholdScope(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-hh", Password: "password123!A", Email: "search-hh@example.com"}
	require.NoError(t, db.Create(&user).Error)

	smith1 := models.Contact{UserID: user.ID, Firstname: "Ann", Lastname: "Smith"}
	smith2 := models.Contact{UserID: user.ID, Firstname: "Ben", Lastname: "Smith"}
	jones := models.Contact{UserID: user.ID, Firstname: "Zoe", Lastname: "Smith"}
	require.NoError(t, db.Create(&smith1).Error)
	require.NoError(t, db.Create(&smith2).Error)
	require.NoError(t, db.Create(&jones).Error)

	household := models.Household{UserID: user.ID, Name: "Smith household", Type: models.HouseholdTypeFamilyUnit}
	require.NoError(t, db.Create(&household).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: smith1.VCardUID, Role: "adult"}).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: smith2.VCardUID, Role: "adult"}).Error)

	// Unscoped: all three Smiths findable.
	all, err := Search(db, user.ID, "smith", 0, nil)
	require.NoError(t, err)
	require.Len(t, all.Contacts, 3)

	// Scoped to the household: only the two members.
	hhID := household.ID
	scoped, err := Search(db, user.ID, "smith", 0, &hhID)
	require.NoError(t, err)
	require.Len(t, scoped.Contacts, 2, "only household members are returned")
	for _, c := range scoped.Contacts {
		assert.NotEqual(t, jones.VCardUID, c.UID, "the non-member must be excluded")
	}
}

func TestRebuildSearchIndex_Idempotent(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-5", Password: "password123!A", Email: "search-5@example.com"}
	require.NoError(t, db.Create(&user).Error)
	seedSearchData(t, db, user.ID)

	// Rebuild twice — idempotent.
	require.NoError(t, RebuildSearchIndex(db))
	require.NoError(t, RebuildSearchIndex(db))

	result, err := Search(db, user.ID, "symphony", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 1, "rebuild must not duplicate rows")
	require.Len(t, result.Notes, 1)
	require.Len(t, result.Activities, 1)

	// After a rebuild, a soft-deleted row still disappears (index rebuilt
	// from live rows only).
	note := models.Note{UserID: user.ID, Content: "another needle", Date: time.Now()}
	require.NoError(t, db.Create(&note).Error)
	require.NoError(t, db.Delete(&note).Error)
	require.NoError(t, RebuildSearchIndex(db))
	found, err := Search(db, user.ID, "another needle", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, found.Notes)
}

// TestSearch_SpecialCharactersDoNotError pins that adversarial user input
// (quotes, operators, wildcards) never turns into an FTS5 syntax error — the
// escaping in ftsQuery must keep every query runnable even if it matches
// nothing.
func TestSearch_SpecialCharactersDoNotError(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-sp", Password: "password123!A", Email: "search-sp@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "the quick brown fox", Date: time.Now()}).Error)

	inputs := []string{
		`say "hi"`,         // embedded quotes
		`it's`,             // apostrophe
		`AND OR NOT`,       // FTS5 boolean operators
		`NEAR(quick, fox)`, // FTS5 operator syntax
		`quick*`,           // user-supplied wildcard
		`a "b" c`,          // quotes mid-phrase
		`-exclude`,         // negation operator
		`"`,                // lone quote
		`*`,                // lone wildcard
	}
	for _, in := range inputs {
		result, err := Search(db, user.ID, in, 0, nil)
		require.NoError(t, err, "query %q must not error", in)
		assert.Equal(t, in, result.Query, "query echoed back")
	}
}

// TestSearch_SecretSensitivityNotFindable pins T11's sensitivity rule:
// a secret-sensitivity item must never surface through full-text search. The
// FTS index intentionally covers only non-sensitive columns (contacts' basic
// fields, notes, activity titles) — relationship edges, tags, and custom-field
// values are NOT indexed — so a secret edge's type, a secret tag, or a secret
// custom field's value must not make anything findable.
func TestSearch_SecretSensitivityNotFindable(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-sec", Password: "password123!A", Email: "search-sec@example.com"}
	require.NoError(t, db.Create(&user).Error)

	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&alice).Error)
	// A secret-sensitivity edge whose type doubles as a plausible search term.
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: alice.VCardUID, TargetID: alice.VCardUID, Type: "conflicts_with",
		Source: models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)
	// A secret custom field value.
	fieldDef := models.FieldDefinition{UserID: user.ID, Label: "Secret", Key: "secret", Target: "contact", Type: "text", Sensitivity: models.RelationshipSensitivitySecret}
	require.NoError(t, db.Create(&fieldDef).Error)
	require.NoError(t, db.Create(&models.FieldValue{FieldDefinitionID: fieldDef.ID, UserID: user.ID, EntityID: alice.VCardUID, Value: []byte(`"hidden-codeword"`)}).Error)

	// Neither the secret relation type nor the secret custom-field value is
	// findable.
	for _, term := range []string{"conflicts_with", "hidden-codeword"} {
		result, err := Search(db, user.ID, term, 0, nil)
		require.NoError(t, err)
		assert.Empty(t, result.Contacts, "a secret item must not make a contact findable via %q", term)
		assert.Empty(t, result.Notes)
		assert.Empty(t, result.Activities)
	}

	// Sanity: the contact's ordinary name IS still findable.
	result, err := Search(db, user.ID, "alice", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 1)
}

// TestSearch_AddressMatch pins T38's core gap: address text (street, city,
// postal) is part of the searchable surface, not just names/email/phone/org.
// The tokens are chosen to be absent from every other indexed field so the
// match can only come from the addresses_flat column.
func TestSearch_AddressMatch(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-addr", Password: "password123!A", Email: "search-addr@example.com"}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Arthur", Lastname: "Harding",
		Addresses: []models.ContactAddress{
			{Type: "home", Street: "Mangosteen Lane", City: "Clarkston", Region: "IL", Postal: "62701", Country: "USA"},
			{Type: "work", Street: "14 Nectarine St", City: "Chicago"},
		},
	}).Error)

	// Street of the first address, city of the first address, street of the
	// second address — and a postal code.
	for _, term := range []string{"mangosteen", "clarkston", "nectarine", "62701"} {
		result, err := Search(db, user.ID, term, 0, nil)
		require.NoError(t, err, "term %q must not error", term)
		require.Len(t, result.Contacts, 1, "term %q must find the contact by its address", term)
		assert.Equal(t, "Arthur", result.Contacts[0].Firstname)
	}

	// Prefix matching within an address term (same FTS behavior as names).
	result, err := Search(db, user.ID, "mangost", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 1, "a prefix of an address token must match")
}

// TestSearch_SoftDeletedAddressNotFindable re-applies T11's soft-delete trap
// to T38's new surface: a soft-deleted contact's address must drop out of
// the index (the AFTER UPDATE trigger re-inserts only when deleted_at IS
// NULL).
func TestSearch_SoftDeletedAddressNotFindable(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-addr-del", Password: "password123!A", Email: "search-addr-del@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{
		UserID: user.ID, Firstname: "Jane", Lastname: "Doe",
		Addresses: []models.ContactAddress{{Type: "home", Street: "Rosebud Avenue", City: "Bedford"}},
	}
	require.NoError(t, db.Create(&contact).Error)

	found, err := Search(db, user.ID, "rosebud", 0, nil)
	require.NoError(t, err)
	require.Len(t, found.Contacts, 1, "the address must be findable before deletion")

	require.NoError(t, db.Delete(&contact).Error)
	found2, err := Search(db, user.ID, "rosebud", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, found2.Contacts, "a soft-deleted contact's address must not be findable")
}

// TestSearch_CrossUserAddressDoesNotLeak re-applies T11's user-scoping rule
// (the highest-risk correctness issue of that ticket) to T38's new surface:
// an address match in another user's index rows must never surface for this
// user.
func TestSearch_CrossUserAddressDoesNotLeak(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-addr-u1", Password: "password123!A", Email: "search-addr-u1@example.com"}
	other := models.User{Username: "search-addr-u2", Password: "password123!A", Email: "search-addr-u2@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&other).Error)

	// Only the OTHER user has a contact whose address matches the term.
	require.NoError(t, db.Create(&models.Contact{
		UserID: other.ID, Firstname: "Rita", Lastname: "Bell",
		Addresses: []models.ContactAddress{{Type: "home", Street: "Primrose Court", City: "Alberta"}},
	}).Error)

	result, err := Search(db, user.ID, "primrose", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Contacts, "a cross-user address match must not leak")
	assert.Empty(t, result.Notes)
	assert.Empty(t, result.Activities)

	// Sanity: the owner CAN find it.
	owner, err := Search(db, other.ID, "primrose", 0, nil)
	require.NoError(t, err)
	require.Len(t, owner.Contacts, 1)
}

// TestRebuildSearchIndex_AddressesSearchable pins the ticket's hand-verified
// requirement as a real test: RebuildSearchIndex against existing contacts
// makes their addresses searchable without editing them first. The trigger
// path is bypassed (index wiped) to prove the rebuild itself carries the
// address text, not the live trigger.
func TestRebuildSearchIndex_AddressesSearchable(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-rebuild-addr", Password: "password123!A", Email: "search-rebuild-addr@example.com"}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Pam", Lastname: "Post",
		Addresses: []models.ContactAddress{{Type: "home", Street: "Heliotrope Drive", City: "Oakville"}},
	}).Error)

	// Simulate a bulk operation that bypassed the triggers (raw SQL wipe).
	require.NoError(t, db.Exec("DELETE FROM contacts_fts").Error)
	stale, err := Search(db, user.ID, "heliotrope", 0, nil)
	require.NoError(t, err)
	require.Empty(t, stale.Contacts, "the wiped index must not find the address yet")

	require.NoError(t, RebuildSearchIndex(db))
	result, err := Search(db, user.ID, "heliotrope", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 1, "after a rebuild, an existing contact's address must be findable without editing it")
}

// TestPhoneQueryTokens pins the phone-shape detection T69's search paths rely
// on: a term is phone-shaped when it is mostly digits with only phone
// punctuation/whitespace alongside, and is then reduced to its digit string
// and PhoneKey. Anything else — plain text, or digits mixed with letters —
// must be left alone so ordinary search is untouched.
func TestPhoneQueryTokens(t *testing.T) {
	digits, key, ok := PhoneQueryTokens("(800) 555-1234")
	require.True(t, ok)
	assert.Equal(t, "8005551234", digits)
	assert.Equal(t, "8005551234", key)

	digits, key, ok = PhoneQueryTokens("+18005551234")
	require.True(t, ok)
	assert.Equal(t, "18005551234", digits)
	assert.Equal(t, "8005551234", key, "an 11-digit number's key is its last 10")

	digits, key, ok = PhoneQueryTokens("8005551234")
	require.True(t, ok)
	assert.Equal(t, "8005551234", digits)

	// Plain text and mixed text are not phone-shaped.
	_, _, ok = PhoneQueryTokens("alice")
	assert.False(t, ok)
	_, _, ok = PhoneQueryTokens("800 flowers")
	assert.False(t, ok)
	_, _, ok = PhoneQueryTokens("ext 5599")
	assert.False(t, ok)
	// No digits at all is not phone-shaped.
	_, _, ok = PhoneQueryTokens("(  )")
	assert.False(t, ok)
}

// TestSearch_PhoneCrossFormat pins T69's core gap on path 1 (global FTS
// search): a phone-shaped query finds a contact regardless of punctuation,
// grouping, or country-code differences between the query and the stored
// value. The two contacts' tokens are absent from every other indexed field,
// so the match can only come from the phones_normalized column.
func TestSearch_PhoneCrossFormat(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-phone", Password: "password123!A", Email: "search-phone@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Stored with punctuation, no country code.
	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Grace", Lastname: "Hopper",
		Phones: []models.ContactPhone{{Type: "cell", Value: "(800) 555-1234"}},
	}).Error)
	// Stored with a +1 country code (11 digits — exercises the key token).
	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Alan", Lastname: "Turing",
		Phones: []models.ContactPhone{{Type: "cell", Value: "+18005551234"}},
	}).Error)

	// Query as unpunctuated 10 digits → finds the punctuated-stored contact.
	result, err := Search(db, user.ID, "8005551234", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 2, "both stored forms must match the 10-digit query")

	// Query as the international form → finds the country-code-stored contact
	// and the plain one (via the shared last-10 key).
	result, err = Search(db, user.ID, "(800) 555-1234", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 2, "the punctuated query must match both stored forms")

	// The full-digit query matches both stored forms too — they are the same
	// number (the whole point of T68/T69), and the query's own last-10 key
	// covers the punctuated form.
	result, err = Search(db, user.ID, "18005551234", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 2, "the full 11-digit query must match both stored forms of the same number")

	// A different number must not match.
	result, err = Search(db, user.ID, "8005559999", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Contacts, "a different number must not match")
}

// TestSearch_NonPrimaryPhoneFindable pins T69's fourth gap on path 1: the FTS
// index previously covered only the flat `phone` scalar (the primary number),
// so a contact could not be found by their second or third number even when
// typed perfectly. phones_normalized is built from every Phones[] entry, so a
// non-primary number must now be findable.
func TestSearch_NonPrimaryPhoneFindable(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-phone2", Password: "password123!A", Email: "search-phone2@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// Primary scalar + a distinct second number that appears nowhere else.
	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Emmy", Lastname: "Noether",
		Phone: "(800) 555-1111",
		Phones: []models.ContactPhone{
			{Type: "cell", Value: "(800) 555-1111"},
			{Type: "work", Value: "(800) 555-2222"},
		},
	}).Error)

	result, err := Search(db, user.ID, "8005552222", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 1, "a non-primary phone must be findable through global search")
	assert.Equal(t, "Emmy", result.Contacts[0].Firstname)
}

// TestSearch_PhoneSoftDeletedNotFindable re-applies T11's soft-delete trap to
// T69's new surface: a soft-deleted contact's phone must drop out of the index
// (the AFTER UPDATE trigger re-inserts only when deleted_at IS NULL).
func TestSearch_PhoneSoftDeletedNotFindable(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-phone-del", Password: "password123!A", Email: "search-phone-del@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{
		UserID: user.ID, Firstname: "Katherine", Lastname: "Johnson",
		Phones: []models.ContactPhone{{Type: "cell", Value: "(800) 555-3333"}},
	}
	require.NoError(t, db.Create(&contact).Error)

	found, err := Search(db, user.ID, "8005553333", 0, nil)
	require.NoError(t, err)
	require.Len(t, found.Contacts, 1, "the phone must be findable before deletion")

	require.NoError(t, db.Delete(&contact).Error)
	found2, err := Search(db, user.ID, "8005553333", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, found2.Contacts, "a soft-deleted contact's phone must not be findable")
}

// TestSearch_CrossUserPhoneDoesNotLeak re-applies T11's user-scoping rule to
// T69's new surface: a phone match in another user's index rows must never
// surface for this user.
func TestSearch_CrossUserPhoneDoesNotLeak(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-phone-u1", Password: "password123!A", Email: "search-phone-u1@example.com"}
	other := models.User{Username: "search-phone-u2", Password: "password123!A", Email: "search-phone-u2@example.com"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&other).Error)

	// Only the OTHER user has a contact whose phone matches the term.
	require.NoError(t, db.Create(&models.Contact{
		UserID: other.ID, Firstname: "Rita", Lastname: "Bell",
		Phones: []models.ContactPhone{{Type: "cell", Value: "(800) 555-4444"}},
	}).Error)

	result, err := Search(db, user.ID, "8005554444", 0, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Contacts, "a cross-user phone match must not leak")

	// Sanity: the owner CAN find it.
	owner, err := Search(db, other.ID, "8005554444", 0, nil)
	require.NoError(t, err)
	require.Len(t, owner.Contacts, 1)
}

// TestRebuildSearchIndex_PhonesSearchable pins the ticket's hand-verified
// requirement as a real test: RebuildSearchIndex against existing contacts
// makes their phones searchable without editing them first. The trigger path
// is bypassed (index wiped) to prove the rebuild itself carries the phone
// tokens, not the live trigger.
func TestRebuildSearchIndex_PhonesSearchable(t *testing.T) {
	db := newSearchDB(t)
	user := models.User{Username: "search-rebuild-phone", Password: "password123!A", Email: "search-rebuild-phone@example.com"}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, db.Create(&models.Contact{
		UserID: user.ID, Firstname: "Pam", Lastname: "Post",
		Phones: []models.ContactPhone{{Type: "cell", Value: "(800) 555-6666"}},
	}).Error)

	// Simulate a bulk operation that bypassed the triggers (raw SQL wipe).
	require.NoError(t, db.Exec("DELETE FROM contacts_fts").Error)
	stale, err := Search(db, user.ID, "8005556666", 0, nil)
	require.NoError(t, err)
	require.Empty(t, stale.Contacts, "the wiped index must not find the phone yet")

	require.NoError(t, RebuildSearchIndex(db))
	result, err := Search(db, user.ID, "8005556666", 0, nil)
	require.NoError(t, err)
	require.Len(t, result.Contacts, 1, "after a rebuild, an existing contact's phone must be findable without editing it")
}
