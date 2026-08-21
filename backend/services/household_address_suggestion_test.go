package services

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// householdAddressTestDB opens a REAL migrated schema (CLAUDE.md backend
// trap 1 — DismissedHouseholdSuggestion's composite unique index and
// HouseholdMember's member_vcard_uid column must come from the hand-written
// migration SQL, not AutoMigrate) and creates a fresh test user.
func householdAddressTestDB(t *testing.T) (*gorm.DB, models.User) {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), "address-suggestion.db"))
	require.NoError(t, err)
	user := models.User{Username: "addruser", Password: "password123!A", Email: "addr@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

// addrContact creates a contact with the given street (and optionally more
// fields). The Addresses JSON column is set directly on the flat model — the
// same shape T40's scanner reads.
func addrContact(t *testing.T, db *gorm.DB, userID uint, firstname string, addr models.ContactAddress, archived bool) models.Contact {
	t.Helper()
	c := models.Contact{UserID: userID, Firstname: firstname, Archived: archived}
	if addr.Street != "" || addr.City != "" || addr.Region != "" || addr.Postal != "" || addr.Country != "" {
		c.Addresses = []models.ContactAddress{addr}
	}
	require.NoError(t, db.Create(&c).Error)
	return c
}

func TestAddressNormalizedKey(t *testing.T) {
	// Exercise the documented comparison scope: casing, punctuation, and
	// spacing normalize away; type and sub-street fields are NOT part of the
	// key.
	cases := []struct {
		name string
		a, b models.ContactAddress
		same bool
	}{
		{
			name: "casing and trailing punctuation normalize away",
			a:    models.ContactAddress{Street: "123 Main St.", City: "Springfield", Region: "IL", Postal: "62701", Country: "USA"},
			b:    models.ContactAddress{Street: "123 main st", City: "springfield", Region: "il", Postal: "62701", Country: "usa"},
			same: true,
		},
		{
			name: "different street is a different key",
			a:    models.ContactAddress{Street: "123 Main St", City: "Springfield", Country: "USA"},
			b:    models.ContactAddress{Street: "456 Oak Ave", City: "Springfield", Country: "USA"},
			same: false,
		},
		{
			name: "home vs work type is deliberately NOT part of the key",
			a:    models.ContactAddress{Type: "home", Street: "1 Main St", City: "X", Country: "USA"},
			b:    models.ContactAddress{Type: "work", Street: "1 Main St", City: "X", Country: "USA"},
			same: true,
		},
		{
			name: "different apartment still the same building (sub-street excluded)",
			a:    models.ContactAddress{Street: "1 Main St", Apartment: "1A", City: "X", Country: "USA"},
			b:    models.ContactAddress{Street: "1 Main St", Apartment: "2B", City: "X", Country: "USA"},
			same: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ka, kb := AddressNormalizedKey(tc.a), AddressNormalizedKey(tc.b)
			if (ka == kb) != tc.same {
				t.Errorf("keys %q vs %q: same=%v, want %v", ka, kb, ka == kb, tc.same)
			}
		})
	}
}

func TestDedupeContacts(t *testing.T) {
	db, user := householdAddressTestDB(t)
	a := addrContact(t, db, user.ID, "Alice", models.ContactAddress{}, false)
	b := addrContact(t, db, user.ID, "Bob", models.ContactAddress{}, false)

	// Duplicates by VCardUID must collapse, first occurrence order preserved.
	in := []models.Contact{a, b, a, b}
	out := dedupeContacts(in)
	require.Len(t, out, 2)
	assert.Equal(t, a.VCardUID, out[0].VCardUID)
	assert.Equal(t, b.VCardUID, out[1].VCardUID)
}

