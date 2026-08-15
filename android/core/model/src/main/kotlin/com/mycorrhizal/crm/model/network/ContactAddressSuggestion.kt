package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * One contact-address suggestion (the inverse of T40's household-address
 * suggestions): contact [contactVCardUid] should consider the address the
 * source carries, because a confirmed parent/child, spouse, or roommate edge
 * ([sourceKind] == "relationship") or household membership ([sourceKind] ==
 * "household") implies a shared residence. For a relationship source,
 * [relationType] reads from the contact's perspective.
 */
@JsonClass(generateAdapter = true)
data class ContactAddressSuggestion(
    @Json(name = "contact_vcard_uid") val contactVCardUid: String = "",
    @Json(name = "contact_name") val contactName: String = "",
    @Json(name = "source_kind") val sourceKind: String = "",
    @Json(name = "source_id") val sourceId: String = "",
    @Json(name = "source_name") val sourceName: String = "",
    @Json(name = "relation_type") val relationType: String? = null,
    val address: Address? = null,
    @Json(name = "address_key") val addressKey: String = "",
)

/** POST /contacts/address-suggestions response. */
@JsonClass(generateAdapter = true)
data class ContactAddressSuggestionsResponse(
    val suggestions: List<ContactAddressSuggestion> = emptyList(),
    val total: Int = 0,
)

/**
 * POST /contacts/address-suggestions/apply body. Only the suggestion's
 * identity is sent; the server re-derives the address from the current graph.
 */
@JsonClass(generateAdapter = true)
data class ApplyContactAddressSuggestionInput(
    @Json(name = "contact_vcard_uid") val contactVCardUid: String,
    @Json(name = "source_kind") val sourceKind: String,
    @Json(name = "source_id") val sourceId: String,
    @Json(name = "address_key") val addressKey: String,
)
