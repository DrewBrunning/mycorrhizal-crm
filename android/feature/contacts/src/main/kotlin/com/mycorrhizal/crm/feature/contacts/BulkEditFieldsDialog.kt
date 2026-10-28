package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.BULK_EDITABLE_FIELDS
import com.mycorrhizal.crm.ui.R

/**
 * Issue #2140: bulk "Edit fields" dialog (web parity). One text field plus a Clear checkbox per
 * allowlisted core field; blank + unchecked = untouched, Clear checked disables the text field.
 * Apply stays disabled until something is filled or cleared.
 */
@Composable
internal fun BulkEditFieldsDialog(
    selectedCount: Int,
    onApply: (values: Map<String, String>, clear: Set<String>) -> Unit,
    onDismiss: () -> Unit
) {
    val values = remember { mutableStateMapOf<String, String>() }
    val cleared = remember { mutableStateMapOf<String, Boolean>() }
    val clearKeys = cleared.filterValues { it }.keys
    val canApply = clearKeys.isNotEmpty() || values.any { (key, value) ->
        key !in clearKeys && value.isNotBlank()
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(
                pluralStringResource(R.plurals.bulk_edit_fields_title, selectedCount, selectedCount)
            )
        },
        text = {
            Column(
                verticalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.verticalScroll(rememberScrollState())
            ) {
                Text(
                    stringResource(R.string.bulk_edit_fields_hint),
                    style = MaterialTheme.typography.bodySmall
                )
                BULK_EDITABLE_FIELDS.forEach { key ->
                    val isCleared = cleared[key] == true
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        OutlinedTextField(
                            value = values[key].orEmpty(),
                            onValueChange = { values[key] = it },
                            enabled = !isCleared,
                            label = { Text(stringResource(bulkFieldLabel(key))) },
                            singleLine = true,
                            modifier = Modifier
                                .weight(1f)
                                .testTag("bulk-field-$key")
                        )
                        Checkbox(
                            checked = isCleared,
                            onCheckedChange = { cleared[key] = it },
                            modifier = Modifier.testTag("bulk-field-clear-$key")
                        )
                        Text(
                            stringResource(R.string.bulk_edit_fields_clear),
                            style = MaterialTheme.typography.labelSmall
                        )
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onApply(values.toMap(), clearKeys.toSet()) },
                enabled = canApply,
                modifier = Modifier.testTag("bulk-fields-apply")
            ) {
                Text(stringResource(R.string.bulk_edit_fields_apply))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    )
}

private fun bulkFieldLabel(key: String): Int = when (key) {
    "organization" -> R.string.bulk_field_organization
    "department" -> R.string.bulk_field_department
    "job_title" -> R.string.bulk_field_job_title
    "role" -> R.string.bulk_field_role
    "gender" -> R.string.bulk_field_gender
    "how_we_met" -> R.string.bulk_field_how_we_met
    "work_information" -> R.string.bulk_field_work_information
    else -> R.string.bulk_field_contact_information
}
