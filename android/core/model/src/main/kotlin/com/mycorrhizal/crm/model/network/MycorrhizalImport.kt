package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

/**
 * ADR 0028 Decision 3 / issues #1259 + #1260: the account-bundle attach flow's
 * wire types. Mirrors `MycorrhizalUploadResponse`, `MonicaImportStatus` and
 * `MonicaPreviewResponse` in `backend/openapi.yaml` (the mycorrhizal import
 * source reuses the shared source-import engine's status/preview shapes).
 */

/** Per-section tally of an uploaded bundle (`MycorrhizalBundleCounts`). */
@JsonClass(generateAdapter = true)
data class MycorrhizalBundleCounts(
    val contacts: Int = 0,
    val relationships: Int = 0,
    val notes: Int = 0,
    val reminders: Int = 0,
    @Json(name = "reminder_completions") val reminderCompletions: Int = 0,
    val activities: Int = 0,
    @Json(name = "life_events") val lifeEvents: Int = 0,
    val gifts: Int = 0,
    val preferences: Int = 0,
    @Json(name = "conversation_agenda") val conversationAgenda: Int = 0,
    @Json(name = "cadence_policies") val cadencePolicies: Int = 0,
    @Json(name = "data_decay_policies") val dataDecayPolicies: Int = 0,
    val households: Int = 0,
)

@JsonClass(generateAdapter = true)
data class MycorrhizalUploadResponse(
    @Json(name = "session_id") val sessionId: String = "",
    val version: Int = 0,
    val totals: MycorrhizalBundleCounts = MycorrhizalBundleCounts(),
)

@JsonClass(generateAdapter = true)
data class MycorrhizalFetchRequest(
    @Json(name = "session_id") val sessionId: String,
)

/** Import outcome summary; the fields the wizard reports (the rest are ignored). */
@JsonClass(generateAdapter = true)
data class MycorrhizalImportResult(
    @Json(name = "total_processed") val totalProcessed: Int = 0,
    val created: Int = 0,
    val updated: Int = 0,
    val skipped: Int = 0,
    val errors: List<String> = emptyList(),
)

/** Poll payload (`MonicaImportStatus`). [phase] is one of the enum tokens in the spec. */
@JsonClass(generateAdapter = true)
data class MycorrhizalImportStatus(
    @Json(name = "session_id") val sessionId: String = "",
    val phase: String = "",
    @Json(name = "phase_done") val phaseDone: Int = 0,
    @Json(name = "phase_total") val phaseTotal: Int = 0,
    val error: String? = null,
    val result: MycorrhizalImportResult? = null,
) {
    val isReady: Boolean get() = phase == PHASE_READY
    val isDone: Boolean get() = phase == PHASE_DONE
    val isFailed: Boolean get() = phase == PHASE_FAILED || phase == PHASE_CANCELLED

    companion object {
        const val PHASE_READY = "ready"
        const val PHASE_DONE = "done"
        const val PHASE_FAILED = "failed"
        const val PHASE_CANCELLED = "cancelled"
    }
}

/** One entry of the pre-confirm loss report. */
@JsonClass(generateAdapter = true)
data class MycorrhizalImportIssue(
    val record: String = "",
    val field: String = "",
    val category: String = "",
    val message: String = "",
)

/** The review payload: the shared per-row preview plus the loss report. */
@JsonClass(generateAdapter = true)
data class MycorrhizalPreviewResponse(
    @Json(name = "session_id") val sessionId: String = "",
    val rows: List<ImportRowPreview> = emptyList(),
    @Json(name = "total_rows") val totalRows: Int = 0,
    @Json(name = "valid_rows") val validRows: Int = 0,
    @Json(name = "duplicate_count") val duplicateCount: Int = 0,
    @Json(name = "error_count") val errorCount: Int = 0,
    @Json(name = "loss_report") val lossReport: List<MycorrhizalImportIssue> = emptyList(),
)
