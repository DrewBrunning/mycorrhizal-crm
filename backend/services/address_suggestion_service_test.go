package services

import (
	"testing"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAddressSuggestionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	return db
}

func createAddressTestUser(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()
	user := models.User{Username: username, Password: "password123!A", Email: username + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createAddressTestContact(t *testing.T, db *gorm.DB, userID uint, firstname string, addresses ...models.ContactAddress) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: firstname, Addresses: addresses}
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

func createAddressTestEdge(t *testing.T, db *gorm.DB, userID uint, source, target, edgeType, status, sensitivity string) {
	t.Helper()
	edge := models.RelationshipEdge{
		UserID: userID, SourceID: source, TargetID: target, Type: edgeType,
		Directional: !models.IsSymmetricRelationType(edgeType),
		Source:      models.RelationshipSourceUserConfirmed, Confidence: 1.0,
		Status: status, Sensitivity: sensitivity,
	}
	require.NoError(t, db.Create(&edge).Error)
}

func createAddressTestHousehold(t *testing.T, db *gorm.DB, userID uint, name string, flat models.ContactAddress, members ...models.Contact) models.Household {
	t.Helper()
	addr := models.AddressFromContactAddress(flat)
	household := models.Household{UserID: userID, Name: name, Type: models.HouseholdTypeFamilyUnit, Address: &addr}
	require.NoError(t, db.Create(&household).Error)
	for _, m := range members {
		require.NoError(t, db.Create(&models.HouseholdMember{
			HouseholdID: household.ID, UserID: userID, MemberVCardUID: m.VCardUID, Role: models.HouseholdRoleAdult,
		}).Error)
	}
	return household
}

var clarkSt = models.ContactAddress{Street: "742 Clark St", City: "Springfield", Region: "IL", Postal: "62701", Country: "USA"}

// TestGenerateContactAddressSuggestions_SpouseProposesAddress pins the core
// relationship rule: a confirmed spouse edge proposes the other party's
// address to the side that lacks it.
func TestGenerateContactAddressSuggestions_SpouseProposesAddress(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
	createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	s := suggestions[0]
	assert.Equal(t, alice.VCardUID, s.ContactVCardUID)
	assert.Equal(t, bob.VCardUID, s.SourceID)
	assert.Equal(t, addressSuggestionSourceRelationship, s.SourceKind)
	assert.Equal(t, "spouse_of", s.RelationType)
	assert.Equal(t, AddressNormalizedKey(clarkSt), s.AddressKey)
	assert.Equal(t, models.FormatAddress(clarkSt), s.Address.Full)
}

// TestGenerateContactAddressSuggestions_ParentChildPinsRelationPerspective
// pins the relation-token perspective: a child receiving a parent's address
// sees the inverse token (child_of), because the edge reads "what the
// recipient is to the source".
func TestGenerateContactAddressSuggestions_ParentChildPinsRelationPerspective(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	parent := createAddressTestContact(t, db, user.ID, "Parent", clarkSt)
	child := createAddressTestContact(t, db, user.ID, "Child")
	createAddressTestEdge(t, db, user.ID, parent.VCardUID, child.VCardUID, "parent_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, child.VCardUID, suggestions[0].ContactVCardUID)
	assert.Equal(t, "child_of", suggestions[0].RelationType, "the child sees the edge from their own perspective")
}

// TestGenerateContactAddressSuggestions_SkipsExistingAddress pins that a
// recipient who already carries the address (by normalized key) gets no
// suggestion for it.
func TestGenerateContactAddressSuggestions_SkipsExistingAddress(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	alice := createAddressTestContact(t, db, user.ID, "Alice", clarkSt)
	bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
	createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

// TestGenerateContactAddressSuggestions_OnlyConfirmedAndNonSecret pins the
// participation rules: suggested and secret edges never drive an address
// suggestion.
func TestGenerateContactAddressSuggestions_OnlyConfirmedAndNonSecret(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")

	// A *suggested* edge must not propose.
	sAlice := createAddressTestContact(t, db, user.ID, "SAlice")
	sBob := createAddressTestContact(t, db, user.ID, "SBob", clarkSt)
	createAddressTestEdge(t, db, user.ID, sAlice.VCardUID, sBob.VCardUID, "spouse_of", models.RelationshipStatusSuggested, models.RelationshipSensitivityNormal)

	// A *secret* confirmed edge must not propose.
	kAlice := createAddressTestContact(t, db, user.ID, "KAlice")
	kBob := createAddressTestContact(t, db, user.ID, "KBob", clarkSt)
	createAddressTestEdge(t, db, user.ID, kAlice.VCardUID, kBob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivitySecret)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

// TestGenerateContactAddressSuggestions_NonResidentialTypesSkipped pins that
// only the four shared-residence types drive suggestions — a sibling or friend
// edge does not imply a shared address.
func TestGenerateContactAddressSuggestions_NonResidentialTypesSkipped(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
	createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "sibling_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

// TestGenerateContactAddressSuggestions_Household pins the household rule:
// a household with an address proposes it to every member who lacks it.
func TestGenerateContactAddressSuggestions_Household(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	carol := createAddressTestContact(t, db, user.ID, "Carol")
	createAddressTestHousehold(t, db, user.ID, "Home", clarkSt, alice, carol)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 2)
	for _, s := range suggestions {
		assert.Equal(t, addressSuggestionSourceHousehold, s.SourceKind)
		assert.Equal(t, "Home", s.SourceName)
		assert.Equal(t, AddressNormalizedKey(clarkSt), s.AddressKey)
	}
}

// TestGenerateContactAddressSuggestions_HouseholdSkipsMembersWhoHaveIt pins
// the household variant of the already-present skip.
func TestGenerateContactAddressSuggestions_HouseholdSkipsMembersWhoHaveIt(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	carol := createAddressTestContact(t, db, user.ID, "Carol", clarkSt)
	createAddressTestHousehold(t, db, user.ID, "Home", clarkSt, alice, carol)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, alice.VCardUID, suggestions[0].ContactVCardUID)
}

// TestGenerateContactAddressSuggestions_DeduplicatesAcrossSources pins that
// the same address reachable from both a relationship and a household yields
// exactly one suggestion, deterministically.
func TestGenerateContactAddressSuggestions_DeduplicatesAcrossSources(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
	createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	createAddressTestHousehold(t, db, user.ID, "Home", clarkSt, alice, bob)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, alice.VCardUID, suggestions[0].ContactVCardUID)
}

// TestGenerateContactAddressSuggestions_DoesNotLeakAcrossUsers pins ownership
// scoping: another user's relationship and household must never produce a
// suggestion for this user's contacts.
func TestGenerateContactAddressSuggestions_DoesNotLeakAcrossUsers(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "suggest")
	other := createAddressTestUser(t, db, "other")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	bob := createAddressTestContact(t, db, other.ID, "Bob", clarkSt)
	createAddressTestEdge(t, db, other.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
	createAddressTestHousehold(t, db, other.ID, "OtherHome", clarkSt, alice)

	suggestions, err := GenerateContactAddressSuggestions(db, user.ID)
	require.NoError(t, err)
	assert.Empty(t, suggestions)
}

// TestApplyContactAddressSuggestion_PreservesAddressType pins the T91 round-trip:
// applying a relationship-sourced address keeps its home/work type (the
// neutral Contexts vocabulary translates back to the flat Type token).
func TestApplyContactAddressSuggestion_PreservesAddressType(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "apply")
	home := models.ContactAddress{Type: "home", Street: "10 Downing St", City: "London", Country: "UK"}
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	bob := createAddressTestContact(t, db, user.ID, "Bob", home)
	createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	contact, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
		ContactVCardUID: alice.VCardUID,
		SourceKind:      addressSuggestionSourceRelationship,
		SourceID:        bob.VCardUID,
		AddressKey:      AddressNormalizedKey(home),
	})
	require.NoError(t, err)
	require.Len(t, contact.Addresses, 1)
	assert.Equal(t, "home", contact.Addresses[0].Type)
}

