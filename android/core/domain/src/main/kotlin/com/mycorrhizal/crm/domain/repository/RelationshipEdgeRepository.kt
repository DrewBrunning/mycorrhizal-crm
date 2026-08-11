package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput

/**
 * Relationship-edge data access. Online-first. Edges are the graph invariant:
 * source_id/target_id are Contact.VCardUIDs, `type` describes Source's role
 * relative to Target.
 */
interface RelationshipEdgeRepository {
    /** A contact's edges (confirmed or suggested), cursor-paginated. */
    suspend fun listForContact(
        contactId: String,
        status: String? = null,
        cursor: String? = null,
    ): Result<List<RelationshipEdge>>

    /** Create an edge (source/target by VCardUID, or thin-contact for unknown parties). */
    suspend fun create(input: RelationshipEdgeInput): Result<RelationshipEdge>

    /** Promote a suggested edge to confirmed. */
    suspend fun accept(id: String): Result<RelationshipEdge>

    /** Delete an edge (also rejects a suggestion). */
    suspend fun delete(id: String): Result<Unit>
}
