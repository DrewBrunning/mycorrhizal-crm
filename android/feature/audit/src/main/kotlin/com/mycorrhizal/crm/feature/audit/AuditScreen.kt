package com.mycorrhizal.crm.feature.audit

import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Clear
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Undo
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.AuditEvent
import com.mycorrhizal.crm.model.network.AuditEntityTypes
import com.mycorrhizal.crm.model.network.AuditOperations
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * M16: the read-only audit log (mirroring web's AuditPage over T18/T60's
 * backend). Reverse-chronological event list with an entity-type/entity-id
 * filter toolbar, a Contact-only Undo affordance on update events
 * (POST /audit/:id/undo; every other entity or a delete event is 400
 * server-side, so the button is gated to match — [AuditEvent.canUndo]), and
 * entity-id cells that link to the contact detail when the UID resolves.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AuditScreen(
    onBack: () -> Unit,
    onOpenContact: (contactId: Int) -> Unit,
    viewModel: AuditViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    // Local copy so the delegated `state.error` can be smart-cast/sent to
    // suspend APIs (a delegated property's getter defeats Kotlin's smart cast).
    val loadError = state.error

    // Mirrors web's debounced entity-id field: the input echoes keystrokes
    // immediately while the applied (server-side) filter follows 350ms later.
    var entityIdInput by rememberSaveable { mutableStateOf(state.entityId) }

    var pendingUndo by remember { mutableStateOf<AuditEvent?>(null) }

    // One-shot undo outcomes surface as a snackbar (410 → its own message).
    val undoSuccessText = stringResource(R.string.audit_undo_success)
    val undoRetentionText = stringResource(R.string.audit_undo_retention_gone)
    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                is AuditUiEvent.UndoSucceeded -> snackbarHostState.showSnackbar(undoSuccessText)
                is AuditUiEvent.UndoFailed -> {
                    val message = if (event.isRetentionGone) undoRetentionText else event.message
                    snackbarHostState.showSnackbar(message)
                }
            }
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.audit_title), style = MaterialTheme.typography.titleLarge)
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
            Text(
                text = stringResource(R.string.audit_description),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
            )

            AuditFilterToolbar(
                entityType = state.entityType,
                entityIdInput = entityIdInput,
                hasFilters = state.hasActiveFilters,
                onEntityTypeChange = viewModel::applyEntityType,
                onEntityIdChange = {
                    entityIdInput = it
                    viewModel.onEntityIdChange(it)
                },
                onClearFilters = {
                    entityIdInput = ""
                    viewModel.clearFilters()
                },
            )

            Box(modifier = Modifier.fillMaxSize()) {
                when {
                    state.isLoading && state.events.isEmpty() ->
                        LoadingSkeleton(modifier = Modifier.testTag("audit-loading"))

                    state.events.isEmpty() && loadError != null ->
                        Box(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
                            Text(
                                text = loadError,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.align(Alignment.Center),
                            )
                        }

                    state.events.isEmpty() ->
                        EmptyState(
                            message = stringResource(
                                if (state.hasActiveFilters) R.string.audit_empty
                                else R.string.audit_empty_no_filters
                            ),
                            icon = {
                                Icon(
                                    Icons.Outlined.History,
                                    contentDescription = null,
                                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            },
                        )

                    else -> AuditEventList(
                        events = state.events,
                        contactsByUid = state.contactsByUid,
                        canUndo = !state.isUndoing,
                        onUndoClick = { pendingUndo = it },
                        onOpenContact = onOpenContact,
                        canLoadMore = state.canLoadMore,
                        isLoadingMore = state.isLoading,
                        onLoadMore = viewModel::loadMore,
                    )
                }
            }
        }
    }

    // Transient list-load failures are toasted; the inline error above only
    // shows when there's nothing else to render (loading/empty).
    if (loadError != null && state.events.isNotEmpty()) {
        LaunchedEffect(loadError) {
            snackbarHostState.showSnackbar(loadError)
            viewModel.onErrorShown()
        }
    }

    val undoEvent = pendingUndo
    if (undoEvent != null) {
        AuditUndoDialog(
            event = undoEvent,
            isUndoing = state.isUndoing,
            onConfirm = {
                viewModel.undo(undoEvent.id)
                pendingUndo = null
            },
            onDismiss = { pendingUndo = null },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun AuditFilterToolbar(
    entityType: String?,
    entityIdInput: String,
    hasFilters: Boolean,
    onEntityTypeChange: (String?) -> Unit,
    onEntityIdChange: (String) -> Unit,
    onClearFilters: () -> Unit,
) {
    var menuExpanded by remember { mutableStateOf(false) }

    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ExposedDropdownMenuBox(
                expanded = menuExpanded,
                onExpandedChange = { menuExpanded = it },
            ) {
                OutlinedTextField(
                    value = entityType?.let { entityTypeLabel(it) } ?: stringResource(R.string.audit_filters_entity_type_all),
                    onValueChange = {},
                    readOnly = true,
                    label = { Text(stringResource(R.string.audit_filters_entity_type)) },
                    trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = menuExpanded) },
                    modifier = Modifier
                        .menuAnchor(MenuAnchorType.PrimaryNotEditable)
                        .testTag("audit-entity-type")
                        .weight(1f),
                )
                ExposedDropdownMenu(
                    expanded = menuExpanded,
                    onDismissRequest = { menuExpanded = false },
                ) {
                    DropdownMenuItem(
                        text = { Text(stringResource(R.string.audit_filters_entity_type_all)) },
                        onClick = {
                            onEntityTypeChange(null)
                            menuExpanded = false
                        },
                    )
                    AuditEntityTypes.ALL.forEach { token ->
                        DropdownMenuItem(
                            text = { Text(entityTypeLabel(token)) },
                            onClick = {
                                onEntityTypeChange(token)
                                menuExpanded = false
                            },
                        )
                    }
                }
            }
            OutlinedButton(
                onClick = onClearFilters,
                enabled = hasFilters,
                modifier = Modifier.testTag("audit-clear-filters"),
            ) {
                Icon(Icons.Outlined.Clear, contentDescription = null)
                Text(stringResource(R.string.audit_filters_clear), modifier = Modifier.padding(start = 4.dp))
            }
        }
        OutlinedTextField(
            value = entityIdInput,
            onValueChange = onEntityIdChange,
            label = { Text(stringResource(R.string.audit_filters_entity_id)) },
            singleLine = true,
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 8.dp)
                .testTag("audit-entity-id"),
        )
    }
}

