package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

// ---------------------------------------------------------------------------
// Contact merge
// ---------------------------------------------------------------------------

@JsonClass(generateAdapter = true)
data class ContactMergeFieldConflict(
    val field: String = "",
    val label: String = "",
    @Json(name = "keeper_value") val keeperValue: String = "",
    @Json(name = "loser_value") val loserValue: String = "",
)

@JsonClass(generateAdapter = true)
data class ContactMergeResolution(
    val emails: List<ContactMergeValue> = emptyList(),
    val phones: List<ContactMergeValue> = emptyList(),
    val addresses: List<ContactMergeAddress> = emptyList(),
    val urls: List<ContactMergeValue> = emptyList(),
    val impps: List<ContactMergeValue> = emptyList(),
    val circles: List<String> = emptyList(),
    @Json(name = "resolved_scalars") val resolvedScalars: Map<String, String> = emptyMap(),
    val conflicts: List<ContactMergeFieldConflict> = emptyList(),
    @Json(name = "field_value_conflicts") val fieldValueConflicts: List<ContactMergeFieldConflict> = emptyList(),
)

@JsonClass(generateAdapter = true)
data class ContactMergeValue(
    val type: String? = null,
    val value: String = "",
)

@JsonClass(generateAdapter = true)
data class ContactMergeAddress(
    val type: String? = null,
    val street: String? = null,
    val city: String? = null,
    val region: String? = null,
    val postal: String? = null,
    val country: String? = null,
)

@JsonClass(generateAdapter = true)
data class ContactMergeAssociationCounts(
    val notes: Long = 0,
    val activities: Long = 0,
    val reminders: Long = 0,
    @Json(name = "reminder_completions") val reminderCompletions: Long = 0,
    @Json(name = "relationship_edges") val relationshipEdges: Long = 0,
    @Json(name = "household_memberships") val householdMemberships: Long = 0,
    @Json(name = "circle_memberships") val circleMemberships: Long = 0,
    val tags: Long = 0,
    @Json(name = "life_events") val lifeEvents: Long = 0,
    @Json(name = "life_event_references") val lifeEventReferences: Long = 0,
    @Json(name = "conversation_agenda_items") val conversationAgendaItems: Long = 0,
    @Json(name = "gift_items") val giftItems: Long = 0,
    @Json(name = "field_values") val fieldValues: Long = 0,
    @Json(name = "contact_sync_links") val contactSyncLinks: Long = 0,
    // T107: merge used to silently destroy these; the backend now re-points them
    // (or adopts cadence policies as a conflict). Mirrored so M23's breakdown
    // can show them, matching web's category set.
    val attachments: Long = 0,
    val preferences: Long = 0,
    @Json(name = "external_identities") val externalIdentities: Long = 0,
    @Json(name = "external_activities") val externalActivities: Long = 0,
    @Json(name = "cadence_policies") val cadencePolicies: Long = 0,
)

@JsonClass(generateAdapter = true)
data class ContactMergeRequest(
    @Json(name = "keep_id") val keepId: Long,
    @Json(name = "merge_id") val mergeId: Long,
    val resolutions: Map<String, String>? = null,
)

@JsonClass(generateAdapter = true)
data class ContactMergePreviewResponse(
    @Json(name = "keep_id") val keepId: Long = 0,
    @Json(name = "merge_id") val mergeId: Long = 0,
    val resolution: ContactMergeResolution = ContactMergeResolution(),
    @Json(name = "association_counts") val associationCounts: ContactMergeAssociationCounts =
        ContactMergeAssociationCounts(),
)

/** Commit response — `{ message, contact }` (the merged full contact record). */
@JsonClass(generateAdapter = true)
data class ContactMergeCommitResponse(
    val message: String? = null,
    val contact: ContactRecordResponse? = null,
)

// ---------------------------------------------------------------------------
// Bulk operations
// ---------------------------------------------------------------------------

object BulkActions {
    const val ADD_CIRCLE = "add_circle"
    const val REMOVE_CIRCLE = "remove_circle"
    const val ADD_TAG = "add_tag"
    const val REMOVE_TAG = "remove_tag"
    const val ARCHIVE = "archive"
    const val UNARCHIVE = "unarchive"
    const val DELETE = "delete"
}

@JsonClass(generateAdapter = true)
data class BulkContactOperationInput(
    val action: String,
    @Json(name = "vcard_uids") val vcardUids: List<String>,
    @Json(name = "circle_id") val circleId: String? = null,
    @Json(name = "tag_id") val tagId: String? = null,
)

@JsonClass(generateAdapter = true)
data class BulkFailureEntry(
    @Json(name = "vcard_uid") val vcardUid: String = "",
    val reason: String = "",
)

@JsonClass(generateAdapter = true)
data class BulkOperationResult(
    val action: String = "",
    val total: Int = 0,
    val succeeded: Int = 0,
    val failed: Int = 0,
    val failures: List<BulkFailureEntry> = emptyList(),
)
