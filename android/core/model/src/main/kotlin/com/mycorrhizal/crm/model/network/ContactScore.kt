package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// ---------------------------------------------------------------------------
// Relationship health score (issue #383, ADR-0023) — GET /contacts/:id/score
// ---------------------------------------------------------------------------

/**
 * `GET /contacts/:id/score` response — the full explainable breakdown behind
 * a contact's relationship health score, mirroring the backend's
 * `models.ContactScoreResponse` field-for-field (`backend/models/contact_score.go`).
 * [score] is 0-100; [band] is one of "moss"/"chanterelle"/"russula". The score
 * (and every facet sub-score) is computed server-side — Android renders it,
 * it is never recomputed on-device, the same read-only contract
 * [BriefingCadence]'s health carries.
 */
@JsonClass(generateAdapter = true)
data class ContactScoreResponse(
    @Json(name = "contact_id") val contactId: Int = 0,
    val score: Int = 0,
    val band: String = "",
    val recency: ContactScoreFacet = ContactScoreFacet(),
    val frequency: ContactScoreFacet = ContactScoreFacet(),
    val closeness: ContactScoreFacet = ContactScoreFacet(),
    @Json(name = "reach_out") val reachOut: ContactScoreFacet = ContactScoreFacet(),
    @Json(name = "last_updated") val lastUpdated: ContactScoreFacet = ContactScoreFacet(),
    /**
     * Issue #1733: true when the contact counts as in regular contact without logged activity.
     * Always present on current servers; defaults to false for one that predates it.
     */
    val ambient: Boolean = false,
    /** [AMBIENT_REASON_HOUSEHOLD] or [AMBIENT_REASON_FLAG]; absent when [ambient] is false. */
    @Json(name = "ambient_reason") val ambientReason: String? = null,
    /**
     * Issue #1798 (ADR 0023 amendment): how the contact is doing against its
     * user-set target closeness. Always present on current servers and null
     * when the contact has no target.
     */
    val target: ContactScoreTarget? = null,
)

/**
 * The target-closeness status of a contact, mirroring backend
 * `models.ContactScoreTarget` (issue #1796). [status] is one of
 * [TargetStatus.ON_TRACK]/[TargetStatus.DRIFTING]/[TargetStatus.BEHIND];
 * [intervalSource] is "cadence_policy" or "target_tier". [reason] is the
 * server's plain-language explanation, rendered verbatim.
 */
@JsonClass(generateAdapter = true)
data class ContactScoreTarget(
    val tier: String = "",
    val status: String = "",
    val value: Double = 0.0,
    @Json(name = "interval_days") val intervalDays: Int = 0,
    @Json(name = "interval_source") val intervalSource: String = "",
    val reason: String = "",
)

/** `ambient_reason`: a current member of a household the user's self-contact is also in. */
const val AMBIENT_REASON_HOUSEHOLD = "household"

/** `ambient_reason`: the user set `crm.ambient` on the contact. */
const val AMBIENT_REASON_FLAG = "flag"

/**
 * One weighted facet's contribution and plain-language explanation — the
 * score is never a black box (backend `models.ContactScoreFacet`). [value] is
 * the facet's own 0-100 sub-score before weighting; [weight] is its share of
 * the total (all five facets sum to 100); [reason] is the short,
 * plain-language explanation shown in the facet breakdown.
 */
@JsonClass(generateAdapter = true)
data class ContactScoreFacet(
    val value: Double = 0.0,
    val weight: Double = 0.0,
    val reason: String = "",
)
