package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass

@JsonClass(generateAdapter = true)
data class ColumnMapping(
    @Json(name = "csv_column") val csvColumn: String = "",
    @Json(name = "contact_field") val contactField: String = "",
    val group: Int = 0,
)

@JsonClass(generateAdapter = true)
data class ImportUploadResponse(
    @Json(name = "session_id") val sessionId: String = "",
    val headers: List<String> = emptyList(),
    @Json(name = "suggested_mappings") val suggestedMappings: List<ColumnMapping> = emptyList(),
    @Json(name = "row_count") val rowCount: Int = 0,
    @Json(name = "sample_data") val sampleData: List<List<String>> = emptyList(),
)

/**
 * Request body for `POST /contacts/import/preview` (CSV). Mirrors backend
 * `models.ImportPreviewRequest` and web's `getImportPreview(sessionId,
 * mappings)` — NOT [ImportConfirmRequest]'s `{session_id, actions}` shape,
 * which is a different endpoint's request.
 */
@JsonClass(generateAdapter = true)
data class ImportPreviewRequest(
    @Json(name = "session_id") val sessionId: String,
    val mappings: List<ColumnMapping>,
)

@JsonClass(generateAdapter = true)
data class DuplicateMatch(
    @Json(name = "existing_contact_id") val existingContactId: Long = 0,
    @Json(name = "existing_firstname") val existingFirstname: String? = null,
    @Json(name = "existing_lastname") val existingLastname: String? = null,
    @Json(name = "existing_email") val existingEmail: String? = null,
    @Json(name = "existing_phone") val existingPhone: String? = null,
    @Json(name = "match_reason") val matchReason: String? = null,
)

@JsonClass(generateAdapter = true)
data class ImportScalarChange(
    val field: String = "",
    val label: String = "",
    val old: String = "",
    val new: String = "",
)

@JsonClass(generateAdapter = true)
data class ImportAddedValue(
    val kind: String = "",
    val value: String = "",
)

/**
 * T96: exactly what the "Merge" (update) action will change on the matched
 * existing contact — scalars overwritten (incoming-wins-when-non-empty) and
 * multi-valued entries appended (additive). The backend always sends `updated`
 * / `added` as arrays, never null.
 */
@JsonClass(generateAdapter = true)
data class ImportMergeDiff(
    val updated: List<ImportScalarChange> = emptyList(),
    val added: List<ImportAddedValue> = emptyList(),
)

@JsonClass(generateAdapter = true)
data class ImportRowPreview(
    @Json(name = "row_index") val rowIndex: Int = 0,
    @Json(name = "parsed_contact") val parsedContact: Map<String, Any?> = emptyMap(),
    @Json(name = "validation_errors") val validationErrors: List<String> = emptyList(),
    @Json(name = "duplicate_match") val duplicateMatch: DuplicateMatch? = null,
    @Json(name = "suggested_action") val suggestedAction: String = "add",
    val diagnostics: List<String>? = null,
    @Json(name = "merge_diff") val mergeDiff: ImportMergeDiff? = null,
    @Json(name = "batch_duplicate_of") val batchDuplicateOf: Int? = null,
)

@JsonClass(generateAdapter = true)
data class ImportPreviewResponse(
    @Json(name = "session_id") val sessionId: String = "",
    val rows: List<ImportRowPreview> = emptyList(),
    @Json(name = "total_rows") val totalRows: Int = 0,
    @Json(name = "valid_rows") val validRows: Int = 0,
    @Json(name = "duplicate_count") val duplicateCount: Int = 0,
    @Json(name = "error_count") val errorCount: Int = 0,
)

@JsonClass(generateAdapter = true)
data class RowImportAction(
    @Json(name = "row_index") val rowIndex: Int,
    val action: String,
)

@JsonClass(generateAdapter = true)
data class ImportConfirmRequest(
    @Json(name = "session_id") val sessionId: String,
    val actions: List<RowImportAction>,
)

/** T96: request body for the records-import endpoint (device-contacts path). */
@JsonClass(generateAdapter = true)
data class ImportRecordsRequest(
    val records: List<ContactRecordInput>,
)

