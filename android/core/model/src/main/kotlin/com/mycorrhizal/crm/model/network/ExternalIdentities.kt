package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * The generic ExternalIdentity substrate (T14): "this contact IS this thing in
 * that external system". System-agnostic — integration-specific data lives in
 * [metadata]. Mirrors `frontend/src/api/externalLinks.ts`'s interface. Rows
 * are edge/join-shaped (hard-deleted server-side), so a client re-pulls them
 * wholesale rather than tracking deaths.
 */
@JsonClass(generateAdapter = true)
data class ExternalIdentity(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    val system: String? = null,
    @Json(name = "external_id") val externalId: String = "",
    val url: String? = null,
    val metadata: Map<String, Any?>? = null,
    @Json(name = "sync_status") val syncStatus: String? = null,
    @Json(name = "last_synced_at") val lastSyncedAt: String? = null,
)

/** Known `system` values on the substrate; the Android client renders Immich specially. */
object ExternalSystems {
    const val IMMICH = "immich"
}

/** GET /external-identities — cursor-paginated, full_resync (total kept). */
@JsonClass(generateAdapter = true)
data class ExternalIdentitiesPage(
    @Json(name = "external_identities") val externalIdentities: List<ExternalIdentity> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
)

// ---------------------------------------------------------------------------
// Immich — the first concrete integration on the substrate (T15/T16). All
// calls go through the backend, never straight to Immich (the API key stays
// server-side). Thumbnails/photos are rendered by loading the backend's
// proxied URLs through Coil with the shared auth stack, or fetched as bytes
// for the profile-photo flow.
// ---------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class ImmichPerson(
    val id: String = "",
    val name: String = "",
)

@JsonClass(generateAdapter = true)
data class ImmichAssetSummary(
    val id: String = "",
    @Json(name = "occurred_at") val occurredAt: String? = null,
)

/** GET /immich/contacts/:uid/summary — `{ summary: {...} }`; null when unlinked. */
@JsonClass(generateAdapter = true)
data class ImmichPersonSummary(
    val identity: ExternalIdentity? = null,
    @Json(name = "person_name") val personName: String? = null,
    @Json(name = "photo_count") val photoCount: Int = 0,
    @Json(name = "latest_asset_id") val latestAssetId: String? = null,
    @Json(name = "latest_at") val latestAt: String? = null,
)

/** GET /immich/config — `has_api_key` decides whether the Immich entry points are shown. */
@JsonClass(generateAdapter = true)
data class ImmichConfigResponse(
    @Json(name = "base_url") val baseUrl: String? = null,
    @Json(name = "has_api_key") val hasApiKey: Boolean = false,
    @Json(name = "sync_enabled") val syncEnabled: Boolean = false,
    @Json(name = "last_synced_at") val lastSyncedAt: String? = null,
    @Json(name = "last_sync_status") val lastSyncStatus: String? = null,
    @Json(name = "last_sync_error") val lastSyncError: String? = null,
)

/** PUT /immich/config body — an empty [apiKey] keeps the stored key unchanged (issue #236). */
@JsonClass(generateAdapter = true)
data class ImmichConfigInput(
    @Json(name = "base_url") val baseUrl: String,
    @Json(name = "api_key") val apiKey: String = "",
    @Json(name = "sync_enabled") val syncEnabled: Boolean? = null,
)

/** POST /immich/test-connection — 200 even when [ok] is false (issue #236). */
@JsonClass(generateAdapter = true)
data class ImmichConnectionTestResult(
    val ok: Boolean = false,
    val stage: String? = null,
    val message: String? = null,
)

/** POST /immich/contacts/:uid/link body. */
@JsonClass(generateAdapter = true)
data class ImmichLinkRequest(
    @Json(name = "person_id") val personId: String,
    @Json(name = "person_name") val personName: String = "",
)

@JsonClass(generateAdapter = true)
data class ImmichPeopleResponse(
    val people: List<ImmichPerson> = emptyList(),
)

@JsonClass(generateAdapter = true)
data class ImmichAssetsResponse(
    val assets: List<ImmichAssetSummary> = emptyList(),
)

@JsonClass(generateAdapter = true)
data class ImmichSummaryResponse(
    val summary: ImmichPersonSummary? = null,
)

@JsonClass(generateAdapter = true)
data class CreateExternalIdentityResponse(
    val message: String? = null,
    @Json(name = "external_identity") val externalIdentity: ExternalIdentity? = null,
)
