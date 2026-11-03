package com.mycorrhizal.crm.feature.timeline

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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
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
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.domain.repository.NotesScope
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FabListContentPadding
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.components.localizedMessage
import com.mycorrhizal.crm.ui.markdown.MarkdownText

/**
 * The "Notes" drawer entry — every note by default with an All notes / Unfiled filter and each
 * note's contact (matching web's `NotesPage.tsx`, issue #2224). Issue #1757: under the Unfiled
 * filter, which lists only the notes with no contact, the title and the empty state say
 * "unassigned" rather than "Notes". [onNoteClick] and [onCreateNote] reuse the per-contact note form routes with contact id `0`
 * (never a real id): [NoteFormViewModel] treats it as "no contact", so a new note saved from here
 * is created unassigned and an existing unassigned note edits unmodified.
 */
@Composable
fun NotesInboxScreen(
    onNoteClick: (Int) -> Unit,
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    onCreateNote: () -> Unit = {},
    onContactClick: (Int) -> Unit = {},
    viewModel: NotesInboxViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    // Issue #1757: reload after returning from the new-note form.
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.onResumed() }
    NotesInboxScreenContent(
        uiState = state,
        onMenuClick = onMenuClick,
        onNoteClick = onNoteClick,
        onCreateNote = onCreateNote,
        onContactClick = onContactClick,
        onScopeChange = viewModel::setScope,
        onLoadMore = viewModel::loadMore,
        onRefresh = viewModel::load,
        onErrorShown = viewModel::onErrorShown
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
    onCreateNote: () -> Unit = {},
    onContactClick: (Int) -> Unit = {},
    onScopeChange: (NotesScope) -> Unit = {},
    onLoadMore: () -> Unit = {},
    onRefresh: () -> Unit = {},
    onErrorShown: () -> Unit = {}
) {
    val state = uiState
    val snackbarHostState = remember { SnackbarHostState() }
    val currentOnErrorShown = rememberUpdatedState(onErrorShown)

    Scaffold(
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
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            stringResource(
                                if (state.scope == NotesScope.UNFILED) {
                                    R.string.nav_notes_unassigned
                                } else {
                                    R.string.nav_notes
                                }
                            ),
                            style = MaterialTheme.typography.titleLarge
                        )
                        if (state.total > 0) {
                            Text(
                                text = pluralStringResource(
                                    R.plurals.notes_inbox_total,
                                    state.total,
                                    state.total
                                ),
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.padding(start = 8.dp)
                            )
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        floatingActionButton = {
            BrandFab(onClick = onCreateNote) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.cd_new_note))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            NotesScopeFilter(scope = state.scope, onScopeChange = onScopeChange)
            RefreshableContent(
                isRefreshing = state.isLoading && state.notes.isNotEmpty(),
                onRefresh = onRefresh,
                modifier = Modifier.fillMaxWidth().weight(1f)
            ) {
                when {
                    state.isLoading && state.notes.isEmpty() ->
                        LoadingSkeleton(modifier = Modifier.testTag("notes-inbox-loading"))

                    state.notes.isEmpty() && state.error == null ->
                        // Issue #1757: says which set is empty and where its notes come from.
                        EmptyState(
                            title = stringResource(
                                if (state.scope == NotesScope.UNFILED) {
                                    R.string.notes_unassigned_empty
                                } else {
                                    R.string.notes_empty
                                }
                            ),
                            body = stringResource(
                                if (state.scope == NotesScope.UNFILED) {
                                    R.string.notes_unassigned_empty_body
                                } else {
                                    R.string.notes_empty_body
                                }
                            ),
                            actionLabel = stringResource(R.string.cd_new_note),
                            onAction = onCreateNote
                        )

                    state.notes.isEmpty() && state.error != null ->
                        EmptyState(state.error.localizedMessage())

                    else -> {
                        LazyColumn(
                            modifier = Modifier.fillMaxSize().testTag("notes-inbox-list"),
                            contentPadding = FabListContentPadding
                        ) {
                            items(state.notes, key = { it.id }) { note ->
                                InboxNoteRow(
                                    note = note,
                                    onClick = { onNoteClick(note.id) },
                                    onContactClick = onContactClick
                                )
                            }
                            if (!state.nextCursor.isNullOrEmpty()) {
                                item {
                                    Box(
                                        modifier = Modifier.fillMaxWidth().padding(16.dp),
                                        contentAlignment = Alignment.Center
                                    ) {
                                        Button(
                                            onClick = onLoadMore,
                                            enabled = !state.isLoadingMore
                                        ) {
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

    val listError = state.error?.localizedMessage()
    if (listError != null && state.notes.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            currentOnErrorShown.value()
        }
    }
}

@Composable
private fun NotesScopeFilter(scope: NotesScope, onScopeChange: (NotesScope) -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        FilterChip(
            selected = scope == NotesScope.ALL,
            onClick = { onScopeChange(NotesScope.ALL) },
            label = { Text(stringResource(R.string.notes_filter_all)) },
            modifier = Modifier.heightIn(min = 48.dp)
        )
        FilterChip(
            selected = scope == NotesScope.UNFILED,
            onClick = { onScopeChange(NotesScope.UNFILED) },
            label = { Text(stringResource(R.string.notes_filter_unfiled)) },
            modifier = Modifier.heightIn(min = 48.dp)
        )
    }
}

@Composable
private fun InboxNoteRow(
    note: Note,
    onClick: () -> Unit,
    onContactClick: (Int) -> Unit,
    modifier: Modifier = Modifier
) {
    val dateFormat = LocalDateFormat.current
    val date = note.date?.let {
        DateFormat.formatTimestamp(
            it,
            dateFormat
        )
    }?.takeIf { it.isNotBlank() }
    androidx.compose.material3.ListItem(
        headlineContent = {
            MarkdownText(
                source = note.content.orEmpty(),
                maxLines = 2,
                overflow = TextOverflow.Ellipsis
            )
        },
        supportingContent = if (date != null || note.contact != null) {
            {
                Column {
                    date?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                    // Issue #2224: the contact each filed note belongs to; unfiled notes have none.
                    note.contact?.let { contact ->
                        AssistChip(
                            onClick = { onContactClick(contact.id) },
                            label = { Text(contact.displayName) },
                            // #214: 48dp touch target, as in the Activities contact chips.
                            modifier = Modifier.heightIn(min = 48.dp)
                        )
                    }
                }
            }
        } else {
            null
        },
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
    )
}
