package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// expectedLifeEventTypeCount is 7 pre-existing constants + 37 new ones added
// by T36, the exact
// count the ticket itself states.
const expectedLifeEventTypeCount = 44

func TestLifeEventCategories(t *testing.T) {
	t.Parallel()
	cats := LifeEventCategories()
	assert.Equal(t, []string{
		LifeEventCategoryHomeLiving,
		LifeEventCategoryHealthWellness,
		LifeEventCategoryWorkEducation,
		LifeEventCategoryTravelExperiences,
		LifeEventCategoryFamilyRelationships,
	}, cats, "category order must match the ticket's authoritative list")
}

func TestLifeEventCategoriesReturnsACopy(t *testing.T) {
	t.Parallel()
	cats := LifeEventCategories()
	cats[0] = "mutated"
	assert.Equal(t, LifeEventCategoryHomeLiving, LifeEventCategories()[0],
		"mutating the returned slice must not corrupt the package-level order")
}

func TestIsKnownLifeEventCategory(t *testing.T) {
	t.Parallel()
	assert.True(t, IsKnownLifeEventCategory("home_living"))
	assert.True(t, IsKnownLifeEventCategory("family_relationships"))
	assert.False(t, IsKnownLifeEventCategory("not_a_real_category"))
	assert.False(t, IsKnownLifeEventCategory(""))
}

// TestLifeEventTypeCategoriesCoversEveryConstant pins the registry against
// silent drift: every LifeEventType* constant referenced anywhere in this
// package (the 7 pre-existing plus T36's 37 new ones) must have exactly one
// entry in LifeEventTypeCategories, and the map must have no stray entries
// beyond those 44.
func TestLifeEventTypeCategoriesCoversEveryConstant(t *testing.T) {
	t.Parallel()
	allTypes := []string{
		// pre-existing
		LifeEventTypeMarried, LifeEventTypeGraduated, LifeEventTypeJobChange,
		LifeEventTypeHadChild, LifeEventTypeAdoptedPet, LifeEventTypeRetired, LifeEventTypeMoved,
		// Home & Living
		LifeEventTypeBoughtAHome, LifeEventTypeMadeAHomeImprovement, LifeEventTypeWentOnHolidays,
		LifeEventTypeGotANewVehicle, LifeEventTypeGotARoommate,
		// Health & Wellness
		LifeEventTypeOvercameAnIllness, LifeEventTypeQuitAHabit, LifeEventTypeStartedNewEatingHabits,
		LifeEventTypeLostWeight, LifeEventTypeStartedWearingGlassesOrContacts, LifeEventTypeBrokeABone,
		LifeEventTypeRemovedBraces, LifeEventTypeHadSurgery, LifeEventTypeWentToTheDentist,
		// Work & Education
		LifeEventTypeStartedSchool, LifeEventTypeStudiedAbroad, LifeEventTypeStartedVolunteering,
		LifeEventTypePublishedAPaper, LifeEventTypeStartedMilitaryService,
		// Travel & Experiences
		LifeEventTypeStartedASport, LifeEventTypeStartedAHobby, LifeEventTypeLearnedANewInstrument,
		LifeEventTypeLearnedANewLanguage, LifeEventTypeGotATattooOrPiercing, LifeEventTypeGotALicense,
		LifeEventTypeTraveled, LifeEventTypeGotAnAchievementOrAward, LifeEventTypeChangedBeliefs,
		LifeEventTypeSpokeForTheFirstTime, LifeEventTypeKissedForTheFirstTime,
		// Family & Relationships
		LifeEventTypeStartedARelationship, LifeEventTypeGotEngaged, LifeEventTypeAnniversary,
		LifeEventTypeExpectsABaby, LifeEventTypeAddedAFamilyMember, LifeEventTypeEndedARelationship,
		LifeEventTypeLostALovedOne,
	}

	assert.Len(t, allTypes, expectedLifeEventTypeCount, "test fixture itself must list all 44 tokens")
	assert.Len(t, LifeEventTypeCategories, expectedLifeEventTypeCount,
		"the registry must have exactly 44 entries, no more, no fewer")

	seen := make(map[string]bool, len(allTypes))
	for _, token := range allTypes {
		seen[token] = true
		category, ok := LifeEventTypeCategories[token]
		assert.True(t, ok, "token %q is missing from LifeEventTypeCategories", token)
		assert.True(t, IsKnownLifeEventCategory(category), "token %q maps to unregistered category %q", token, category)
	}

	// No entries in the map beyond the 44 accounted for above.
	for token := range LifeEventTypeCategories {
		assert.True(t, seen[token], "LifeEventTypeCategories has an entry %q not covered by this test's fixture", token)
	}
}

// TestLifeEventTypesForCategoryRoundTripsWithTypeCategories asserts the two
// registry views agree: every token LifeEventTypesForCategory(c) returns
// must map back to c via LifeEventTypeCategories, and vice versa — the
// ordered-by-category view (used for backfill/tests) and the flat map (used
// for validation) must never drift apart.
func TestLifeEventTypesForCategoryRoundTripsWithTypeCategories(t *testing.T) {
	t.Parallel()
	for _, category := range LifeEventCategories() {
		types := LifeEventTypesForCategory(category)
		assert.NotEmpty(t, types, "category %q must have at least one type", category)
		for _, token := range types {
			assert.Equal(t, category, LifeEventTypeCategories[token],
				"LifeEventTypesForCategory(%q) returned %q, but LifeEventTypeCategories maps it to %q",
				category, token, LifeEventTypeCategories[token])
		}
	}

	for token, category := range LifeEventTypeCategories {
		assert.Contains(t, LifeEventTypesForCategory(category), token,
			"LifeEventTypeCategories maps %q to %q, but LifeEventTypesForCategory(%q) doesn't list it back", token, category, category)
	}
}

func TestLifeEventTypesForCategoryUnknownReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, LifeEventTypesForCategory("not_a_real_category"))
}

func TestLifeEventCategoryForType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, LifeEventCategoryFamilyRelationships, LifeEventCategoryForType(LifeEventTypeMarried))
	assert.Equal(t, LifeEventCategoryHomeLiving, LifeEventCategoryForType(LifeEventTypeMoved))
	assert.Equal(t, "", LifeEventCategoryForType("a_custom_user_typed_event"),
		"a custom/unregistered type must return the empty category, not guess")
}

func TestKnownLifeEventTypes(t *testing.T) {
	t.Parallel()
	tokens := KnownLifeEventTypes()
	assert.Len(t, tokens, expectedLifeEventTypeCount)
	assert.Contains(t, tokens, LifeEventTypeMarried)
	assert.Contains(t, tokens, LifeEventTypeBoughtAHome)
	// Sorted, so callers get deterministic output.
	for i := 1; i < len(tokens); i++ {
		assert.LessOrEqual(t, tokens[i-1], tokens[i], "KnownLifeEventTypes must be sorted")
	}
}
