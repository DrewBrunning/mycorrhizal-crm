package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.InsertDriveFile
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.WebDAVItem
import com.mycorrhizal.crm.ui.R

/**
 * Issue #236: the Nextcloud/WebDAV file picker, mirroring web's
 * `NextcloudFilePickerDialog.tsx`. Single-level directory browse at
 * [browsePath] (defaults to the dav root "/"); navigation is driven by the
 * ViewModel's `loadNextcloudDir` — this composable is stateless. Tapping a
 * directory calls [onEnterDir]; tapping a file hands it to [onSelect].
 */
@Composable
fun NextcloudFilePickerDialog(
    browsePath: String,
    browseItems: List<WebDAVItem>,
    loading: Boolean,
    onEnterDir: (String) -> Unit,
    onSelect: (WebDAVItem) -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.nextcloud_picker_title)) },
        text = {
            Column(modifier = Modifier.fillMaxWidth()) {
                Breadcrumbs(path = browsePath)
                if (browsePath != "/") {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.End,
                    ) {
                        TextButton(onClick = { onEnterDir(parentDirPath(browsePath) ?: "/") }) {
                            Text(stringResource(R.string.action_back))
                        }
                    }
                }
                when {
                    loading -> Box(
                        modifier = Modifier.fillMaxWidth().padding(24.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator()
                    }
                    browseItems.isEmpty() -> EmptyPickerText(stringResource(R.string.nextcloud_picker_empty_folder))
                    else -> {
                        LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                            items(browseItems, key = { it.path }) { item ->
                                val isDir = item.type == "dir"
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clickable { if (isDir) onEnterDir(item.path) else onSelect(item) }
                                        .padding(vertical = 10.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                                ) {
                                    Icon(
                                        if (isDir) Icons.Outlined.Folder else Icons.Outlined.InsertDriveFile,
                                        contentDescription = null,
                                    )
                                    Column {
                                        Text(
                                            item.name,
                                            style = MaterialTheme.typography.bodyLarge,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis,
                                        )
                                        val size = item.size
                                        if (!isDir && size != null) {
                                            Text(
                                                formatFileSize(size),
                                                style = MaterialTheme.typography.bodySmall,
                                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                            )
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}
