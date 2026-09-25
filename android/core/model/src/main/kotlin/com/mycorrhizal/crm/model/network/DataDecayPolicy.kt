package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Server-derived data-decay readout (issue #352). Never recomputed on the
 * client. Unlike [CadenceHealth], there is no undefined state: every policy
 * has a baseline (`created_at` when never verified), so `next_due` is always
 * present.
 */
@JsonClass(generateAdapter = true)
data class DataDecayHealth(
    @Json(name = "next_due") val nextDue: String = "",
    @Json(name = "overdue_by") val overdueBy: Int = 0,
) {
    /** Mirrors web's `isOverdue = health.overdue_by > 0` — due today is NOT overdue. */
    val isOverdue: Boolean
        get() = overdueBy > 0
}

/**
 * An opt-in, per-contact rule to periodically re-verify stored info is still
 * accurate (issue #352, docs/adrs/0026-data-decay.md), plus its derived
 * [DataDecayHealth]. Mirrors the backend's `DataDecayPolicyWithHealth`
 * composite.
 */
@JsonClass(generateAdapter = true)
data class DataDecayPolicy(
    val id: String = "",
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
    @Json(name = "entity_id") val entityId: String = "",
    @Json(name = "interval_days") val intervalDays: Int = 0,
    @Json(name = "last_verified_at") val lastVerifiedAt: String? = null,
    val active: Boolean = true,
    val health: DataDecayHealth? = null,
)
