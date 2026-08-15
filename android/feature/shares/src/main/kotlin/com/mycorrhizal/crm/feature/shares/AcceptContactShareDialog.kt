package com.mycorrhizal.crm.feature.shares

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.ui.R

/**
 * M15: the accept flow for an incoming share (mirrors web's
 * AcceptContactShareDialog). Accept is preview-then-confirm, never a one-tap
 * button: the recipient explicitly picks add (create new) or update (merge via
 * the existing MergeImportedContact policy) or skip per row, then confirms.
 * A share is always exactly one contact, so there is exactly one row.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AcceptContactShareDialog(
    previewLoading: Boolean,
    preview: ImportPreviewResponse?,
    previewError: String?,
    confirmAction: String,
    confirming: Boolean,
    onActionChange: (String) -> Unit,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = { if (!confirming) onDismiss() },
        title = { Text(stringResource(R.string.shares_accept_dialog_title)) },
        text = {
            when {
                previewLoading -> {
                    Column(
                        modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp),
                        horizontalAlignment = androidx.compose.ui.Alignment.CenterHorizontally,
                    ) {
                        CircularProgressIndicator(modifier = Modifier.padding(vertical = 8.dp))
                    }
                }
                previewError != null -> {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Icon(
                            imageVector = Icons.Outlined.WarningAmber,
                            contentDescription = null,
                            tint = MaterialTheme.colorScheme.error,
                        )
                        Text(
                            text = previewError,
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                        )
                    }
                }
                else -> {
                    val row = preview?.rows?.firstOrNull()
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        val duplicate = row?.duplicateMatch
                        if (duplicate != null) {
                            val existingName = listOfNotNull(
                                duplicate.existingFirstname,
                                duplicate.existingLastname,
                            ).joinToString(" ").ifBlank { "#${duplicate.existingContactId}" }
                            Text(
                                text = stringResource(
                                    R.string.shares_accept_dialog_duplicate_of,
                                    existingName,
                                    duplicate.matchReason.orEmpty(),
                                ),
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.tertiary,
                            )
                        }
                        val options = buildList {
                            add("add" to stringResource(R.string.shares_accept_dialog_action_add))
                            if (duplicate != null) {
                                add("update" to stringResource(R.string.shares_accept_dialog_action_update))
                            }
                            add("skip" to stringResource(R.string.shares_accept_dialog_action_skip))
                        }
                        options.forEach { (value, label) ->
                            val selected = confirmAction == value
                            OutlinedButton(
                                onClick = { onActionChange(value) },
                                modifier = Modifier.fillMaxWidth(),
                                enabled = !confirming,
                            ) {
                                Text(
                                    text = label,
                                    color = if (selected) {
                                        MaterialTheme.colorScheme.primary
                                    } else {
                                        MaterialTheme.colorScheme.onSurface
                                    },
                                )
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = onConfirm,
                enabled = !confirming && !previewLoading && previewError == null &&
                    preview?.rows?.firstOrNull() != null,
            ) {
                Text(
                    if (confirming) {
                        stringResource(R.string.shares_accept_dialog_confirming)
                    } else {
                        stringResource(R.string.shares_accept_dialog_confirm)
                    },
                )
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !confirming) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}
