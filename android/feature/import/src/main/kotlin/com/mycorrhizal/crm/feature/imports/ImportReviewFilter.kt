package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.ImportRowEdit
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

/**
 * [filter] null means "show everything". A row whose validation errors were fixed by a saved
 * edit in [rowEdits] (issue #2611) is no longer an error row: it is re-checked on import, so it
 * counts as new (a fixed row has no match yet) and drops out of the Errors group.
 */
internal fun ImportRowPreview.matches(
    filter: ImportReviewFilter?,
    rowEdits: Map<Int, ImportRowEdit> = emptyMap()
): Boolean {
    val hasOpenErrors = validationErrors.isNotEmpty() && rowIndex !in rowEdits
    return when (filter) {
        null -> true
        ImportReviewFilter.ERRORS -> hasOpenErrors
        ImportReviewFilter.DUPLICATES -> !hasOpenErrors && isDuplicate()
        ImportReviewFilter.NEW -> !hasOpenErrors && !isDuplicate()
    }
}

/**
 * Issue #2611: the row as it will be re-checked, with the user's saved fix applied to the parsed
 * values the card summarises. A non-null empty value removes the entry, as the server does.
 */
internal fun ImportRowPreview.withEdit(edit: ImportRowEdit?): ImportRowPreview {
    if (edit == null) return this
    val merged = parsedContact.toMutableMap()
    listOf(
        "firstname" to edit.firstname,
        "lastname" to edit.lastname,
        "email" to edit.email,
        "phone" to edit.phone
    ).forEach { (key, value) ->
        if (value == null) return@forEach
        if (value.isEmpty()) merged.remove(key) else merged[key] = value
    }
    return copy(parsedContact = merged)
}

/**
 * Issue #2610: the fix form edits name, email and phone only. A row with any other validation
 * error (an unparseable card, or a birthday/anniversary from a server older than #2653) can never
 * be made valid there, so the form is not offered and the row says what to do instead. The
 * messages are the backend's `ValidateImportedContact` / parse failures; the fixable ones name a
 * name, email or phone. A current server no longer errors on an invalid birthday or anniversary:
 * it drops the date and attaches a warning diagnostic, which is not a validation error, so such a
 * row is valid, importable and never reaches this check.
 */
internal fun ImportRowPreview.hasUnfixableError(): Boolean = validationErrors.any { error ->
    val text = error.lowercase()
    text.startsWith("failed to parse") || FIXABLE_ERROR_WORDS.none { it in text }
}

private val FIXABLE_ERROR_WORDS = listOf("name", "email", "phone")

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
