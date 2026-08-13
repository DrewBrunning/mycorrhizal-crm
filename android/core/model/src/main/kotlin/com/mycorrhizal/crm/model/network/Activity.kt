package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * An activity/Interaction — the backend's `models.Activity` (gorm.Model
 * identity keys serialize in PascalCase). `type` is a conventional, open
 * classifier (call, visit, meal, message, gift, photo, ...) — not a closed
 * enum — so it is a plain String. [qualifying] mirrors the backend's
 * Activity.Qualifying() (everything counts toward cadence except `photo`).
 */
@JsonClass(generateAdapter = true)
data class Activity(
    @Json(name = "ID") val id: Int = 0,
    @Json(name = "CreatedAt") val createdAt: String? = null,
    @Json(name = "UpdatedAt") val updatedAt: String? = null,
    val uuid: String? = null,
    val title: String? = null,
    val description: String? = null,
    val location: String? = null,
    val date: String? = null,
    val contacts: List<ContactFlat>? = null,
    val type: String? = null,
    @Json(name = "external_ref") val externalRef: String? = null,
    /** T17 change-feed tombstone marker. */
    val deleted: Boolean = false,
) {
    val qualifying: Boolean get() = type != "photo"
}

/** Request body for POST /activities and PUT /activities/{id}. */
@JsonClass(generateAdapter = true)
data class ActivityInput(
    val title: String? = null,
    val description: String? = null,
    val location: String? = null,
    val date: String? = null,
    @Json(name = "contact_ids") val contactIds: List<Int>? = null,
    val type: String? = null,
    @Json(name = "external_ref") val externalRef: String? = null,
)

/** POST /activities response — wrapped `{ message, activity }` (200, not 201). */
@JsonClass(generateAdapter = true)
data class CreateActivityResponse(
    val message: String? = null,
    val activity: Activity? = null,
)

/** GET /contacts/{id}/activities — bare array under `activities`. */
@JsonClass(generateAdapter = true)
data class ContactActivitiesResponse(
    val activities: List<Activity> = emptyList(),
)

/**
 * GET /activities — T17 cursor-paginated page. `GetActivities` does
 * `var activities []models.Activity; ...Find(&activities)`, and Go marshals a nil slice (zero
 * activities) as JSON `null`, not `[]` (`/CLAUDE.md` frontend trap #8). A non-null Kotlin default
 * only covers an *absent* key, not an explicit `null` value — Moshi codegen still rejects the
 * latter — so the raw field stays nullable and [activities] normalizes absent/null/`[]` to a
 * plain empty list, mirroring `FieldDefinitionsResponse.definitions`' fix for the same trap.
 * `listActivities()` had zero UI callers before M9's Activities inbox, so this was a live bug
 * nothing had exercised yet.
 */
@JsonClass(generateAdapter = true)
data class ActivitiesPage(
    @Json(name = "activities") val activitiesRaw: List<Activity>? = null,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
) {
    val activities: List<Activity> get() = activitiesRaw.orEmpty()
}

/**
 * Raw serialization of models.Contact (legacy flat DTO) — used for an
 * activity's participant list. Identity keys serialize in PascalCase.
 */
@JsonClass(generateAdapter = true)
data class ContactFlat(
    @Json(name = "ID") val id: Int = 0,
    val firstname: String? = null,
    val lastname: String? = null,
    val nickname: String? = null,
    val email: String? = null,
    val phone: String? = null,
    val uid: String? = null,
) {
    val displayName: String
        get() = listOfNotNull(firstname, lastname).joinToString(" ").ifBlank { "#$id" }
}
