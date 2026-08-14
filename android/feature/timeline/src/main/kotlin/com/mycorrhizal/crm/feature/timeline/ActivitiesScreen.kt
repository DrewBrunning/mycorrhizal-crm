package com.mycorrhizal.crm.feature.timeline

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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
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
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

/**
 * M19: per-contact activities list. Gained a search field + from/to-date
 * filters (debounced server-side filters), T17 cursor "load more", delete
 * with confirmation (M17's rule), and participant chips on each card that
 * navigate to the participant contact.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ActivitiesScreen(
    onBack: () -> Unit,
    onCreateActivity: () -> Unit,
    onEditActivity: (Int) -> Unit,
    onContactClick: (Int) -> Unit,
    viewModel: ActivitiesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    ActivitiesScreenContent(
        uiState = state,
        onBack = onBack,
        onCreateActivity = onCreateActivity,
        onEditActivity = onEditActivity,
        onContactClick = onContactClick,
        onSearchChange = viewModel::onSearchChange,
        onFromDateChange = viewModel::onFromDateChange,
        onToDateChange = viewModel::onToDateChange,
        onLoadMore = viewModel::loadMore,
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless screen content, split out from [ActivitiesScreen] so it's directly
 * testable without a Hilt-backed ViewModel (mirrors `ContactListScreenContent`).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ActivitiesScreenContent(
    uiState: ActivitiesUiState,
    onBack: () -> Unit = {},
    onCreateActivity: () -> Unit = {},
    onEditActivity: (Int) -> Unit = {},
    onContactClick: (Int) -> Unit = {},
    onSearchChange: (String) -> Unit = {},
    onFromDateChange: (String) -> Unit = {},
    onToDateChange: (String) -> Unit = {},
    onLoadMore: () -> Unit = {},
    onDelete: (Int) -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val state = uiState
    val snackbarHostState = remember { SnackbarHostState() }
    var pendingDelete by remember { mutableStateOf<Activity?>(null) }

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
                    Text(stringResource(R.string.nav_activities), style = MaterialTheme.typography.titleLarge)
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
            BrandFab(onClick = onCreateActivity) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.cd_new_activity))
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
                searchLabelRes = R.string.activities_search,
            )
            Box(modifier = Modifier.fillMaxSize()) {
                when {
                    state.isLoading -> LoadingSkeleton()
                    state.activities.isEmpty() && state.error == null ->
                        EmptyState(message = stringResource(if (hasFilters) R.string.activities_no_results else R.string.activities_empty))
                    state.activities.isEmpty() && (state.errorRes != null || state.error != null) ->
                        EmptyState(state.errorRes?.let { stringResource(it) } ?: state.error.orEmpty())
                    else -> {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(state.activities, key = { it.id }) { activity ->
                                ActivityListItem(
                                    activity = activity,
                                    onClick = { onEditActivity(activity.id) },
                                    onContactClick = onContactClick,
                                    onDelete = { pendingDelete = activity },
                                    isDeleting = state.deletingId == activity.id,
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

    pendingDelete?.let { activity ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.activities_delete_title)) },
            text = { Text(stringResource(R.string.activities_delete_confirm, activity.title.orEmpty().take(80))) },
            confirmButton = {
                TextButton(onClick = {
                    onDelete(activity.id)
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
    // "No activities yet". Only surface a snackbar for errors over a populated list.
    val listError = state.error
    if (listError != null && state.activities.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            onErrorShown()
        }
    }
}

@Composable
fun ActivityListItem(
    activity: Activity,
    onClick: () -> Unit,
    onContactClick: (Int) -> Unit,
    onDelete: () -> Unit,
    isDeleting: Boolean = false,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = activity.title.orEmpty(),
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            val subtitle = listOfNotNull(
                activity.type?.takeIf { it.isNotBlank() },
                activity.location?.takeIf { it.isNotBlank() },
                activity.date?.take(10),
            ).joinToString(" · ")
            if (subtitle.isNotBlank()) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            val contacts = activity.contacts.orEmpty()
            if (contacts.isNotEmpty()) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                    modifier = Modifier.padding(top = 6.dp),
                ) {
                    contacts.forEach { contact: ContactFlat ->
                        AssistChip(
                            onClick = { onContactClick(contact.id) },
                            label = { Text(contact.displayName) },
                        )
                    }
                }
            }
        }
        IconButton(onClick = onDelete, enabled = !isDeleting) {
            Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.action_delete))
        }
    }
}
