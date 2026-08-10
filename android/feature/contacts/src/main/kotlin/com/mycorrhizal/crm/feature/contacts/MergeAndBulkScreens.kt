package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Merge
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.BulkActions
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ContactMergeFieldConflict
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MergeContactsScreen(
    onBack: () -> Unit,
    keepId: Long,
    viewModel: MergeContactsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(keepId) {
        if (keepId != 0L) viewModel.setPair(keepId, 0L)
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    androidx.compose.material3.IconButton(onClick = onBack) {
                        androidx.compose.material3.Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.merge_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding).padding(16.dp)) {
            var otherId by remember { mutableStateOf("") }
            OutlinedTextField(
                value = otherId,
                onValueChange = { otherId = it },
                label = { Text(stringResource(R.string.merge_other_id)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            Button(
                onClick = { viewModel.setPair(keepId, otherId.toLongOrNull() ?: 0L); viewModel.preview() },
                enabled = otherId.toLongOrNull() != null && otherId.toLongOrNull() != keepId,
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
            ) { Text(stringResource(R.string.merge_preview)) }

            state.preview?.let { preview ->
                val conflicts = viewModel.allConflicts(preview)
                Text(
                    text = stringResource(
                        R.string.merge_summary,
                        preview.associationCounts.notes,
                        preview.associationCounts.activities,
                        preview.associationCounts.relationshipEdges,
                    ),
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.padding(vertical = 12.dp),
                )
                if (conflicts.isEmpty()) {
                    Text(stringResource(R.string.merge_no_conflicts), style = MaterialTheme.typography.bodyMedium)
                } else {
                    LazyColumn(modifier = Modifier.weight(1f)) {
                        items(conflicts, key = { it.field }) { conflict ->
                            ConflictRow(
                                conflict = conflict,
                                chosen = state.resolutions[conflict.field],
                                onChoose = { value -> viewModel.resolve(conflict.field, value) },
                            )
                        }
                    }
                }
                Button(
                    onClick = viewModel::commit,
                    enabled = !state.isCommitting && conflicts.all { state.resolutions.containsKey(it.field) },
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
                ) {
                    Text(stringResource(R.string.merge_commit))
                }
            }
            if (state.merged) {
                Text(stringResource(R.string.merge_done), color = MaterialTheme.colorScheme.primary)
            }
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@Composable
private fun ConflictRow(
    conflict: ContactMergeFieldConflict,
    chosen: String?,
    onChoose: (String) -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Text(
            text = conflict.label,
            style = MaterialTheme.typography.titleSmall,
        )
        val options = listOfNotNull(conflict.keeperValue, conflict.loserValue).distinct()
        options.forEach { option ->
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable { onChoose(option) },
            ) {
                Checkbox(checked = chosen == option, onCheckedChange = { onChoose(option) })
                Text(option, modifier = Modifier.weight(1f))
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BulkOperationsScreen(
    onBack: () -> Unit,
    viewModel: BulkOperationsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var pendingAction by remember { mutableStateOf<String?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    androidx.compose.material3.IconButton(onClick = onBack) {
                        androidx.compose.material3.Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.bulk_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
            ) {
                TextButton(onClick = { pendingAction = BulkActions.ARCHIVE }) {
                    Text(stringResource(R.string.bulk_archive))
                }
                TextButton(onClick = { pendingAction = BulkActions.UNARCHIVE }) {
                    Text(stringResource(R.string.bulk_unarchive))
                }
                TextButton(onClick = { pendingAction = BulkActions.DELETE }) {
                    Text(stringResource(R.string.action_delete))
                }
            }
            LazyColumn(modifier = Modifier.weight(1f)) {
                items(state.contacts, key = { it.id }) { contact ->
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable { viewModel.toggle(contact.id) }
                            .padding(horizontal = 16.dp, vertical = 8.dp),
                    ) {
                        Checkbox(checked = contact.id in state.selected, onCheckedChange = { viewModel.toggle(contact.id) })
                        Text(
                            text = contact.displayName,
                            style = MaterialTheme.typography.bodyLarge,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.weight(1f),
                        )
                    }
                }
            }
            state.result?.let { result ->
                Text(
                    text = stringResource(R.string.bulk_result, result.succeeded, result.failed),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(16.dp),
                )
            }
        }
    }

    pendingAction?.let { action ->
        AlertDialog(
            onDismissRequest = { pendingAction = null },
            title = { Text(stringResource(R.string.bulk_confirm_title)) },
            text = { Text(stringResource(R.string.bulk_confirm_body, state.selected.size)) },
            confirmButton = {
                TextButton(onClick = { viewModel.run(action); pendingAction = null }) {
                    Text(stringResource(R.string.action_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingAction = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}
