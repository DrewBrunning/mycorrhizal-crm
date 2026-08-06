package models

import "sort"

// lifeEventCategoryOrder is the display/definition order for the five T36
// categories, matching the order the user supplied in the ticket (docs/
// fork-plan/tickets/45-T36-life-event-categories.md) — the same order Monica
// settled on through real use. Exported via LifeEventCategories() rather
// than as a public var so callers can't mutate the backing order.
var lifeEventCategoryOrder = []string{
	LifeEventCategoryHomeLiving,
	LifeEventCategoryHealthWellness,
	LifeEventCategoryWorkEducation,
	LifeEventCategoryTravelExperiences,
	LifeEventCategoryFamilyRelationships,
}

// LifeEventTypeCategories is the single source of truth for which category a
// predefined LifeEventType* token belongs to — the same "map lives in one
// place backend-side" role relationTypeRegistry plays for RelationshipEdge
// types (relationship_type_registry.go). Only the 44 predefined tokens are
// present; a custom (user-typed) Type has no entry here and its Category
// comes directly from whichever category's picker the frontend opened it
// from, not from this map.
//
// Order within each category matches the ticket's authoritative list, which
// is itself the order Monica settled on through real use — this map isn't
// ordered (Go maps can't be), but LifeEventTypesByCategory below preserves
// it for anything that needs to render the list in that order.
var LifeEventTypeCategories = map[string]string{
	// Home & Living
	LifeEventTypeMoved:                LifeEventCategoryHomeLiving,
	LifeEventTypeBoughtAHome:          LifeEventCategoryHomeLiving,
	LifeEventTypeMadeAHomeImprovement: LifeEventCategoryHomeLiving,
	LifeEventTypeWentOnHolidays:       LifeEventCategoryHomeLiving,
	LifeEventTypeGotANewVehicle:       LifeEventCategoryHomeLiving,
	LifeEventTypeGotARoommate:         LifeEventCategoryHomeLiving,

	// Health & Wellness
	LifeEventTypeOvercameAnIllness:               LifeEventCategoryHealthWellness,
	LifeEventTypeQuitAHabit:                      LifeEventCategoryHealthWellness,
	LifeEventTypeStartedNewEatingHabits:          LifeEventCategoryHealthWellness,
	LifeEventTypeLostWeight:                      LifeEventCategoryHealthWellness,
	LifeEventTypeStartedWearingGlassesOrContacts: LifeEventCategoryHealthWellness,
	LifeEventTypeBrokeABone:                      LifeEventCategoryHealthWellness,
	LifeEventTypeRemovedBraces:                   LifeEventCategoryHealthWellness,
	LifeEventTypeHadSurgery:                      LifeEventCategoryHealthWellness,
	LifeEventTypeWentToTheDentist:                LifeEventCategoryHealthWellness,

	// Work & Education (job_change/retired/graduated are the three
	// pre-existing constants that land here)
	LifeEventTypeJobChange:              LifeEventCategoryWorkEducation,
	LifeEventTypeRetired:                LifeEventCategoryWorkEducation,
	LifeEventTypeStartedSchool:          LifeEventCategoryWorkEducation,
	LifeEventTypeStudiedAbroad:          LifeEventCategoryWorkEducation,
	LifeEventTypeStartedVolunteering:    LifeEventCategoryWorkEducation,
	LifeEventTypePublishedAPaper:        LifeEventCategoryWorkEducation,
	LifeEventTypeStartedMilitaryService: LifeEventCategoryWorkEducation,
	LifeEventTypeGraduated:              LifeEventCategoryWorkEducation,

	// Travel & Experiences
	LifeEventTypeStartedASport:           LifeEventCategoryTravelExperiences,
	LifeEventTypeStartedAHobby:           LifeEventCategoryTravelExperiences,
	LifeEventTypeLearnedANewInstrument:   LifeEventCategoryTravelExperiences,
	LifeEventTypeLearnedANewLanguage:     LifeEventCategoryTravelExperiences,
	LifeEventTypeGotATattooOrPiercing:    LifeEventCategoryTravelExperiences,
	LifeEventTypeGotALicense:             LifeEventCategoryTravelExperiences,
	LifeEventTypeTraveled:                LifeEventCategoryTravelExperiences,
	LifeEventTypeGotAnAchievementOrAward: LifeEventCategoryTravelExperiences,
	LifeEventTypeChangedBeliefs:          LifeEventCategoryTravelExperiences,
	LifeEventTypeSpokeForTheFirstTime:    LifeEventCategoryTravelExperiences,
	LifeEventTypeKissedForTheFirstTime:   LifeEventCategoryTravelExperiences,

	// Family & Relationships
	LifeEventTypeStartedARelationship: LifeEventCategoryFamilyRelationships,
	LifeEventTypeGotEngaged:           LifeEventCategoryFamilyRelationships,
	LifeEventTypeMarried:              LifeEventCategoryFamilyRelationships,
	LifeEventTypeAnniversary:          LifeEventCategoryFamilyRelationships,
	LifeEventTypeExpectsABaby:         LifeEventCategoryFamilyRelationships,
	LifeEventTypeHadChild:             LifeEventCategoryFamilyRelationships,
	LifeEventTypeAddedAFamilyMember:   LifeEventCategoryFamilyRelationships,
	LifeEventTypeAdoptedPet:           LifeEventCategoryFamilyRelationships,
	LifeEventTypeEndedARelationship:   LifeEventCategoryFamilyRelationships,
	LifeEventTypeLostALovedOne:        LifeEventCategoryFamilyRelationships,
}

