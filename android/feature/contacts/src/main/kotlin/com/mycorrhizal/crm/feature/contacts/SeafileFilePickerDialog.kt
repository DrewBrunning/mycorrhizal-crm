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
import com.mycorrhizal.crm.model.network.SeafileItem
import com.mycorrhizal.crm.model.network.SeafileLibrary
import com.mycorrhizal.crm.ui.R
import kotlin.math.ln
import kotlin.math.pow

/**
 * Issue #236: the Seafile file picker, mirroring web's
 * `SeafileFilePickerDialog.tsx`. Two-level navigation: [browseRepoId] null
 * means the library list ([libraries]); non-null means a directory browse
 * within that library ([browseItems] at [browsePath]). All navigation is
 * driven by the ViewModel (`enterSeafileDir`/`backSeafileDir`) — this
 * composable is stateless. Tapping a file hands it to [onSelect]; tapping a
 * library or a directory calls [onEnterLibrary]/[onEnterDir].
 */
@Composable
fun SeafileFilePickerDialog(
    libraries: List<SeafileLibrary>,
    browseRepoId: String?,
    browsePath: String,
    browseItems: List<SeafileItem>,
    loading: Boolean,
    onEnterLibrary: (String) -> Unit,
    onEnterDir: (String) -> Unit,
    onBack: () -> Unit,
    onSelect: (SeafileItem) -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.seafile_picker_title)) },
        text = {
            Column(modifier = Modifier.fillMaxWidth()) {
                if (browseRepoId != null) {
                    Breadcrumbs(path = browsePath)
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.End,
                    ) {
                        TextButton(onClick = onBack) { Text(stringResource(R.string.action_back)) }
                    }
                }
                when {
                    loading -> Box(
                        modifier = Modifier.fillMaxWidth().padding(24.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator()
                    }
                    browseRepoId == null -> {
                        if (libraries.isEmpty()) {
                            EmptyPickerText(stringResource(R.string.seafile_picker_no_libraries))
                        } else {
                            LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                                items(libraries, key = { it.id }) { library ->
                                    Row(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .clickable { onEnterLibrary(library.id) }
                                            .padding(vertical = 10.dp),
                                        verticalAlignment = Alignment.CenterVertically,
                                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                                    ) {
                                        Icon(Icons.Outlined.Folder, contentDescription = null)
                                        Text(library.name, style = MaterialTheme.typography.bodyLarge)
                                    }
                                }
                            }
                        }
                    }
                    else -> {
                        if (browseItems.isEmpty()) {
                            EmptyPickerText(stringResource(R.string.seafile_picker_empty_folder))
                        } else {
                            LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                                items(browseItems, key = { it.id }) { item ->
                                    val isDir = item.type == "dir"
                                    Row(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .clickable {
                                                if (isDir) {
                                                    onEnterDir(joinDirPath(browsePath, item.name))
                                                } else {
                                                    onSelect(item)
                                                }
                                            }
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
                                            if (!isDir) {
                                                Text(
                                                    formatFileSize(item.size),
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
            }
        },
        confirmButton = {},
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

/** A `/`-separated breadcrumb strip for the current browse [path] (root always shown). */
@Composable
internal fun Breadcrumbs(path: String) {
    val segments = pathSegments(path)
    Text(
        text = "/" + segments.joinToString("/"),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = Modifier.padding(bottom = 4.dp),
    )
}

@Composable
internal fun EmptyPickerText(text: String) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(vertical = 8.dp),
    )
}

/** B/KB/MB/GB formatting for a file picker row's size subtitle (mirrors web's `formatFileSize`). */
internal fun formatFileSize(bytes: Long): String {
    if (bytes <= 0) return "0 B"
    val units = arrayOf("B", "KB", "MB", "GB", "TB")
    val exponent = (ln(bytes.toDouble()) / ln(1024.0)).toInt().coerceIn(0, units.lastIndex)
    val value = bytes / 1024.0.pow(exponent)
    return if (exponent == 0) "$bytes B" else "%.1f %s".format(value, units[exponent])
}
