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
)

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
