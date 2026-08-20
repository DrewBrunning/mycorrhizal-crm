package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.PaperlessDocument
import com.mycorrhizal.crm.ui.R

/**
 * Issue #236: the Paperless-ngx document picker, mirroring web's
 * `PaperlessDocumentSearchDialog.tsx`. The full document list is fetched once
 * when the dialog opens ([documents]/[loading] come from the ViewModel); the
 * search box filters that list client-side (title substring, case-insensitive)
 * — web never issues a server-side search from this dialog either, despite
 * the backend supporting `?query=`. Tapping a row hands it to [onSelect]; the
 * caller (ContactDetailScreen) closes the dialog immediately and fires the
 * link call, same pattern as the Immich picker.
 */
@Composable
fun PaperlessDocumentPickerDialog(
    documents: List<PaperlessDocument>,
    loading: Boolean,
    onSelect: (PaperlessDocument) -> Unit,
    onDismiss: () -> Unit,
) {
    var query by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.paperless_picker_title)) },
        text = {
            Column(modifier = Modifier.fillMaxWidth()) {
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    label = { Text(stringResource(R.string.paperless_picker_search_placeholder)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                Spacer(Modifier.height(8.dp))
                when {
                    loading -> Box(
                        modifier = Modifier.fillMaxWidth().padding(24.dp),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator()
                    }
                    else -> {
                        val filtered = documents.filter {
                            query.isBlank() || (it.title ?: "").contains(query.trim(), ignoreCase = true)
                        }
                        if (filtered.isEmpty()) {
                            Text(
                                text = stringResource(R.string.paperless_picker_no_matches),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.padding(vertical = 8.dp),
                            )
                        } else {
                            LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                                items(filtered, key = { it.id }) { document ->
                                    Column(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .clickable { onSelect(document) }
                                            .padding(vertical = 10.dp),
                                    ) {
                                        Text(
                                            text = document.title?.ifBlank { null }
                                                ?: stringResource(R.string.paperless_picker_untitled),
                                            style = MaterialTheme.typography.bodyLarge,
                                        )
                                        Text(
                                            text = document.fileName ?: "#${document.id}",
                                            style = MaterialTheme.typography.bodyMedium,
                                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        )
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
