package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.clickable
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
import androidx.compose.material3.Button
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

/**
 * M19: per-contact notes list. Gained a search field + from/to-date filters
 * (debounced server-side filters), T17 cursor "load more", and delete with
 * confirmation (M17's rule) — the note edit form stays for content/date and
 * the new contact reassignment picker.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotesScreen(
    onBack: () -> Unit,
    onCreateNote: () -> Unit,
    onEditNote: (Int) -> Unit,
    viewModel: NotesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    NotesScreenContent(
        uiState = state,
        onBack = onBack,
        onCreateNote = onCreateNote,
        onEditNote = onEditNote,
        onSearchChange = viewModel::onSearchChange,
        onFromDateChange = viewModel::onFromDateChange,
        onToDateChange = viewModel::onToDateChange,
        onLoadMore = viewModel::loadMore,
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless screen content, split out from [NotesScreen] so it's directly
 * testable without a Hilt-backed ViewModel (mirrors `ContactListScreenContent`).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotesScreenContent(
    uiState: NotesUiState,
    onBack: () -> Unit = {},
    onCreateNote: () -> Unit = {},
    onEditNote: (Int) -> Unit = {},
    onSearchChange: (String) -> Unit = {},
    onFromDateChange: (String) -> Unit = {},
    onToDateChange: (String) -> Unit = {},
    onLoadMore: () -> Unit = {},
    onDelete: (Int) -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val state = uiState
    val snackbarHostState = remember { SnackbarHostState() }
    var pendingDelete by remember { mutableStateOf<Note?>(null) }

    val hasFilters = state.searchQuery.isNotBlank() || state.fromDate.isNotBlank() || state.toDate.isNotBlank()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.nav_notes), style = MaterialTheme.typography.titleLarge)
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
            BrandFab(onClick = onCreateNote) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.cd_new_note))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            TimelineFilterRow(
                searchQuery = state.searchQuery,
                onSearchChange = onSearchChange,
                fromDate = state.fromDate,
                onFromDateChange = onFromDateChange,
                toDate = state.toDate,
                onToDateChange = onToDateChange,
                searchLabelRes = R.string.notes_search,
            )
            Box(modifier = Modifier.fillMaxSize()) {
                when {
                    state.isLoading -> LoadingSkeleton()
                    state.notes.isEmpty() && state.error == null ->
                        EmptyState(message = stringResource(if (hasFilters) R.string.notes_no_results else R.string.notes_empty))
                    state.notes.isEmpty() && (state.errorRes != null || state.error != null) ->
                        EmptyState(state.errorRes?.let { stringResource(it) } ?: state.error.orEmpty())
                    else -> {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(state.notes, key = { it.id }) { note ->
                                NoteListItem(
                                    note = note,
                                    onClick = { onEditNote(note.id) },
                                    onDelete = { pendingDelete = note },
                                    isDeleting = state.deletingId == note.id,
                                )
                            }
                            if (!state.nextCursor.isNullOrEmpty()) {
                                item {
                                    Box(modifier = Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
                                        Button(onClick = onLoadMore, enabled = !state.isLoadingMore) {
                                            Text(stringResource(R.string.action_load_more))
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    pendingDelete?.let { note ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.notes_delete_title)) },
            text = { Text(stringResource(R.string.notes_delete_confirm, note.content.orEmpty().take(80))) },
            confirmButton = {
                TextButton(onClick = {
                    onDelete(note.id)
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

    // When the list is empty the error text is the persistent body content
    // (EmptyState above), so don't toast-and-clear it into a misleading
    // "No notes yet". Only surface a snackbar for errors over a populated list.
    val listError = state.error
    if (listError != null && state.notes.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            onErrorShown()
        }
    }
}

/** Search + from/to-date filter row shared by the notes and activities lists. */
@Composable
fun TimelineFilterRow(
    searchQuery: String,
    onSearchChange: (String) -> Unit,
    fromDate: String,
    onFromDateChange: (String) -> Unit,
    toDate: String,
    onToDateChange: (String) -> Unit,
    searchLabelRes: Int,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        OutlinedTextField(
            value = searchQuery,
            onValueChange = onSearchChange,
            label = { Text(stringResource(searchLabelRes)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth(),
        )
        Row(modifier = Modifier.fillMaxWidth().padding(top = 8.dp)) {
            OutlinedTextField(
                value = fromDate,
                onValueChange = onFromDateChange,
                label = { Text(stringResource(R.string.timeline_from_date)) },
                placeholder = { Text("YYYY-MM-DD") },
                singleLine = true,
                modifier = Modifier
                    .weight(1f)
                    .testTag("timeline-from-date"),
            )
            OutlinedTextField(
                value = toDate,
                onValueChange = onToDateChange,
                label = { Text(stringResource(R.string.timeline_to_date)) },
                placeholder = { Text("YYYY-MM-DD") },
                singleLine = true,
                modifier = Modifier
                    .weight(1f)
                    .padding(start = 8.dp)
                    .testTag("timeline-to-date"),
            )
        }
    }
}

@Composable
fun NoteListItem(
    note: Note,
    onClick: () -> Unit,
    onDelete: () -> Unit,
    isDeleting: Boolean = false,
    modifier: Modifier = Modifier,
) {
    androidx.compose.material3.ListItem(
        headlineContent = {
            Text(
                text = note.content.orEmpty(),
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        },
        supportingContent = note.date?.take(10)?.let { date ->
            { Text(date, style = MaterialTheme.typography.bodySmall) }
        },
        trailingContent = {
            IconButton(onClick = onDelete, enabled = !isDeleting) {
                Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
            }
        },
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
    )
}
