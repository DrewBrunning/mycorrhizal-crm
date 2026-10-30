package com.mycorrhizal.crm.feature.trash

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalResources
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.TrashedContact
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.components.localizedMessage
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Contact Trash (issue #2002, ADR 0037; mirrors web's TrashPage): restore or
 * permanently delete the contacts the user deleted. Delete forever and Empty
 * trash are confirmed; a restore that loses rows or hits a UID conflict says so.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TrashScreen(
    modifier: Modifier = Modifier,
    // A top-level drawer destination; null hides the hamburger (no drawer at Expanded).
    onMenuClick: (() -> Unit)? = {},
    viewModel: TrashViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    val loadError = state.error?.localizedMessage()

    var pendingPurge by remember { mutableStateOf<TrashedContact?>(null) }
    var confirmEmpty by remember { mutableStateOf(false) }
    var actionFailure by remember { mutableStateOf<ApiError?>(null) }

    // Reload when returning to the destination: the drawer restores this entry's view model.
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) viewModel.reload()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    val resources = LocalResources.current
    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            val message = when (event) {
                is TrashUiEvent.Restored ->
                    if (event.lostRows > 0) {
                        resources.getQuantityString(
                            R.plurals.trash_restored_with_skipped,
                            event.lostRows,
                            event.name,
                            event.lostRows
                        )
                    } else {
                        resources.getString(R.string.trash_restored, event.name)
                    }

                is TrashUiEvent.RestoreConflict ->
                    resources.getString(R.string.trash_restore_conflict, event.name)

                is TrashUiEvent.Purged -> resources.getString(R.string.trash_purged, event.name)

                is TrashUiEvent.Emptied ->
                    resources.getQuantityString(R.plurals.trash_emptied, event.count, event.count)

                is TrashUiEvent.Failed -> null.also { actionFailure = event.error }
            }
            if (message != null) snackbarHostState.showSnackbar(message)
        }
    }
    actionFailure?.let { error ->
        val message = error.localizedMessage()
        LaunchedEffect(error) {
            snackbarHostState.showSnackbar(message)
            actionFailure = null
        }
    }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    onMenuClick?.let { onMenu ->
                        AccessibleIconButton(onClick = onMenu) {
                            Icon(
                                Icons.Outlined.Menu,
                                contentDescription = stringResource(R.string.cd_menu)
                            )
                        }
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.trash_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            if (state.items.isNotEmpty()) {
                OutlinedButton(
                    onClick = { confirmEmpty = true },
                    enabled = !state.isWorking,
                    modifier = Modifier
                        .align(Alignment.End)
                        .padding(horizontal = 16.dp, vertical = 8.dp)
                        .testTag("trash-empty")
                ) {
                    Text(stringResource(R.string.trash_empty_action))
                }
            }
            RefreshableContent(
                isRefreshing = state.isLoading && state.items.isNotEmpty(),
                onRefresh = viewModel::load,
                modifier = Modifier.fillMaxSize()
            ) {
                when {
                    state.isLoading && state.items.isEmpty() ->
                        LoadingSkeleton(modifier = Modifier.testTag("trash-loading"))

                    state.items.isEmpty() && loadError != null ->
                        Text(
                            text = loadError,
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.fillMaxWidth().padding(16.dp)
                        )

                    state.items.isEmpty() -> EmptyState(
                        title = stringResource(R.string.trash_empty_state),
                        body = trashRetentionText(state.retentionDays),
                        icon = Icons.Outlined.Delete
                    )

                    else -> TrashList(
                        items = state.items,
                        retentionDays = state.retentionDays,
                        enabled = !state.isWorking,
                        onRestore = viewModel::restore,
                        onPurge = { pendingPurge = it }
                    )
                }
            }
        }
    }

    if (loadError != null && state.items.isNotEmpty()) {
        LaunchedEffect(loadError) { snackbarHostState.showSnackbar(loadError) }
    }

    pendingPurge?.let { item ->
        ConfirmDialog(
            title = stringResource(R.string.trash_delete_forever),
            message = stringResource(R.string.trash_purge_confirm, item.contact.displayName),
            confirmLabel = stringResource(R.string.trash_delete_forever),
            confirmTag = "trash-purge-confirm",
            onConfirm = {
                viewModel.purge(item)
                pendingPurge = null
            },
            onDismiss = { pendingPurge = null }
        )
    }
    if (confirmEmpty) {
        ConfirmDialog(
            title = stringResource(R.string.trash_empty_action),
            message = pluralStringResource(
                R.plurals.trash_empty_confirm,
                state.items.size,
                state.items.size
            ),
            confirmLabel = stringResource(R.string.trash_empty_action),
            confirmTag = "trash-empty-confirm",
            onConfirm = {
                viewModel.emptyTrash()
                confirmEmpty = false
            },
            onDismiss = { confirmEmpty = false }
        )
    }
}

@Composable
private fun trashRetentionText(retentionDays: Int): String = if (retentionDays > 0) {
    pluralStringResource(R.plurals.trash_retention, retentionDays, retentionDays)
} else {
    stringResource(R.string.trash_retention_forever)
}

@Composable
private fun ConfirmDialog(
    title: String,
    message: String,
    confirmLabel: String,
    confirmTag: String,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { Text(message) },
        confirmButton = {
            TextButton(onClick = onConfirm, modifier = Modifier.testTag(confirmTag)) {
                Text(confirmLabel, color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        }
    )
}

@Composable
internal fun TrashList(
    items: List<TrashedContact>,
    retentionDays: Int,
    enabled: Boolean,
    onRestore: (TrashedContact) -> Unit,
    onPurge: (TrashedContact) -> Unit
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize().testTag("trash-list"),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        item(key = "retention") {
            Text(
                text = trashRetentionText(retentionDays),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.testTag("trash-retention")
            )
        }
        items(items, key = { it.contact.id }) { item ->
            TrashRow(item, enabled, onRestore = { onRestore(item) }, onPurge = { onPurge(item) })
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun TrashRow(
    item: TrashedContact,
    enabled: Boolean,
    onRestore: () -> Unit,
    onPurge: () -> Unit
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerLow
        )
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp)
        ) {
            Text(
                text = item.contact.displayName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            Text(
                text = stringResource(R.string.trash_deleted_on, formatTrashTime(item.deletedAt)),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            item.purgeAt?.let { purgeAt ->
                Text(
                    text = stringResource(R.string.trash_purge_on, formatTrashTime(purgeAt)),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
            // A FlowRow so the two actions wrap instead of squeezing at 200% font scale.
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Button(
                    onClick = onRestore,
                    enabled = enabled,
                    modifier = Modifier.testTag("trash-restore-${item.contact.id}")
                ) {
                    Text(stringResource(R.string.trash_restore))
                }
                TextButton(
                    onClick = onPurge,
                    enabled = enabled,
                    modifier = Modifier.testTag("trash-delete-${item.contact.id}")
                ) {
                    Text(
                        stringResource(R.string.trash_delete_forever),
                        color = MaterialTheme.colorScheme.error
                    )
                }
            }
        }
    }
}

private fun formatTrashTime(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    return runCatching {
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM)
            .withZone(ZoneId.systemDefault())
            .format(Instant.parse(iso))
    }.getOrDefault(iso)
}