func TestGroupAlreadyCoMembers(t *testing.T) {
	db, user := householdAddressTestDB(t)

	a := addrContact(t, db, user.ID, "Alice", models.ContactAddress{}, false)
	b := addrContact(t, db, user.ID, "Bob", models.ContactAddress{}, false)
	c := addrContact(t, db, user.ID, "Charlie", models.ContactAddress{}, false)

	// No households at all: not co-members.
	ok, err := groupAlreadyCoMembers(db, user.ID, []string{a.VCardUID, b.VCardUID})
	require.NoError(t, err)
	assert.False(t, ok)

	// Two of the three share a household: the whole group is vetoed.
	hh := models.Household{UserID: user.ID, Name: "Home", Type: models.HouseholdTypeFamilyUnit}
	require.NoError(t, db.Create(&hh).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: a.VCardUID, Role: "adult"}).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: b.VCardUID, Role: "adult"}).Error)

	ok, err = groupAlreadyCoMembers(db, user.ID, []string{a.VCardUID, b.VCardUID, c.VCardUID})
	require.NoError(t, err)
	assert.True(t, ok)

	// A different user's household membership must not count.
	other := models.User{Username: "otheruser", Password: "password123!A", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	ok, err = groupAlreadyCoMembers(db, other.ID, []string{a.VCardUID, b.VCardUID})
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestGroupIsDismissed(t *testing.T) {
	db, user := householdAddressTestDB(t)

	// Nothing dismissed yet.
	d, err := groupIsDismissed(db, user.ID, "addrhash1", "memberhash1")
	require.NoError(t, err)
	assert.False(t, d)

	require.NoError(t, db.Create(&models.DismissedHouseholdSuggestion{
		UserID: user.ID, AddressHash: "addrhash1", MemberHash: "memberhash1",
	}).Error)

	d, err = groupIsDismissed(db, user.ID, "addrhash1", "memberhash1")
	require.NoError(t, err)
	assert.True(t, d)

	// The hash pair is the identity — a different member set is not dismissed.
	d, err = groupIsDismissed(db, user.ID, "addrhash1", "memberhash2")
	require.NoError(t, err)
	assert.False(t, d)
}

func TestValidHouseholdType(t *testing.T) {
	assert.True(t, validHouseholdType(models.HouseholdTypeFamilyUnit))
	assert.True(t, validHouseholdType(models.HouseholdTypeRoommates))
	assert.True(t, validHouseholdType(models.HouseholdTypeOther))
	assert.False(t, validHouseholdType(""))
	assert.False(t, validHouseholdType("roommate"))
	assert.False(t, validHouseholdType("extended_family"))
}

func TestFirstnames(t *testing.T) {
	db, user := householdAddressTestDB(t)
	c := addrContact(t, db, user.ID, "Alice", models.ContactAddress{}, false)
	assert.Equal(t, "Alice", firstnames(c))

	cOnlyFN := models.Contact{UserID: user.ID, FN: "Ada Lovelace", Firstname: ""}
	assert.Equal(t, "Ada Lovelace", firstnames(cOnlyFN))

	cOnlyNick := models.Contact{UserID: user.ID, Nickname: "Lovelace", Firstname: ""}
	assert.Equal(t, "Lovelace", firstnames(cOnlyNick))
}

func TestSharedAddressKey(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)
	c := addrContact(t, db, user.ID, "Charlie", models.ContactAddress{Street: "999 Nowhere", City: "X", Country: "USA"}, false)

	key, addr := sharedAddressKey([]models.Contact{a, b})
	assert.NotEmpty(t, key)
	assert.Equal(t, "1 Main St", addr.Street)
	assert.Equal(t, AddressNormalizedKey(shared), key)

	// A contact with no shared key at all yields "".
	key, _ = sharedAddressKey([]models.Contact{a, c})
	assert.Empty(t, key)

	// Empty input is safe.
	key, _ = sharedAddressKey(nil)
	assert.Empty(t, key)
}

func TestSHA256Hex(t *testing.T) {
	sum := sha256.Sum256([]byte("hello"))
	want := hex.EncodeToString(sum[:])
	assert.Equal(t, want, sha256Hex("hello"))
	assert.Equal(t, 64, len(sha256Hex("hello")))
}

