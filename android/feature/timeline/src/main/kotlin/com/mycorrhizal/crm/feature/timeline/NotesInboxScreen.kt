package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * M9 item 1: the "Notes" drawer entry — a contact-agnostic view of the N4 unfiled-notes queue
 * (matching web's `NotesPage.tsx`), reachable from the drawer instead of the old
 * `PlaceholderScreen` stub. Read + tap-to-edit only: creating a *new* unfiled note has no wired
 * client endpoint yet (only `listNotes` was added for this ticket), so there's no FAB here — an
 * unfiled note is still created the same way it always was, by leaving the contact field blank
 * when editing an existing note. [onNoteClick] reuses the existing per-contact edit route with
 * contact id `0` (never a real id) — [NoteFormViewModel] only reads that id for a brand-new note,
 * so editing an already-unfiled note works unmodified.
 */
@Composable
fun NotesInboxScreen(
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    onNoteClick: (Int) -> Unit,
    viewModel: NotesInboxViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    NotesInboxScreenContent(
        uiState = state,
        onMenuClick = onMenuClick,
        onNoteClick = onNoteClick,
        onLoadMore = viewModel::loadMore,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless screen content, split out from [NotesInboxScreen] so it's directly testable without a
 * Hilt-backed ViewModel (mirrors `ContactListScreenContent`'s split in `ContactListScreen.kt`).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotesInboxScreenContent(
    uiState: NotesInboxUiState,
    // Issue #150: see NotesInboxScreen — null hides the hamburger.
    onMenuClick: (() -> Unit)? = {},
    onNoteClick: (Int) -> Unit = {},
    onLoadMore: () -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val state = uiState
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    onMenuClick?.let { onMenu ->
                        AccessibleIconButton(onClick = onMenu) {
                            Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                        }
                    }
                },
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(stringResource(R.string.nav_notes), style = MaterialTheme.typography.titleLarge)
                        if (state.total > 0) {
                            Text(
                                text = stringResource(R.string.notes_inbox_total, state.total),
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.padding(start = 8.dp),
                            )
                        }
                    }
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
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton(modifier = Modifier.testTag("notes-inbox-loading"))
                state.notes.isEmpty() && state.error == null ->
                    EmptyState(message = stringResource(R.string.notes_empty))
                state.notes.isEmpty() && state.error != null ->
                    EmptyState(state.error.orEmpty())
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize().testTag("notes-inbox-list")) {
                        items(state.notes, key = { it.id }) { note ->
                            InboxNoteRow(note = note, onClick = { onNoteClick(note.id) })
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

    val listError = state.error
    if (listError != null && state.notes.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            onErrorShown()
        }
    }
}

@Composable
private fun InboxNoteRow(
    note: Note,
    onClick: () -> Unit,
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
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
    )
}
