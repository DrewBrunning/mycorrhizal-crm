package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SearchActivityHit
import com.mycorrhizal.crm.model.network.SearchNoteHit
import com.mycorrhizal.crm.model.network.SearchResult
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactListScreen(
    onContactClick: (Int) -> Unit,
    onCreateContact: () -> Unit = {},
    onMenuClick: () -> Unit = {},
    onImportContacts: () -> Unit = {},
    viewModel: ContactListViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    // Reload when returning from the create/edit form so a newly added or
    // renamed contact shows up without a manual refresh.
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) viewModel.loadContacts()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                is ContactListEvent.NavigateToContact -> onContactClick(event.contactId)
                ContactListEvent.ForceLogout -> Unit // handled by the app-level nav
            }
        }
    }

    ContactListScreenContent(
        uiState = state,
        onSearchQueryChange = viewModel::onSearchQueryChange,
        // A click only routes through the ViewModel event; the LaunchedEffect
        // above performs the actual navigation so each tap navigates once.
        onContactClick = viewModel::onContactClick,
        onCreateContact = onCreateContact,
        onMenuClick = onMenuClick,
        onImportContacts = onImportContacts,
        onErrorShown = viewModel::onErrorShown,
        onLoadMore = viewModel::loadNextPage,
    )
}

/**
 * Stateless screen content — the four canonical states (loading / empty /
 * error / populated) are testable directly via this composable (ticket §10.4).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactListScreenContent(
    uiState: ContactListUiState,
    onSearchQueryChange: (String) -> Unit,
    onContactClick: (Int) -> Unit,
    onCreateContact: () -> Unit = {},
    onMenuClick: () -> Unit = {},
    onImportContacts: () -> Unit = {},
    onErrorShown: () -> Unit = {},
    onLoadMore: () -> Unit = {},
) {
    val snackbarHostState = remember { SnackbarHostState() }
    var search by rememberSaveable { mutableStateOf(uiState.searchQuery) }
    val listState = rememberLazyListState()

    // M9: infinite scroll — fire loadNextPage() once the user scrolls within 5 rows of the end
    // of the loaded contacts, matching ContactListViewModel.loadNextPage()'s own re-entrancy
    // guards (isLoading / isLoadingMore / hasMore), so this can fire repeatedly as the list
    // grows without risking duplicate in-flight requests.
    //
    // `remember` MUST be keyed on lastContactIndex. uiState is a plain parameter, not a State,
    // so an unkeyed `remember { derivedStateOf { ... uiState ... } }` captures the *first*
    // composition's uiState permanently — and the first composition is always the ViewModel's
    // empty initial state, pinning lastIndex at -1 and silently disabling pagination forever.
    // Only listState.layoutInfo is a real State and re-triggers the block on its own.
    val lastContactIndex = uiState.contacts.lastIndex
    val shouldLoadMore by remember(lastContactIndex) {
        derivedStateOf {
            val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: return@derivedStateOf false
            lastContactIndex >= 0 && lastVisible >= lastContactIndex - 5
        }
    }
    LaunchedEffect(shouldLoadMore, uiState.pagination.hasMore, uiState.pagination.isLoadingMore) {
        if (shouldLoadMore && uiState.pagination.hasMore && !uiState.pagination.isLoadingMore) {
            onLoadMore()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onMenuClick) {
                        Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                    }
                },
                title = {
                    Text(stringResource(R.string.nav_contacts), style = MaterialTheme.typography.titleLarge)
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
            BrandFab(onClick = onCreateContact) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.contacts_new))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            OutlinedTextField(
                value = search,
                onValueChange = {
                    search = it
                    onSearchQueryChange(it)
                },
                label = { Text(stringResource(R.string.contacts_search_hint)) },
                singleLine = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 8.dp),
            )

            // T87: the notes/activities section trails every state (loading/empty/error/
            // populated) as the LazyColumn's last item, so it's reachable regardless of
            // whether the contact list itself has rows — the two result sets are independent.
            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize().testTag("contact-list"),
            ) {
                when {
                    uiState.isLoading -> item {
                        LoadingSkeleton(modifier = Modifier.testTag("contact-list-loading"))
                    }
                    uiState.contacts.isEmpty() && uiState.error == null -> item {
                        Column(
                            horizontalAlignment = Alignment.CenterHorizontally,
                            modifier = Modifier.fillMaxWidth().padding(24.dp),
                        ) {
                            EmptyState(message = stringResource(R.string.contacts_empty))
                            Button(onClick = onImportContacts) {
                                Text(stringResource(R.string.import_title))
                            }
                        }
                    }
                    uiState.contacts.isEmpty() && uiState.error != null -> item {
                        Box(modifier = Modifier.fillMaxWidth().padding(24.dp)) {
                            Text(
                                text = uiState.error.orEmpty(),
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.align(Alignment.Center),
                            )
                        }
                    }
                    else -> {
                        items(uiState.contacts, key = { it.id }) { contact ->
                            ContactListItem(
                                contact = contact,
                                onClick = { onContactClick(contact.id) },
                            )
                        }
                        if (uiState.pagination.isLoadingMore) {
                            item {
                                Box(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
                                    CircularProgressIndicator(
                                        modifier = Modifier.size(32.dp).align(Alignment.Center),
                                    )
                                }
                            }
                        }
                    }
                }
                item {
                    SearchNotesActivitiesSection(
                        query = uiState.searchQuery,
                        searchResult = uiState.searchResult,
                        onContactClick = onContactClick,
                    )
                }
            }
        }
    }

    uiState.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            onErrorShown()
        }
    }
}

@Composable
fun ContactListItem(
    contact: ContactSummary,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        ContactAvatar(
            photoUri = contact.photoThumbnail,
            contentDescription = stringResource(R.string.contacts_photo_description, contact.displayName),
            size = 40.dp,
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = contact.displayName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            val subtitle = listOfNotNull(contact.primaryEmail, contact.primaryPhone)
                .joinToString(" · ")
            if (subtitle.isNotBlank()) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/**
 * T87: the cross-entity (notes/activities) half of the folded search, below the contact list.
 * Collapsed by default and re-collapses on every new [query] — a stale expanded panel left open
 * across a query change would show the old query's hits, matching web's `SearchNotesActivities`.
 * Renders nothing when [searchResult] is null (no query, below the two-character gate, or the
 * request failed — see `ContactListViewModel.loadCrossEntitySearch`'s doc comment) or empty.
 */
