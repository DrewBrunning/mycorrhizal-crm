package com.mycorrhizal.crm.ui.components

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.ui.R

/**
 * Issue #1751: the confirmation shown before an in-progress create/edit form is discarded.
 * [entityLabel] is the localized noun for what is being edited ("contact", "note", …), so the
 * body reads naturally in each locale.
 */
@Composable
fun DiscardChangesDialog(
    entityLabel: String,
    onKeepEditing: () -> Unit,
    onDiscard: () -> Unit,
    modifier: Modifier = Modifier,
) {
    AlertDialog(
        onDismissRequest = onKeepEditing,
        modifier = modifier.testTag("discard-changes-dialog"),
        title = { Text(stringResource(R.string.unsaved_changes_title)) },
        text = { Text(stringResource(R.string.unsaved_changes_body, entityLabel)) },
        confirmButton = {
            TextButton(onClick = onDiscard) {
                Text(
                    text = stringResource(R.string.unsaved_changes_discard),
                    color = MaterialTheme.colorScheme.error,
                )
            }
        },
        dismissButton = {
            TextButton(onClick = onKeepEditing) {
                Text(stringResource(R.string.unsaved_changes_keep_editing))
            }
        },
    )
}