// TestApplyContactAddressSuggestion_Relationship applies a relationship-sourced
// suggestion end to end: the address is appended to the contact and the save
// survives the T75 merge path (Card-only data preserved).
func TestApplyContactAddressSuggestion_Relationship(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "apply")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
	createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

	contact, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
		ContactVCardUID: alice.VCardUID,
		SourceKind:      addressSuggestionSourceRelationship,
		SourceID:        bob.VCardUID,
		AddressKey:      AddressNormalizedKey(clarkSt),
	})
	require.NoError(t, err)
	require.Len(t, contact.Addresses, 1)
	assert.Equal(t, AddressNormalizedKey(clarkSt), AddressNormalizedKey(contact.Addresses[0]))

	// Persisted, not just in-memory.
	var reloaded models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", alice.VCardUID).First(&reloaded).Error)
	assert.Equal(t, AddressNormalizedKey(clarkSt), AddressNormalizedKey(reloaded.Addresses[0]))
}

// TestApplyContactAddressSuggestion_Household applies a household-sourced
// suggestion end to end.
func TestApplyContactAddressSuggestion_Household(t *testing.T) {
	db := setupAddressSuggestionTestDB(t)
	user := createAddressTestUser(t, db, "apply")
	alice := createAddressTestContact(t, db, user.ID, "Alice")
	household := createAddressTestHousehold(t, db, user.ID, "Home", clarkSt, alice)

	contact, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
		ContactVCardUID: alice.VCardUID,
		SourceKind:      addressSuggestionSourceHousehold,
		SourceID:        household.ID,
		AddressKey:      AddressNormalizedKey(clarkSt),
	})
	require.NoError(t, err)
	require.Len(t, contact.Addresses, 1)
	assert.Equal(t, AddressNormalizedKey(clarkSt), AddressNormalizedKey(contact.Addresses[0]))
}

