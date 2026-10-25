package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Checklist
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.ExpandLess
import androidx.compose.material.icons.outlined.ExpandMore
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.outlined.MergeType
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.StarBorder
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.toggleableState
import androidx.compose.ui.state.ToggleableState
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.BulkActions
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.SearchActivityHit
import com.mycorrhizal.crm.model.network.SearchNoteHit
import com.mycorrhizal.crm.model.network.SearchResult
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FabListContentPadding
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactListScreen(
    onContactClick: (Int) -> Unit,
    onCreateContact: () -> Unit = {},
    // Issue #150: nullable — null when there is no app-level drawer to open
    // (the tablet two-pane's persistent list pane, where the NavigationRail
    // replaces the drawer), which hides the hamburger.
    onMenuClick: (() -> Unit)? = {},
    onImportContacts: () -> Unit = {},
    // T93 (issue #710): opens the duplicate-review surface (web's "Review
    // duplicates" button on the contacts page).
    onReviewDuplicates: () -> Unit = {},
    // ADR 0029 §6 (issue #1271): non-null puts the list in single-select picker
    // mode for share-to-CRM — see ContactListScreenContent.
    pickerTitle: String? = null,
    onPickerBack: () -> Unit = {},
    viewModel: ContactListViewModel = hiltViewModel()
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
        onCircleFilterChange = viewModel::onCircleFilterChange,
        onIncludeArchivedChange = viewModel::onIncludeArchivedChange,
        onIncludeFavoritesChange = viewModel::onIncludeFavoritesChange,
        onSortChange = viewModel::onSortChange,
        onToggleFavorite = viewModel::toggleFavorite,
        onToggleSelection = viewModel::toggleSelection,
        onToggleSelectAll = viewModel::toggleSelectAll,
        onRunBulkAction = viewModel::runBulkAction,
        onBulkResultShown = viewModel::onBulkResultShown,
        onCreateContact = onCreateContact,
        onMenuClick = onMenuClick,
        onImportContacts = onImportContacts,
        onReviewDuplicates = onReviewDuplicates,
        pickerTitle = pickerTitle,
        onPickerBack = onPickerBack,
        onErrorShown = viewModel::onErrorShown,
        onLoadMore = viewModel::loadNextPage,
        onRefresh = viewModel::refresh
    )
}

