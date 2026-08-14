package com.mycorrhizal.crm.feature.timelineentities

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
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.OpenInNew
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

// M17 (99-M17-android-entity-scaffold-edit-delete-confirm.md): the shared
// scaffold behind Life Events/Gifts/Preferences/Conversation Agenda gets a
// delete-confirmation dialog and a tap-to-edit path, once, here -- so all
// four entity types get both fixes from one change instead of four separate
// patches. Field-level richness beyond what each create dialog already
// modeled is M18's job, not this one's.

/**
 * The shared entity-list scaffold. `internal` (not `private`) so
 * EntityListScaffoldTest can drive it directly with a fake [EntityListUiState]
 * and plain callbacks -- no ViewModel, no coroutines, matching this module's
 * existing test-layer split (ViewModel logic is tested separately in
 * TimelineEntitiesViewModelTest).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun EntityListScaffold(
    title: String,
    addLabel: String,
    uiState: EntityListUiState,
    onAdd: () -> Unit,
    onItemClick: (String) -> Unit,
    onDelete: (String) -> Unit,
    onErrorShown: () -> Unit,
    onBack: () -> Unit,
    dialog: @Composable () -> Unit,
) {
    val snackbarHostState = remember { SnackbarHostState() }
    val errorMessage = uiState.errorRes?.let { stringResource(it) } ?: uiState.error
    // One delete-confirm dialog for the whole list rather than per-row state:
    // only one row's delete can plausibly be in flight/confirmed at a time.
    var pendingDeleteId by remember { mutableStateOf<String?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(title, style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
        floatingActionButton = {
            BrandFab(onClick = onAdd) {
                Icon(Icons.Outlined.Add, contentDescription = addLabel)
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                uiState.isLoading -> LoadingSkeleton()
                uiState.items.isEmpty() && errorMessage == null ->
                    EmptyState(message = stringResource(R.string.entities_empty))
                uiState.items.isEmpty() && errorMessage != null -> {
                    Text(
                        text = errorMessage,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                else -> {
                    val uriHandler = LocalUriHandler.current
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(uiState.items, key = { it.id }) { item ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable(onClick = { onItemClick(item.id) })
                                    .padding(horizontal = 16.dp, vertical = 12.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp),
                            ) {
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(
                                        text = item.label,
                                        style = MaterialTheme.typography.bodyLarge,
                                        maxLines = 2,
                                        overflow = TextOverflow.Ellipsis,
                                    )
                                    // T62 Android port: a gift/agenda item's
                                    // URL wasn't rendered anywhere before --
                                    // now shown as a second line, colored
                                    // brand green to match the contact page's
                                    // LinkRow (ContactDetailScreen.kt).
                                    if (!item.url.isNullOrBlank()) {
                                        Text(
                                            text = item.url,
                                            style = MaterialTheme.typography.bodyMedium,
                                            color = MaterialTheme.colorScheme.primary,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis,
                                        )
                                    }
                                }
                                if (!item.url.isNullOrBlank()) {
                                    IconButton(onClick = { uriHandler.openUri(item.url) }) {
                                        Icon(
                                            Icons.Outlined.OpenInNew,
                                            contentDescription = stringResource(R.string.cd_open_link),
                                            tint = MaterialTheme.colorScheme.primary,
                                        )
                                    }
                                }
                                // Opens the shared confirm dialog below rather than
                                // calling onDelete directly -- the whole point of
                                // this ticket's delete-confirmation requirement.
                                IconButton(
                                    onClick = { pendingDeleteId = item.id },
                                    enabled = uiState.deletingId != item.id,
                                ) {
                                    Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    dialog()

    pendingDeleteId?.let { id ->
        val label = uiState.items.firstOrNull { it.id == id }?.label
        AlertDialog(
            onDismissRequest = { pendingDeleteId = null },
            title = { Text(stringResource(R.string.entities_delete_title)) },
            text = {
                Text(
                    label?.let { stringResource(R.string.entities_delete_confirm, it) }
                        ?: stringResource(R.string.entities_delete_title),
                )
            },
            confirmButton = {
                TextButton(onClick = { onDelete(id); pendingDeleteId = null }) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteId = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            onErrorShown()
        }
    }
}

// ---------------------------------------------------------------------------
// Life events
// ---------------------------------------------------------------------------

/**
 * `initial == null` is add mode; a non-null [LifeEvent] pre-fills every field
 * this form models and switches the dialog to edit mode (title/button label,
 * and [onConfirm] is expected to route to update() instead of create() --
 * that branch lives in the caller, which is the one that knows whether it has
 * an `editingItem`).
 */
