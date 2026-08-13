package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Mirrors backend/models/relationship_type_registry.go exactly (token ->
 * inverse/symmetric). No dynamic "list valid types" endpoint exists anywhere
 * (frontend trap 4 — hardcoded mirror, kept in sync by hand).
 */
object RelationshipEdgeTypes {
    const val PARENT_OF = "parent_of"
    const val CHILD_OF = "child_of"
    const val SPOUSE_OF = "spouse_of"
    const val SIBLING_OF = "sibling_of"
    const val FRIEND_OF = "friend_of"
    const val ROOMMATE_OF = "roommate_of"
    const val COWORKER_OF = "coworker_of"
    const val PARTNER_OF = "partner_of"
    const val CO_PARENT_OF = "co_parent_of"
    const val MENTOR_OF = "mentor_of"
    const val MENTEE_OF = "mentee_of"
    const val OWNED_BY = "owned_by"
    const val OWNS = "owns"
    const val GETS_ALONG_WITH = "gets_along_with"
    const val CONFLICTS_WITH = "conflicts_with"
    const val RELATED_TO = "related_to"

    data class Meta(val inverse: String, val symmetric: Boolean)

    val ALL: Map<String, Meta> = mapOf(
        PARENT_OF to Meta(CHILD_OF, false),
        CHILD_OF to Meta(PARENT_OF, false),
        SPOUSE_OF to Meta(SPOUSE_OF, true),
        SIBLING_OF to Meta(SIBLING_OF, true),
        FRIEND_OF to Meta(FRIEND_OF, true),
        ROOMMATE_OF to Meta(ROOMMATE_OF, true),
        COWORKER_OF to Meta(COWORKER_OF, true),
        PARTNER_OF to Meta(PARTNER_OF, true),
        CO_PARENT_OF to Meta(CO_PARENT_OF, true),
        MENTOR_OF to Meta(MENTEE_OF, false),
        MENTEE_OF to Meta(MENTOR_OF, false),
        OWNED_BY to Meta(OWNS, false),
        OWNS to Meta(OWNED_BY, false),
        GETS_ALONG_WITH to Meta(GETS_ALONG_WITH, true),
        CONFLICTS_WITH to Meta(CONFLICTS_WITH, true),
        RELATED_TO to Meta(RELATED_TO, true),
    )

    val TYPE_TOKENS: List<String> = ALL.keys.toList()
}

object RelationshipEdgeStatuses {
    const val CONFIRMED = "confirmed"
    const val SUGGESTED = "suggested"
}

object RelationshipEdgeSensitivities {
    const val NORMAL = "normal"
    const val PRIVATE = "private"
    const val SECRET = "secret"
}

@JsonClass(generateAdapter = true)
data class RelationshipEdge(
    val id: String = "",
    @Json(name = "source_id") val sourceId: String = "",
    @Json(name = "target_id") val targetId: String = "",
    val type: String = RelationshipEdgeTypes.RELATED_TO,
    val directional: Boolean = false,
    val metadata: Map<String, Any?>? = null,
    val source: String = "user-confirmed",
    val confidence: Double = 1.0,
    val status: String = RelationshipEdgeStatuses.CONFIRMED,
    val sensitivity: String = RelationshipEdgeSensitivities.NORMAL,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null,
)

/** Thin contact for creating an edge to a contact that isn't in the CRM yet. */
@JsonClass(generateAdapter = true)
data class ThinContactInput(
    val name: String,
    val gender: String? = null,
    val birthday: String? = null,
)

@JsonClass(generateAdapter = true)
data class RelationshipEdgeInput(
    @Json(name = "source_id") val sourceId: String? = null,
    @Json(name = "source_thin") val sourceThin: ThinContactInput? = null,
    @Json(name = "target_id") val targetId: String? = null,
    @Json(name = "target_thin") val targetThin: ThinContactInput? = null,
    val type: String,
    val sensitivity: String? = null,
    val metadata: Map<String, Any?>? = null,
)

/** GET /relationship-edges response — cursor-paginated. */
@JsonClass(generateAdapter = true)
data class RelationshipEdgesPage(
    @Json(name = "relationship_edges") val relationshipEdges: List<RelationshipEdge> = emptyList(),
    val total: Int = 0,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0,
    val sync: SyncInfo? = null,
)

/** POST /relationship-edges response — wrapped `{ relationship_edge }` (§2.6). */
@JsonClass(generateAdapter = true)
data class CreateRelationshipEdgeResponse(
    @Json(name = "relationship_edge") val relationshipEdge: RelationshipEdge? = null,
)