@Composable
private fun SearchNotesActivitiesSection(
    query: String,
    searchResult: SearchResult?,
    onContactClick: (Int) -> Unit,
) {
    if (searchResult == null) return
    val notes = searchResult.notes
    val activities = searchResult.activities
    val total = notes.size + activities.size
    val resolvedRelation = searchResult.resolvedRelation
    if (resolvedRelation.isNullOrBlank() && total == 0) return

    var expanded by remember(query) { mutableStateOf(false) }

    Column(modifier = Modifier.padding(top = 8.dp)) {
        if (!resolvedRelation.isNullOrBlank()) {
            Card(
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer),
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            ) {
                Text(
                    text = stringResource(R.string.contacts_search_resolved_relation, resolvedRelation),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSecondaryContainer,
                    modifier = Modifier.padding(12.dp),
                )
            }
        }
        if (total > 0) {
            Card(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { expanded = !expanded }
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = stringResource(R.string.contacts_search_results_header, total),
                            style = MaterialTheme.typography.titleSmall,
                        )
                        Text(
                            text = stringResource(R.string.contacts_search_results_hint),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Icon(
                        imageVector = if (expanded) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                        contentDescription = null,
                    )
                }
                if (expanded) {
                    Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
                        if (notes.isNotEmpty()) {
                            Text(
                                text = stringResource(R.string.contacts_search_notes_group),
                                style = MaterialTheme.typography.labelLarge,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            notes.forEach { note ->
                                SearchNoteRow(note = note, onContactClick = onContactClick)
                            }
                        }
                        if (activities.isNotEmpty()) {
                            Text(
                                text = stringResource(R.string.contacts_search_activities_group),
                                style = MaterialTheme.typography.labelLarge,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.padding(top = if (notes.isNotEmpty()) 8.dp else 0.dp),
                            )
                            activities.forEach { activity ->
                                SearchActivityRow(activity = activity, onContactClick = onContactClick)
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SearchNoteRow(
    note: SearchNoteHit,
    onContactClick: (Int) -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Text(text = note.content.orEmpty(), style = MaterialTheme.typography.bodyMedium)
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.padding(top = 4.dp),
        ) {
            val contactId = note.contactId
            val contactName = note.contactName
            if (contactId != null && !contactName.isNullOrBlank()) {
                AssistChip(
                    onClick = { onContactClick(contactId) },
                    label = { Text(contactName) },
                )
            } else {
                AssistChip(onClick = {}, enabled = false, label = { Text(stringResource(R.string.contacts_search_unfiled)) })
            }
            note.date?.take(10)?.let { date ->
                Text(text = date, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun SearchActivityRow(
    activity: SearchActivityHit,
    onContactClick: (Int) -> Unit,
) {
    // "Navigate to theirs" (T87 spec): an activity's own detail screen doesn't exist yet
    // (M9), so a hit navigates to its first participant's contact detail instead — the same
    // destination tapping that contact anywhere else in the app reaches.
    val firstContactId = activity.contacts?.firstOrNull()?.id
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .let { if (firstContactId != null) it.clickable { onContactClick(firstContactId) } else it }
            .padding(vertical = 6.dp),
    ) {
        Text(text = activity.title.orEmpty(), style = MaterialTheme.typography.bodyMedium)
        activity.date?.take(10)?.let { date ->
            Text(text = date, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
