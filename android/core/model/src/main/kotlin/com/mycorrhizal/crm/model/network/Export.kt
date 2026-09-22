package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * DATA-02 (issue #442) export-loss preflight DTOs, mirroring
 * `backend/models/export_loss.go`'s `LossReport`/`ExportLossPreflightResponse`.
 * Issue #835 (Android parity for T9 selective export): one fidelity-loss event
 * a proposed export would incur for one contact.
 */
@JsonClass(generateAdapter = true)
data class ExportLossReport(
    val format: String,
    @Json(name = "contact_id") val contactId: Long,
    @Json(name = "contact_name") val contactName: String,
    @Json(name = "vcard_uid") val vcardUid: String,
    val severity: String,
    val concept: String,
    val bucket: String,
    val reason: String,
    val message: String,
)

/** GET /export/preflight response — what an export with the given params would lose. */
@JsonClass(generateAdapter = true)
data class ExportLossPreflightResponse(
    val format: String,
    @Json(name = "contact_count") val contactCount: Int,
    val diagnostics: List<ExportLossReport> = emptyList(),
)
