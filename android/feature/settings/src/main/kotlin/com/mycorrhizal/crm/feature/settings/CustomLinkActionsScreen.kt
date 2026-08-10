package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.domain.repository.CustomLinkAction
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CustomLinkActionsScreen(
    onBack: () -> Unit,
    viewModel: CustomLinkActionsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.custom_links_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surface),
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = { showAdd = true }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.custom_links_new))
            }
        },
    ) { padding ->
        if (state.actions.isEmpty()) {
            EmptyState(message = stringResource(R.string.custom_links_empty), modifier = Modifier.padding(padding))
        } else {
            LazyColumn(modifier = Modifier.fillMaxSize().padding(padding)) {
                items(state.actions, key = { it.id }) { action ->
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp),
                        verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
                    ) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = "${action.protocol} — ${action.label}",
                                style = MaterialTheme.typography.bodyLarge,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                            Text(
                                text = action.intentUriTemplate,
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                        IconButton(onClick = { viewModel.delete(action) }) {
                            Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
                        }
                    }
                }
            }
        }
    }

    if (showAdd) {
        AddCustomLinkActionDialog(
            onConfirm = { p, l, m, t ->
                viewModel.add(p, l, m, t)
                showAdd = false
            },
            onDismiss = { showAdd = false },
        )
    }
}

@Composable
fun AddCustomLinkActionDialog(
    onConfirm: (protocol: String, label: String, mimeType: String, template: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var protocol by remember { mutableStateOf("") }
    var label by remember { mutableStateOf("") }
    var mimeType by remember { mutableStateOf("") }
    var template by remember { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.custom_links_new)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = protocol, onValueChange = { protocol = it },
                    label = { Text(stringResource(R.string.custom_links_protocol)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = label, onValueChange = { label = it },
                    label = { Text(stringResource(R.string.custom_links_label)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = mimeType, onValueChange = { mimeType = it },
                    label = { Text(stringResource(R.string.custom_links_mime)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = template, onValueChange = { template = it },
                    label = { Text(stringResource(R.string.custom_links_template)) }, singleLine = true,
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(protocol, label, mimeType, template) },
                enabled = protocol.isNotBlank() && label.isNotBlank() && mimeType.isNotBlank() && template.isNotBlank(),
            ) { Text(stringResource(R.string.action_add)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}
