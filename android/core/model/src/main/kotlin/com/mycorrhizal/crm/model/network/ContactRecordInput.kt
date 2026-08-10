package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Request body for POST /contacts and PUT /contacts/{id} — matches the
 * OpenAPI ContactRecordInput (symmetric read/write contract). The neutral
 * Card/CRM/Passthrough models are reused for both directions.
 */
@JsonClass(generateAdapter = true)
data class ContactRecordInput(
    val gender: String? = null,
    val card: Card? = null,
    val crm: CRMEnvelope? = null,
    val passthrough: Passthrough? = null,
)

/**
 * Wrapper for POST /contacts — the one deliberate response-envelope asymmetry
 * (ticket §2.6): the create endpoint returns `{ message, contact }` while PUT
 * returns the raw ContactRecordResponse. The repository unwraps this before
 * returning.
 */
@JsonClass(generateAdapter = true)
data class CreateContactResponse(
    val message: String? = null,
    val contact: ContactRecordResponse? = null,
)
