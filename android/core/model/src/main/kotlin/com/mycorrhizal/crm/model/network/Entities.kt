package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// Reusable entities - organisations, places, pets (issue #2302, ADR 0043).
//
// Hand-written wire types mirroring backend/openapi.yaml `Entity*` / `ContactEntityLink*`
// (frontend trap #4: the kind, role and sensitivity lists below are hardcoded mirrors of
// backend/models/entity.go and backend/models/contact_entity_link.go; keep them in sync).
// List fields are nullable on the wire (trap #8): a Go nil slice serializes as `null`, which
// Moshi rejects for a non-nullable List even with a default.

/** The closed entity-kind set (ADR 0043 decision 1). */
object EntityKind {
    const val ORGANISATION = "organisation"
    const val PLACE = "place"
    const val PET = "pet"

    val ALL: List<String> = listOf(ORGANISATION, PLACE, PET)

    /** Only organisations (-> ORG) and places (-> ADR) project into a contact; pets never do. */
    fun projects(kind: String): Boolean = kind == ORGANISATION || kind == PLACE
}

/** `normal | private | secret` (ADR 0043 decision 4). */
object EntitySensitivity {
    const val NORMAL = "normal"
    val ALL: List<String> = listOf("normal", "private", "secret")
}

/** The closed role vocabulary per kind (ADR 0043 decision 2). */
object EntityLinkRoles {
    private val byKind = mapOf(
        EntityKind.ORGANISATION to
            listOf("employee", "owner", "contractor", "volunteer", "member", "other"),
        EntityKind.PLACE to listOf("resident", "owner", "visitor", "other"),
        EntityKind.PET to listOf("owner", "carer", "other")
    )

    fun forKind(kind: String): List<String> = byKind[kind].orEmpty()
}

@JsonClass(generateAdapter = true)
data class Entity(
    val id: String = "",
    val kind: String = EntityKind.ORGANISATION,
    val name: String = "",
    /** Kind-specific JSON (organisation / place / pet core); see [EntityCore]. */
    val core: Map<String, Any?>? = null,
    val sensitivity: String = EntitySensitivity.NORMAL,
    val revision: Int? = null,
    /** Only on GET /entities/{id}. */
    @Json(name = "link_count") val linkCount: Int? = null,
    @Json(name = "projection_count") val projectionCount: Int? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null
)

/** POST/PUT /entities body. `kind` is required on create and immutable afterwards. */
@JsonClass(generateAdapter = true)
data class EntityInput(
    val kind: String? = null,
    val name: String,
    val core: Map<String, Any?>? = null,
    val sensitivity: String? = null
)

@JsonClass(generateAdapter = true)
data class EntityListResponse(
    val entities: List<Entity>? = null,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0
) {
    val entitiesOrEmpty: List<Entity> get() = entities.orEmpty()
}

@JsonClass(generateAdapter = true)
data class EntityDeleteResponse(
    val message: String? = null,
    @Json(name = "link_count") val linkCount: Int = 0,
    @Json(name = "projection_count") val projectionCount: Int = 0
)

@JsonClass(generateAdapter = true)
data class EntityRestoreSkip(val table: String = "", val reason: String = "", val count: Int = 0)

@JsonClass(generateAdapter = true)
data class EntityRestoreResponse(
    val entity: Entity? = null,
    @Json(name = "links_revived") val linksRevived: Int = 0,
    @Json(name = "same_name_live") val sameNameLive: Boolean = false,
    /** Set when the entity was a merge loser: only the entity came back. */
    @Json(name = "merged_into") val mergedInto: String? = null,
    val skipped: List<EntityRestoreSkip>? = null
)

/** One contact linked to an entity (GET /entities/{id}/contacts). */
@JsonClass(generateAdapter = true)
data class EntityContactLink(
    @Json(name = "link_id") val linkId: String = "",
    @Json(name = "contact_id") val contactId: Int = 0,
    @Json(name = "contact_vcard_uid") val contactVcardUid: String = "",
    val firstname: String = "",
    val lastname: String = "",
    val role: String = "",
    @Json(name = "role_label") val roleLabel: String = "",
    @Json(name = "is_primary") val isPrimary: Boolean = false,
    val period: TemporalRange? = null,
    val sensitivity: String = EntitySensitivity.NORMAL
) {
    val displayName: String get() = listOf(
        firstname,
        lastname
    ).filter { it.isNotBlank() }.joinToString(" ")
}

