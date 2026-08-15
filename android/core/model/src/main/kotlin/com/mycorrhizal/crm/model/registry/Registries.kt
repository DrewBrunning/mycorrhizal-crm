package com.mycorrhizal.crm.model.registry

/**
 * Android mirrors of the backend's `oneof` validator token sets.
 *
 * These are hand-maintained Kotlin copies of the values in
 * backend/models/relationship_type_registry.go and the frontend's
 * TypeScript type catalogs. There is no dynamic type-list endpoint anywhere
 * in this codebase, by design — when a token is added backend-side, this
 * object must be updated by hand (see /CLAUDE.md frontend trap 4).
 */
// The relationship-type mirror that used to live here was a second, unused
// copy of network/RelationshipEdge.kt's RelationshipEdgeTypes -- the one the
// UI actually reads. Deleted in T105 rather than updated: two hand-synced
// mirrors of the same registry, one of them with no consumer to notice when
// it goes stale, is the drift hazard trap 4 exists to warn about. Add new
// relationship tokens to RelationshipEdgeTypes.

object LifeEventCategory {
    const val HOME_LIVING = "home_living"
    const val HEALTH_WELLNESS = "health_wellness"
    const val WORK_EDUCATION = "work_education"
    const val TRAVEL_EXPERIENCES = "travel_experiences"
    const val FAMILY_RELATIONSHIPS = "family_relationships"

    val ALL = listOf(
        HOME_LIVING, HEALTH_WELLNESS, WORK_EDUCATION, TRAVEL_EXPERIENCES, FAMILY_RELATIONSHIPS,
    )
}

/**
 * The category-scoped life-event type tokens (M18), mirroring the backend's
 * `life_event_type_registry.go` and web's `LIFE_EVENT_TYPES_BY_CATEGORY` by
 * hand (`/CLAUDE.md` frontend trap #4). The type set is open — a custom typed
 * value is always legal — but these are the suggestions offered per category.
 */
object LifeEventTypes {
    val BY_CATEGORY: Map<String, List<String>> = mapOf(
        LifeEventCategory.HOME_LIVING to listOf(
            "moved", "bought_a_home", "made_a_home_improvement", "went_on_holidays",
            "got_a_new_vehicle", "got_a_roommate",
        ),
        LifeEventCategory.HEALTH_WELLNESS to listOf(
            "overcame_an_illness", "quit_a_habit", "started_new_eating_habits", "lost_weight",
            "started_wearing_glasses_or_contacts", "broke_a_bone", "removed_braces", "had_surgery",
            "went_to_the_dentist",
        ),
        LifeEventCategory.WORK_EDUCATION to listOf(
            "job_change", "retired", "started_school", "studied_abroad", "started_volunteering",
            "published_a_paper", "started_military_service", "graduated",
        ),
        LifeEventCategory.TRAVEL_EXPERIENCES to listOf(
            "started_a_sport", "started_a_hobby", "learned_a_new_instrument", "learned_a_new_language",
            "got_a_tattoo_or_piercing", "got_a_license", "traveled", "got_an_achievement_or_award",
            "changed_beliefs", "spoke_for_the_first_time", "kissed_for_the_first_time",
        ),
        LifeEventCategory.FAMILY_RELATIONSHIPS to listOf(
            "started_a_relationship", "got_engaged", "married", "anniversary", "expects_a_baby",
            "had_child", "added_a_family_member", "adopted_pet", "ended_a_relationship", "lost_a_loved_one",
        ),
    )

    fun forCategory(category: String): List<String> = BY_CATEGORY[category].orEmpty()
}

object GiftStatus {
    const val IDEA = "idea"
    const val PURCHASED = "purchased"
    const val GIVEN = "given"
    const val RECEIVED = "received"

    val ALL = listOf(IDEA, PURCHASED, GIVEN, RECEIVED)
}

object PreferenceCategory {
    const val FOOD = "food"
    const val DRINK = "drink"
    const val MEDIA = "media"
    const val CLOTHING_SIZE = "clothing_size"

    val ALL = listOf(FOOD, DRINK, MEDIA, CLOTHING_SIZE)
}

object Sensitivity {
    const val NORMAL = "normal"
    const val PRIVATE = "private"
    const val SECRET = "secret"

    val ALL = listOf(NORMAL, PRIVATE, SECRET)
}

object HouseholdType {
    const val FAMILY_UNIT = "family_unit"
    const val ROOMMATES = "roommates"
    const val OTHER = "other"

    val ALL = listOf(FAMILY_UNIT, ROOMMATES, OTHER)
}

/**
 * Interaction types that can reset a cadence policy's clock, mirroring web's
 * `QUALIFYING_INTERACTION_TYPE_TOKENS` and the backend's token set.
 *
 * [photo] is deliberately absent — it is the one globally non-qualifying
 * activity type (`models/activity.go`'s `Activity.Qualifying()`), so offering
 * it as a checkbox would be a dead option. If the backend adds a qualifying
 * token, update this list AND the cadence screen's `cadenceTypeLabel` map by
 * hand (see /CLAUDE.md frontend trap 4).
 */
object CadenceQualifyingType {
    const val CALL = "call"
    const val VIDEO_CALL = "video_call"
    const val VISIT = "visit"
    const val MEAL = "meal"
    const val GIFT = "gift"
    const val MESSAGE = "message"
    const val SHARED_ACTIVITY = "shared_activity"
    const val PHOTO = "photo"

    val ALL = listOf(CALL, VIDEO_CALL, VISIT, MEAL, GIFT, MESSAGE, SHARED_ACTIVITY)
}

object ContactKind {
    const val INDIVIDUAL = "individual"
    const val GROUP = "group"
    const val ORG = "org"
    const val LOCATION = "location"
    const val APPLICATION = "application"
    const val DEVICE = "device"

    val ALL = listOf(INDIVIDUAL, GROUP, ORG, LOCATION, APPLICATION, DEVICE)
}

object PhoneFeature {
    const val VOICE = "voice"
    const val FAX = "fax"
    const val CELL = "cell"
    const val VIDEO = "video"
    const val PAGER = "pager"
    const val TEXT = "text"
    const val TEXTPHONE = "textphone"
    const val MAIN_NUMBER = "main-number"

    val ALL = listOf(VOICE, FAX, CELL, VIDEO, PAGER, TEXT, TEXTPHONE, MAIN_NUMBER)
}