@Composable
internal fun LifeEventDialog(
    initial: LifeEvent?,
    onConfirm: (type: String, description: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var type by remember(initial) { mutableStateOf(initial?.type ?: "") }
    var description by remember(initial) { mutableStateOf(initial?.description ?: "") }
    val isEditing = initial != null
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.life_events_edit else R.string.life_events_new)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = type, onValueChange = { type = it },
                    label = { Text(stringResource(R.string.life_events_type)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = description, onValueChange = { description = it },
                    label = { Text(stringResource(R.string.life_events_description)) }, singleLine = true,
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(type, description) },
                enabled = description.isNotBlank(),
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LifeEventsScreen(
    onBack: () -> Unit,
    viewModel: LifeEventsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<LifeEvent?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.life_events_title),
        addLabel = stringResource(R.string.life_events_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showAdd || editingItem != null) {
            LifeEventDialog(
                initial = editingItem,
                onConfirm = { type, description ->
                    editingItem?.let { viewModel.update(it, type, description) }
                        ?: viewModel.create(type, description)
                    showAdd = false
                    editingItem = null
                },
                onDismiss = { showAdd = false; editingItem = null },
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Gifts
// ---------------------------------------------------------------------------

@Composable
internal fun GiftDialog(
    initial: Gift?,
    onConfirm: (description: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var description by remember(initial) { mutableStateOf(initial?.description ?: "") }
    val isEditing = initial != null
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.gifts_edit else R.string.gifts_new)) },
        text = {
            OutlinedTextField(
                value = description, onValueChange = { description = it },
                label = { Text(stringResource(R.string.gifts_description)) }, singleLine = true,
            )
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(description) },
                enabled = description.isNotBlank(),
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GiftsScreen(
    onBack: () -> Unit,
    viewModel: GiftsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<Gift?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.gifts_title),
        addLabel = stringResource(R.string.gifts_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showAdd || editingItem != null) {
            GiftDialog(
                initial = editingItem,
                onConfirm = { description ->
                    editingItem?.let { viewModel.update(it, description) }
                        ?: viewModel.create(description)
                    showAdd = false
                    editingItem = null
                },
                onDismiss = { showAdd = false; editingItem = null },
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

@Composable
internal fun PreferenceDialog(
    initial: Preference?,
    onConfirm: (category: String, value: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var category by remember(initial) { mutableStateOf(initial?.category ?: "") }
    var value by remember(initial) { mutableStateOf(initial?.value ?: "") }
    val isEditing = initial != null
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.preferences_edit else R.string.preferences_new)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = category, onValueChange = { category = it },
                    label = { Text(stringResource(R.string.preferences_category)) }, singleLine = true,
                )
                OutlinedTextField(
                    value = value, onValueChange = { value = it },
                    label = { Text(stringResource(R.string.preferences_value)) }, singleLine = true,
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(category, value) },
                enabled = category.isNotBlank() && value.isNotBlank(),
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PreferencesScreen(
    onBack: () -> Unit,
    viewModel: PreferencesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<Preference?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.preferences_title),
        addLabel = stringResource(R.string.preferences_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showAdd || editingItem != null) {
            PreferenceDialog(
                initial = editingItem,
                onConfirm = { category, value ->
                    editingItem?.let { viewModel.update(it, category, value) }
                        ?: viewModel.create(category, value)
                    showAdd = false
                    editingItem = null
                },
                onDismiss = { showAdd = false; editingItem = null },
            )
        }
    }
}

// ---------------------------------------------------------------------------
// Conversation agenda
// ---------------------------------------------------------------------------

@Composable
internal fun AgendaDialog(
    initial: ConversationAgenda?,
    onConfirm: (content: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var content by remember(initial) { mutableStateOf(initial?.content ?: "") }
    val isEditing = initial != null
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(if (isEditing) R.string.agenda_edit else R.string.agenda_new)) },
        text = {
            OutlinedTextField(
                value = content, onValueChange = { content = it },
                label = { Text(stringResource(R.string.agenda_content)) }, singleLine = true,
            )
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(content) },
                enabled = content.isNotBlank(),
            ) { Text(stringResource(if (isEditing) R.string.action_save else R.string.action_create)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConversationAgendaScreen(
    onBack: () -> Unit,
    viewModel: ConversationAgendaViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showAdd by remember { mutableStateOf(false) }
    var editingItem by remember { mutableStateOf<ConversationAgenda?>(null) }

    EntityListScaffold(
        title = stringResource(R.string.agenda_title),
        addLabel = stringResource(R.string.agenda_new),
        uiState = state,
        onAdd = { showAdd = true },
        onItemClick = { id -> viewModel.findById(id)?.let { editingItem = it } },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showAdd || editingItem != null) {
            AgendaDialog(
                initial = editingItem,
                onConfirm = { content ->
                    editingItem?.let { viewModel.update(it, content) }
                        ?: viewModel.create(content)
                    showAdd = false
                    editingItem = null
                },
                onDismiss = { showAdd = false; editingItem = null },
            )
        }
    }
}
