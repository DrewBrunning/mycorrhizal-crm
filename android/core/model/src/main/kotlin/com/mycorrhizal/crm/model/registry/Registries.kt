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

/**
 * The section a preference category belongs to — which also decides which
 * screen it surfaces on. Mirrors frontend/src/api/preferences.ts's
 * PreferenceSection: foodDrink/media/hobby are "get to know them" facts
 * (PreferencesScreen); jewelry/giftPreferences/giftAvoid are "check this
 * right before buying" facts (GiftsScreen, alongside clothing sizes).
 */
object PreferenceSection {
    const val FOOD_DRINK = "food_drink"
    const val MEDIA = "media"
    const val HOBBY = "hobby"
    const val JEWELRY = "jewelry"
    const val GIFT_PREFERENCES = "gift_preferences"
    const val GIFT_AVOID = "gift_avoid"
    // Catch-all for a category not in PreferenceCategory.CONFIG (legacy data,
    // or a future addition this build doesn't know about yet) — matches
    // web's PreferenceList "Other" bucket, so no data ever hides.
    const val OTHER = "other"

    /** Sections shown on GiftsScreen, alongside clothing sizes. */
    val GIFTS_TAB: Set<String> = setOf(JEWELRY, GIFT_PREFERENCES, GIFT_AVOID)
    val GIFTS_TAB_ORDERED: List<String> = listOf(JEWELRY, GIFT_PREFERENCES, GIFT_AVOID)

    /** Sections shown on PreferencesScreen. Every real (non-OTHER) section
     * must appear in exactly one of GIFTS_TAB/OVERVIEW_TAB. */
    val OVERVIEW_TAB: Set<String> = setOf(FOOD_DRINK, MEDIA, HOBBY)
    val OVERVIEW_TAB_ORDERED: List<String> = listOf(FOOD_DRINK, MEDIA, HOBBY)
}

enum class PreferenceKeyMode { DISPOSITION, FREE_SOLO }

data class PreferenceCategoryConfig(
    val category: String,
    val section: String,
    val keyMode: PreferenceKeyMode,
    val keySuggestions: List<String>,
)

/**
 * Android mirror of frontend/src/api/preferences.ts's PREFERENCE_CATEGORY_CONFIG
 * (see that file's doc comment for the full rationale, condensed here): a
 * domain that needs both a "what kind of thing" axis and a "how do they feel
 * about it" axis (media, jewelry) pushes the kind into the category (cheap to
 * extend, same convention CLOTHING_SIZE already used) and reserves `key`
 * uniformly for disposition (favorite/like/dislike/allergy, a suggestion list
 * per category, still free text). `dislike` is the one exception: a general,
 * non-domain-specific gift-avoidance note (e.g. "no candles"), so it has no
 * disposition of its own — FREE_SOLO with no suggestions.
 *
 * CLOTHING_SIZE stays outside CONFIG (its key is a free-solo clothing *type*
 * like "shirt"/"ring", not a disposition — sizing is a fact, not a taste).
 */
object PreferenceCategory {
    const val FOOD = "food"
    const val DRINK = "drink"
    const val CLOTHING_SIZE = "clothing_size"

    // Legacy/deprecated — superseded by the media_* categories below. Not in
    // CONFIG (not offered in the dialog); a stray row (pre-backfill, or a
    // migration rollback) falls to PreferenceSection.OTHER, matching web.
    const val MEDIA_LEGACY = "media"

    private val DISPOSITION = listOf("favorite", "like", "dislike")
    private val DISPOSITION_WITH_ALLERGY = listOf("favorite", "like", "dislike", "allergy")

    val CONFIG: List<PreferenceCategoryConfig> = listOf(
        // Food & Drink — presented as one merged section, kept as two
        // categories so food-vs-drink stays separately filterable/exportable.
        PreferenceCategoryConfig(FOOD, PreferenceSection.FOOD_DRINK, PreferenceKeyMode.DISPOSITION, listOf("favorite", "dislike", "allergy")),
        PreferenceCategoryConfig(DRINK, PreferenceSection.FOOD_DRINK, PreferenceKeyMode.DISPOSITION, listOf("favorite", "dislike", "allergy")),

        // Media — medium (and, for music/books, facet) lives in the category.
        PreferenceCategoryConfig("media_movie", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_tv", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_game", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_podcast", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_music_artist", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_music_album", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_music_genre", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_music_song", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_book_author", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_book_series", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("media_book_title", PreferenceSection.MEDIA, PreferenceKeyMode.DISPOSITION, DISPOSITION),

        // Activities & Hobbies — a "get to know them" fact, stays with Preferences.
        PreferenceCategoryConfig("hobby", PreferenceSection.HOBBY, PreferenceKeyMode.DISPOSITION, DISPOSITION),

        // Jewelry & Style — aspect (metal/stone/style/type) lives in the category.
        PreferenceCategoryConfig("jewelry_metal", PreferenceSection.JEWELRY, PreferenceKeyMode.DISPOSITION, DISPOSITION_WITH_ALLERGY),
        PreferenceCategoryConfig("jewelry_stone", PreferenceSection.JEWELRY, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("jewelry_style", PreferenceSection.JEWELRY, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("jewelry_type", PreferenceSection.JEWELRY, PreferenceKeyMode.DISPOSITION, DISPOSITION),

        // Gift Preferences — single-facet "tastes", each its own category chip.
        PreferenceCategoryConfig("flowers", PreferenceSection.GIFT_PREFERENCES, PreferenceKeyMode.DISPOSITION, DISPOSITION_WITH_ALLERGY),
        PreferenceCategoryConfig("color", PreferenceSection.GIFT_PREFERENCES, PreferenceKeyMode.DISPOSITION, DISPOSITION),
        PreferenceCategoryConfig("fragrance", PreferenceSection.GIFT_PREFERENCES, PreferenceKeyMode.DISPOSITION, DISPOSITION_WITH_ALLERGY),
        PreferenceCategoryConfig("cause", PreferenceSection.GIFT_PREFERENCES, PreferenceKeyMode.DISPOSITION, listOf("favorite", "like")),

        // Gift Avoid — general, non-domain-specific avoidance notes.
        PreferenceCategoryConfig("dislike", PreferenceSection.GIFT_AVOID, PreferenceKeyMode.FREE_SOLO, emptyList()),
    )

    val ALL: List<String> = CONFIG.map { it.category }

    private val BY_CATEGORY: Map<String, PreferenceCategoryConfig> = CONFIG.associateBy { it.category }

    fun sectionOf(category: String): String? = BY_CATEGORY[category]?.section
    fun keySuggestionsFor(category: String): List<String> = BY_CATEGORY[category]?.keySuggestions.orEmpty()
    fun keyModeFor(category: String): PreferenceKeyMode = BY_CATEGORY[category]?.keyMode ?: PreferenceKeyMode.DISPOSITION
    fun isGiftsTabCategory(category: String): Boolean = sectionOf(category) in PreferenceSection.GIFTS_TAB

    /** Free-solo clothing-type suggestions for CLOTHING_SIZE's key field — a
     * fact (which garment), not a disposition. */
    val CLOTHING_TYPE_SUGGESTIONS: List<String> = listOf(
        "shirt", "pants", "dress", "skirt", "undergarments", "outerwear",
        "shoe", "hat", "glove", "belt", "ring", "socks",
    )
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