@JsonClass(generateAdapter = true)
data class EntityContactsResponse(val contacts: List<EntityContactLink>? = null)

/**
 * POST /entities/{id}/merge body. [resolutions] maps a core field name to `loser` (fill an empty
 * survivor field from the loser) or `survivor` (explicit no-op).
 */
@JsonClass(generateAdapter = true)
data class EntityMergeInput(val into: String, val resolutions: Map<String, String>? = null)

@JsonClass(generateAdapter = true)
data class EntityMergeDemotion(
    @Json(name = "link_id") val linkId: String = "",
    @Json(name = "contact_vcard_uid") val contactVcardUid: String = ""
)

@JsonClass(generateAdapter = true)
data class EntityMergeResponse(
    val survivor: Entity? = null,
    @Json(name = "loser_id") val loserId: String = "",
    @Json(name = "links_repointed") val linksRepointed: Int = 0,
    @Json(name = "links_collapsed") val linksCollapsed: Int = 0,
    @Json(name = "primaries_demoted") val primariesDemoted: List<EntityMergeDemotion>? = null,
    @Json(name = "field_values_moved") val fieldValuesMoved: Int = 0,
    @Json(name = "field_values_dropped") val fieldValuesDropped: List<String>? = null,
    @Json(name = "resolved_fields") val resolvedFields: List<String>? = null,
    @Json(name = "restore_scope") val restoreScope: String? = null
)

/** One contact -> entity link as the contact page sees it. */
@JsonClass(generateAdapter = true)
data class ContactEntityLink(
    val id: String = "",
    @Json(name = "contact_vcard_uid") val contactVcardUid: String = "",
    @Json(name = "entity_id") val entityId: String = "",
    @Json(name = "entity_name") val entityName: String = "",
    val kind: String = EntityKind.ORGANISATION,
    val role: String = "",
    @Json(name = "role_label") val roleLabel: String = "",
    @Json(name = "is_primary") val isPrimary: Boolean = false,
    val period: TemporalRange? = null,
    val sensitivity: String = EntitySensitivity.NORMAL,
    /** The period includes today (non-comparable endpoints count as current). */
    val current: Boolean = true,
    /** `ORG`, `ADR`, or empty when the link does not currently project. */
    @Json(name = "projects_as") val projectsAs: String = "",
    val revision: Int? = null,
    @Json(name = "created_at") val createdAt: String? = null,
    @Json(name = "updated_at") val updatedAt: String? = null
)

@JsonClass(generateAdapter = true)
data class ContactEntityLinksResponse(val links: List<ContactEntityLink>? = null)

/** POST/PUT /contacts/{id}/entity-links body. [entityId] is required on create only. */
@JsonClass(generateAdapter = true)
data class ContactEntityLinkInput(
    @Json(name = "entity_id") val entityId: String? = null,
    val role: String,
    @Json(name = "role_label") val roleLabel: String? = null,
    @Json(name = "is_primary") val isPrimary: Boolean? = null,
    val period: TemporalRange? = null,
    val sensitivity: String? = null
)

/** Which of the contact's own values to link from: exactly one of [id] and [index]. */
@JsonClass(generateAdapter = true)
data class ContactEntityLinkSource(
    /** `organization` or `address`. */
    val kind: String,
    val id: String? = null,
    val index: Int? = null
)

@JsonClass(generateAdapter = true)
data class ContactEntityLinkFromExistingInput(
    val source: ContactEntityLinkSource,
    /** An existing entity of the matching kind; omit to create one from the value. */
    @Json(name = "entity_id") val entityId: String? = null,
    val role: String,
    @Json(name = "role_label") val roleLabel: String? = null,
    @Json(name = "is_primary") val isPrimary: Boolean? = null,
    val period: TemporalRange? = null,
    val sensitivity: String? = null
)

@JsonClass(generateAdapter = true)
data class ContactEntityLinkFromExistingResponse(
    val link: ContactEntityLink? = null,
    val entity: Entity? = null,
    @Json(name = "entity_created") val entityCreated: Boolean = false
)

