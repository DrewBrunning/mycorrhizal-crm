package com.mycorrhizal.crm.feature.imports

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.model.network.ImportDiagnostic
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.ui.R

/** Diagnostic severities on the wire (`contactmodel.Diagnostic`). */
internal const val DIAGNOSTIC_WARN = "warn"

/** Concepts the server keys import warnings by (issues #2653, #2731). */
private const val CONCEPT_BIRTHDAY = "birthday"
private const val CONCEPT_ANNIVERSARY = "anniversary"
private const val CONCEPT_ENCODING = "encoding"

/**
 * The translated text for a diagnostic, keyed on concept and severity (issues #2653, #2731), or
 * null for a pair this build does not know: the server's [ImportDiagnostic.message] is English
 * text and is only the fallback.
 */
@StringRes
internal fun importDiagnosticRes(concept: String, severity: String): Int? {
    if (severity != DIAGNOSTIC_WARN) return null
    return when (concept) {
        CONCEPT_BIRTHDAY -> R.string.import_diag_birthday_warn
        CONCEPT_ANNIVERSARY -> R.string.import_diag_anniversary_warn
        CONCEPT_ENCODING -> R.string.import_diag_encoding_warn
        else -> null
    }
}

@Composable
internal fun ImportDiagnostic.localizedText(): String =
    importDiagnosticRes(concept, severity)?.let { stringResource(it) } ?: message

/** The result-report wording for a `warning` issue (a row that imported without a date). */
@StringRes
internal fun importWarningIssueRes(concept: String): Int? = when (concept) {
    CONCEPT_BIRTHDAY -> R.string.import_report_warning_birthday
    CONCEPT_ANNIVERSARY -> R.string.import_report_warning_anniversary
    else -> null
}

/**
 * The report line for one issue. Skipped and failed rows keep the server's reason; a warning is
 * localized by its concept; a status this client does not know (a newer server) is a neutral note
 * rather than being mislabelled as skipped.
 */
@Composable
internal fun ImportRowIssue.localizedLine(): String = when (status) {
    "failed" -> stringResource(R.string.import_report_failed, reason)

    "skipped" -> stringResource(R.string.import_report_skipped, reason)

    "warning" -> importWarningIssueRes(concept)?.let { stringResource(it) }
        ?: stringResource(R.string.import_report_warning, reason)

    else -> stringResource(R.string.import_report_note, reason)
}

/** Rows that were left out of the import. */
private fun ImportRowIssue.notImported() = status == "skipped" || status == "failed"

/**
 * The report heading: red "Rows not imported" when only skipped or failed rows are listed,
 * neutral "Imported with warnings" when every row did import, and a combined heading for a mix.
 */
@StringRes
internal fun importReportTitleRes(issues: List<ImportRowIssue>): Int = when {
    issues.none { it.notImported() } -> R.string.import_report_title_warnings
    issues.all { it.notImported() } -> R.string.import_report_title
    else -> R.string.import_report_title_mixed
}
