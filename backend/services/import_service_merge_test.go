package services

import (
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
)

// Tier 3c item 11c : MergeImportedContact's
// "incoming wins when non-empty, existing survives when incoming blank" policy had only ever
// been asserted for a single scalar field (Phone, in TestParseVCF_DuplicateDetectionAndMerge).
// These tests pin the same policy down for every array field (Emails/Phones/Addresses/URLs/
// IMPPs/Circles) and for the "existing survives" direction, which was never asserted at all.
//
// T49 replaced
// the array fields' "replace whenever incoming has any entries" semantics with an additive
// merge: existing entries always survive, and only genuinely new (non-blank, not-already-
// present) incoming entries get appended. TestMergeImportedContact_ArrayFieldsOverwriteWhenIncomingNonEmpty
// below is updated accordingly -- it used to assert the old, buggy "incoming replaces existing"
// behavior as if it were the intended policy; that assertion is exactly what this ticket fixes.
// Circles is unaffected: it isn't a vCard multi-valued field in this sense (ParseCircles already
// filters blanks before Circles is ever populated) and T49 did not change its replace semantics.

// TestMergeImportedContact_ArrayFieldsAdditivelyMerge proves the additive-merge policy (T49)
// holds for every multi-valued field: existing entries survive, and a genuinely new incoming
// entry (different value, not blank) is appended alongside them -- not "incoming replaces
// existing" as this test used to assert before T49.
func TestMergeImportedContact_ArrayFieldsAdditivelyMerge(t *testing.T) {
	existing := &models.Contact{
		Emails:    []models.ContactEmail{{Type: "old", Value: "old@example.com"}},
		Phones:    []models.ContactPhone{{Type: "old", Value: "555-0000"}},
		Addresses: []models.ContactAddress{{Type: "old", Street: "1 Old St"}},
		URLs:      []models.ContactURL{{Type: "old", Value: "https://old.example.com"}},
		IMPPs:     []models.ContactIMPP{{Type: "old", Value: "old-impp"}},
		Circles:   []string{"OldCircle"},
	}
	incoming := &models.Contact{
		Emails:    []models.ContactEmail{{Type: "new", Value: "new@example.com"}},
		Phones:    []models.ContactPhone{{Type: "new", Value: "555-1111"}},
		Addresses: []models.ContactAddress{{Type: "new", Street: "2 New Ave"}},
		URLs:      []models.ContactURL{{Type: "new", Value: "https://new.example.com"}},
		IMPPs:     []models.ContactIMPP{{Type: "new", Value: "new-impp"}},
		Circles:   []string{"NewCircle"},
	}

	MergeImportedContact(existing, incoming)

	// Both the existing and the new entry must be present -- "merge" means
	// "combine", not "last import wins" (T49).
	assert.Equal(t, []models.ContactEmail{
		{Type: "old", Value: "old@example.com"},
		{Type: "new", Value: "new@example.com"},
	}, existing.Emails)
	assert.Equal(t, []models.ContactPhone{
		{Type: "old", Value: "555-0000"},
		{Type: "new", Value: "555-1111"},
	}, existing.Phones)
	assert.Equal(t, []models.ContactAddress{
		{Type: "old", Street: "1 Old St"},
		{Type: "new", Street: "2 New Ave"},
	}, existing.Addresses)
	assert.Equal(t, []models.ContactURL{
		{Type: "old", Value: "https://old.example.com"},
		{Type: "new", Value: "https://new.example.com"},
	}, existing.URLs)
	assert.Equal(t, []models.ContactIMPP{
		{Type: "old", Value: "old-impp"},
		{Type: "new", Value: "new-impp"},
	}, existing.IMPPs)
	// Circles is untouched by T49 -- still a full replace.
	assert.Equal(t, incoming.Circles, existing.Circles)
}

// TestMergeImportedContact_ArrayFieldsDontDuplicateExistingValue proves the additive merge
// dedupes: an incoming entry that repeats a value the contact already has (as a re-import of
// the same vCard would produce) must not create a second copy.
func TestMergeImportedContact_ArrayFieldsDontDuplicateExistingValue(t *testing.T) {
	existing := &models.Contact{
		Emails: []models.ContactEmail{{Type: "home", Value: "same@example.com"}},
		Phones: []models.ContactPhone{{Type: "home", Value: "555-0000"}},
	}
	incoming := &models.Contact{
		Emails: []models.ContactEmail{{Type: "work", Value: "same@example.com"}}, // same value, different label
		Phones: []models.ContactPhone{{Type: "home", Value: "555-0000"}},         // exact repeat
	}

	MergeImportedContact(existing, incoming)

	assert.Equal(t, []models.ContactEmail{{Type: "home", Value: "same@example.com"}}, existing.Emails)
	assert.Equal(t, []models.ContactPhone{{Type: "home", Value: "555-0000"}}, existing.Phones)
}

