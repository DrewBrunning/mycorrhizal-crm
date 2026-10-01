package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.TimelineBuckets
import com.mycorrhizal.crm.model.network.TimelineTypes
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * Issue #1401 (web T78 `TimelineExplorerDialog` parity): the full-screen, paged version of a
 * contact's merged timeline behind the contact page's "View all". Type multi-select + recency
 * filters, "Load more", every event its own lazy item, and the same row actions as the page's
 * preview (edit an activity / note, undo a completion).
 *
 * The first page is (re)fetched on every ON_RESUME — including the return from an activity or
 * note edit form — so edits and deletes made from here are never shown stale, and the contact
 * page (which reloads on resume the same way) is current when the user backs out.
 */
@Composable
fun TimelineExplorerScreen(
    onBack: () -> Unit,
    onEditActivity: (Int) -> Unit,
    onEditNote: (Int) -> Unit,
    viewModel: TimelineExplorerViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val lifecycleOwner = LocalLifecycleOwner.current
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) viewModel.refresh()
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }
    TimelineExplorerContent(
        state = state,
        onBack = onBack,
        onEditActivity = onEditActivity,
        onEditNote = onEditNote,
        onTypesChange = viewModel::setTypes,
        onBucketChange = viewModel::setBucket,
        onLoadMore = viewModel::loadMore,
        onRetry = viewModel::refresh,
        onUndoCompletion = viewModel::undoCompletion,
        onErrorShown = viewModel::onErrorShown,
    )
}

/** Stateless content, split out so it is testable without a Hilt-backed ViewModel. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TimelineExplorerContent(
    state: TimelineExplorerUiState,
    onBack: () -> Unit = {},
    onEditActivity: (Int) -> Unit = {},
    onEditNote: (Int) -> Unit = {},
    onTypesChange: (Set<String>) -> Unit = {},
    onBucketChange: (String) -> Unit = {},
    onLoadMore: () -> Unit = {},
    onRetry: () -> Unit = {},
    onUndoCompletion: (Int) -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val snackbarHostState = remember { SnackbarHostState() }
    // Undoing a completion deletes its timeline entry — confirm first (M17), as the contact page does.
    var pendingUndoCompletionId by remember { mutableStateOf<Int?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.timeline_explorer_title), style = MaterialTheme.typography.titleLarge)
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
            TimelineExplorerFilters(
                types = state.types,
                bucket = state.bucket,
                onTypesChange = onTypesChange,
                onBucketChange = onBucketChange,
            )
            Box(modifier = Modifier.fillMaxSize()) {
                when {
                    state.isLoading && state.items.isEmpty() -> LoadingSkeleton()
                    state.items.isEmpty() && state.error != null -> Column(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        EmptyState(state.error)
                        Button(onClick = onRetry) { Text(stringResource(R.string.action_retry)) }
                    }
                    state.items.isEmpty() -> EmptyState(stringResource(R.string.timeline_explorer_empty))
                    else -> LazyColumn(modifier = Modifier.fillMaxSize().testTag("timeline-explorer-list")) {
                        // Each event is its own lazy item — rows outside the viewport are never composed.
                        items(state.items, key = { it.key }) { item ->
                            TimelineItemRow(
                                item = item,
                                onEditActivity = onEditActivity,
                                onEditNote = onEditNote,
                                onUndoCompletion = { id -> pendingUndoCompletionId = id },
                            )
                        }
                        if (!state.nextCursor.isNullOrEmpty()) {
                            item(key = "load-more") {
                                Box(
                                    modifier = Modifier.fillMaxWidth().padding(16.dp),
                                    contentAlignment = Alignment.Center,
                                ) {
                                    // Disabled mid-refresh too: the cursor belongs to the page it was returned with.
                                    Button(
                                        onClick = onLoadMore,
                                        enabled = !state.isLoadingMore && !state.isLoading,
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

    pendingUndoCompletionId?.let { completionId ->
        AlertDialog(
            onDismissRequest = { pendingUndoCompletionId = null },
            title = { Text(stringResource(R.string.reminder_completion_undo_title)) },
            text = { Text(stringResource(R.string.reminder_completion_undo_confirm)) },
            confirmButton = {
                TextButton(onClick = {
                    pendingUndoCompletionId = null
                    onUndoCompletion(completionId)
                }) {
                    Text(stringResource(R.string.reminder_completion_undo), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingUndoCompletionId = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    // Over a populated list an error (a failed Load more / undo) is a snackbar; with no rows it is
    // the persistent body (above), so don't toast-and-clear it.
    val listError = state.error
    if (listError != null && state.items.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            onErrorShown()
        }
    }
}

@Composable
private fun TimelineExplorerFilters(
    types: Set<String>,
    bucket: String,
    onTypesChange: (Set<String>) -> Unit,
    onBucketChange: (String) -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Text(
            text = stringResource(R.string.timeline_filter_type),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp),
        )
        Row(
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            // Empty selection means "all" (the backend treats an absent ?type= the same way).
            FilterChip(
                selected = types.isEmpty(),
                onClick = { onTypesChange(emptySet()) },
                label = { Text(stringResource(R.string.timeline_all_types)) },
            )
            TimelineTypes.ALL.forEach { type ->
                FilterChip(
                    selected = type in types,
                    onClick = { onTypesChange(if (type in types) types - type else types + type) },
                    label = { Text(stringResource(timelineTypeLabel(type))) },
                )
            }
        }
        Text(
            text = stringResource(R.string.timeline_filter_bucket),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 8.dp),
        )
        Row(
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            TimelineBuckets.ALL_BUCKETS.forEach { b ->
                FilterChip(
                    selected = b == bucket,
                    onClick = { onBucketChange(b) },
                    label = { Text(stringResource(timelineBucketLabel(b))) },
                )
            }
        }
    }
}

private fun timelineTypeLabel(type: String): Int = when (type) {
    TimelineTypes.NOTE -> R.string.timeline_type_note
    TimelineTypes.ACTIVITY -> R.string.timeline_type_activity
    TimelineTypes.COMPLETION -> R.string.timeline_type_completion
    TimelineTypes.LIFE_EVENT -> R.string.timeline_type_life_event
    TimelineTypes.EXTERNAL_ACTIVITY -> R.string.timeline_type_external_activity
    else -> R.string.timeline_type_gift
}

private fun timelineBucketLabel(bucket: String): Int = when (bucket) {
    TimelineBuckets.LAST_7_DAYS -> R.string.timeline_bucket_last_7_days
    TimelineBuckets.LAST_30_DAYS -> R.string.timeline_bucket_last_30_days
    TimelineBuckets.LAST_90_DAYS -> R.string.timeline_bucket_last_90_days
    TimelineBuckets.THIS_YEAR -> R.string.timeline_bucket_this_year
    else -> R.string.timeline_bucket_all
}
