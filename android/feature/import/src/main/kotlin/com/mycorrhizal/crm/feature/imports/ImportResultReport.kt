package com.mycorrhizal.crm.feature.imports

import android.content.Context
import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.ui.R
import java.io.File

/**
 * Issue #2187 (Android parity, #2210): the final import result — the created/updated/skipped
 * counts, then every skipped or failed row with its reason (`ImportResult.issues`) and a share
 * button that hands the list over as a CSV. A clean import shows no list. [onShareCsv] defaults
 * to the system share sheet; tests pass their own.
 */
@Composable
fun ImportResultReport(
    result: ImportResult,
    onDone: () -> Unit,
    modifier: Modifier = Modifier,
    onShareCsv: ((List<ImportRowIssue>) -> Unit)? = null,
    summary: String? = null,
    summaryTag: String? = null,
    doneTag: String? = null
) {
    val context = LocalContext.current
    val share = onShareCsv ?: { issues -> shareImportIssuesCsv(context, issues) }
    Column(
        horizontalAlignment = Alignment.CenterHorizontally,
        modifier = modifier.fillMaxSize().padding(24.dp)
    ) {
        Text(
            text = summary ?: pluralStringResource(
                R.plurals.import_vcf_result,
                result.created,
                result.created,
                result.updated,
                result.skipped
            ),
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.padding(top = 24.dp, bottom = 16.dp).optionalTestTag(summaryTag)
        )
        if (result.issues.isEmpty()) {
            Button(onClick = onDone, modifier = Modifier.optionalTestTag(doneTag)) {
                Text(stringResource(R.string.action_confirm))
            }
            return
        }
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween,
            modifier = Modifier.fillMaxWidth()
        ) {
            Text(
                text = stringResource(R.string.import_report_title),
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.error
            )
            OutlinedButton(onClick = { share(result.issues) }) {
                Text(stringResource(R.string.import_report_share))
            }
        }
        LazyColumn(modifier = Modifier.weight(1f).testTag("import-report-list")) {
            itemsIndexed(result.issues) { _, issue -> IssueRow(issue) }
        }
        Button(
            onClick = onDone,
            modifier = Modifier.padding(top = 8.dp).optionalTestTag(doneTag)
        ) {
            Text(stringResource(R.string.action_confirm))
        }
    }
}

private fun Modifier.optionalTestTag(tag: String?): Modifier =
    if (tag == null) this else testTag(tag)

@Composable
private fun IssueRow(issue: ImportRowIssue) {
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Text(
            text = stringResource(
                R.string.import_report_row,
                issue.row,
                issue.name.ifBlank { "—" }
            ),
            style = MaterialTheme.typography.bodyMedium
        )
        Text(
            text = stringResource(
                if (issue.status == "failed") {
                    R.string.import_report_failed
                } else {
                    R.string.import_report_skipped
                },
                issue.reason
            ),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}

/**
 * Writes the report CSV to the cache `exports/` directory (already granted by the app's
 * FileProvider paths) and opens the share sheet, so the user can save it to Files or send it on.
 */
internal fun shareImportIssuesCsv(context: Context, issues: List<ImportRowIssue>) {
    val dir = File(context.cacheDir, "exports").apply { mkdirs() }
    val file = File(dir, "import-report.csv")
    file.writeText(importIssuesToCsv(issues))
    val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", file)
    val intent = Intent(Intent.ACTION_SEND).apply {
        type = "text/csv"
        putExtra(Intent.EXTRA_STREAM, uri)
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    try {
        context.startActivity(
            Intent.createChooser(intent, context.getString(R.string.import_report_share_title))
        )
    } catch (_: Exception) {
        // No activity can handle the share — nothing to do.
    }
}
