package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Server-derived relationship-health readout (T19). Never recomputed on the
 * client — the UI must show the server's verdict verbatim (the M12 test cases
 * pin this). `last_interaction`/`next_due` are RFC 3339 date-times, absent
 * (not null) when undefined; `has_qualifying_interaction` and `overdue_by`
 * are always present.
 *
 * Field-for-field identical to [BriefingCadenceHealth] (M11's prep-view
 * projection of the same backend object) — if one grows a new backend field,
 * update both; they must stay in sync.
 */
@JsonClass(generateAdapter = true)
data class CadenceHealth(
    @Json(name = "has_qualifying_interaction") val hasQualifyingInteraction: Boolean = false,
    @Json(name = "last_interaction") val lastInteraction: String? = null,
    @Json(name = "next_due") val nextDue: String? = null,
    @Json(name = "overdue_by") val overdueBy: Int = 0,
) {
    /** Mirrors web's `isOverdue = health.overdue_by > 0` — due today is NOT overdue. */
    val isOverdue: Boolean
        get() = overdueBy > 0
}

/**
 * A cadence policy plus its derived [CadenceHealth]. Mirrors the backend's
 * `CadencePolicyWithHealth` composite. `qualifying_types` empty (or absent)
 * means "every default-qualifying interaction type counts" — it must never be
 * defaulted to a populated list on round-trip.
 */
@JsonClass(generateAdapter = true)
data class CadencePolicy(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    @Json(name = "target_interval_days") val targetIntervalDays: Int = 0,
    @Json(name = "qualifying_types") val qualifyingTypes: List<String> = emptyList(),
    /** Change-feed tombstone marker (T17); only present via `?since=`. */
    val deleted: Boolean = false,
    val health: CadenceHealth? = null,
)

/** POST /cadence-policies request body. `entity_id` is the Contact.VCardUID. */
@JsonClass(generateAdapter = true)
data class CadencePolicyInput(
    @Json(name = "entity_id") val entityId: String,
    @Json(name = "target_interval_days") val targetIntervalDays: Int,
    @Json(name = "qualifying_types") val qualifyingTypes: List<String> = emptyList(),
)

/** GET /cadence-policies — cursor-paginated `{ cadence_policies, total, next_cursor, limit }`. */
@JsonClass(generateAdapter = true)
data class CadencePoliciesResponse(
    @Json(name = "cadence_policies") val cadencePolicies: List<CadencePolicy> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
)

/**
 * POST /cadence-policies response — wrapped `{ cadence_policy }`. Unlike PUT,
 * which returns the policy raw (the frontend api module documents the
 * asymmetry deliberately).
 */
@JsonClass(generateAdapter = true)
data class CreateCadencePolicyResponse(
    @Json(name = "cadence_policy") val cadencePolicy: CadencePolicy? = null,
)
