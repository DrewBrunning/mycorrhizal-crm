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
object RelationshipType {
    const val PARENT_OF = "parent_of"
    const val CHILD_OF = "child_of"
    const val SPOUSE_OF = "spouse_of"
    const val SIBLING_OF = "sibling_of"
    const val FRIEND_OF = "friend_of"
    const val ROOMMATE_OF = "roommate_of"
    const val PARTNER_OF = "partner_of"
    const val CO_PARENT_OF = "co_parent_of"
    const val MENTOR_OF = "mentor_of"
    const val MENTEE_OF = "mentee_of"
    const val OWNED_BY = "owned_by"
    const val OWNS = "owns"
    const val GETS_ALONG_WITH = "gets_along_with"
    const val CONFLICTS_WITH = "conflicts_with"
    const val RELATED_TO = "related_to"

    val ALL = listOf(
        PARENT_OF, CHILD_OF, SPOUSE_OF, SIBLING_OF, FRIEND_OF,
        ROOMMATE_OF, PARTNER_OF, CO_PARENT_OF, MENTOR_OF, MENTEE_OF,
        OWNED_BY, OWNS, GETS_ALONG_WITH, CONFLICTS_WITH, RELATED_TO,
    )
}

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

object CadenceQualifyingType {
    const val CALL = "call"
    const val VIDEO_CALL = "video_call"
    const val VISIT = "visit"
    const val MEAL = "meal"
    const val GIFT = "gift"
    const val MESSAGE = "message"
    const val SHARED_ACTIVITY = "shared_activity"

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
