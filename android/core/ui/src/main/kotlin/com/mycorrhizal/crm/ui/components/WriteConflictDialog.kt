package com.mycorrhizal.crm.ui.components

import androidx.annotation.StringRes
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.ui.R

/**
 * ADR 0008/0009 (issue #1665): shown when a PUT/DELETE answers 412 because the record changed
 * elsewhere since this screen loaded it. There is deliberately no merge — the person either
 * discards their edit and reloads the server's version ([onLoadLatest]) or dismisses the
 * dialog ([onKeep]) to keep what they typed on screen and copy it out.
 *
 * Edit and delete conflicts share the dialog; [messageRes] and [keepLabelRes] carry the
 * wording difference (nothing was saved vs nothing was deleted).
 */
@Composable
fun WriteConflictDialog(
    onLoadLatest: () -> Unit,
    onKeep: () -> Unit,
    @StringRes messageRes: Int = R.string.conflict_message_edit,
    @StringRes keepLabelRes: Int = R.string.conflict_keep_editing,
) {
    AlertDialog(
        onDismissRequest = onKeep,
        title = { Text(stringResource(R.string.conflict_title)) },
        text = { Text(stringResource(messageRes)) },
        confirmButton = {
            TextButton(onClick = onLoadLatest) { Text(stringResource(R.string.conflict_load_latest)) }
        },
        dismissButton = {
            TextButton(onClick = onKeep) { Text(stringResource(keepLabelRes)) }
        },
    )
}
