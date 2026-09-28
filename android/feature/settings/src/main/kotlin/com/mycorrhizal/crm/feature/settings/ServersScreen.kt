package com.mycorrhizal.crm.feature.settings

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
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState

/**
 * ADR 0028 Decision 1: the "Servers" screen — list, add (Remote), rename,
 * switch and remove server profiles. Switching wipes the Room mirror and, when
 * the outbox is non-empty, first asks the user to confirm discarding the count.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServersScreen(
    onBack: () -> Unit,
    // The Local kind is modelled but not offered until the embedded host
    // (issue #1262) lands — ADR 0028 Decision 2 / "arm64-v8a only".
    localModeEnabled: Boolean = false,
    onUseLocalOnly: () -> Unit = {},
    viewModel: ServersViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var addOpen by remember { mutableStateOf(false) }
    var renaming by remember { mutableStateOf<ServerProfile?>(null) }
    var removing by remember { mutableStateOf<ServerProfile?>(null) }

    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                ServersEvent.Switched, ServersEvent.Removed -> onBack()
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.settings_servers_title),
                        style = MaterialTheme.typography.titleLarge,
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            BrandFab(onClick = { addOpen = true }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.settings_servers_add))
            }
        },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            state.errorRes?.let { res ->
                Text(
                    text = stringResource(res),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier
                        .padding(horizontal = 16.dp, vertical = 8.dp)
                        .semantics { liveRegion = LiveRegionMode.Assertive },
                )
            }
            if (state.profiles.isEmpty()) {
                EmptyState(message = stringResource(R.string.settings_servers_empty))
            } else {
                LazyColumn(modifier = Modifier.fillMaxSize().testTag("servers-list")) {
                    items(state.profiles, key = { it.id }) { profile ->
                        ServerProfileRow(
                            profile = profile,
                            isActive = profile.id == state.activeProfileId,
                            isBusy = state.isBusy,
                            onSelect = { viewModel.select(profile.id) },
                            onRename = { renaming = profile },
                            onRemove = { removing = profile },
                        )
                    }
                }
            }
            if (localModeEnabled) {
                TextButton(
                    onClick = onUseLocalOnly,
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(16.dp),
                ) {
                    Text(stringResource(R.string.login_use_local_only))
                }
            }
        }
    }

    if (addOpen) {
        AddServerDialog(
            isBusy = state.isBusy,
            onConfirm = { label, url ->
                viewModel.addRemote(label, url)
                addOpen = false
            },
            onDismiss = { addOpen = false },
        )
    }

    renaming?.let { profile ->
        RenameServerDialog(
            initialLabel = profile.label,
            onConfirm = { label ->
                viewModel.rename(profile.id, label)
                renaming = null
            },
            onDismiss = { renaming = null },
        )
    }

    removing?.let { profile ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = { Text(stringResource(R.string.settings_servers_remove_title)) },
            text = { Text(stringResource(R.string.settings_servers_remove_body, profile.label)) },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.remove(profile.id)
                    removing = null
                }) { Text(stringResource(R.string.settings_servers_remove)) }
            },
            dismissButton = {
                TextButton(onClick = { removing = null }) {
                    Text(stringResource(R.string.settings_cancel))
                }
            },
        )
    }

    state.pendingSwitch?.let { pending ->
        val label = state.profiles.find { it.id == pending.profileId }?.label.orEmpty()
        AlertDialog(
            onDismissRequest = viewModel::dismissPendingSwitch,
            title = { Text(stringResource(R.string.settings_servers_switch_title)) },
            text = { Text(stringResource(R.string.settings_servers_switch_body, pending.count, label)) },
            confirmButton = {
                TextButton(onClick = viewModel::confirmDiscard) {
                    Text(stringResource(R.string.settings_servers_switch_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = viewModel::dismissPendingSwitch) {
                    Text(stringResource(R.string.settings_cancel))
                }
            },
        )
    }
}

@Composable
internal fun ServerProfileRow(
    profile: ServerProfile,
    isActive: Boolean,
    isBusy: Boolean,
    onSelect: () -> Unit,
    onRename: () -> Unit,
    onRemove: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(enabled = !isBusy && !isActive) { onSelect() }
            .padding(horizontal = 16.dp, vertical = 12.dp)
            .testTag("server-row-${profile.id}"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(profile.label, style = MaterialTheme.typography.bodyLarge)
                if (isActive) {
                    Text(
                        text = stringResource(R.string.settings_servers_active),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.tertiary,
                    )
                }
            }
            Text(
                text = profile.remoteUrl ?: stringResource(R.string.settings_servers_local_kind),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (isActive) {
            Icon(
                Icons.Outlined.Check,
                contentDescription = stringResource(R.string.settings_servers_active),
                tint = MaterialTheme.colorScheme.tertiary,
            )
        }
        AccessibleIconButton(onClick = onRename, enabled = !isBusy) {
            Icon(Icons.Outlined.Edit, contentDescription = stringResource(R.string.settings_servers_rename_named, profile.label))
        }
        AccessibleIconButton(onClick = onRemove, enabled = !isBusy) {
            Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.settings_servers_remove_named, profile.label))
        }
    }
}

@Composable
internal fun AddServerDialog(
    isBusy: Boolean,
    onConfirm: (label: String, url: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var label by remember { mutableStateOf("") }
    var url by remember { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.settings_servers_add)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(
                    value = label,
                    onValueChange = { label = it },
                    label = { Text(stringResource(R.string.settings_servers_label)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().testTag("servers-add-label"),
                )
                OutlinedTextField(
                    value = url,
                    onValueChange = { url = it },
                    label = { Text(stringResource(R.string.settings_servers_url)) },
                    placeholder = { Text(stringResource(R.string.login_server_url_hint)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().testTag("servers-add-url"),
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(label, url) },
                enabled = !isBusy && url.isNotBlank(),
                modifier = Modifier.testTag("servers-add-confirm"),
            ) {
                Text(stringResource(R.string.settings_servers_add))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.settings_cancel)) }
        },
    )
}

@Composable
internal fun RenameServerDialog(
    initialLabel: String,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var label by remember { mutableStateOf(initialLabel) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.settings_servers_rename_title)) },
        text = {
            OutlinedTextField(
                value = label,
                onValueChange = { label = it },
                label = { Text(stringResource(R.string.settings_servers_label)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(label) }, enabled = label.isNotBlank()) {
                Text(stringResource(R.string.settings_servers_rename))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.settings_cancel)) }
        },
    )
}