// TestApplyContactAddressSuggestion_RejectsStaleSuggestion pins the re-derivation
// contract: applying after the relationship is gone, after the address changed,
// or after the contact already has the address is each a checked rejection.
func TestApplyContactAddressSuggestion_RejectsStaleSuggestion(t *testing.T) {
	t.Run("relationship deleted", func(t *testing.T) {
		db := setupAddressSuggestionTestDB(t)
		user := createAddressTestUser(t, db, "apply")
		alice := createAddressTestContact(t, db, user.ID, "Alice")
		bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
		createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
		require.NoError(t, db.Where("user_id = ?", user.ID).Delete(&models.RelationshipEdge{}).Error)

		_, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
			ContactVCardUID: alice.VCardUID, SourceKind: addressSuggestionSourceRelationship,
			SourceID: bob.VCardUID, AddressKey: AddressNormalizedKey(clarkSt),
		})
		require.Error(t, err)
		require.IsType(t, &apperrors.AppError{}, err)
		assert.Equal(t, apperrors.ErrCodeConflict, err.(*apperrors.AppError).Code)
	})
	t.Run("address no longer on source", func(t *testing.T) {
		db := setupAddressSuggestionTestDB(t)
		user := createAddressTestUser(t, db, "apply")
		alice := createAddressTestContact(t, db, user.ID, "Alice")
		bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
		createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)
		require.NoError(t, db.Model(&models.Contact{}).Where("vcard_uid = ?", bob.VCardUID).Update("addresses", "[]").Error)

		_, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
			ContactVCardUID: alice.VCardUID, SourceKind: addressSuggestionSourceRelationship,
			SourceID: bob.VCardUID, AddressKey: AddressNormalizedKey(clarkSt),
		})
		require.Error(t, err)
		require.IsType(t, &apperrors.AppError{}, err)
		assert.Equal(t, apperrors.ErrCodeConflict, err.(*apperrors.AppError).Code)
	})
	t.Run("contact already has it", func(t *testing.T) {
		db := setupAddressSuggestionTestDB(t)
		user := createAddressTestUser(t, db, "apply")
		alice := createAddressTestContact(t, db, user.ID, "Alice", clarkSt)
		bob := createAddressTestContact(t, db, user.ID, "Bob", clarkSt)
		createAddressTestEdge(t, db, user.ID, alice.VCardUID, bob.VCardUID, "spouse_of", models.RelationshipStatusConfirmed, models.RelationshipSensitivityNormal)

		_, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
			ContactVCardUID: alice.VCardUID, SourceKind: addressSuggestionSourceRelationship,
			SourceID: bob.VCardUID, AddressKey: AddressNormalizedKey(clarkSt),
		})
		require.Error(t, err)
		require.IsType(t, &apperrors.AppError{}, err)
		assert.Equal(t, apperrors.ErrCodeConflict, err.(*apperrors.AppError).Code)
	})
	t.Run("household membership ended", func(t *testing.T) {
		db := setupAddressSuggestionTestDB(t)
		user := createAddressTestUser(t, db, "apply")
		alice := createAddressTestContact(t, db, user.ID, "Alice")
		household := createAddressTestHousehold(t, db, user.ID, "Home", clarkSt, alice)
		require.NoError(t, db.Where("household_id = ?", household.ID).Delete(&models.HouseholdMember{}).Error)

		_, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
			ContactVCardUID: alice.VCardUID, SourceKind: addressSuggestionSourceHousehold,
			SourceID: household.ID, AddressKey: AddressNormalizedKey(clarkSt),
		})
		require.Error(t, err)
		require.IsType(t, &apperrors.AppError{}, err)
		assert.Equal(t, apperrors.ErrCodeConflict, err.(*apperrors.AppError).Code)
	})
	t.Run("foreign contact rejected", func(t *testing.T) {
		db := setupAddressSuggestionTestDB(t)
		user := createAddressTestUser(t, db, "apply")
		other := createAddressTestUser(t, db, "other")
		bob := createAddressTestContact(t, db, other.ID, "Bob", clarkSt)
		alice := createAddressTestContact(t, db, user.ID, "Alice")

		_, err := ApplyContactAddressSuggestion(db, user.ID, &models.ApplyContactAddressSuggestionInput{
			ContactVCardUID: bob.VCardUID, SourceKind: addressSuggestionSourceRelationship,
			SourceID: alice.VCardUID, AddressKey: AddressNormalizedKey(clarkSt),
		})
		require.Error(t, err)
		require.IsType(t, &apperrors.AppError{}, err)
		assert.Equal(t, apperrors.ErrCodeNotFound, err.(*apperrors.AppError).Code)
	})
}
