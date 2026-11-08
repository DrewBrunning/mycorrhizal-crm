package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRowPreview

/**
 * Issue #2184 (Android parity, #2210): the review step's row filters. They mirror the web
 * summary badges: only valid rows are "new" or "duplicates"; an errored row is an error first.
 */
enum class ImportReviewFilter { NEW, DUPLICATES, ERRORS }

/** A row that matches an existing contact or an earlier row of the same file. */
internal fun ImportRowPreview.isDuplicate(): Boolean =
    duplicateMatch != null || batchDuplicateOf != null

/** [filter] null means "show everything". */
internal fun ImportRowPreview.matches(filter: ImportReviewFilter?): Boolean = when (filter) {
    null -> true
    ImportReviewFilter.ERRORS -> validationErrors.isNotEmpty()
    ImportReviewFilter.DUPLICATES -> validationErrors.isEmpty() && isDuplicate()
    ImportReviewFilter.NEW -> validationErrors.isEmpty() && !isDuplicate()
}

/**
 * Issue #2185: the wire actions a row offers, in display order. "Both" and "Discard new" only
 * mean something against a duplicate, and Merge needs an existing contact to merge into (a
 * within-batch twin has none).
 */
internal fun ImportRowPreview.availableActions(): List<String> = when {
    duplicateMatch != null -> listOf("add", "update", "skip")
    else -> listOf("add", "skip")
}

private val FORMULA_START = Regex("^[=+\\-@\\t\\r]")

private fun csvCell(value: String): String {
    val safe = if (FORMULA_START.containsMatchIn(value)) "'$value" else value
    return if (safe.any { it == '"' || it == ',' || it == '\r' || it == '\n' }) {
        "\"" + safe.replace("\"", "\"\"") + "\""
    } else {
        safe
    }
}

/**
 * Issue #2187: the skipped/failed rows as a CSV, byte-for-byte what the web report downloads.
 * Imported text is attacker-controlled, so a cell a spreadsheet would read as a formula gets a
 * leading apostrophe.
 */
fun importIssuesToCsv(issues: List<ImportRowIssue>): String {
    val lines = mutableListOf("Row,Name,Email,Phone,Status,Reason")
    issues.forEach {
        lines += listOf(
            it.row.toString(),
            csvCell(it.name),
            csvCell(it.email),
            csvCell(it.phone),
            it.status,
            csvCell(it.reason)
        ).joinToString(",")
    }
    return lines.joinToString("\r\n") + "\r\n"
}
