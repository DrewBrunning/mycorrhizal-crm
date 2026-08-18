package com.mycorrhizal.crm.feature.imports

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.FileUpload
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ImportContactsScreen(
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    onImported: () -> Unit = {},
    onImportVcf: () -> Unit = {},
    viewModel: ImportContactsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    LaunchedEffect(Unit) { viewModel.load() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    onMenuClick?.let { onMenu ->
                        IconButton(onClick = onMenu) {
                            Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                        }
                    }
                },
                title = {
                    Text(stringResource(R.string.import_title), style = MaterialTheme.typography.titleLarge)
                },
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
            val preview = state.preview
            when {
                state.isLoading -> LoadingSkeleton()
                state.step == ImportStep.REVIEW && preview != null ->
                    ImportReviewStep(
                        rows = preview.rows,
                        rowActions = state.rowActions,
                        onRowActionChange = viewModel::setRowAction,
                        onResolveAll = viewModel::resolveAll,
                        onConfirm = viewModel::confirmImport,
                    )
                state.step == ImportStep.RESULT -> {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        modifier = Modifier.fillMaxSize().padding(24.dp),
                    ) {
                        Text(
                            text = stringResource(R.string.import_done, state.importedCount),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.padding(top = 48.dp, bottom = 16.dp),
                        )
                        Button(
                            onClick = {
                                viewModel.startOver()
                                onImported()
                            },
                        ) {
                            Text(stringResource(R.string.action_confirm))
                        }
                    }
                }
                state.contacts.isEmpty() && state.error == null ->
                    EmptyState(message = stringResource(R.string.import_empty))
                state.contacts.isEmpty() && state.error != null -> {
                    Text(
                        text = state.error.orEmpty(),
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(24.dp),
                    )
                }
                else -> {
                    // M9 item 4: a second, file-based import path alongside this screen's existing
                    // device-contacts import — ApiClient.uploadVcfImport() already hit this backend
                    // endpoint but had zero callers.
                    TextButton(
                        onClick = onImportVcf,
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                    ) {
                        Icon(Icons.Outlined.FileUpload, contentDescription = null, modifier = Modifier.padding(end = 8.dp))
                        Text(stringResource(R.string.import_vcf_entry))
                    }
                    LazyColumn(modifier = Modifier.weight(1f)) {
                        items(state.contacts, key = { it.device.contactId }) { candidate ->
                            DeviceContactRow(
                                candidate = candidate,
                                checked = candidate.device.contactId in state.selected,
                                onToggle = { viewModel.toggle(candidate.device.contactId) },
                            )
                        }
                    }
                    Button(
                        onClick = { viewModel.submitSelected() },
                        enabled = state.selected.isNotEmpty() && !state.isImporting,
                        modifier = Modifier.fillMaxWidth().padding(16.dp),
                    ) {
                        Text(
                            stringResource(
                                R.string.import_button,
                                state.selected.size,
                                state.contacts.count { it.duplicateOf != null },
                            ),
                        )
                    }
                }
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
private fun DeviceContactRow(
    candidate: DeviceContactCandidate,
    checked: Boolean,
    onToggle: () -> Unit,
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onToggle)
            .padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        Checkbox(checked = checked, onCheckedChange = { onToggle() })
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = candidate.device.displayName ?: stringResource(R.string.import_unnamed),
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (candidate.duplicateOf != null) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        Icons.Outlined.Warning,
                        contentDescription = null,
                        tint = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(end = 4.dp),
                    )
                    Text(
                        text = stringResource(R.string.import_duplicate, candidate.duplicateOf.displayName),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        }
    }
}
