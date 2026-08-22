package com.mycorrhizal.crm.feature.relationships

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeSensitivities
import com.mycorrhizal.crm.model.network.RelationshipEdgeStatuses
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RelationshipsScreen(
    onBack: () -> Unit,
    onNavigateToContact: (Int) -> Unit,
    viewModel: RelationshipsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var showDialogFor by remember { mutableStateOf<RelationshipEdge?>(null) }
    var showCreateDialog by remember { mutableStateOf(false) }
    var pendingDelete by remember { mutableStateOf<RelationshipEdge?>(null) }
    var pendingReject by remember { mutableStateOf<RelationshipEdge?>(null) }
    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error

    @Composable
    fun displayName(edge: RelationshipEdge): String {
        val uid = otherPartyId(edge, state.contactVCardUid)
        val contact = state.contactsByUid[uid]
        return contact?.let { displayNameFor(it) } ?: stringResource(R.string.relationships_unknown_contact)
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
                    Text(stringResource(R.string.relationships_title), style = MaterialTheme.typography.titleLarge)
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
                viewModel.clearContactSearch()
                showCreateDialog = true
            }) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.relationships_new))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.edges.isEmpty() && errorMessage == null ->
                    EmptyState(message = stringResource(R.string.relationships_empty))
                state.edges.isEmpty() && errorMessage != null -> {
                    Text(
                        text = errorMessage,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.confirmedEdges, key = { it.id }) { edge ->
                            RelationshipEdgeRow(
                                edge = edge,
                                displayName = displayName(edge),
                                resolvedContactId = state.contactsByUid[otherPartyId(edge, state.contactVCardUid)]?.id,
                                viewedUid = state.contactVCardUid,
                                onNavigateToContact = onNavigateToContact,
                                onEdit = { showDialogFor = edge },
                                onDelete = { pendingDelete = edge },
                            )
                        }
                        if (state.suggestedEdges.isNotEmpty()) {
                            item {
                                if (state.confirmedEdges.isNotEmpty()) {
                                    HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                                }
                                Text(
                                    text = stringResource(R.string.relationships_suggested_section),
                                    style = MaterialTheme.typography.labelLarge,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp),
                                )
                            }
                            items(state.suggestedEdges, key = { it.id }) { edge ->
                                RelationshipEdgeRow(
                                    edge = edge,
                                    displayName = displayName(edge),
                                    resolvedContactId = state.contactsByUid[otherPartyId(edge, state.contactVCardUid)]?.id,
                                    viewedUid = state.contactVCardUid,
                                    onNavigateToContact = onNavigateToContact,
                                    accepting = state.acceptingId == edge.id,
                                    onAccept = { viewModel.accept(edge.id) },
                                    onReject = { pendingReject = edge },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    if (showCreateDialog) {
        RelationshipEdgeDialog(
            edge = null,
            state = state,
            onSearchChange = viewModel::searchContacts,
            onConfirm = { type, uid, name, gender, birthday, sensitivity, linked ->
                viewModel.create(type, uid, name, gender, birthday, sensitivity, linked)
                showCreateDialog = false
            },
            onDismiss = {
                showCreateDialog = false
                viewModel.clearContactSearch()
            },
        )
    }

    showDialogFor?.let { edge ->
        RelationshipEdgeDialog(
            edge = edge,
            state = state,
            onSearchChange = viewModel::searchContacts,
            onConfirm = { type, _, _, _, _, sensitivity, _ ->
                viewModel.update(edge.id, type, sensitivity)
                showDialogFor = null
            },
            onDismiss = { showDialogFor = null },
        )
    }

    pendingDelete?.let { edge ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.relationships_delete_title)) },
            text = { Text(stringResource(R.string.relationships_delete_confirm, displayName(edge))) },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.delete(edge.id)
                    pendingDelete = null
                }) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDelete = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    pendingReject?.let { edge ->
        AlertDialog(
            onDismissRequest = { pendingReject = null },
            title = { Text(stringResource(R.string.relationships_reject_title)) },
            text = { Text(stringResource(R.string.relationships_reject_confirm, displayName(edge))) },
            confirmButton = {
                TextButton(onClick = {
                    viewModel.delete(edge.id)
                    pendingReject = null
                }) {
                    Text(stringResource(R.string.relationships_reject))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingReject = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

private fun displayNameFor(contact: ContactSummary): String {
    // M5 §3.2: use the shared components-first derivation (firstname "nickname"
    // lastname) instead of a local fn-first copy that regressed to
    // given-name-only — same fix as HouseholdDetailScreen.
    return contact.displayName.takeIf { !it.startsWith("#") } ?: contact.nickname.orEmpty()
}

@Composable
private fun RelationshipEdgeRow(
    edge: RelationshipEdge,
    displayName: String,
    resolvedContactId: Int?,
    viewedUid: String,
    onNavigateToContact: (Int) -> Unit,
    accepting: Boolean = false,
    onAccept: (() -> Unit)? = null,
    onReject: (() -> Unit)? = null,
    onEdit: (() -> Unit)? = null,
    onDelete: (() -> Unit)? = null,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .let { m -> if (resolvedContactId != null) m.clickable { onNavigateToContact(resolvedContactId) } else m }
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Person,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = displayName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = relationshipLabel(edge.type, edge, viewedUid),
                style = MaterialTheme.typography.labelMedium,
                color = if (edge.status == RelationshipEdgeStatuses.SUGGESTED) {
                    MaterialTheme.colorScheme.tertiary
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                },
            )
            if (edge.status == RelationshipEdgeStatuses.SUGGESTED) {
                Text(
                    text = stringResource(R.string.relationships_suggested),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.tertiary,
                )
            }
        }
        if (edge.status == RelationshipEdgeStatuses.SUGGESTED) {
            AssistChip(
                onClick = { onAccept?.invoke() },
                enabled = !accepting,
                label = { Text(stringResource(R.string.relationships_accept)) },
            )
            IconButton(onClick = { onReject?.invoke() }) {
                // #205: the row-action label carries the other party's name so
                // TalkBack doesn't read a bare "Reject" on every row.
                Icon(
                    Icons.Outlined.Close,
                    contentDescription = stringResource(R.string.relationships_reject_named, displayName),
                )
            }
        } else {
            IconButton(onClick = { onEdit?.invoke() }) {
                // #205: the row-action labels carry the other party's name so
                // TalkBack doesn't read a bare "Edit"/"Delete" on every row.
                Icon(
                    Icons.Outlined.Edit,
                    contentDescription = stringResource(R.string.relationships_edit_named, displayName),
                )
            }
            IconButton(onClick = { onDelete?.invoke() }) {
                Icon(
                    Icons.Outlined.Delete,
                    contentDescription = stringResource(R.string.relationships_delete_named, displayName),
                )
            }
        }
    }
}

/** Human label for an edge's effective type token. */
fun relationshipLabel(@Suppress("UnusedParameter") type: String, edge: RelationshipEdge, viewedUid: String): String {
    return effectiveType(edge, viewedUid).replace('_', ' ')
}

private enum class EntryMode { MANUAL, LINKED }

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RelationshipEdgeDialog(
    edge: RelationshipEdge?,
    state: RelationshipsUiState,
    onSearchChange: (String) -> Unit,
    onConfirm: (
        type: String,
        vcardUid: String,
        name: String,
        gender: String,
        birthday: String,
        sensitivity: String,
        linkedContact: ContactSummary?,
    ) -> Unit,
    onDismiss: () -> Unit,
) {
    val isEditing = edge != null
    var entryMode by remember { mutableStateOf(EntryMode.MANUAL) }
    var type by remember {
        mutableStateOf(
            edge?.let { effectiveType(it, state.contactVCardUid) } ?: RelationshipEdgeTypes.FRIEND_OF,
        )
    }
    var typeExpanded by remember { mutableStateOf(false) }
    var sensitivity by remember { mutableStateOf(edge?.sensitivity ?: RelationshipEdgeSensitivities.NORMAL) }
    var sensitivityExpanded by remember { mutableStateOf(false) }
    var name by remember { mutableStateOf("") }
    var gender by remember { mutableStateOf("") }
    var birthday by remember { mutableStateOf("") }
    var selectedContact by remember { mutableStateOf<ContactSummary?>(null) }

    val otherPartyName = edge?.let { e ->
        state.contactsByUid[otherPartyId(e, state.contactVCardUid)]?.let { displayNameFor(it) }
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.relationships_edit_title else R.string.relationships_new)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (isEditing) {
                    Column {
                        Text(
                            text = otherPartyName ?: stringResource(R.string.relationships_unknown_contact),
                            style = MaterialTheme.typography.bodyLarge,
                        )
                        Text(
                            text = stringResource(R.string.relationships_other_party_readonly_hint),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                } else {
                    // #199: bare RadioButtons have no text of their own, and the
                    // label Text carried its own separate .clickable — TalkBack
                    // found two adjacent focusable nodes per row (an unnamed
                    // radio, then a plain clickable label with no role/state).
                    // Modifier.selectable on each row merges the label into the
                    // radio's accessible name; selectableGroup on the Column
                    // groups them for correct radio-group navigation.
                    Column(modifier = Modifier.selectableGroup()) {
                        Text(stringResource(R.string.relationships_entry_mode), style = MaterialTheme.typography.labelMedium)
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.selectable(
                                selected = entryMode == EntryMode.MANUAL,
                                onClick = { entryMode = EntryMode.MANUAL },
                                role = Role.RadioButton,
                            ),
                        ) {
                            RadioButton(
                                selected = entryMode == EntryMode.MANUAL,
                                onClick = null,
                            )
                            Text(text = stringResource(R.string.relationships_enter_manually))
                        }
                        Row(
                            verticalAlignment = Alignment.CenterVertically,
                            modifier = Modifier.selectable(
                                selected = entryMode == EntryMode.LINKED,
                                onClick = { entryMode = EntryMode.LINKED },
                                role = Role.RadioButton,
                            ),
                        ) {
                            RadioButton(
                                selected = entryMode == EntryMode.LINKED,
                                onClick = null,
                            )
                            Text(text = stringResource(R.string.relationships_link_to_contact))
                        }
                    }

                    if (entryMode == EntryMode.LINKED) {
                        if (selectedContact != null) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Text(
                                    text = displayNameFor(selectedContact!!),
                                    style = MaterialTheme.typography.bodyLarge,
                                    modifier = Modifier.weight(1f),
                                )
                                TextButton(onClick = {
                                    selectedContact = null
                                    onSearchChange("")
                                }) {
                                    Text(stringResource(R.string.relationships_change_selection))
                                }
                            }
                        } else {
                            OutlinedTextField(
                                value = state.contactSearchQuery,
                                onValueChange = onSearchChange,
                                label = { Text(stringResource(R.string.relationships_search_contacts)) },
                                singleLine = true,
                                trailingIcon = {
                                    if (state.contactSearchLoading) {
                                        CircularProgressIndicator(modifier = Modifier.padding(4.dp), strokeWidth = 2.dp)
                                    }
                                },
                                modifier = Modifier.fillMaxWidth(),
                            )
                            when {
                                state.contactSearchQuery.isBlank() -> Text(
                                    text = stringResource(R.string.relationships_type_to_search),
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                state.contactSearchResults.isEmpty() && !state.contactSearchLoading -> Text(
                                    text = stringResource(R.string.relationships_no_contacts_found),
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                else -> LazyColumn(modifier = Modifier.heightIn(max = 200.dp)) {
                                    items(state.contactSearchResults, key = { it.id }) { contact ->
                                        Text(
                                            text = displayNameFor(contact),
                                            style = MaterialTheme.typography.bodyMedium,
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .clickable { selectedContact = contact }
                                                .padding(vertical = 8.dp),
                                        )
                                    }
                                }
                            }
                        }
                    } else {
                        OutlinedTextField(
                            value = name,
                            onValueChange = { name = it },
                            label = { Text(stringResource(R.string.relationships_other_name)) },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                        )
                    }
                }

                ExposedDropdownMenuBox(
                    expanded = typeExpanded,
                    onExpandedChange = { typeExpanded = it },
                ) {
                    OutlinedTextField(
                        value = type.replace('_', ' '),
                        onValueChange = {},
                        readOnly = true,
                        label = { Text(stringResource(R.string.relationships_type)) },
                        trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = typeExpanded) },
                        modifier = Modifier
                            .fillMaxWidth()
                            .menuAnchor(),
                    )
                    ExposedDropdownMenu(
                        expanded = typeExpanded,
                        onDismissRequest = { typeExpanded = false },
                    ) {
                        RelationshipEdgeTypes.TYPE_TOKENS.forEach { t ->
                            DropdownMenuItem(
                                text = { Text(t.replace('_', ' ')) },
                                onClick = { type = t; typeExpanded = false },
                            )
                        }
                    }
                }

                if (!isEditing && entryMode == EntryMode.MANUAL) {
                    OutlinedTextField(
                        value = gender,
                        onValueChange = { gender = it },
                        label = { Text(stringResource(R.string.relationships_gender)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = birthday,
                        onValueChange = { birthday = it },
                        label = { Text(stringResource(R.string.relationships_birthday)) },
                        placeholder = { Text(stringResource(R.string.contact_birthday_hint)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                }

                ExposedDropdownMenuBox(
                    expanded = sensitivityExpanded,
                    onExpandedChange = { sensitivityExpanded = it },
                ) {
                    val sensitivityLabel = when (sensitivity) {
                        RelationshipEdgeSensitivities.PRIVATE -> stringResource(R.string.relationships_sensitivity_private)
                        RelationshipEdgeSensitivities.SECRET -> stringResource(R.string.relationships_sensitivity_secret)
                        else -> stringResource(R.string.relationships_sensitivity_normal)
                    }
                    OutlinedTextField(
                        value = sensitivityLabel,
                        onValueChange = {},
                        readOnly = true,
                        label = { Text(stringResource(R.string.relationships_sensitivity)) },
                        trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = sensitivityExpanded) },
                        modifier = Modifier
                            .fillMaxWidth()
                            .menuAnchor(),
                    )
                    ExposedDropdownMenu(
                        expanded = sensitivityExpanded,
                        onDismissRequest = { sensitivityExpanded = false },
                    ) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.relationships_sensitivity_normal)) },
                            onClick = { sensitivity = RelationshipEdgeSensitivities.NORMAL; sensitivityExpanded = false },
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.relationships_sensitivity_private)) },
                            onClick = { sensitivity = RelationshipEdgeSensitivities.PRIVATE; sensitivityExpanded = false },
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.relationships_sensitivity_secret)) },
                            onClick = { sensitivity = RelationshipEdgeSensitivities.SECRET; sensitivityExpanded = false },
                        )
                    }
                }
            }
        },
        confirmButton = {
            val canConfirm = isEditing ||
                (entryMode == EntryMode.MANUAL && name.isNotBlank()) ||
                (entryMode == EntryMode.LINKED && selectedContact != null)
            TextButton(
                onClick = {
                    onConfirm(type, selectedContact?.uid.orEmpty(), name, gender, birthday, sensitivity, selectedContact)
                },
                enabled = canConfirm,
            ) {
                Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        },
    )
}
