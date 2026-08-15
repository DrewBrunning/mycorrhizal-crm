package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// M14: ego-centric network graph over `GET /graph/connections` (T10's
// traversal backend). The server has already resolved every name and applied
// each inverse relation, so these DTOs are pure read models — the client must
// never re-resolve names or re-derive inverses.

/**
 * `GET /graph/connections` response — every contact reachable from
 * [fromVCardUid] within [depth] hops.
 *
 * [chains] is declared nullable (`/CLAUDE.md` frontend trap #8): the backend
 * always serializes `[]`, but Moshi rejects an explicit JSON `null` for a
 * non-nullable list even with a Kotlin default. Callers use [chainsOrEmpty].
 */
@JsonClass(generateAdapter = true)
data class GraphConnectionsResponse(
    @Json(name = "from_vcard_uid") val fromVCardUid: String = "",
    @Json(name = "from_name") val fromName: String = "",
    val depth: Int = 0,
    val chains: List<GraphChain>? = null,
) {
    val chainsOrEmpty: List<GraphChain> get() = chains ?: emptyList()
}

/** One reachable contact: its [depth] in hops and the [steps] walked to it. */
@JsonClass(generateAdapter = true)
data class GraphChain(
    @Json(name = "target_id") val targetId: Int = 0,
    @Json(name = "target_vcard_uid") val targetVCardUid: String = "",
    @Json(name = "target_name") val targetName: String = "",
    val depth: Int = 0,
    val steps: List<GraphChainStep>? = null,
) {
    val stepsOrEmpty: List<GraphChainStep> get() = steps ?: emptyList()

    /**
     * The readable hop-by-hop path to this target, e.g.
     * `"Sister (sibling of) → Bob (spouse of)"`. Relations are display tokens
     * with the inverse already applied server-side; Android renders them the
     * same way the Relationships screen does (token, underscores as spaces) —
     * there is no per-token translation table on Android.
     */
    val readablePath: String
        get() = stepsOrEmpty.joinToString(" → ") { step ->
            val relation = step.relation.replace('_', ' ')
            "${step.contactName} ($relation)"
        }
}

/** One hop: the intermediate contact and the relation from it to the next. */
@JsonClass(generateAdapter = true)
data class GraphChainStep(
    @Json(name = "contact_id") val contactId: Int = 0,
    @Json(name = "contact_vcard_uid") val contactVCardUid: String = "",
    @Json(name = "contact_name") val contactName: String = "",
    val relation: String = "",
)
