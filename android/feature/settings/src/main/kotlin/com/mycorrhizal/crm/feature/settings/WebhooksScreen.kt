package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.PlayArrow
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
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
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.Webhook
import com.mycorrhizal.crm.model.network.WebhookInput
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState

/**
 * Webhook events, hand-mirrored from the backend's `oneof` validator
 * (`backend/models/dtos.go` WebhookInput) — there is no dynamic type-list
 * endpoint anywhere in this codebase (`/CLAUDE.md` frontend trap #4). Keep in
 * sync with `frontend/src/components/WebhooksSettings.tsx`'s SUPPORTED_EVENTS.
 */
val WEBHOOK_EVENTS: List<String> = listOf(
    "contact.created",
    "contact.updated",
    "contact.deleted",
    "note.created",
    "note.updated",
    "note.deleted",
    "activity.created",
    "activity.updated",
    "activity.deleted",
    "reminder.triggered",
    "birthday.occurred",
)

/** Localized label for a webhook event token; falls back to the raw token. */
@Composable
private fun eventLabel(event: String): String = when (event) {
    "contact.created" -> stringResource(R.string.settings_webhooks_event_contact_created)
    "contact.updated" -> stringResource(R.string.settings_webhooks_event_contact_updated)
    "contact.deleted" -> stringResource(R.string.settings_webhooks_event_contact_deleted)
    "note.created" -> stringResource(R.string.settings_webhooks_event_note_created)
    "note.updated" -> stringResource(R.string.settings_webhooks_event_note_updated)
    "note.deleted" -> stringResource(R.string.settings_webhooks_event_note_deleted)
    "activity.created" -> stringResource(R.string.settings_webhooks_event_activity_created)
    "activity.updated" -> stringResource(R.string.settings_webhooks_event_activity_updated)
    "activity.deleted" -> stringResource(R.string.settings_webhooks_event_activity_deleted)
    "reminder.triggered" -> stringResource(R.string.settings_webhooks_event_reminder_triggered)
    "birthday.occurred" -> stringResource(R.string.settings_webhooks_event_birthday_occurred)
    else -> event
}