@JsonClass(generateAdapter = true)
data class ContactEntityLinkAdoptResponse(
    val link: ContactEntityLink? = null,
    /** False when a value-equal own entry already existed and nothing was written. */
    val adopted: Boolean = false,
    @Json(name = "element_kind") val elementKind: String = "",
    @Json(name = "element_id") val elementId: String = ""
)

/** A display-only entity node from `GET /graph?include_entities=true` (id `e-<entityID>`). */
@JsonClass(generateAdapter = true)
data class GraphEntityNode(
    val id: String = "",
    val kind: String = EntityKind.ORGANISATION,
    val label: String = ""
)

/** Contact node (`c-<contactID>`) to entity node (`e-<entityID>`); no type, role or status. */
@JsonClass(generateAdapter = true)
data class GraphEntityEdge(val id: String = "", val source: String = "", val target: String = "")

/** A contact node of `GET /graph` (only the fields the entity overlay needs). */
@JsonClass(generateAdapter = true)
data class GraphContactNode(val id: String = "", val type: String = "", val label: String = "")

/** `GET /graph?include_entities=true` - the overlay keys only; traversal data stays in chains. */
@JsonClass(generateAdapter = true)
data class GraphEntitiesResponse(
    val nodes: List<GraphContactNode>? = null,
    @Json(name = "entity_nodes") val entityNodes: List<GraphEntityNode>? = null,
    @Json(name = "entity_edges") val entityEdges: List<GraphEntityEdge>? = null
)

/**
 * ADR 0043 decision 3: a projected ORG/ADR entry in a contact record carries the reserved element
 * id `ent-<linkId>`. It is read-time synthesis, never the contact's own value: the UI never edits
 * or deletes one and never writes one back.
 */
const val PROJECTED_ELEMENT_ID_PREFIX = "ent-"

fun isProjectedElementId(id: String?): Boolean =
    !id.isNullOrEmpty() && id.startsWith(PROJECTED_ELEMENT_ID_PREFIX)

/** The link id behind a projected element id, or null for an ordinary id. */
fun entityLinkIdFromElementId(id: String?): String? =
    if (isProjectedElementId(id)) id!!.removePrefix(PROJECTED_ELEMENT_ID_PREFIX) else null

/** The contact's own organizations: projected `ent-` entries are not the contact's data. */
fun Card.ownOrganizations(): List<Organization> =
    organizations.orEmpty().filterNot { isProjectedElementId(it.id) }

/** The contact's own addresses: projected `ent-` entries are not the contact's data. */
fun Card.ownAddresses(): List<Address> =
    addresses.orEmpty().filterNot { isProjectedElementId(it.id) }

/**
 * The client half of ADR 0043 echo suppression: no request body may carry a projected `ent-`
 * entry (or a period keyed by one) back to the server. Returns the same instance when there is
 * nothing to strip, and a null list (not an empty one) when only projected entries were present.
 */
fun ContactRecordInput.stripProjectedEntries(): ContactRecordInput {
    val currentCard = card
    val currentPeriods = crm?.periods
    val hasProjectedOrg = currentCard?.organizations.orEmpty().any { isProjectedElementId(it.id) }
    val hasProjectedAdr = currentCard?.addresses.orEmpty().any { isProjectedElementId(it.id) }
    val hasProjectedPeriod = currentPeriods.orEmpty().any { isProjectedElementId(it.entryId) }
    if (!hasProjectedOrg && !hasProjectedAdr && !hasProjectedPeriod) return this
    val nextCard = if (currentCard != null && (hasProjectedOrg || hasProjectedAdr)) {
        currentCard.copy(
            organizations = if (hasProjectedOrg) {
                currentCard.ownOrganizations().ifEmpty { null }
            } else {
                currentCard.organizations
            },
            addresses = if (hasProjectedAdr) {
                currentCard.ownAddresses().ifEmpty { null }
            } else {
                currentCard.addresses
            }
        )
    } else {
        currentCard
    }
    val nextCrm = if (crm != null && hasProjectedPeriod) {
        crm.copy(
            periods = currentPeriods.orEmpty().filterNot { isProjectedElementId(it.entryId) }
                .ifEmpty { null }
        )
    } else {
        crm
    }
    return copy(card = nextCard, crm = nextCrm)
}