@JsonClass(generateAdapter = true)
data class ImportResult(
    @Json(name = "total_processed") val totalProcessed: Int = 0,
    val created: Int = 0,
    val updated: Int = 0,
    val skipped: Int = 0,
    val errors: List<String> = emptyList(),
)

/**
 * One persisted import outcome (issue #651), returned newest-first by
 * `GET /contacts/import/history`. Mirrors backend `models.ImportRun` and
 * `frontend/src/api/import.ts`'s `ImportRun` field-for-field.
 */
@JsonClass(generateAdapter = true)
data class ImportRun(
    val id: Long = 0,
    val format: String = "",
    @Json(name = "total_processed") val totalProcessed: Int = 0,
    val created: Int = 0,
    val updated: Int = 0,
    val skipped: Int = 0,
    @Json(name = "error_count") val errorCount: Int = 0,
    @Json(name = "created_at") val createdAt: String? = null,
)

// --- Mycorrhizal account-bundle import source (issues #1260/#1264, ADR 0028) ---
// Mirrors backend/openapi.yaml's MycorrhizalUploadResponse / MonicaImportStatus /
// MonicaPreviewResponse (the shared source-import engine's status + preview shapes).

/** Per-section tally of an uploaded bundle. Section names are the bundle plan's keys. */
@JsonClass(generateAdapter = true)
data class MycorrhizalBundleCounts(
    val contacts: Int = 0,
    val relationships: Int = 0,
    val notes: Int = 0,
    val reminders: Int = 0,
    val activities: Int = 0,
    @Json(name = "life_events") val lifeEvents: Int = 0,
    val gifts: Int = 0,
    val households: Int = 0,
    val circles: Int = 0,
    val tags: Int = 0,
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

/** Outcome of a finished source import: the shared [ImportResult] counts plus graph-entity counts. */
@JsonClass(generateAdapter = true)
data class SourceImportResult(
    @Json(name = "total_processed") val totalProcessed: Int = 0,
    val created: Int = 0,
    val updated: Int = 0,
    val skipped: Int = 0,
    val errors: List<String> = emptyList(),
    @Json(name = "relationships_created") val relationshipsCreated: Int = 0,
    @Json(name = "notes_created") val notesCreated: Int = 0,
    @Json(name = "activities_created") val activitiesCreated: Int = 0,
    @Json(name = "reminders_created") val remindersCreated: Int = 0,
)

/**
 * Poll payload for a source-import session. [phase] is one of the backend's
 * `connecting … building_preview, ready, importing, importing_photos, done,
 * failed, cancelled`; [error] is set on `failed`.
 */
@JsonClass(generateAdapter = true)
data class SourceImportStatus(
    @Json(name = "session_id") val sessionId: String = "",
    val phase: String = "",
    @Json(name = "phase_done") val phaseDone: Int = 0,
    @Json(name = "phase_total") val phaseTotal: Int = 0,
    val error: String? = null,
    val result: SourceImportResult? = null,
) {
    val isReady: Boolean get() = phase == "ready"
    val isDone: Boolean get() = phase == "done"
    val isFailed: Boolean get() = phase == "failed" || phase == "cancelled"
}

/** One mapping issue the loss report lists before the user commits (issue #442). */
@JsonClass(generateAdapter = true)
data class SourceImportIssue(
    val record: String = "",
    val field: String = "",
    val category: String = "",
    val message: String = "",
)

@JsonClass(generateAdapter = true)
data class SourceImportPreviewResponse(
    @Json(name = "session_id") val sessionId: String = "",
    val rows: List<ImportRowPreview> = emptyList(),
    @Json(name = "total_rows") val totalRows: Int = 0,
    @Json(name = "valid_rows") val validRows: Int = 0,
    @Json(name = "duplicate_count") val duplicateCount: Int = 0,
    @Json(name = "error_count") val errorCount: Int = 0,
    @Json(name = "loss_report") val lossReport: List<SourceImportIssue> = emptyList(),
)
