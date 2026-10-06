package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Issue #160 / ADR 0033: GeoPulse location-history correlation. Mirrors
 * `frontend/src/api/geopulse.ts` and the `GeoPulse*` schemas in
 * `backend/openapi.yaml`. The connection is per-user-global; the API token stays
 * server-side and is never returned.
 */

/** GET/PUT /geopulse/config — `has_api_key` gates the "Log from location history" entry points. */
@JsonClass(generateAdapter = true)
data class GeoPulseConfigResponse(
    @Json(name = "base_url") val baseUrl: String? = null,
    @Json(name = "has_api_key") val hasApiKey: Boolean = false,
)

/**
 * PUT /geopulse/config body — an empty [apiKey] keeps the stored token, except on first connect
 * and when the base URL's origin changes (the server answers 400 on `api_key` then).
 */
@JsonClass(generateAdapter = true)
data class GeoPulseConfigInput(
    @Json(name = "base_url") val baseUrl: String,
    @Json(name = "api_key") val apiKey: String = "",
)

/** POST /geopulse/test-connection — 200 even when [ok] is false; [stage] is reachability|auth|ok. */
@JsonClass(generateAdapter = true)
data class GeoPulseConnectionTestResult(
    val ok: Boolean = false,
    val stage: String? = null,
    val message: String? = null,
)

/** Display-only: never persisted on the Activity. */
@JsonClass(generateAdapter = true)
data class GeoPulsePhotoSuggestion(
    val id: String = "",
    @Json(name = "file_name") val fileName: String = "",
    @Json(name = "taken_at") val takenAt: String? = null,
)

/** One ephemeral stay suggestion; confirming it is an ordinary POST /activities carrying [externalRef]. */
@JsonClass(generateAdapter = true)
data class GeoPulseStaySuggestion(
    @Json(name = "stay_id") val stayId: Long = 0,
    /** `geopulse:stay:<id>` — passed through as the Activity's external_ref. */
    @Json(name = "external_ref") val externalRef: String = "",
    val location: String = "",
    val city: String = "",
    val country: String = "",
    val latitude: Double = 0.0,
    val longitude: Double = 0.0,
    /** RFC 3339 instant the stay began. */
    val timestamp: String = "",
    @Json(name = "duration_seconds") val durationSeconds: Long = 0,
    val photos: List<GeoPulsePhotoSuggestion> = emptyList(),
    /** True when the photo lookup failed/was skipped: empty [photos] then means "could not check". */
    @Json(name = "photos_unavailable") val photosUnavailable: Boolean = false,
    /** Present when an Activity already carries this stay's external_ref. */
    @Json(name = "existing_activity_id") val existingActivityId: Int? = null,
)

/** GET /geopulse/suggestions?date=&timezone= */
@JsonClass(generateAdapter = true)
data class GeoPulseSuggestionsResponse(
    val date: String = "",
    val suggestions: List<GeoPulseStaySuggestion> = emptyList(),
)