/** True when [start] and [end] are each blank or four digits, and the end is not before the start. */
fun entityPeriodYearsValid(start: String, end: String): Boolean {
    val fourDigits = Regex("^\\d{4}$")
    val s = start.trim()
    val e = end.trim()
    if (s.isNotEmpty() && !s.matches(fourDigits)) return false
    if (e.isNotEmpty() && !e.matches(fourDigits)) return false
    return s.isEmpty() || e.isEmpty() || s.toInt() <= e.toInt()
}

/**
 * A link period from whole-year inputs (the same granularity as work/address periods). A side
 * whose year is unchanged keeps the finer date already stored in [original], so editing only the
 * end year never degrades a stored start of 2019-04-09 to 2019. Null when both are blank.
 */
fun entityPeriodFromYears(start: String, end: String, original: TemporalRange?): TemporalRange? {
    fun side(text: String, stored: PartialDate?): PartialDate? {
        val year = text.trim().toIntOrNull() ?: return null
        return if (stored?.year == year) stored else PartialDate(year = year)
    }
    val s = side(start, original?.start)
    val e = side(end, original?.end)
    return if (s == null && e == null) null else TemporalRange(start = s, end = e)
}

/**
 * Whether [range] includes the given calendar day. Endpoints are compared at the precision they
 * share with the day; one that cannot be compared (no year) counts as current (ADR 0015 rule 3:
 * non-comparability is not an error). A null or empty range is always current.
 */
fun isPeriodCurrent(range: TemporalRange?, year: Int, month: Int, day: Int): Boolean {
    fun cmp(d: PartialDate?): Int? {
        val y = d?.year ?: return null
        if (y != year) return y.compareTo(year)
        val m = d.month ?: return 0
        if (m != month) return m.compareTo(month)
        val dd = d.day ?: return 0
        return dd.compareTo(day)
    }
    val startCmp = cmp(range?.start)
    val endCmp = cmp(range?.end)
    return (startCmp == null || startCmp <= 0) && (endCmp == null || endCmp >= 0)
}

/** True for a primary link whose period includes the given day - the links that project today. */
fun EntityContactLink.projectsToday(year: Int, month: Int, day: Int): Boolean =
    isPrimary && isPeriodCurrent(period, year, month, day)

/**
 * The record as the contact cache may keep it: projected `ent-` entries (and periods keyed by
 * them) removed. A projection is the server's read-time view of a link; caching it would let a
 * cached contact diverge from the server after a link or entity change on another device, so only
 * the contact's own values are ever persisted (issue #2302).
 */
fun ContactRecordResponse.withoutProjectedEntries(): ContactRecordResponse {
    val stripped = ContactRecordInput(card = card, crm = crm).stripProjectedEntries()
    return if (stripped.card === card && stripped.crm === crm) {
        this
    } else {
        copy(card = stripped.card, crm = stripped.crm)
    }
}

/** One side of a suggested duplicate pair (GET /entities/suggestions, `mode=pairs`). */
@JsonClass(generateAdapter = true)
data class EntitySuggestionEntity(
    val id: String = "",
    val kind: String = EntityKind.ORGANISATION,
    val name: String = ""
)

/**
 * A candidate duplicate pair (#2299). [reason] is `exact_name`, `normalized_name` or
 * `address_components` - a hardcoded mirror of the backend token set (unknown tokens are shown
 * with the generic label).
 */
@JsonClass(generateAdapter = true)
data class EntitySuggestionPair(
    val reason: String = "",
    val a: EntitySuggestionEntity = EntitySuggestionEntity(),
    val b: EntitySuggestionEntity = EntitySuggestionEntity()
)

/** One contact in a [EntityValueGroup], with the spelling it has on that contact. */
@JsonClass(generateAdapter = true)
data class EntityValueGroupContact(
    @Json(name = "contact_id") val contactId: Int = 0,
    val firstname: String = "",
    val lastname: String = "",
    /** The organisation / address text as typed on this contact. */
    @Json(name = "value_name") val valueName: String = "",
    /** The contact already has a primary organisation / place, so this link would not be it. */
    @Json(name = "has_primary") val hasPrimary: Boolean = false
) {
    val displayName: String get() = listOf(
        firstname,
        lastname
    ).filter { it.isNotBlank() }.joinToString(" ")
}