/**
 * Stateless screen content — the four canonical states (loading / empty /
 * error / populated) are testable directly via this composable (ticket §10.4).
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun ContactListScreenContent(
    uiState: ContactListUiState,
    onSearchQueryChange: (String) -> Unit,
    onContactClick: (Int) -> Unit,
    onCircleFilterChange: (String?) -> Unit = {},
    onIncludeArchivedChange: (Boolean) -> Unit = {},
    // Issue #212: the favorites-only lens + per-row star toggle (web #173).
    onIncludeFavoritesChange: (Boolean) -> Unit = {},
    // Issue #1762: name / recently contacted / recently added.
    onSortChange: (ContactSort) -> Unit = {},
    onToggleFavorite: (ContactSummary) -> Unit = {},
    onToggleSelection: (Int) -> Unit = {},
    onToggleSelectAll: () -> Unit = {},
    onRunBulkAction: (String, String?, String?) -> Unit = { _, _, _ -> },
    onBulkResultShown: () -> Unit = {},
    onCreateContact: () -> Unit = {},
    // Issue #150: see ContactListScreen — null hides the hamburger (no drawer).
    onMenuClick: (() -> Unit)? = {},
    onImportContacts: () -> Unit = {},
    onReviewDuplicates: () -> Unit = {},
    // ADR 0029 §6 (issue #1271): when non-null the screen is a single-select contact
    // picker — this title, a back arrow instead of the drawer, and no FAB, bulk
    // selection, duplicate review or import affordances. A row tap only calls
    // onContactClick; the caller decides what picking means.
    pickerTitle: String? = null,
    onPickerBack: () -> Unit = {},
    onErrorShown: () -> Unit = {},
    onLoadMore: () -> Unit = {},
    onRefresh: () -> Unit = {}
) {
    val snackbarHostState = remember { SnackbarHostState() }
    var search by rememberSaveable { mutableStateOf(uiState.searchQuery) }
    // M23: inline bulk-selection mode (web parity). The selection *set* lives in the
    // ViewModel so filter changes can clear it; this flag is pure screen chrome.
    var selectMode by rememberSaveable { mutableStateOf(false) }
    // Set while a circle/tag picker dialog is open; the action it's picking
    // for (add vs remove) travels alongside it so confirming the picker can
    // set pendingAction directly (ported from the retired BulkOperationsScreen).
    var picker by remember { mutableStateOf<Pair<BulkPickerTarget, String>?>(null) }
    var pendingAction by remember { mutableStateOf<String?>(null) }
    var pendingCircleId by remember { mutableStateOf<String?>(null) }
    var pendingTagId by remember { mutableStateOf<String?>(null) }
    val listState = rememberLazyListState()
    val isPicker = pickerTitle != null

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
            val lastVisible =
                listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index
                    ?: return@derivedStateOf false
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
                    if (isPicker) {
                        AccessibleIconButton(
                            onClick = onPickerBack,
                            modifier = Modifier.testTag("picker-back")
                        ) {
                            Icon(
                                Icons.AutoMirrored.Outlined.ArrowBack,
                                contentDescription = stringResource(R.string.cd_back)
                            )
                        }
                    } else {
                        onMenuClick?.let { onMenu ->
                            AccessibleIconButton(onClick = onMenu) {
                                Icon(
                                    Icons.Outlined.Menu,
                                    contentDescription = stringResource(R.string.cd_menu)
                                )
                            }
                        }
                    }
                },
                title = {
                    when {
                        pickerTitle != null -> Text(
                            text = pickerTitle,
                            style = MaterialTheme.typography.titleLarge
                        )

                        selectMode -> Text(
                            text = stringResource(
                                R.string.contacts_selected_count,
                                uiState.selected.size
                            ),
                            style = MaterialTheme.typography.titleLarge
                        )

                        // Issue #1762: the search bar lives in the app bar, not
                        // in a fixed block that steals a third of the viewport.
                        else -> OutlinedTextField(
                            value = search,
                            onValueChange = {
                                search = it
                                onSearchQueryChange(it)
                            },
                            label = { Text(stringResource(R.string.contacts_search_hint)) },
                            singleLine = true,
                            leadingIcon = {
                                Icon(Icons.Outlined.Search, contentDescription = null)
                            },
                            modifier = Modifier
                                .fillMaxWidth()
                                .testTag("contact-search"),
                            colors = OutlinedTextFieldDefaults.colors(
                                focusedTextColor = MaterialTheme.colorScheme.onPrimary,
                                unfocusedTextColor = MaterialTheme.colorScheme.onPrimary,
                                focusedLabelColor = MaterialTheme.colorScheme.onPrimary,
                                unfocusedLabelColor = MaterialTheme.colorScheme.onPrimary,
                                focusedLeadingIconColor = MaterialTheme.colorScheme.onPrimary,
                                unfocusedLeadingIconColor = MaterialTheme.colorScheme.onPrimary,
                                cursorColor = MaterialTheme.colorScheme.onPrimary,
                                focusedBorderColor = Color.Transparent,
                                unfocusedBorderColor = Color.Transparent
                            )
                        )
                    }
                },
                actions = {
                    if (isPicker) {
                        // Single-select picker: no bulk mode / duplicate review.
                    } else if (selectMode) {
                        TextButton(
                            onClick = onToggleSelectAll,
                            modifier = Modifier.testTag("select-all")
                        ) {
                            Text(
                                stringResource(R.string.contacts_select_all),
                                color = MaterialTheme.colorScheme.onPrimary
                            )
                        }
                        AccessibleIconButton(
                            onClick = { selectMode = false },
                            modifier = Modifier.testTag("exit-select-mode")
                        ) {
                            Icon(
                                Icons.Outlined.Close,
                                contentDescription = stringResource(
                                    R.string.contacts_exit_select_mode
                                ),
                                tint = MaterialTheme.colorScheme.onPrimary
                            )
                        }
                    } else {
                        // T93: duplicate review (web's "Review duplicates"
                        // button) — only meaningful outside selection mode.
                        AccessibleIconButton(
                            onClick = onReviewDuplicates,
                            modifier = Modifier.testTag("review-duplicates")
                        ) {
                            Icon(
                                // Issue #1762: ContentCopy read as "copy"; the
                                // merge glyph names the action.
                                Icons.Outlined.MergeType,
                                contentDescription = stringResource(R.string.duplicates_title),
                                tint = MaterialTheme.colorScheme.onPrimary
                            )
                        }
                        AccessibleIconButton(
                            onClick = { selectMode = true },
                            modifier = Modifier.testTag("enter-select-mode")
                        ) {
                            Icon(
                                Icons.Outlined.Checklist,
                                contentDescription = stringResource(R.string.contacts_select_mode),
                                tint = MaterialTheme.colorScheme.onPrimary
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
            if (!selectMode && !isPicker) {
                BrandFab(
                    onClick = onCreateContact,
                    modifier = Modifier.testTag("create-contact-fab")
                ) {
                    Icon(
                        Icons.Outlined.Add,
                        contentDescription = stringResource(R.string.contacts_new)
                    )
                }
            }
        },
        bottomBar = {
            if (!isPicker && selectMode && uiState.selected.isNotEmpty()) {
                BulkActionBar(
                    isRunning = uiState.isBulkRunning,
                    onArchive = { pendingAction = BulkActions.ARCHIVE },
                    onUnarchive = { pendingAction = BulkActions.UNARCHIVE },
                    onDelete = { pendingAction = BulkActions.DELETE },
                    onAddCircle = { picker = BulkPickerTarget.CIRCLE to BulkActions.ADD_CIRCLE },
                    onRemoveCircle = {
                        picker = BulkPickerTarget.CIRCLE to BulkActions.REMOVE_CIRCLE
                    },
                    onAddTag = { picker = BulkPickerTarget.TAG to BulkActions.ADD_TAG },
                    onRemoveTag = { picker = BulkPickerTarget.TAG to BulkActions.REMOVE_TAG }
                )
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
        ) {
            // T87: the notes/activities section trails every state (loading/empty/error/
            // populated) as the LazyColumn's last item, so it's reachable regardless of
            // whether the contact list itself has rows — the two result sets are independent.
            RefreshableContent(
                isRefreshing = uiState.isLoading && uiState.contacts.isNotEmpty(),
                onRefresh = onRefresh,
                modifier = Modifier.fillMaxSize()
            ) {
                LazyColumn(
                    state = listState,
                    modifier = Modifier.fillMaxSize().testTag("contact-list"),
                    contentPadding = FabListContentPadding
                ) {
                    // Issue #1762: one compact, horizontally-scrollable chip row
                    // as the list's first item — it scrolls away with the rows.
                    item(key = "filters") {
                        ContactListFilterChips(
                            circles = uiState.circles,
                            circleFilter = uiState.circleFilter,
                            includeArchived = uiState.includeArchived,
                            includeFavorites = uiState.includeFavorites,
                            sort = uiState.sort,
                            onCircleFilterChange = onCircleFilterChange,
                            onIncludeArchivedChange = onIncludeArchivedChange,
                            onIncludeFavoritesChange = onIncludeFavoritesChange,
                            onSortChange = onSortChange
                        )
                    }
                    val contactRow: @Composable (ContactSummary) -> Unit = { contact ->
                        ContactListItem(
                            contact = contact,
                            selected = contact.id in uiState.selected,
                            selectMode = selectMode,
                            isMe = uiState.selfContactVCardUid != null &&
                                contact.uid == uiState.selfContactVCardUid,
                            onToggleFavorite = { onToggleFavorite(contact) },
                            onClick = {
                                if (selectMode) {
                                    onToggleSelection(
                                        contact.id
                                    )
                                } else {
                                    onContactClick(contact.id)
                                }
                            },
                            onLongClick = {
                                if (!isPicker) {
                                    selectMode = true
                                    onToggleSelection(contact.id)
                                }
                            }
                        )
                    }
                    when {
                        uiState.isLoading && uiState.contacts.isEmpty() -> item {
                            LoadingSkeleton(modifier = Modifier.testTag("contact-list-loading"))
                        }

                        uiState.contacts.isEmpty() && uiState.error == null -> item {
                            Column(
                                horizontalAlignment = Alignment.CenterHorizontally,
                                modifier = Modifier.fillMaxWidth().padding(24.dp)
                            ) {
                                EmptyState(
                                    title = stringResource(R.string.contacts_empty),
                                    body = if (isPicker) {
                                        null
                                    } else {
                                        stringResource(
                                            R.string.contacts_empty_body
                                        )
                                    },
                                    actionLabel = if (isPicker) {
                                        null
                                    } else {
                                        stringResource(
                                            R.string.contacts_new
                                        )
                                    },
                                    onAction = if (isPicker) null else onCreateContact
                                )
                                if (!isPicker) {
                                    OutlinedButton(onClick = onImportContacts) {
                                        Text(stringResource(R.string.import_title))
                                    }
                                }
                            }
                        }

                        uiState.contacts.isEmpty() && uiState.error != null -> item {
                            Box(modifier = Modifier.fillMaxWidth().padding(24.dp)) {
                                Text(
                                    text = uiState.error.orEmpty(),
                                    color = MaterialTheme.colorScheme.error,
                                    modifier = Modifier.align(Alignment.Center)
                                )
                            }
                        }

                        else -> {
                            if (uiState.sort == ContactSort.NAME) {
                                // Issue #1762: sticky letter headers for the
                                // default alphabetical order (grouped by the
                                // same lastname-else-firstname key the server
                                // sorts on).
                                uiState.contacts.groupBy(
                                    ::contactSectionLetter
                                ).forEach { (letter, rows) ->
                                    stickyHeader(key = "header-$letter") {
                                        Text(
                                            text = letter,
                                            style = MaterialTheme.typography.labelLarge,
                                            color = MaterialTheme.colorScheme.primary,
                                            modifier = Modifier
                                                .fillMaxWidth()
                                                .background(MaterialTheme.colorScheme.surface)
                                                .padding(horizontal = 16.dp, vertical = 4.dp)
                                                .testTag("contact-header-$letter")
                                        )
                                    }
                                    items(rows, key = { it.id }) { contact -> contactRow(contact) }
                                }
                            } else {
                                items(
                                    uiState.contacts,
                                    key = { it.id }
                                ) { contact -> contactRow(contact) }
                            }
                            if (uiState.pagination.isLoadingMore) {
                                item {
                                    Box(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
                                        CircularProgressIndicator(
                                            modifier = Modifier.size(32.dp).align(Alignment.Center)
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
                            onContactClick = onContactClick
                        )
                    }
                }
            }
        }
    }

    picker?.let { (target, action) ->
        val names: List<Pair<String, String>> = when (target) {
            BulkPickerTarget.CIRCLE -> uiState.circles.map { it.id to it.name }
            BulkPickerTarget.TAG -> uiState.tags.map { it.id to it.name }
        }
        AlertDialog(
            onDismissRequest = { picker = null },
            title = {
                Text(
                    stringResource(
                        if (target ==
                            BulkPickerTarget.CIRCLE
                        ) {
                            R.string.bulk_pick_circle
                        } else {
                            R.string.bulk_pick_tag
                        }
                    )
                )
            },
            text = {
                if (names.isEmpty()) {
                    Text(
                        stringResource(
                            if (target ==
                                BulkPickerTarget.CIRCLE
                            ) {
                                R.string.bulk_no_circles
                            } else {
                                R.string.bulk_no_tags
                            }
                        )
                    )
                } else {
                    LazyColumn(modifier = Modifier.heightIn(max = 320.dp)) {
                        items(names, key = { it.first }) { (id, name) ->
                            Text(
                                text = name,
                                style = MaterialTheme.typography.bodyLarge,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable {
                                        when (target) {
                                            BulkPickerTarget.CIRCLE -> pendingCircleId = id
                                            BulkPickerTarget.TAG -> pendingTagId = id
                                        }
                                        pendingAction = action
                                        picker = null
                                    }
                                    .padding(vertical = 12.dp)
                            )
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = { picker = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        )
    }

    pendingAction?.let { action ->
        AlertDialog(
            onDismissRequest = {
                pendingAction = null
                pendingCircleId = null
                pendingTagId = null
            },
            title = { Text(stringResource(R.string.bulk_confirm_title)) },
            text = { Text(stringResource(R.string.bulk_confirm_body, uiState.selected.size)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        onRunBulkAction(action, pendingCircleId, pendingTagId)
                        pendingAction = null
                        pendingCircleId = null
                        pendingTagId = null
                    }
                ) {
                    Text(stringResource(R.string.action_confirm))
                }
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        pendingAction = null
                        pendingCircleId = null
                        pendingTagId = null
                    }
                ) {
                    Text(stringResource(R.string.action_cancel))
                }
            }
        )
    }

    uiState.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            onErrorShown()
        }
    }

    uiState.bulkResult?.let { result ->
        val message = stringResource(R.string.bulk_result, result.succeeded, result.failed)
        LaunchedEffect(result) {
            snackbarHostState.showSnackbar(message)
            onBulkResultShown()
        }
    }
}

/**
 * M23: which circle/tag-scoped action a picker dialog is choosing an id for.
 * Archive/unarchive/delete need no id and skip the picker, going straight to
 * the existing confirm dialog.
 */
