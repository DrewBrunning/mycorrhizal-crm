package com.mycorrhizal.crm.feature.imports

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.ImportAddedValue
import com.mycorrhizal.crm.model.network.ImportMergeDiff
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.ui.R

/**
 * T96: the shared per-row merge-decision review step, used by both the
 * VCF-file
 * import screen and the device-contacts import screen. Each row shows the
 * contact, its match/diff summary (what "Merge" will change), and the
 * Merge / Keep Both / Discard New choice. The confirm button reports the
 * number of decisions that will be applied.
 */
@Composable
fun ImportReviewStep(
    rows: List<ImportRowPreview>,
    rowActions: Map<Int, String>,
    onRowActionChange: (Int, String) -> Unit,
    onResolveAll: () -> Unit,
    onConfirm: () -> Unit,
) {
    val conflictsRemaining = rows.count { row ->
        row.validationErrors.isEmpty() &&
            (row.duplicateMatch != null || row.batchDuplicateOf != null) &&
            (rowActions[row.rowIndex] ?: row.suggestedAction) == row.suggestedAction
    }
    val decisions = rows.count { it.validationErrors.isEmpty() }

    Column(modifier = Modifier.fillMaxSize()) {
        Text(
            text = if (conflictsRemaining > 0) {
                stringResource(R.string.import_review_remaining, conflictsRemaining)
            } else {
                stringResource(R.string.import_review_no_conflicts)
            },
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        TextButton(onClick = onResolveAll, modifier = Modifier.padding(horizontal = 8.dp)) {
            Text(stringResource(R.string.import_resolve_all))
        }
        LazyColumn(modifier = Modifier.weight(1f).testTag("import-review-list")) {
            items(rows, key = { it.rowIndex }) { row ->
                ImportRowCard(
                    row = row,
                    action = rowActions[row.rowIndex] ?: row.suggestedAction,
                    onActionChange = { action -> onRowActionChange(row.rowIndex, action) },
                )
            }
        }
        Button(
            onClick = onConfirm,
            modifier = Modifier.fillMaxWidth().padding(16.dp),
        ) {
            Text(stringResource(R.string.import_apply_decisions, decisions))
        }
    }
}

@Composable
private fun ImportRowCard(
    row: ImportRowPreview,
    action: String,
    onActionChange: (String) -> Unit,
) {
    val hasErrors = row.validationErrors.isNotEmpty()
    // Local copies so the `when` branches can smart-cast (public API property
    // from another module cannot be smart-cast directly).
    val duplicateMatch = row.duplicateMatch
    val batchDuplicateOf = row.batchDuplicateOf
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(row.displayName(), style = MaterialTheme.typography.bodyLarge)
        val sub = listOfNotNull(row.parsedContact["email"] as? String, row.parsedContact["phone"] as? String)
            .filter { it.isNotBlank() }
            .joinToString(" · ")
        if (sub.isNotEmpty()) {
            Text(
                text = sub,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        when {
            hasErrors -> row.validationErrors.forEach { error ->
                Text(
                    text = error,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                )
            }
            duplicateMatch != null -> {
                val existing = listOfNotNull(duplicateMatch.existingFirstname, duplicateMatch.existingLastname)
                    .joinToString(" ")
                    .ifBlank { "#${duplicateMatch.existingContactId}" }
                Text(
                    text = stringResource(
                        R.string.import_matches,
                        existing,
                        stringResource(reasonRes(duplicateMatch.matchReason)),
                    ),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                )
            }
            batchDuplicateOf != null -> Text(
                text = stringResource(R.string.import_batch_duplicate_of, batchDuplicateOf + 1),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
            )
            else -> Text(
                text = stringResource(R.string.import_new_contact),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        row.mergeDiff?.let { MergeDiffSummary(it) }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(top = 4.dp)) {
            listOf(
                "update" to stringResource(R.string.import_action_merge),
                "add" to stringResource(R.string.import_action_keep_both),
                "skip" to stringResource(R.string.import_action_discard),
            ).forEach { (value, label) ->
                FilterChip(
                    selected = action == value,
                    onClick = { if (!hasErrors) onActionChange(value) },
                    enabled = !hasErrors && (value != "update" || row.duplicateMatch != null),
                    label = { Text(label) },
                )
            }
        }
    }
}

@Composable
private fun MergeDiffSummary(diff: ImportMergeDiff) {
    if (diff.updated.isEmpty() && diff.added.isEmpty()) {
        Text(
            text = stringResource(R.string.import_diff_none),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        return
    }
    Text(
        text = stringResource(R.string.import_diff_title),
        style = MaterialTheme.typography.labelMedium,
        modifier = Modifier.padding(top = 4.dp),
    )
    diff.added.forEach { added ->
        Text(
            text = stringResource(
                R.string.import_diff_added,
                stringResource(kindRes(added.kind)),
                added.value,
            ),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
    diff.updated.forEach { updated ->
        Text(
            text = stringResource(
                R.string.import_diff_updated,
                updated.label,
                updated.old.ifBlank { "\u2014" },
                updated.new,
            ),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

private fun reasonRes(reason: String?): Int = when (reason) {
    "email" -> R.string.import_reason_email
    "name" -> R.string.import_reason_name
    "phone" -> R.string.import_reason_phone
    else -> R.string.import_reason_name
}

private fun kindRes(kind: String): Int = when (kind) {
    "email" -> R.string.import_diff_kind_email
    "phone" -> R.string.import_diff_kind_phone
    "address" -> R.string.import_diff_kind_address
    "url" -> R.string.import_diff_kind_url
    "impp" -> R.string.import_diff_kind_impp
    else -> R.string.import_diff_kind_email
}

private fun ImportRowPreview.displayName(): String {
    val first = parsedContact["firstname"] as? String
    val last = parsedContact["lastname"] as? String
    return listOfNotNull(first, last).joinToString(" ").ifBlank { "#${rowIndex + 1}" }
}