func TestGenerateAddressHouseholdSuggestions(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Postal: "62701", Country: "USA"}
	addrContact(t, db, user.ID, "Alice", shared, false)
	addrContact(t, db, user.ID, "Bob", shared, false)
	// Same normalized address but a different street string (casing + trailing
	// punctuation) — must still group.
	almost := models.ContactAddress{Street: "1 MAIN ST.", City: "springfield", Postal: "62701", Country: "usa"}
	addrContact(t, db, user.ID, "Charlie", almost, false)

	suggestions, err := GenerateAddressHouseholdSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)

	s := suggestions[0]
	assert.Len(t, s.MemberVCardUIDs, 3)
	// Members sorted by VCardUID.
	assert.True(t, sort.StringsAreSorted(s.MemberVCardUIDs))
	assert.Equal(t, s.AddressHash, sha256Hex(AddressNormalizedKey(shared)))
	memberHash := sha256Hex(s.MemberVCardUIDs[0] + "," + s.MemberVCardUIDs[1] + "," + s.MemberVCardUIDs[2])
	assert.Equal(t, memberHash, s.MemberHash)
	// The shared address displayed is the lexicographically-first member's —
	// which is random, so assert on the normalized key (which is what grouped
	// them) rather than the raw rendered street.
	assert.Equal(t, AddressNormalizedKey(shared), AddressNormalizedKey(models.ContactAddress{
		Street: s.Address.Components[0].Value, City: "Springfield", Postal: "62701", Country: "USA",
	}))
}

func TestGenerateAddressHouseholdSuggestions_DeterministicOrder(t *testing.T) {
	db, user := householdAddressTestDB(t)

	// Two distinct shared-address groups.
	group1 := models.ContactAddress{Street: "1 Main St", City: "A", Country: "USA"}
	group2 := models.ContactAddress{Street: "2 Oak Ave", City: "B", Country: "USA"}
	addrContact(t, db, user.ID, "A1", group1, false)
	addrContact(t, db, user.ID, "A2", group1, false)
	addrContact(t, db, user.ID, "B1", group2, false)
	addrContact(t, db, user.ID, "B2", group2, false)

	first, err := GenerateAddressHouseholdSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, first, 2)
	second, err := GenerateAddressHouseholdSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, second, 2)

	assert.Equal(t, first[0].AddressHash, second[0].AddressHash)
	assert.Equal(t, first[1].AddressHash, second[1].AddressHash)
	assert.True(t, first[0].AddressHash < first[1].AddressHash, "suggestions must be sorted by address_hash")
}

func TestGenerateAddressHouseholdSuggestions_Filters(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)

	// Archived contacts are excluded from the scan entirely.
	addrContact(t, db, user.ID, "Zoe", shared, true)

	// A single contact at an address has no group.
	addrContact(t, db, user.ID, "Solo", models.ContactAddress{Street: "9 Lone Way", City: "X", Country: "USA"}, false)

	suggestions, err := GenerateAddressHouseholdSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	wantUIDs := []string{a.VCardUID, b.VCardUID}
	sort.Strings(wantUIDs)
	assert.Equal(t, wantUIDs, suggestions[0].MemberVCardUIDs)

	// Once they become co-members of a household, the group is suppressed.
	hh := models.Household{UserID: user.ID, Name: "Home", Type: models.HouseholdTypeFamilyUnit}
	require.NoError(t, db.Create(&hh).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: a.VCardUID, Role: "adult"}).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: b.VCardUID, Role: "adult"}).Error)

	suggestions, err = GenerateAddressHouseholdSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

func TestGenerateAddressHouseholdSuggestions_DismissedGroupHidden(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)

	require.NoError(t, DismissAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, b.VCardUID}))

	suggestions, err := GenerateAddressHouseholdSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions, "a dismissed group must not be re-offered")
}

func TestAcceptAddressHouseholdSuggestion(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Postal: "62701", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)

	hh, err := AcceptAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, b.VCardUID}, "", "")
	require.NoError(t, err)
	require.NotNil(t, hh)

	// Default name from firstnames joined with " & ". Accept sorts the UIDs
	// first, so the name's order follows the sorted VCardUIDs — which are
	// random UUIDs — not the call order.
	wantName := "Alice & Bob"
	if b.VCardUID < a.VCardUID {
		wantName = "Bob & Alice"
	}
	assert.Equal(t, wantName, hh.Name)
	// Default type.
	assert.Equal(t, models.HouseholdTypeFamilyUnit, hh.Type)
	// The shared address is copied onto the household.
	require.NotNil(t, hh.Address)
	assert.Equal(t, "1 Main St", hh.Address.Components[0].Value)

	var members []models.HouseholdMember
	require.NoError(t, db.Where("household_id = ?", hh.ID).Find(&members).Error)
	require.Len(t, members, 2)
	for _, m := range members {
		assert.Equal(t, models.HouseholdRoleAdult, m.Role)
		assert.Equal(t, user.ID, m.UserID)
	}
}

