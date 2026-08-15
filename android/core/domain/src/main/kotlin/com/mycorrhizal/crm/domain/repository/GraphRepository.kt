package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.GraphConnectionsResponse

/**
 * Ego-centric graph data access (M14). Online-first and deliberately uncached:
 * the traversal runs server-side and a stale copy would mislead (edges change
 * elsewhere all the time). Names and inverse relations are already resolved by
 * the endpoint, so this surface is pure read-through.
 */
interface GraphRepository {
    /**
     * GET /graph/connections — every reachable contact from [from] (a
     * Contact.VCardUID) within [depth] hops. [relation] passes a registry
     * token or synonym through verbatim (the server resolves it).
     */
    suspend fun getConnections(
        from: String,
        depth: Int? = null,
        relation: String? = null,
    ): Result<GraphConnectionsResponse>

    /**
     * The caller's "Me" contact VCardUID (GET /users/me →
     * `self_contact_vcard_uid`), used to default the network screen's
     * "start from" picker. Null when the account has no self contact set.
     */
    suspend fun selfContactVCardUid(): Result<String?>

    /**
     * Every circle with its member VCardUIDs, for the network screen's
     * client-side circle filter (`GET /circles?include_members=true`). The
     * graph endpoint has no circle param — the web filters the whole graph
     * client-side too, so Android mirrors that.
     */
    suspend fun circlesWithMembers(): Result<List<CircleWithMembers>>
}

/** A circle plus the VCardUIDs of its members, for a client-side membership filter. */
data class CircleWithMembers(
    val id: String,
    val name: String,
    val memberVCardUids: Set<String>,
)