@Composable
internal fun AuditEventList(
    events: List<AuditEvent>,
    contactsByUid: Map<String, ContactSummary>,
    canUndo: Boolean,
    onUndoClick: (AuditEvent) -> Unit,
    onOpenContact: (Int) -> Unit,
    canLoadMore: Boolean,
    isLoadingMore: Boolean,
    onLoadMore: () -> Unit,
) {
    LazyColumn(
        modifier = Modifier.fillMaxSize().testTag("audit-list"),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        items(events, key = { it.id }) { event ->
            AuditEventRow(
                event = event,
                contact = contactsByUid[event.entityId],
                canUndo = canUndo,
                onUndoClick = { onUndoClick(event) },
                onOpenContact = onOpenContact,
            )
        }
        if (canLoadMore) {
            item(key = "load-more") {
                Box(modifier = Modifier.fillMaxWidth().padding(top = 4.dp), contentAlignment = Alignment.Center) {
                    OutlinedButton(
                        onClick = onLoadMore,
                        enabled = !isLoadingMore,
                        modifier = Modifier.testTag("audit-load-more"),
                    ) {
                        Text(stringResource(R.string.action_load_more))
                    }
                }
            }
        }
    }
}

@Composable
internal fun AuditEventRow(
    event: AuditEvent,
    contact: ContactSummary?,
    canUndo: Boolean,
    onUndoClick: () -> Unit,
    onOpenContact: (Int) -> Unit,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerLow),
    ) {
        Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OperationBadge(event.operation)
                Text(
                    text = entityTypeLabel(event.entityType),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Text(
                    text = formatAuditTime(event.createdAt),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            // The entity-id cell: contact events link to the detail page when
            // the UID resolves (archived contacts included — the resolution
            // endpoint returns them). A deleted contact falls back to its raw
            // uid as plain text, matching web.
            val resolved = contact
            if (resolved != null && event.entityType == AuditEntityTypes.CONTACT) {
                Text(
                    text = resolved.displayName,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.primary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier
                        .testTag("audit-contact-link-${event.id}")
                        .clickable {
                            onOpenContact(resolved.id)
                        }
                        .padding(vertical = 2.dp),
                )
            } else {
                Text(
                    text = event.entityId.ifBlank { "#${event.id}" },
                    style = MaterialTheme.typography.bodySmall,
                    fontFamily = FontFamily.Monospace,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }

            if (event.canUndo) {
                TextButton(
                    onClick = onUndoClick,
                    enabled = canUndo,
                    modifier = Modifier
                        .testTag("audit-undo-${event.id}")
                        .align(Alignment.End),
                ) {
                    Icon(Icons.Outlined.Undo, contentDescription = null)
                    Text(stringResource(R.string.audit_undo_button), modifier = Modifier.padding(start = 4.dp))
                }
            }
        }
    }
}

@Composable
private fun OperationBadge(operation: String) {
    val (label, color) = when (operation) {
        AuditOperations.CREATE -> stringResource(R.string.audit_operations_create) to MaterialTheme.colorScheme.tertiary
        AuditOperations.UPDATE -> stringResource(R.string.audit_operations_update) to MaterialTheme.colorScheme.primary
        AuditOperations.DELETE -> stringResource(R.string.audit_operations_delete) to MaterialTheme.colorScheme.error
        else -> operation to MaterialTheme.colorScheme.onSurfaceVariant
    }
    Text(
        text = label,
        style = MaterialTheme.typography.labelSmall,
        color = color,
        modifier = Modifier
            .border(
                width = 1.dp,
                color = color,
                shape = MaterialTheme.shapes.small,
            )
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

@Composable
private fun AuditUndoDialog(
    event: AuditEvent,
    isUndoing: Boolean,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = { if (!isUndoing) onDismiss() },
        title = { Text(stringResource(R.string.audit_undo_confirm_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    text = stringResource(
                        R.string.audit_undo_confirm_message,
                        entityTypeLabel(event.entityType),
                        formatAuditTime(event.createdAt),
                    ),
                )
                Text(
                    text = stringResource(R.string.audit_undo_partial_note),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = onConfirm, enabled = !isUndoing) {
                Text(stringResource(R.string.audit_undo_confirm))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isUndoing) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}

/** Human label for an entity-type token, falling back to the raw token for future ones. */
@Composable
fun entityTypeLabel(token: String): String = when (token) {
    AuditEntityTypes.CONTACT -> stringResource(R.string.audit_entity_types_contact)
    AuditEntityTypes.NOTE -> stringResource(R.string.audit_entity_types_note)
    AuditEntityTypes.ACTIVITY -> stringResource(R.string.audit_entity_types_activity)
    AuditEntityTypes.LIFE_EVENT -> stringResource(R.string.audit_entity_types_life_event)
    AuditEntityTypes.GIFT -> stringResource(R.string.audit_entity_types_gift)
    AuditEntityTypes.CIRCLE -> stringResource(R.string.audit_entity_types_circle)
    AuditEntityTypes.TAG -> stringResource(R.string.audit_entity_types_tag)
    AuditEntityTypes.HOUSEHOLD -> stringResource(R.string.audit_entity_types_household)
    AuditEntityTypes.REMINDER -> stringResource(R.string.audit_entity_types_reminder)
    else -> token
}

/**
 * Audit timestamps need date + time (multiple events can share a day), so this
 * uses the device locale rather than the date-only DateFormat preference
 * (mirroring web's formatDateTime → toLocaleString).
 */
private fun formatAuditTime(iso: String?): String {
    if (iso.isNullOrBlank()) return ""
    return runCatching {
        val instant = Instant.parse(iso)
        DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM)
            .withZone(ZoneId.systemDefault())
            .format(instant)
    }.getOrDefault(iso)
}
