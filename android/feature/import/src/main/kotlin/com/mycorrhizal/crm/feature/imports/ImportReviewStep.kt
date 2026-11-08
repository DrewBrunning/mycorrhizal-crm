package com.mycorrhizal.crm.feature.imports

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.ImportMergeDiff
import com.mycorrhizal.crm.model.network.ImportRowEdit
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.ui.R

/**
 * T96: the shared per-row merge-decision review step, used by the VCF, CSV, Meerkat, bundle and
 * device-contacts import screens. Each row shows the contact, its match/diff summary (what
 * "Merge" will change) and its decision: Keep / Skip for a row without a duplicate, Keep both /
 * Merge / Discard new for a duplicate (issue #2185). The chips under the header filter the list
 * to new rows, duplicates or errors (issue #2184). Error rows can be fixed in place when the
 * flow passes [onRowEditSave] (issue #2186; only the CSV/VCF/records confirm endpoints accept
 * `row_edits`). The confirm button reports the number of decisions that will be applied.
 */
@Composable
fun ImportReviewStep(
    rows: List<ImportRowPreview>,
    rowActions: Map<Int, String>,
    onRowActionChange: (Int, String) -> Unit,
    onResolveAll: () -> Unit,
    onConfirm: () -> Unit,
    modifier: Modifier = Modifier,
    rowEdits: Map<Int, ImportRowEdit> = emptyMap(),
    onRowEditSave: ((ImportRowEdit) -> Unit)? = null
) {
    var filter by rememberSaveable { mutableStateOf<ImportReviewFilter?>(null) }
    val conflictsRemaining = rows.count { row ->
        row.validationErrors.isEmpty() &&
            (row.duplicateMatch != null || row.batchDuplicateOf != null) &&
            (rowActions[row.rowIndex] ?: row.suggestedAction) == row.suggestedAction
    }
    val decisions = rows.count { it.validationErrors.isEmpty() || it.rowIndex in rowEdits }
    val visibleRows = rows.filter { it.matches(filter) }

    Column(modifier = modifier.fillMaxSize()) {
        Text(
            text = if (conflictsRemaining > 0) {
                pluralStringResource(
                    R.plurals.import_review_remaining,
                    conflictsRemaining,
                    conflictsRemaining
                )
            } else {
                stringResource(R.string.import_review_no_conflicts)
            },
            style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
        )
        TextButton(onClick = onResolveAll, modifier = Modifier.padding(horizontal = 8.dp)) {
            Text(stringResource(R.string.import_resolve_all))
        }
        FilterChips(
            rows = rows,
            selected = filter,
            onSelect = { picked -> filter = if (filter == picked) null else picked }
        )
        LazyColumn(modifier = Modifier.weight(1f).testTag("import-review-list")) {
            items(visibleRows, key = { it.rowIndex }) { row ->
                ImportRowCard(
                    row = row,
                    action = rowActions[row.rowIndex] ?: row.suggestedAction,
                    onActionChange = { action -> onRowActionChange(row.rowIndex, action) },
                    edit = rowEdits[row.rowIndex],
                    onEditSave = onRowEditSave
                )
            }
        }
        Button(
            onClick = onConfirm,
            modifier = Modifier.fillMaxWidth().padding(16.dp)
        ) {
            Text(stringResource(R.string.import_apply_decisions, decisions))
        }
    }
}

/** Issue #2184: tapping a chip filters the list to that group; tapping it again clears it. */
@Composable
private fun FilterChips(
    rows: List<ImportRowPreview>,
    selected: ImportReviewFilter?,
    onSelect: (ImportReviewFilter) -> Unit
) {
    Row(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.horizontalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 4.dp)
    ) {
        listOf(
            Triple(ImportReviewFilter.NEW, R.string.import_filter_new, "import-filter-new"),
            Triple(
                ImportReviewFilter.DUPLICATES,
                R.string.import_filter_duplicates,
                "import-filter-duplicates"
            ),
            Triple(ImportReviewFilter.ERRORS, R.string.import_filter_errors, "import-filter-errors")
        ).forEach { (filter, label, tag) ->
            FilterChip(
                selected = selected == filter,
                onClick = { onSelect(filter) },
                label = { Text(stringResource(label, rows.count { it.matches(filter) })) },
                modifier = Modifier.testTag(tag)
            )
        }
    }
}