func TestAcceptAddressHouseholdSuggestion_Validation(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)

	// Fewer than two members.
	_, err := AcceptAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID}, "", "")
	require.Error(t, err)

	// Duplicates collapse to one distinct member.
	_, err = AcceptAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, a.VCardUID}, "", "")
	require.Error(t, err)

	// Invalid household type.
	_, err = AcceptAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, b.VCardUID}, "", "extended_family")
	require.Error(t, err)

	// A UID that isn't the user's contact.
	_, err = AcceptAddressHouseholdSuggestion(db, user.ID, []string{"00000000-0000-4000-8000-000000000999", b.VCardUID}, "", "")
	require.Error(t, err)

	// Contacts that no longer share an address.
	c := addrContact(t, db, user.ID, "Charlie", models.ContactAddress{Street: "999 Nowhere", City: "X", Country: "USA"}, false)
	_, err = AcceptAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, c.VCardUID}, "", "")
	require.Error(t, err)
}

func TestAcceptAddressHouseholdSuggestion_RejectsCoMembersAndDismissed(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)
	c := addrContact(t, db, user.ID, "Charlie", shared, false)
	d := addrContact(t, db, user.ID, "Dana", shared, false)

	// Already co-members elsewhere: conflict.
	hh := models.Household{UserID: user.ID, Name: "Home", Type: models.HouseholdTypeFamilyUnit}
	require.NoError(t, db.Create(&hh).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: a.VCardUID, Role: "adult"}).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: b.VCardUID, Role: "adult"}).Error)
	_, err := AcceptAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, b.VCardUID}, "", "")
	require.Error(t, err)

	// Dismissed group: conflict.
	require.NoError(t, DismissAddressHouseholdSuggestion(db, user.ID, []string{c.VCardUID, d.VCardUID}))
	_, err = AcceptAddressHouseholdSuggestion(db, user.ID, []string{c.VCardUID, d.VCardUID}, "", "")
	require.Error(t, err)
}

func TestAcceptAddressHouseholdSuggestion_ExplicitNameAndType(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)

	hh, err := AcceptAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, b.VCardUID}, "The Apts", models.HouseholdTypeRoommates)
	require.NoError(t, err)
	assert.Equal(t, "The Apts", hh.Name)
	assert.Equal(t, models.HouseholdTypeRoommates, hh.Type)
}

func TestDismissAddressHouseholdSuggestion(t *testing.T) {
	db, user := householdAddressTestDB(t)

	shared := models.ContactAddress{Street: "1 Main St", City: "Springfield", Country: "USA"}
	a := addrContact(t, db, user.ID, "Alice", shared, false)
	b := addrContact(t, db, user.ID, "Bob", shared, false)

	require.NoError(t, DismissAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, b.VCardUID}))

	// The dismissal row exists with the recomputed hashes.
	var rows []models.DismissedHouseholdSuggestion
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	key := AddressNormalizedKey(shared)
	assert.Equal(t, sha256Hex(key), rows[0].AddressHash)
	uids := []string{a.VCardUID, b.VCardUID}
	sort.Strings(uids)
	assert.Equal(t, sha256Hex(uids[0]+","+uids[1]), rows[0].MemberHash)

	// Re-dismissing is a checked ErrAlreadyExists.
	err := DismissAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, b.VCardUID})
	require.Error(t, err)

	// Fewer than two distinct members.
	err = DismissAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, a.VCardUID})
	require.Error(t, err)

	// Members with no shared address.
	c := addrContact(t, db, user.ID, "Charlie", models.ContactAddress{Street: "999 Nowhere", City: "X", Country: "USA"}, false)
	err = DismissAddressHouseholdSuggestion(db, user.ID, []string{a.VCardUID, c.VCardUID})
	require.Error(t, err)
}
