package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * The four target-closeness tiers (issue #1798, ADR 0023 amendment).
 *
 * Hand-mirrored from backend `models.TargetClosenessTiers()`, the API's `oneof`
 * and the DB CHECK (CLAUDE.md trap #4: there is no dynamic type-list endpoint;
 * a new backend token must be copied here by hand).
 */
object TargetClosenessTier {
    const val IMMEDIATE = "immediate"
    const val CLOSE = "close"
    const val CASUAL = "casual"
    const val DISTANT = "distant"

    /** Closest first, matching `models.TargetClosenessTiers()`. */
    val ALL = listOf(IMMEDIATE, CLOSE, CASUAL, DISTANT)
}

/** `ContactScoreTarget.status` tokens (backend `scoring` target status). */
object TargetStatus {
    const val ON_TRACK = "on_track"
    const val DRIFTING = "drifting"
    const val BEHIND = "behind"
}

/** Body of `PUT /contacts/{id}/target-closeness` — just the tier token. */
@JsonClass(generateAdapter = true)
data class TargetClosenessInput(val tier: String)

/**
 * `PUT`/`DELETE /contacts/{id}/target-closeness` response — mirrors backend
 * `models.ContactTargetClosenessResponse`. Every field is always present; the
 * last three are null after a DELETE. [cadencePolicyOffer] is non-null only
 * when a tier was set and the contact has no live cadence policy; its shape is
 * exactly the `POST /cadence-policies` body, so accepting it is a plain create.
 */
@JsonClass(generateAdapter = true)
data class ContactTargetClosenessResponse(
    @Json(name = "contact_id") val contactId: Int = 0,
    @Json(name = "target_closeness") val targetCloseness: String? = null,
    @Json(name = "default_interval_days") val defaultIntervalDays: Int? = null,
    @Json(name = "cadence_policy_offer") val cadencePolicyOffer: CadencePolicyInput? = null
)
