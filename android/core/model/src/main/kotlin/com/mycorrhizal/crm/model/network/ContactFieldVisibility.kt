package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * Issue #1997: the resolved display decision for one contact. [fields] maps
 * every known field key to whether the detail view shows it; [hideEmpty]
 * suppresses enabled fields that carry no value.
 */
@JsonClass(generateAdapter = true)
data class ResolvedFieldVisibility(
    val fields: Map<String, Boolean> = emptyMap(),
    @Json(name = "hide_empty") val hideEmpty: Boolean = false
)

/**
 * GET/PUT/DELETE /contacts/{id}/field-visibility (issue #1997).
 *
 * [overrides] holds only the explicit per-contact "show"/"hide" rows; an
 * inherited field is absent. [hideEmpty] is the raw per-contact tri-state
 * ("inherit" | "show" | "hide"). [defaults] is the account default the contact
 * inherits and [resolved] is what the client renders from. Cosmetic only: this
 * never changes the contact's data or what leaves the instance.
 */
@JsonClass(generateAdapter = true)
data class ContactFieldVisibility(
    val overrides: Map<String, String> = emptyMap(),
    @Json(name = "hide_empty") val hideEmpty: String = "inherit",
    val defaults: ResolvedFieldVisibility = ResolvedFieldVisibility(),
    val resolved: ResolvedFieldVisibility = ResolvedFieldVisibility()
)

/** PUT body: the complete override set (full replace; "inherit" clears a row). */
@JsonClass(generateAdapter = true)
data class ContactFieldVisibilityInput(
    val overrides: Map<String, String> = emptyMap(),
    @Json(name = "hide_empty") val hideEmpty: String = "inherit"
)
