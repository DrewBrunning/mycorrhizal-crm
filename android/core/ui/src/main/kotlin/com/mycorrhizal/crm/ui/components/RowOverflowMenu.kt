package com.mycorrhizal.crm.ui.components

import androidx.compose.foundation.layout.Box
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.ui.R

/**
 * Issue #1760: a list row's secondary actions (rename, delete, …) live behind
 * one `⋮` button instead of a pair of icon buttons per row. The trigger's
 * TalkBack label carries the row's [rowName], so two rows never announce the
 * same bare "More actions" (#205).
 *
 * Rule (docs/android-app.md, "Design system"): a row with one secondary action
 * may show it inline; two or more go in this menu.
 */
@Composable
fun RowOverflowMenu(rowName: String, actions: List<RowAction>, modifier: Modifier = Modifier) {
    var expanded by remember { mutableStateOf(false) }
    // The Box anchors the popup to the button instead of to the row.
    Box(modifier = modifier) {
        AccessibleIconButton(onClick = { expanded = true }) {
            Icon(
                Icons.Outlined.MoreVert,
                contentDescription = stringResource(R.string.row_more_actions_named, rowName)
            )
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            actions.forEach { action ->
                DropdownMenuItem(
                    text = {
                        Text(
                            text = action.label,
                            color = if (action.destructive) {
                                MaterialTheme.colorScheme.error
                            } else {
                                MaterialTheme.colorScheme.onSurface
                            }
                        )
                    },
                    onClick = {
                        expanded = false
                        action.onClick()
                    }
                )
            }
        }
    }
}