/**
 * One group of equal organisation / address values found on contacts (GET /entities/suggestions
 * `mode=contact_values`, #2299). [key] is opaque: echo it to POST /entities/from-contacts.
 * [reason] is `exact_name`, `normalized_name` or `address_components` (hardcoded mirror).
 */
@JsonClass(generateAdapter = true)
data class EntityValueGroup(
    val key: String = "",
    val kind: String = EntityKind.ORGANISATION,
    /** The proposed entity name (the most common spelling). */
    val name: String = "",
    val reason: String = "",
    val variants: List<String>? = null,
    val contacts: List<EntityValueGroupContact>? = null
) {
    val variantsOrEmpty: List<String> get() = variants.orEmpty()
    val contactsOrEmpty: List<EntityValueGroupContact> get() = contacts.orEmpty()
}

/**
 * GET /entities/suggestions. `pairs` is filled in `mode=pairs` and `groups` in
 * `mode=contact_values` (the create-from-contacts helper); the other list is null or empty.
 */
@JsonClass(generateAdapter = true)
data class EntitySuggestionsResponse(
    val mode: String = "pairs",
    val pairs: List<EntitySuggestionPair>? = null,
    val groups: List<EntityValueGroup>? = null,
    @Json(name = "next_cursor") val nextCursor: String? = null,
    val limit: Int = 0
) {
    val pairsOrEmpty: List<EntitySuggestionPair> get() = pairs.orEmpty()
    val groupsOrEmpty: List<EntityValueGroup> get() = groups.orEmpty()
}

/**
 * One confirmed group of POST /entities/from-contacts: the entity to create and the subset of the
 * group's contacts to link (1..1000). A blank [name] keeps the proposed one.
 */
@JsonClass(generateAdapter = true)
data class EntityFromContactsGroup(
    val key: String,
    val name: String? = null,
    val role: String,
    @Json(name = "set_primary") val setPrimary: Boolean = false,
    @Json(name = "contact_ids") val contactIds: List<Int>
)

/** POST /entities/from-contacts body; [kind] is `organisation` or `place` (1..100 groups). */
@JsonClass(generateAdapter = true)
data class EntityFromContactsInput(val kind: String, val groups: List<EntityFromContactsGroup>)

/** Why a confirmed group or contact was not applied (hardcoded mirror of the backend tokens). */
object EntityFromContactsSkipReason {
    const val GROUP_NOT_FOUND = "group_not_found"
    const val CONTACT_NOT_IN_GROUP = "contact_not_in_group"
    const val NAME_INVALID = "name_invalid"
    const val CORE_INVALID = "core_invalid"
}

@JsonClass(generateAdapter = true)
data class EntityFromContactsSkip(
    val key: String = "",
    @Json(name = "contact_id") val contactId: Int? = null,
    val reason: String = ""
)

/** One entity the helper created, with the links it made. */
@JsonClass(generateAdapter = true)
data class EntityFromContactsCreated(
    val key: String = "",
    val entity: Entity? = null,
    val links: List<ContactEntityLink>? = null
) {
    val linksOrEmpty: List<ContactEntityLink> get() = links.orEmpty()
}

@JsonClass(generateAdapter = true)
data class EntityFromContactsResponse(
    val created: List<EntityFromContactsCreated>? = null,
    val skipped: List<EntityFromContactsSkip>? = null
) {
    val createdOrEmpty: List<EntityFromContactsCreated> get() = created.orEmpty()
    val skippedOrEmpty: List<EntityFromContactsSkip> get() = skipped.orEmpty()
}

/** POST /entities/suggestions/dismiss body; either order, the server canonicalises it. */
@JsonClass(generateAdapter = true)
data class EntityDismissInput(
    @Json(name = "entity_a") val entityA: String,
    @Json(name = "entity_b") val entityB: String
)

@JsonClass(generateAdapter = true)
data class EntityDismissResponse(
    @Json(name = "id_low") val idLow: String = "",
    @Json(name = "id_high") val idHigh: String = ""
)
