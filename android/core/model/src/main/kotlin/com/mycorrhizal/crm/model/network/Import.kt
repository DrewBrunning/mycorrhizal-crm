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
data class ImportRowPreview(
    @Json(name = "row_index") val rowIndex: Int = 0,
    @Json(name = "parsed_contact") val parsedContact: Map<String, Any?> = emptyMap(),
    @Json(name = "validation_errors") val validationErrors: List<String> = emptyList(),
    @Json(name = "duplicate_match") val duplicateMatch: DuplicateMatch? = null,
    @Json(name = "suggested_action") val suggestedAction: String = "add",
    val diagnostics: List<String>? = null,
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

@JsonClass(generateAdapter = true)
data class ImportResult(
    @Json(name = "total_processed") val totalProcessed: Int = 0,
    val created: Int = 0,
    val updated: Int = 0,
    val skipped: Int = 0,
    val errors: List<String> = emptyList(),
)