// TestMergeImportedContact_ArrayFieldsIgnoreBlankIncomingEntries proves the specific bug T49
// reproduced against a real migrated DB: an incoming array with a single blank-valued entry
// (T50's vCard 2.1 parsing gap is a real source of these) must not wipe out the existing,
// populated array just because len(incoming) > 0.
func TestMergeImportedContact_ArrayFieldsIgnoreBlankIncomingEntries(t *testing.T) {
	existing := &models.Contact{
		Emails: []models.ContactEmail{{Type: "home", Value: "keep@example.com"}},
		Phones: []models.ContactPhone{{Type: "home", Value: "555-0000"}},
	}
	incoming := &models.Contact{
		Emails: []models.ContactEmail{{Type: "home", Value: ""}}, // len() > 0 but no content
		Phones: []models.ContactPhone{{Type: "home", Value: ""}},
	}

	MergeImportedContact(existing, incoming)

	assert.Equal(t, []models.ContactEmail{{Type: "home", Value: "keep@example.com"}}, existing.Emails)
	assert.Equal(t, []models.ContactPhone{{Type: "home", Value: "555-0000"}}, existing.Phones)
}

// TestMergeImportedContact_SubStreetFieldsDistinguishAddresses pins the T79
// consequence for import merges: the address identity key (contactAddressKey,
// a lowercased FormatAddress) now carries the PO box / apartment / floor
// parts, so a re-import of a contact whose flat addresses gained those parts
// must keep two addresses that differ only in apartment apart — collapsing
// them would be real data loss the projection can now prevent. An exact
// repeat (the T49 no-duplicate rule) still dedups.
func TestMergeImportedContact_SubStreetFieldsDistinguishAddresses(t *testing.T) {
	existing := &models.Contact{
		Addresses: []models.ContactAddress{{Type: "home", Street: "123 Main St", Apartment: "Apt 3B", City: "Springfield"}},
	}
	incoming := &models.Contact{
		Addresses: []models.ContactAddress{
			{Type: "home", Street: "123 Main St", Apartment: "Apt 3B", City: "Springfield"}, // exact repeat -> dedup
			{Type: "home", Street: "123 Main St", Apartment: "Apt 4B", City: "Springfield"}, // same building, different unit -> distinct
		},
	}

	MergeImportedContact(existing, incoming)

	if len(existing.Addresses) != 2 {
		t.Fatalf("Addresses = %+v, want the repeat deduped and the different-apartment address appended", existing.Addresses)
	}
	if existing.Addresses[1].Apartment != "Apt 4B" {
		t.Errorf("Addresses = %+v, want Apt 4B appended (a different unit is a different address)", existing.Addresses)
	}
}

// TestMergeImportedContact_NeverReassignsVCardUID pins the other half of T49's reproduction:
// an existing contact's VCardUID (its identity, keyed on by every graph-adjacent table's
// entity_id) must survive a merge even when the incoming side carries its own (freshly minted,
// per ParseVCF) UID.
func TestMergeImportedContact_NeverReassignsVCardUID(t *testing.T) {
	existing := &models.Contact{VCardUID: "original-stable-uid"}
	incoming := &models.Contact{VCardUID: "freshly-generated-uuid"}

	MergeImportedContact(existing, incoming)

	assert.Equal(t, "original-stable-uid", existing.VCardUID)
}

// TestMergeImportedContact_ExistingSurvivesWhenIncomingBlank proves the other, previously
// wholly-untested half of the policy: an incoming contact with no data for a field (empty
// scalar, nil/empty array) must never blank out the existing contact's value for that field.
func TestMergeImportedContact_ExistingSurvivesWhenIncomingBlank(t *testing.T) {
	existing := &models.Contact{
		Firstname: "Jane",
		Email:     "jane@example.com",
		Phone:     "555-0000",
		Emails:    []models.ContactEmail{{Type: "home", Value: "jane@example.com"}},
		Phones:    []models.ContactPhone{{Type: "home", Value: "555-0000"}},
		Addresses: []models.ContactAddress{{Type: "home", Street: "1 Existing St"}},
		URLs:      []models.ContactURL{{Type: "home", Value: "https://existing.example.com"}},
		IMPPs:     []models.ContactIMPP{{Type: "home", Value: "existing-impp"}},
		Circles:   []string{"Family"},
	}
	// A zero-value incoming Contact: every field blank/nil, as a minimal or
	// partially-parsed import row would produce.
	incoming := &models.Contact{}

	MergeImportedContact(existing, incoming)

	assert.Equal(t, "Jane", existing.Firstname)
	assert.Equal(t, "jane@example.com", existing.Email)
	assert.Equal(t, "555-0000", existing.Phone)
	assert.Equal(t, []models.ContactEmail{{Type: "home", Value: "jane@example.com"}}, existing.Emails)
	assert.Equal(t, []models.ContactPhone{{Type: "home", Value: "555-0000"}}, existing.Phones)
	assert.Equal(t, []models.ContactAddress{{Type: "home", Street: "1 Existing St"}}, existing.Addresses)
	assert.Equal(t, []models.ContactURL{{Type: "home", Value: "https://existing.example.com"}}, existing.URLs)
	assert.Equal(t, []models.ContactIMPP{{Type: "home", Value: "existing-impp"}}, existing.IMPPs)
	assert.Equal(t, []string{"Family"}, existing.Circles)
}
