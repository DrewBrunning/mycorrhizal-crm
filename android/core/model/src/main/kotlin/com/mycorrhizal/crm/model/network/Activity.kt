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
    /**
     * Issue #2006: the optional inclusive last day of a multi-day activity ([date] stays the
     * start). A calendar-day instant at 00:00:00Z, read as its UTC date exactly like [date]
     * (ADR 0015); null/absent for a single-day activity. Mirrors the backend `Activity.EndDate`.
     */
    @Json(name = "end_date") val endDate: String? = null,
    val contacts: List<ContactFlat>? = null,
    val type: String? = null,
    @Json(name = "external_ref") val externalRef: String? = null,
    /** ADR 0006/0008: monotonic per-row write counter; the value an `If-Match` precondition echoes. */
    val revision: Int? = null,
    /** T17 change-feed tombstone marker. */
    val deleted: Boolean = false
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
    /**
     * Issue #2006: always written, `null` included. On a PUT an omitted `end_date`
     * keeps the stored value while `null` clears it, so the editor must send the field every
     * time; on create `null` means no end.
     */
    @Json(name = "end_date") val endDate: String? = null,
    @Json(name = "contact_ids") val contactIds: List<Int>? = null,
    val type: String? = null,
    @Json(name = "external_ref") val externalRef: String? = null
)

/** POST /activities response — wrapped `{ message, activity }` (200, not 201). */
@JsonClass(generateAdapter = true)
data class CreateActivityResponse(val message: String? = null, val activity: Activity? = null)

/** GET /contacts/{id}/activities — bare array under `activities` (M19: T17 cursor envelope). */
@JsonClass(generateAdapter = true)
data class ContactActivitiesResponse(
    val activities: List<Activity> = emptyList(),
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0
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
    val sync: SyncInfo? = null
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
    val uid: String? = null
) {
    val displayName: String
        get() = listOfNotNull(firstname, lastname).joinToString(" ").ifBlank { "#$id" }
}

/** Request body for `POST /contacts/{id}/activities` (issue #2136): 1-100 activity ids. */
@JsonClass(generateAdapter = true)
data class ContactActivitiesRequest(@Json(name = "activity_ids") val activityIds: List<Int>)

/** Per-activity outcome of [ContactActivitiesRequest]; unrecognised wire values are [UNKNOWN]. */
enum class AddToActivityStatus {
    ADDED,
    ALREADY_MEMBER,
    NOT_FOUND,
    UNKNOWN;

    /** `added` / `already_member` both leave the contact on the activity. */
    val isSuccess: Boolean get() = this == ADDED || this == ALREADY_MEMBER

    companion object {
        fun fromWire(value: String): AddToActivityStatus = when (value) {
            "added" -> ADDED
            "already_member" -> ALREADY_MEMBER
            "not_found" -> NOT_FOUND
            else -> UNKNOWN
        }
    }
}

@JsonClass(generateAdapter = true)
data class AddToActivityResult(
    @Json(name = "activity_id") val activityId: Int = 0,
    val status: String = ""
) {
    val outcome: AddToActivityStatus get() = AddToActivityStatus.fromWire(status)
}

@JsonClass(generateAdapter = true)
data class AddContactToActivitiesResponse(
    @Json(name = "contact_id") val contactId: Int = 0,
    val results: List<AddToActivityResult> = emptyList()
)