/**
 * M25: webhook CRUD + test + delivery history, mirroring web's
 * WebhooksSettings.tsx. Deletion is confirmed first (the ticket's test case 4);
 * the one-shot secret reveal dialog after create matches web.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WebhooksScreen(
    onBack: () -> Unit,
    viewModel: WebhooksViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var editorOpen by remember { mutableStateOf(false) }
    var editingWebhook by remember { mutableStateOf<Webhook?>(null) }
    var deletingWebhook by remember { mutableStateOf<Webhook?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_webhooks_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            BrandFab(onClick = {
                editingWebhook = null
                editorOpen = true
            }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.settings_webhooks_add))
            }
        },
    ) { padding ->
        if (state.isLoading && state.webhooks.isEmpty()) {
            Column(
                modifier = Modifier.fillMaxSize().padding(padding),
                verticalArrangement = Arrangement.Center,
                horizontalAlignment = androidx.compose.ui.Alignment.CenterHorizontally,
            ) {
                CircularProgressIndicator()
            }
        } else if (state.webhooks.isEmpty()) {
            EmptyState(
                message = stringResource(R.string.settings_webhooks_empty),
                modifier = Modifier.padding(padding),
            )
        } else {
            LazyColumn(modifier = Modifier.fillMaxSize().padding(padding)) {
                item {
                    Text(
                        text = stringResource(R.string.settings_webhooks_description),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                    )
                }
                if (state.error != null) {
                    item {
                        Text(
                            text = state.error.orEmpty(),
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                            modifier = Modifier
                                .padding(horizontal = 16.dp, vertical = 4.dp)
                                .semantics { liveRegion = LiveRegionMode.Assertive },
                        )
                    }
                }
                if (state.message != null) {
                    item {
                        Text(
                            text = stringResource(R.string.settings_webhooks_test_success, state.message.orEmpty()),
                            color = MaterialTheme.colorScheme.tertiary,
                            style = MaterialTheme.typography.bodySmall,
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                        )
                    }
                }
                items(state.webhooks, key = { it.id }) { webhook ->
                    WebhookRow(
                        webhook = webhook,
                        testing = state.testingIds.contains(webhook.id),
                        expanded = state.expandedIds.contains(webhook.id),
                        deliveries = state.deliveries[webhook.id].orEmpty(),
                        onTest = { viewModel.test(webhook) },
                        onEdit = {
                            editingWebhook = webhook
                            editorOpen = true
                        },
                        onDelete = { deletingWebhook = webhook },
                        onToggleDeliveries = { viewModel.toggleDeliveries(webhook.id) },
                    )
                }
            }
        }
    }

    if (editorOpen) {
        WebhookEditorDialog(
            initial = editingWebhook,
            isSaving = state.isSaving,
            onConfirm = { input ->
                viewModel.save(input, editingWebhook?.id)
                editorOpen = false
            },
            onDismiss = { editorOpen = false },
        )
    }

    deletingWebhook?.let { webhook ->
        AlertDialog(
            onDismissRequest = { deletingWebhook = null },
            title = { Text(stringResource(R.string.settings_webhooks_delete_title)) },
            text = { Text(stringResource(R.string.settings_webhooks_delete_body, webhook.name)) },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.delete(webhook)
                    deletingWebhook = null
                }) { Text(stringResource(R.string.settings_webhooks_delete)) }
            },
            dismissButton = {
                TextButton(onClick = { deletingWebhook = null }) {
                    Text(stringResource(R.string.settings_cancel))
                }
            },
        )
    }

    state.createdWebhook?.let { created ->
        SecretRevealDialog(
            secret = created.secret.orEmpty(),
            onDismiss = { viewModel.dismissCreatedWebhook() },
        )
    }
}

@Composable
internal fun WebhookRow(
    webhook: Webhook,
    testing: Boolean,
    expanded: Boolean,
    deliveries: List<com.mycorrhizal.crm.model.network.WebhookDelivery>,
    onTest: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
    onToggleDeliveries: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
                ) {
                    Text(text = webhook.name, style = MaterialTheme.typography.bodyLarge)
                    Text(
                        text = stringResource(
                            if (webhook.isActive) R.string.settings_webhooks_active else R.string.settings_webhooks_inactive,
                        ),
                        style = MaterialTheme.typography.labelSmall,
                        color = if (webhook.isActive) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Text(
                    text = webhook.url,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(
                    text = stringResource(R.string.settings_webhooks_events_count, webhook.events.size),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            IconButton(onClick = onTest, enabled = !testing) {
                if (testing) {
                    CircularProgressIndicator(modifier = Modifier.padding(4.dp), strokeWidth = 2.dp)
                } else {
                    // #205: the row-action label carries the webhook name so
                    // TalkBack doesn't read a bare "Test" on every row.
                    Icon(
                        Icons.Outlined.PlayArrow,
                        contentDescription = stringResource(R.string.settings_webhooks_test_named, webhook.name),
                    )
                }
            }
            IconButton(onClick = onEdit) {
                // #205: the row-action label carries the webhook name so TalkBack
                // doesn't read a bare "Edit" on every row.
                Icon(
                    Icons.Outlined.Edit,
                    contentDescription = stringResource(R.string.settings_webhooks_edit_named, webhook.name),
                )
            }
            IconButton(onClick = onDelete) {
                Icon(
                    Icons.Outlined.Delete,
                    contentDescription = stringResource(R.string.settings_webhooks_delete_named, webhook.name),
                )
            }
        }
        TextButton(
            onClick = onToggleDeliveries,
            modifier = Modifier.padding(start = 4.dp),
        ) {
            Text(
                stringResource(
                    if (expanded) R.string.settings_webhooks_hide_deliveries else R.string.settings_webhooks_show_deliveries,
                ),
                style = MaterialTheme.typography.labelMedium,
            )
        }
        if (expanded) {
            if (deliveries.isEmpty()) {
                Text(
                    text = stringResource(R.string.settings_webhooks_no_deliveries),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 4.dp),
                )
            } else {
                deliveries.take(5).forEach { delivery ->
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp, vertical = 2.dp),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        Text(
                            text = delivery.statusCode?.toString() ?: "err",
                            style = MaterialTheme.typography.labelSmall,
                            color = delivery.statusCode?.let { if (it in 200..299) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.error }
                                ?: MaterialTheme.colorScheme.error,
                        )
                        Text(
                            text = delivery.eventType,
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                        delivery.error?.let { error ->
                            Text(
                                text = error,
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.error,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
internal fun WebhookEditorDialog(
    initial: Webhook?,
    isSaving: Boolean,
    onConfirm: (WebhookInput) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by remember { mutableStateOf(initial?.name ?: "") }
    var url by remember { mutableStateOf(initial?.url ?: "") }
    var isActive by remember { mutableStateOf(initial?.isActive ?: true) }
    var events by remember { mutableStateOf(initial?.events?.toSet() ?: emptySet()) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(stringResource(if (initial == null) R.string.settings_webhooks_add else R.string.settings_webhooks_edit))
        },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(
                    value = name, onValueChange = { name = it },
                    label = { Text(stringResource(R.string.settings_webhooks_name)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = url, onValueChange = { url = it },
                    label = { Text(stringResource(R.string.settings_webhooks_url)) }, singleLine = true,
                )
                Text(
                    text = stringResource(R.string.settings_webhooks_events),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Column(
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 220.dp)
                        .verticalScroll(rememberScrollState()),
                ) {
                    WEBHOOK_EVENTS.forEach { event ->
                        FilterChip(
                            selected = events.contains(event),
                            onClick = {
                                events = if (events.contains(event)) events - event else events + event
                            },
                            label = { Text(eventLabel(event), style = MaterialTheme.typography.labelMedium) },
                        )
                    }
                }
                // #199: a bare Switch has no text/contentDescription of its own — the
                // adjacent "Active" Text was a separate, unassociated node, so
                // TalkBack announced the switch with no name at all. Modifier.toggleable
                // on the row merges the label into the switch's accessible name.
                Row(
                    horizontalArrangement = Arrangement.SpaceBetween,
                    modifier = Modifier
                        .fillMaxWidth()
                        .toggleable(value = isActive, onValueChange = { isActive = it }, role = Role.Switch),
                    verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
                ) {
                    Text(stringResource(R.string.settings_webhooks_active), style = MaterialTheme.typography.bodyLarge)
                    Switch(checked = isActive, onCheckedChange = null)
                }
            }
        },
        confirmButton = {
            val savingLabel = stringResource(R.string.a11y_state_saving)
            TextButton(
                onClick = { onConfirm(WebhookInput(name = name.trim(), url = url.trim(), events = events.toList(), isActive = isActive)) },
                enabled = !isSaving && name.isNotBlank() && url.isNotBlank() && events.isNotEmpty(),
                modifier = Modifier.semantics { if (isSaving) stateDescription = savingLabel },
            ) {
                if (isSaving) CircularProgressIndicator(modifier = Modifier.padding(end = 4.dp), strokeWidth = 2.dp)
                Text(stringResource(R.string.settings_save))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.settings_cancel)) }
        },
    )
}

@Composable
private fun SecretRevealDialog(
    secret: String,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.settings_webhooks_secret_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.settings_webhooks_secret_warning))
                OutlinedTextField(
                    value = secret,
                    onValueChange = {},
                    readOnly = true,
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.settings_ok)) }
        },
    )
}
