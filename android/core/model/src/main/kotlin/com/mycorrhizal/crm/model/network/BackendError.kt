package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/** The standard error envelope produced by apperrors.AbortWithError. */
@JsonClass(generateAdapter = true)
data class BackendError(
    val error: BackendErrorBody? = null,
    @Json(name = "request_id") val requestId: String? = null,
    val timestamp: String? = null
)

@JsonClass(generateAdapter = true)
data class BackendErrorBody(
    val code: String? = null,
    val message: String? = null,
    val details: Map<String, Any?>? = null,
    // ADR 0038: optional stable reason for client-side localization.
    @Json(name = "reason_code") val reasonCode: String? = null,
    @Json(name = "reason_params") val reasonParams: Map<String, Any?>? = null
) {
    val displayMessage: String
        get() = message?.takeIf { it.isNotBlank() } ?: code ?: "Unknown error"
}