@Composable
private fun ImportRowCard(
    row: ImportRowPreview,
    action: String,
    onActionChange: (String) -> Unit,
    edit: ImportRowEdit?,
    onEditSave: ((ImportRowEdit) -> Unit)?
) {
    val hasErrors = row.validationErrors.isNotEmpty()
    // Issue #2186: a saved fix re-enables the decision chips for an errored row.
    val fixed = hasErrors && edit != null
    // Local copies so the `when` branches can smart-cast (public API property
    // from another module cannot be smart-cast directly).
    val duplicateMatch = row.duplicateMatch
    val batchDuplicateOf = row.batchDuplicateOf
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(row.displayName(), style = MaterialTheme.typography.bodyLarge)
        val sub = listOfNotNull(
            row.parsedContact["email"] as? String,
            row.parsedContact["phone"] as? String
        )
            .filter { it.isNotBlank() }
            .joinToString(" · ")
        if (sub.isNotEmpty()) {
            Text(
                text = sub,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        when {
            hasErrors -> row.validationErrors.forEach { error ->
                Text(
                    text = error,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive }
                )
            }

            duplicateMatch != null -> {
                val existing = listOfNotNull(
                    duplicateMatch.existingFirstname,
                    duplicateMatch.existingLastname
                )
                    .joinToString(" ")
                    .ifBlank { "#${duplicateMatch.existingContactId}" }
                Text(
                    text = stringResource(
                        R.string.import_matches,
                        existing,
                        stringResource(reasonRes(duplicateMatch.matchReason))
                    ),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive }
                )
            }

            batchDuplicateOf != null -> Text(
                text = stringResource(R.string.import_batch_duplicate_of, batchDuplicateOf + 1),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive }
            )

            else -> Text(
                text = stringResource(R.string.import_new_contact),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        if (hasErrors && onEditSave != null) {
            RowFixSection(row = row, edit = edit, onSave = onEditSave)
        }
        row.mergeDiff?.let { MergeDiffSummary(it) }
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.padding(top = 4.dp)
        ) {
            val duplicate = row.isDuplicate()
            row.availableActions().forEach { value ->
                val label = when (value) {
                    "add" -> if (duplicate) R.string.import_action_keep_both else R.string.import_action_keep
                    "update" -> R.string.import_action_merge
                    else -> if (duplicate) R.string.import_action_discard else R.string.import_action_skip
                }
                FilterChip(
                    selected = action == value,
                    onClick = { if (!hasErrors || fixed) onActionChange(value) },
                    enabled = !hasErrors || fixed,
                    label = { Text(stringResource(label)) }
                )
            }
        }
    }
}

/** Issue #2186: inline fix form for a row that failed validation. */
@Composable
private fun RowFixSection(
    row: ImportRowPreview,
    edit: ImportRowEdit?,
    onSave: (ImportRowEdit) -> Unit
) {
    var fixing by rememberSaveable(row.rowIndex) { mutableStateOf(false) }
    if (!fixing) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            TextButton(onClick = { fixing = true }) {
                Text(
                    stringResource(
                        if (edit != null) R.string.import_row_fix_edit else R.string.import_row_fix
                    )
                )
            }
            if (edit != null) {
                Text(
                    text = stringResource(R.string.import_row_fix_saved),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.primary
                )
            }
        }
        return
    }
    val original = FIX_FIELDS.associateWith { (row.parsedContact[it] as? String).orEmpty() }
    val draft = remember(row.rowIndex) {
        mutableStateOf(
            FIX_FIELDS.associateWith { key ->
                when (key) {
                    "firstname" -> edit?.firstname
                    "lastname" -> edit?.lastname
                    "email" -> edit?.email
                    else -> edit?.phone
                } ?: original.getValue(key)
            }
        )
    }
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        FIX_FIELDS.forEach { key ->
            OutlinedTextField(
                value = draft.value.getValue(key),
                onValueChange = { draft.value = draft.value + (key to it) },
                label = { Text(stringResource(fixLabelRes(key))) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("import-fix-$key")
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = {
                // Only the fields the user actually changed are sent; "" removes an entry.
                val changed = FIX_FIELDS.filter {
                    draft.value.getValue(
                        it
                    ) != original.getValue(it)
                }
                    .associateWith { draft.value.getValue(it) }
                onSave(
                    ImportRowEdit(
                        rowIndex = row.rowIndex,
                        firstname = changed["firstname"],
                        lastname = changed["lastname"],
                        email = changed["email"],
                        phone = changed["phone"]
                    )
                )
                fixing = false
            }) { Text(stringResource(R.string.import_row_fix_save)) }
            TextButton(
                onClick = { fixing = false }
            ) { Text(stringResource(R.string.action_cancel)) }
        }
    }
}

private val FIX_FIELDS = listOf("firstname", "lastname", "email", "phone")

private fun fixLabelRes(key: String): Int = when (key) {
    "firstname" -> R.string.import_field_firstname
    "lastname" -> R.string.import_field_lastname
    "email" -> R.string.import_field_email
    else -> R.string.import_field_phone
}

@Composable
private fun MergeDiffSummary(diff: ImportMergeDiff) {
    if (diff.updated.isEmpty() && diff.added.isEmpty()) {
        Text(
            text = stringResource(R.string.import_diff_none),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        return
    }
    Text(
        text = stringResource(R.string.import_diff_title),
        style = MaterialTheme.typography.labelMedium,
        modifier = Modifier.padding(top = 4.dp)
    )
    diff.added.forEach { added ->
        Text(
            text = stringResource(
                R.string.import_diff_added,
                stringResource(kindRes(added.kind)),
                added.value
            ),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
    diff.updated.forEach { updated ->
        Text(
            text = stringResource(
                R.string.import_diff_updated,
                updated.label,
                updated.old.ifBlank { "\u2014" },
                updated.new
            ),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
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
