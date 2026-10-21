package com.mycorrhizal.crm.feature.relationships

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
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
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
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
import androidx.compose.runtime.saveable.listSaver
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.activity.compose.BackHandler
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
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
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.DateField
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.DropdownOption
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.components.LabelledDropdown
import com.mycorrhizal.crm.ui.components.relationshipRoleLabel
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
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
    // Issue #1773: the create/edit form is full screen and replaces the list, so the open
    // flag and the edited edge's id are saved (rotation, process death) and the list's scroll
    // state is hoisted above the switch (Back returns to where the list was).
    var showCreateForm by rememberSaveable { mutableStateOf(false) }
    var editingEdgeId by rememberSaveable { mutableStateOf<String?>(null) }
    val editingEdge = editingEdgeId?.let { id -> state.edges.firstOrNull { it.id == id } }
    val listState = rememberLazyListState()
    var pendingDelete by remember { mutableStateOf<RelationshipEdge?>(null) }
    var pendingReject by remember { mutableStateOf<RelationshipEdge?>(null) }
    val errorMessage = state.errorRes?.let { stringResource(it) } ?: state.error

    @Composable
    fun displayName(edge: RelationshipEdge): String {
        val uid = otherPartyId(edge, state.contactVCardUid)
        val contact = state.contactsByUid[uid]
        return contact?.let { displayNameFor(it) } ?: stringResource(R.string.relationships_unknown_contact)
    }

    if (showCreateForm || editingEdge != null) {
        RelationshipEdgeForm(
            edge = editingEdge,
            state = state,
            snackbarHostState = snackbarHostState,
            onSearchChange = viewModel::searchContacts,
            onConfirm = { type, uid, name, gender, birthday, sensitivity, linked ->
                if (editingEdge != null) {
                    viewModel.update(editingEdge.id, type, sensitivity)
                    editingEdgeId = null
                } else {
                    viewModel.create(type, uid, name, gender, birthday, sensitivity, linked)
                    showCreateForm = false
                }
            },
            onDismiss = {
                showCreateForm = false
                editingEdgeId = null
                viewModel.clearContactSearch()
            },
        )
    } else {
        Scaffold(
            topBar = {
                TopAppBar(
                    navigationIcon = {
                        AccessibleIconButton(onClick = onBack) {
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
                    showCreateForm = true
                }) {
                    Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.relationships_new))
                }
            },
            snackbarHost = { SnackbarHost(snackbarHostState) },
        ) { padding ->
            Box(modifier = Modifier.fillMaxSize().padding(padding)) {
                RefreshableContent(
                    isRefreshing = state.isLoading && state.edges.isNotEmpty(),
                    onRefresh = viewModel::load,
                    modifier = Modifier.fillMaxSize(),
                ) {
                    when {
                        state.isLoading && state.edges.isEmpty() -> LoadingSkeleton()
                        state.edges.isEmpty() && errorMessage == null ->
                            EmptyState(
                                title = stringResource(R.string.relationships_empty),
                                body = stringResource(R.string.relationships_empty_body),
                                actionLabel = stringResource(R.string.relationships_new),
                                onAction = {
                                    viewModel.clearContactSearch()
                                    showCreateForm = true
                                },
                            )
                        state.edges.isEmpty() && errorMessage != null -> {
                            Text(
                                text = errorMessage,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.align(Alignment.Center),
                            )
                        }
                        else -> {
                            LazyColumn(state = listState, modifier = Modifier.fillMaxSize().testTag("relationship-list")) {
                                items(state.confirmedEdges, key = { it.id }) { edge ->
                                    RelationshipEdgeRow(
                                        edge = edge,
                                        displayName = displayName(edge),
                                        resolvedContactId = state.contactsByUid[otherPartyId(edge, state.contactVCardUid)]?.id,
                                        viewedUid = state.contactVCardUid,
                                        onNavigateToContact = onNavigateToContact,
                                        onEdit = { editingEdgeId = edge.id },
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
        }
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
            AccessibleIconButton(onClick = { onReject?.invoke() }) {
                // #205: the row-action label carries the other party's name so
                // TalkBack doesn't read a bare "Reject" on every row.
                Icon(
                    Icons.Outlined.Close,
                    contentDescription = stringResource(R.string.relationships_reject_named, displayName),
                )
            }
        } else {
            AccessibleIconButton(onClick = { onEdit?.invoke() }) {
                // #205: the row-action labels carry the other party's name so
                // TalkBack doesn't read a bare "Edit"/"Delete" on every row.
                Icon(
                    Icons.Outlined.Edit,
                    contentDescription = stringResource(R.string.relationships_edit_named, displayName),
                )
            }
            AccessibleIconButton(onClick = { onDelete?.invoke() }) {
                Icon(
                    Icons.Outlined.Delete,
                    contentDescription = stringResource(R.string.relationships_delete_named, displayName),
                )
            }
        }
    }
}

/** Human label for an edge's effective type token. */
@Composable
fun relationshipLabel(@Suppress("UnusedParameter") type: String, edge: RelationshipEdge, viewedUid: String): String {
    return relationshipRoleLabel(effectiveType(edge, viewedUid))
}

/** Linked-contact selection survives rotation and process death (a plain data class is not Parcelable). */
private val ContactSummarySaver = listSaver<ContactSummary?, Any?>(
    save = { c ->
        if (c == null) emptyList() else listOf(c.id, c.uid, c.firstname, c.lastname, c.nickname, c.fn)
    },
    restore = { l ->
        if (l.isEmpty()) {
            null
        } else {
            ContactSummary(
                id = l[0] as Int,
                uid = l[1] as String?,
                firstname = l[2] as String?,
                lastname = l[3] as String?,
                nickname = l[4] as String?,
                fn = l[5] as String?,
            )
        }
    },
)

/**
 * Issue #1773 (maintainer decision 2026-10-20): the relationship create/edit form is full
 * screen -- it has a contact search and two fixed-list pickers, and the rule is that any
 * picker means a full-screen form. [FormScaffold] with the pinned Save/Create button that
 * says what enables it while disabled. System back and the top-bar arrow leave the form,
 * discarding what was typed (what Cancel did in the dialog this replaced). All state is
 * `rememberSaveable`, so the open form and its input survive rotation and process death.
 *
 * The type picker's tokens describe the OTHER party relative to the viewed contact
 * (create sends `target_id = viewed contact`); [onConfirm] gets that token unchanged.
 */
@Composable
fun RelationshipEdgeForm(
    edge: RelationshipEdge?,
    state: RelationshipsUiState,
    snackbarHostState: SnackbarHostState,
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
    BackHandler(onBack = onDismiss)
    val isEditing = edge != null
    var linkedMode by rememberSaveable(edge?.id) { mutableStateOf(false) }
    var type by rememberSaveable(edge?.id) {
        mutableStateOf(
            edge?.let { effectiveType(it, state.contactVCardUid) } ?: RelationshipEdgeTypes.FRIEND_OF,
        )
    }
    var sensitivity by rememberSaveable(edge?.id) {
        mutableStateOf(edge?.sensitivity ?: RelationshipEdgeSensitivities.NORMAL)
    }
    var name by rememberSaveable(edge?.id) { mutableStateOf("") }
    var gender by rememberSaveable(edge?.id) { mutableStateOf("") }
    var birthday by rememberSaveable(edge?.id) { mutableStateOf("") }
    var selectedContact by rememberSaveable(edge?.id, stateSaver = ContactSummarySaver) {
        mutableStateOf<ContactSummary?>(null)
    }

    val otherPartyName = edge?.let { e ->
        state.contactsByUid[otherPartyId(e, state.contactVCardUid)]?.let { displayNameFor(it) }
    }
    val canConfirm = isEditing ||
        (!linkedMode && name.isNotBlank()) ||
        (linkedMode && selectedContact != null)

    FormScaffold(
        title = stringResource(if (isEditing) R.string.relationships_edit_title else R.string.relationships_new),
        onBack = onDismiss,
        saveLabel = stringResource(if (isEditing) R.string.action_save else R.string.action_create),
        isSaving = false,
        onSave = {
            onConfirm(type, selectedContact?.uid.orEmpty(), name, gender, birthday, sensitivity, selectedContact)
        },
        snackbarHostState = snackbarHostState,
        saveEnabled = canConfirm,
        saveDisabledHint = stringResource(
            if (linkedMode) R.string.relationships_save_hint_contact else R.string.relationships_save_hint_name,
        ),
    ) { padding ->
        Column(
            modifier = Modifier
                .padding(padding)
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
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
                            selected = !linkedMode,
                            onClick = { linkedMode = false },
                            role = Role.RadioButton,
                        ),
                    ) {
                        RadioButton(selected = !linkedMode, onClick = null)
                        Text(text = stringResource(R.string.relationships_enter_manually))
                    }
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier.selectable(
                            selected = linkedMode,
                            onClick = { linkedMode = true },
                            role = Role.RadioButton,
                        ),
                    ) {
                        RadioButton(selected = linkedMode, onClick = null)
                        Text(text = stringResource(R.string.relationships_link_to_contact))
                    }
                }

                if (linkedMode) {
                    val chosen = selectedContact
                    if (chosen != null) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(
                                text = displayNameFor(chosen),
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
                            modifier = Modifier.fillMaxWidth().testTag("relationship-contact-search"),
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
                            // A plain Column, not a LazyColumn: the form scrolls as a whole and
                            // the search returns at most a page (25) of results.
                            else -> Column {
                                state.contactSearchResults.forEach { contact ->
                                    Text(
                                        text = displayNameFor(contact),
                                        style = MaterialTheme.typography.bodyMedium,
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .heightIn(min = 48.dp)
                                            .clickable(role = Role.Button) { selectedContact = contact }
                                            .padding(vertical = 12.dp),
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

            LabelledDropdown(
                label = stringResource(R.string.relationships_type),
                valueText = relationshipRoleLabel(type),
                options = RelationshipEdgeTypes.TYPE_TOKENS.map { DropdownOption(it, relationshipRoleLabel(it)) },
                onSelect = { type = it },
                modifier = Modifier.testTag("relationship-type"),
            )

            if (!isEditing && !linkedMode) {
                OutlinedTextField(
                    value = gender,
                    onValueChange = { gender = it },
                    label = { Text(stringResource(R.string.relationships_gender)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                DateField(
                    value = birthday,
                    onValueChange = { birthday = it },
                    label = stringResource(R.string.relationships_birthday),
                    mode = DateFieldMode.PARTIAL,
                    allowClear = true,
                    modifier = Modifier.fillMaxWidth(),
                )
            }

            LabelledDropdown(
                label = stringResource(R.string.relationships_sensitivity),
                valueText = when (sensitivity) {
                    RelationshipEdgeSensitivities.PRIVATE -> stringResource(R.string.relationships_sensitivity_private)
                    RelationshipEdgeSensitivities.SECRET -> stringResource(R.string.relationships_sensitivity_secret)
                    else -> stringResource(R.string.relationships_sensitivity_normal)
                },
                options = listOf(
                    DropdownOption(
                        RelationshipEdgeSensitivities.NORMAL,
                        stringResource(R.string.relationships_sensitivity_normal),
                    ),
                    DropdownOption(
                        RelationshipEdgeSensitivities.PRIVATE,
                        stringResource(R.string.relationships_sensitivity_private),
                    ),
                    DropdownOption(
                        RelationshipEdgeSensitivities.SECRET,
                        stringResource(R.string.relationships_sensitivity_secret),
                    ),
                ),
                onSelect = { sensitivity = it },
                modifier = Modifier.testTag("relationship-sensitivity"),
            )
        }
    }
}