private enum class BulkPickerTarget { CIRCLE, TAG }

/**
 * Issue #1762: one compact chip row — favorites, archived, circle filter and
 * sort — rendered as the list's first item so it scrolls away with the rows.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ContactListFilterChips(
    circles: List<Circle>,
    circleFilter: String?,
    includeArchived: Boolean,
    includeFavorites: Boolean,
    sort: ContactSort,
    onCircleFilterChange: (String?) -> Unit,
    onIncludeArchivedChange: (Boolean) -> Unit,
    onIncludeFavoritesChange: (Boolean) -> Unit,
    onSortChange: (ContactSort) -> Unit
) {
    var circleExpanded by remember { mutableStateOf(false) }
    var sortExpanded by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        FilterChip(
            selected = includeFavorites,
            onClick = { onIncludeFavoritesChange(!includeFavorites) },
            label = { Text(stringResource(R.string.contacts_show_favorites)) },
            modifier = Modifier
                .testTag("favorites-toggle")
                .semantics { toggleableState = ToggleableState(includeFavorites) }
        )
        FilterChip(
            selected = includeArchived,
            onClick = { onIncludeArchivedChange(!includeArchived) },
            label = { Text(stringResource(R.string.contacts_show_archived)) },
            modifier = Modifier
                .testTag("archived-toggle")
                .semantics { toggleableState = ToggleableState(includeArchived) }
        )
        Box {
            FilterChip(
                selected = circleFilter != null,
                onClick = { circleExpanded = true },
                label = { Text(circleFilter ?: stringResource(R.string.contacts_all_circles)) },
                trailingIcon = { Icon(Icons.Outlined.ExpandMore, contentDescription = null) },
                modifier = Modifier.testTag("circle-filter")
            )
            DropdownMenu(
                expanded = circleExpanded,
                onDismissRequest = { circleExpanded = false }
            ) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.contacts_all_circles)) },
                    onClick = {
                        circleExpanded = false
                        onCircleFilterChange(null)
                    }
                )
                circles.forEach { circle ->
                    DropdownMenuItem(
                        text = { Text(circle.name) },
                        onClick = {
                            circleExpanded = false
                            onCircleFilterChange(circle.name)
                        }
                    )
                }
            }
        }
        Box {
            FilterChip(
                selected = false,
                onClick = { sortExpanded = true },
                label = {
                    Text(stringResource(R.string.contacts_sort_label, sortLabel(sort)))
                },
                trailingIcon = { Icon(Icons.Outlined.ExpandMore, contentDescription = null) },
                modifier = Modifier.testTag("sort-filter")
            )
            DropdownMenu(
                expanded = sortExpanded,
                onDismissRequest = { sortExpanded = false }
            ) {
                ContactSort.entries.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(sortLabel(option)) },
                        onClick = {
                            sortExpanded = false
                            onSortChange(option)
                        },
                        modifier = Modifier.testTag("sort-option-${option.preferenceValue}")
                    )
                }
            }
        }
    }
}

/** Display label for the sort control. */
@Composable
private fun sortLabel(sort: ContactSort): String = when (sort) {
    ContactSort.NAME -> stringResource(R.string.contacts_sort_name)
    ContactSort.RECENTLY_CONTACTED -> stringResource(R.string.contacts_sort_recently_contacted)
    ContactSort.RECENTLY_ADDED -> stringResource(R.string.contacts_sort_recently_added)
}

