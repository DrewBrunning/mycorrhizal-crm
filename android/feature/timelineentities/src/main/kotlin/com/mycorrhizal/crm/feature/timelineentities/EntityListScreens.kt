package com.mycorrhizal.crm.feature.timelineentities

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
private fun EntityListScaffold(
    title: String,
    addLabel: String,
    uiState: EntityListUiState,
    onAdd: () -> Unit,
    onDelete: (String) -> Unit,
    onErrorShown: () -> Unit,
    onBack: () -> Unit,
    dialog: @Composable () -> Unit,
) {
    val snackbarHostState = remember { SnackbarHostState() }

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
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = onAdd) {
                Icon(Icons.Outlined.Add, contentDescription = addLabel)
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                uiState.isLoading -> LoadingSkeleton()
                uiState.items.isEmpty() && uiState.error == null ->
                    EmptyState(message = stringResource(R.string.entities_empty))
                uiState.items.isEmpty() && uiState.error != null -> {
                    Text(
                        text = uiState.error.orEmpty(),
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(uiState.items, key = { it.id }) { item ->
                            Row(
                                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp),
                            ) {
                                Text(
                                    text = item.label,
                                    style = MaterialTheme.typography.bodyLarge,
                                    maxLines = 2,
                                    overflow = TextOverflow.Ellipsis,
                                    modifier = Modifier.weight(1f),
                                )
                                IconButton(onClick = { onDelete(item.id) }, enabled = uiState.deletingId != item.id) {
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

    uiState.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            onErrorShown()
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LifeEventsScreen(
    onBack: () -> Unit,
    viewModel: LifeEventsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showDialog by remember { mutableStateOf(false) }

    EntityListScaffold(
        title = stringResource(R.string.life_events_title),
        addLabel = stringResource(R.string.life_events_new),
        uiState = state,
        onAdd = { showDialog = true },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showDialog) {
            var type by remember { mutableStateOf("") }
            var description by remember { mutableStateOf("") }
            AlertDialog(
                onDismissRequest = { showDialog = false },
                title = { Text(stringResource(R.string.life_events_new)) },
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
                        onClick = { viewModel.create(type, description); showDialog = false },
                        enabled = description.isNotBlank(),
                    ) { Text(stringResource(R.string.action_create)) }
                },
                dismissButton = {
                    TextButton(onClick = { showDialog = false }) { Text(stringResource(R.string.action_cancel)) }
                },
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GiftsScreen(
    onBack: () -> Unit,
    viewModel: GiftsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showDialog by remember { mutableStateOf(false) }

    EntityListScaffold(
        title = stringResource(R.string.gifts_title),
        addLabel = stringResource(R.string.gifts_new),
        uiState = state,
        onAdd = { showDialog = true },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showDialog) {
            var description by remember { mutableStateOf("") }
            AlertDialog(
                onDismissRequest = { showDialog = false },
                title = { Text(stringResource(R.string.gifts_new)) },
                text = {
                    OutlinedTextField(
                        value = description, onValueChange = { description = it },
                        label = { Text(stringResource(R.string.gifts_description)) }, singleLine = true,
                    )
                },
                confirmButton = {
                    TextButton(
                        onClick = { viewModel.create(description); showDialog = false },
                        enabled = description.isNotBlank(),
                    ) { Text(stringResource(R.string.action_create)) }
                },
                dismissButton = {
                    TextButton(onClick = { showDialog = false }) { Text(stringResource(R.string.action_cancel)) }
                },
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PreferencesScreen(
    onBack: () -> Unit,
    viewModel: PreferencesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showDialog by remember { mutableStateOf(false) }

    EntityListScaffold(
        title = stringResource(R.string.preferences_title),
        addLabel = stringResource(R.string.preferences_new),
        uiState = state,
        onAdd = { showDialog = true },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showDialog) {
            var category by remember { mutableStateOf("") }
            var value by remember { mutableStateOf("") }
            AlertDialog(
                onDismissRequest = { showDialog = false },
                title = { Text(stringResource(R.string.preferences_new)) },
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
                        onClick = { viewModel.create(category, value); showDialog = false },
                        enabled = category.isNotBlank() && value.isNotBlank(),
                    ) { Text(stringResource(R.string.action_create)) }
                },
                dismissButton = {
                    TextButton(onClick = { showDialog = false }) { Text(stringResource(R.string.action_cancel)) }
                },
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConversationAgendaScreen(
    onBack: () -> Unit,
    viewModel: ConversationAgendaViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showDialog by remember { mutableStateOf(false) }

    EntityListScaffold(
        title = stringResource(R.string.agenda_title),
        addLabel = stringResource(R.string.agenda_new),
        uiState = state,
        onAdd = { showDialog = true },
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
        onBack = onBack,
    ) {
        if (showDialog) {
            var content by remember { mutableStateOf("") }
            AlertDialog(
                onDismissRequest = { showDialog = false },
                title = { Text(stringResource(R.string.agenda_new)) },
                text = {
                    OutlinedTextField(
                        value = content, onValueChange = { content = it },
                        label = { Text(stringResource(R.string.agenda_content)) }, singleLine = true,
                    )
                },
                confirmButton = {
                    TextButton(
                        onClick = { viewModel.create(content); showDialog = false },
                        enabled = content.isNotBlank(),
                    ) { Text(stringResource(R.string.action_create)) }
                },
                dismissButton = {
                    TextButton(onClick = { showDialog = false }) { Text(stringResource(R.string.action_cancel)) }
                },
            )
        }
    }
}