// lifeEventTypesByCategoryOrder preserves the ticket's authoritative
// within-category ordering, since LifeEventTypeCategories (a map) can't.
// Migration 000011's own backfill is static SQL and does NOT read this —
// it hand-duplicates the same seven-constant mapping directly in the
// UPDATE statements, since a migration file can't call Go code. Only
// LifeEventTypesForCategory and tests use this; the frontend keeps its own
// hand-mirrored copy per CLAUDE.md frontend-trap-4, exactly like every
// other backend `oneof`-shaped registry.
var lifeEventTypesByCategoryOrder = map[string][]string{
	LifeEventCategoryHomeLiving: {
		LifeEventTypeMoved,
		LifeEventTypeBoughtAHome,
		LifeEventTypeMadeAHomeImprovement,
		LifeEventTypeWentOnHolidays,
		LifeEventTypeGotANewVehicle,
		LifeEventTypeGotARoommate,
	},
	LifeEventCategoryHealthWellness: {
		LifeEventTypeOvercameAnIllness,
		LifeEventTypeQuitAHabit,
		LifeEventTypeStartedNewEatingHabits,
		LifeEventTypeLostWeight,
		LifeEventTypeStartedWearingGlassesOrContacts,
		LifeEventTypeBrokeABone,
		LifeEventTypeRemovedBraces,
		LifeEventTypeHadSurgery,
		LifeEventTypeWentToTheDentist,
	},
	LifeEventCategoryWorkEducation: {
		LifeEventTypeJobChange,
		LifeEventTypeRetired,
		LifeEventTypeStartedSchool,
		LifeEventTypeStudiedAbroad,
		LifeEventTypeStartedVolunteering,
		LifeEventTypePublishedAPaper,
		LifeEventTypeStartedMilitaryService,
		LifeEventTypeGraduated,
	},
	LifeEventCategoryTravelExperiences: {
		LifeEventTypeStartedASport,
		LifeEventTypeStartedAHobby,
		LifeEventTypeLearnedANewInstrument,
		LifeEventTypeLearnedANewLanguage,
		LifeEventTypeGotATattooOrPiercing,
		LifeEventTypeGotALicense,
		LifeEventTypeTraveled,
		LifeEventTypeGotAnAchievementOrAward,
		LifeEventTypeChangedBeliefs,
		LifeEventTypeSpokeForTheFirstTime,
		LifeEventTypeKissedForTheFirstTime,
	},
	LifeEventCategoryFamilyRelationships: {
		LifeEventTypeStartedARelationship,
		LifeEventTypeGotEngaged,
		LifeEventTypeMarried,
		LifeEventTypeAnniversary,
		LifeEventTypeExpectsABaby,
		LifeEventTypeHadChild,
		LifeEventTypeAddedAFamilyMember,
		LifeEventTypeAdoptedPet,
		LifeEventTypeEndedARelationship,
		LifeEventTypeLostALovedOne,
	},
}

// LifeEventCategories returns the five T36 category tokens in their
// authoritative display order (ticket order == Monica's real-use order).
func LifeEventCategories() []string {
	out := make([]string, len(lifeEventCategoryOrder))
	copy(out, lifeEventCategoryOrder)
	return out
}

// IsKnownLifeEventCategory reports whether token is one of the five
// registered category tokens. Backs the life_event_category validator tag
// (middleware/validation.go's validateLifeEventCategory) — the same role
// IsKnownRelationType plays for the relation_type tag — so LifeEvent.
// Category/LifeEventInput.Category validation reads this registry instead
// of a second hardcoded token list.
func IsKnownLifeEventCategory(token string) bool {
	for _, c := range lifeEventCategoryOrder {
		if c == token {
			return true
		}
	}
	return false
}

// LifeEventCategoryForType returns the category a predefined LifeEventType*
// token belongs to, or "" if token is unregistered (a custom, user-typed
// event, or free text that predates T36 — see this file's package doc and
// migration 000011's own comment on why those are left uncategorized rather
// than guessed).
func LifeEventCategoryForType(token string) string {
	return LifeEventTypeCategories[token]
}

// LifeEventTypesForCategory returns the predefined type tokens filed under
// category, in the ticket's authoritative order, or nil for an unregistered
// category.
func LifeEventTypesForCategory(category string) []string {
	types := lifeEventTypesByCategoryOrder[category]
	if types == nil {
		return nil
	}
	out := make([]string, len(types))
	copy(out, types)
	return out
}

// KnownLifeEventTypes returns every predefined LifeEventType* token,
// sorted, for tests that need to assert completeness against the registry
// rather than duplicating its contents.
func KnownLifeEventTypes() []string {
	tokens := make([]string, 0, len(LifeEventTypeCategories))
	for token := range LifeEventTypeCategories {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return tokens
}