/**
 * Sticky-header letter for the default name order, derived from the same
 * lastname-else-firstname key the backend's sort_name uses. Non-letter
 * leading characters (a number, an emoji) group under "#".
 */
private fun contactSectionLetter(contact: ContactSummary): String {
    val key = sequenceOf(contact.lastname, contact.firstname, contact.fn)
        .mapNotNull { it?.trim()?.takeIf(String::isNotEmpty) }
        .firstOrNull()
        ?: return "#"
    val first = key.first()
    return if (first.isLetter()) first.uppercaseChar().toString() else "#"
}

/**
 * M23: the inline bulk-action bar shown under selection mode (web's BulkActionsBar).
 * The action row is a horizontalScroll'd Row of seven buttons, so the later ones
 * (tag actions) sit off the initial viewport — tests must performScrollTo() first.
 */
@Composable
private fun BulkActionBar(
    isRunning: Boolean,
    onArchive: () -> Unit,
    onUnarchive: () -> Unit,
    onDelete: () -> Unit,
    onAddCircle: () -> Unit,
    onRemoveCircle: () -> Unit,
    onAddTag: () -> Unit,
    onRemoveTag: () -> Unit
) {
    Surface(
        tonalElevation = 3.dp,
        modifier = Modifier.fillMaxWidth()
    ) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp)
        ) {
            TextButton(
                onClick = onArchive,
                enabled = !isRunning,
                modifier = Modifier.testTag("bulk-archive")
            ) {
                Text(stringResource(R.string.bulk_archive))
            }
            TextButton(
                onClick = onUnarchive,
                enabled = !isRunning,
                modifier = Modifier.testTag("bulk-unarchive")
            ) {
                Text(stringResource(R.string.bulk_unarchive))
            }
            TextButton(
                onClick = onDelete,
                enabled = !isRunning,
                modifier = Modifier.testTag("bulk-delete")
            ) {
                Text(stringResource(R.string.action_delete))
            }
            TextButton(
                onClick = onAddCircle,
                enabled = !isRunning,
                modifier = Modifier.testTag("bulk-add-circle")
            ) {
                Text(stringResource(R.string.bulk_add_circle))
            }
            TextButton(
                onClick = onRemoveCircle,
                enabled = !isRunning,
                modifier = Modifier.testTag("bulk-remove-circle")
            ) {
                Text(stringResource(R.string.bulk_remove_circle))
            }
            TextButton(
                onClick = onAddTag,
                enabled = !isRunning,
                modifier = Modifier.testTag("bulk-add-tag")
            ) {
                Text(stringResource(R.string.bulk_add_tag))
            }
            TextButton(
                onClick = onRemoveTag,
                enabled = !isRunning,
                modifier = Modifier.testTag("bulk-remove-tag")
            ) {
                Text(stringResource(R.string.bulk_remove_tag))
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun ContactListItem(
    contact: ContactSummary,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    selected: Boolean = false,
    selectMode: Boolean = false,
    // T90 / issue #831: whether this row is the caller's self-contact
    // pointer — drives the "You" badge (web parity).
    isMe: Boolean = false,
    // Issue #212: the per-row favorite toggle (web #173). Tapping the star
    // must never navigate into the detail page — the nested clickable's own
    // handler consumes the tap before the row's combinedClickable sees it
    // (Compose's stopPropagation equivalent).
    onToggleFavorite: () -> Unit = {},
    onLongClick: () -> Unit = onClick
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .combinedClickable(
                onClick = onClick,
                // #214/#210: without onClickLabel/onLongClickLabel the long-press
                // is invisible in the semantics tree (long-clickable=true with no
                // description of what it does) — unreachable by TalkBack's
                // actions menu and by switch access. onClickLabel replaces the
                // generic "double-tap to activate" with something that says what
                // tapping actually does — which, per the caller's onClick wiring
                // (ContactListScreenContent), is "toggle selection" in select
                // mode and "open the contact" otherwise; a single static label
                // would announce the wrong action in one of the two modes.
                onClickLabel = if (selectMode) {
                    stringResource(R.string.contacts_row_select_action)
                } else {
                    stringResource(R.string.contacts_open_contact)
                },
                onLongClick = onLongClick,
                onLongClickLabel = stringResource(R.string.contacts_row_select_action)
            )
            // #214: the row's own combinedClickable above carries no Role or
            // checked state, and the Checkbox below is decorative
            // (onCheckedChange = null) — without this, TalkBack lost the
            // selected/unselected announcement entirely in select mode, not
            // just its label. Only the checkbox's Role/state, not its click
            // handling, needs to live on the row.
            .then(
                if (selectMode) {
                    Modifier.semantics {
                        role = Role.Checkbox
                        toggleableState = ToggleableState(selected)
                    }
                } else {
                    Modifier
                }
            )
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        if (selectMode) {
            // #214: a bare Checkbox has no text/contentDescription of its own,
            // and (as an independently screen-reader-focusable nested
            // clickable) it never picks up the row's merged name either — it
            // would announce with no label. The row's own combinedClickable
            // above already toggles selection on tap, so the checkbox here is
            // purely a visual indicator; onCheckedChange = null makes that
            // decorative status explicit instead of adding a second,
            // unlabeled way to trigger the same action.
            Checkbox(
                checked = selected,
                onCheckedChange = null
            )
        }
        ContactAvatar(
            photoUri = contact.photoThumbnail,
            contentDescription = stringResource(
                R.string.contacts_photo_description,
                contact.displayName
            ),
            size = 40.dp
        )
        Column(modifier = Modifier.weight(1f)) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Text(
                    text = contact.displayName,
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false)
                )
                if (isMe) {
                    // T90 / issue #831: being yourself is not a status condition,
                    // and issue #1754: a disabled AssistChip renders at 38% opacity
                    // (failing contrast) and is announced as "disabled". A neutral
                    // outlined label with no click semantics reads as a badge.
                    Surface(
                        shape = MaterialTheme.shapes.small,
                        color = MaterialTheme.colorScheme.surface,
                        contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
                        // mergeDescendants makes the badge its own semantics
                        // boundary: without it the row's combinedClickable merges
                        // the label away (and TalkBack reads it as part of the row).
                        modifier = Modifier
                            .semantics(mergeDescendants = true) {}
                            .testTag("you-badge-${contact.id}")
                    ) {
                        Text(
                            text = stringResource(R.string.contact_you_badge),
                            style = MaterialTheme.typography.labelMedium,
                            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp)
                        )
                    }
                }
            }
            // Issue #1754: the list endpoint sends "" (not null) for a missing
            // email/phone, and listOfNotNull only drops nulls — an email-only
            // contact rendered a dangling " · ". Filter blanks before joining.
            val subtitle = listOfNotNull(
                contact.primaryEmail?.takeIf { it.isNotBlank() },
                contact.primaryPhone?.takeIf { it.isNotBlank() }
            ).joinToString(" · ")
            if (subtitle.isNotBlank()) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
        }
        // Issue #212: the always-visible star toggle. Filled when favorite,
        // outlined otherwise; labeled with the contact's name so TalkBack
        // announces who it acts on (web's aria-label does the same).
        AccessibleIconButton(
            onClick = onToggleFavorite,
            modifier = Modifier.testTag("favorite-${contact.id}")
        ) {
            Icon(
                imageVector = if (contact.isFavorite) Icons.Filled.Star else Icons.Outlined.StarBorder,
                contentDescription = stringResource(
                    if (contact.isFavorite) R.string.contacts_unfavorite_contact else R.string.contacts_favorite_contact,
                    contact.displayName
                ),
                tint = if (contact.isFavorite) {
                    MaterialTheme.colorScheme.tertiary
                } else {
                    MaterialTheme.colorScheme.onSurfaceVariant
                }
            )
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
    onContactClick: (Int) -> Unit
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
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.secondaryContainer
                ),
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)
            ) {
                Text(
                    text = stringResource(
                        R.string.contacts_search_resolved_relation,
                        resolvedRelation
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSecondaryContainer,
                    modifier = Modifier.padding(12.dp)
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
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = stringResource(R.string.contacts_search_results_header, total),
                            style = MaterialTheme.typography.titleSmall
                        )
                        Text(
                            text = stringResource(R.string.contacts_search_results_hint),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    Icon(
                        imageVector = if (expanded) Icons.Outlined.ExpandLess else Icons.Outlined.ExpandMore,
                        contentDescription = null
                    )
                }
                if (expanded) {
                    Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
                        if (notes.isNotEmpty()) {
                            Text(
                                text = stringResource(R.string.contacts_search_notes_group),
                                style = MaterialTheme.typography.labelLarge,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
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
                                modifier = Modifier.padding(
                                    top = if (notes.isNotEmpty()) 8.dp else 0.dp
                                )
                            )
                            activities.forEach { activity ->
                                SearchActivityRow(
                                    activity = activity,
                                    onContactClick = onContactClick
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SearchNoteRow(note: SearchNoteHit, onContactClick: (Int) -> Unit) {
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp)) {
        Text(text = note.content.orEmpty(), style = MaterialTheme.typography.bodyMedium)
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.padding(top = 4.dp)
        ) {
            val contactId = note.contactId
            val contactName = note.contactName
            if (contactId != null && !contactName.isNullOrBlank()) {
                AssistChip(
                    onClick = { onContactClick(contactId) },
                    label = { Text(contactName) }
                )
            } else {
                AssistChip(
                    onClick = {},
                    enabled = false,
                    label = { Text(stringResource(R.string.contacts_search_unfiled)) }
                )
            }
            note.date?.let {
                DateFormat.formatTimestamp(
                    it,
                    LocalDateFormat.current
                )
            }?.takeIf { it.isNotBlank() }?.let { date ->
                Text(
                    text = date,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
    }
}

@Composable
private fun SearchActivityRow(activity: SearchActivityHit, onContactClick: (Int) -> Unit) {
    // "Navigate to theirs" (T87 spec): an activity's own detail screen doesn't exist yet
    // (M9), so a hit navigates to its first participant's contact detail instead — the same
    // destination tapping that contact anywhere else in the app reaches.
    val firstContactId = activity.contacts?.firstOrNull()?.id
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .let {
                if (firstContactId != null) {
                    it.clickable {
                        onContactClick(
                            firstContactId
                        )
                    }
                } else {
                    it
                }
            }
            .padding(vertical = 6.dp)
    ) {
        Text(text = activity.title.orEmpty(), style = MaterialTheme.typography.bodyMedium)
        activity.date?.let {
            DateFormat.formatTimestamp(
                it,
                LocalDateFormat.current
            )
        }?.takeIf { it.isNotBlank() }?.let { date ->
            Text(
                text = date,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}
