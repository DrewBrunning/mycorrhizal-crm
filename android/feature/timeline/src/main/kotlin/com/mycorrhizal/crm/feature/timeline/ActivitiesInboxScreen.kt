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
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material3.AssistChip
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
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ContactFlat
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * M9 item 1: the "Activities" drawer entry — every activity across every contact (matching web's
 * `ActivitiesPage.tsx`), reachable from the drawer instead of the old `PlaceholderScreen` stub.
 * Read + tap-to-edit-existing only, same reasoning as [NotesInboxScreen] (no wired create-without-
 * a-contact endpoint yet): [onActivityClick] reuses the existing per-contact edit route with
 * contact id `0` (never a real id) — editing hydrates its participant list straight from the
 * loaded activity, so the route's own contact id is never read for that path.
 */
@Composable
fun ActivitiesInboxScreen(
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    onActivityClick: (Int) -> Unit,
    onContactClick: (Int) -> Unit,
    viewModel: ActivitiesInboxViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    ActivitiesInboxScreenContent(
        uiState = state,
        onMenuClick = onMenuClick,
        onActivityClick = onActivityClick,
        onContactClick = onContactClick,
        onLoadMore = viewModel::loadMore,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless screen content, split out from [ActivitiesInboxScreen] so it's directly testable
 * without a Hilt-backed ViewModel (mirrors `ContactListScreenContent`'s split in
 * `ContactListScreen.kt`).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ActivitiesInboxScreenContent(
    uiState: ActivitiesInboxUiState,
    // Issue #150: see ActivitiesInboxScreen — null hides the hamburger.
    onMenuClick: (() -> Unit)? = {},
    onActivityClick: (Int) -> Unit = {},
    onContactClick: (Int) -> Unit = {},
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
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton(modifier = Modifier.testTag("activities-inbox-loading"))
                state.activities.isEmpty() && state.error == null ->
                    EmptyState(message = stringResource(R.string.activities_empty))
                state.activities.isEmpty() && state.error != null ->
                    EmptyState(state.error.orEmpty())
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize().testTag("activities-inbox-list")) {
                        items(state.activities, key = { it.id }) { activity ->
                            InboxActivityRow(
                                activity = activity,
                                onClick = { onActivityClick(activity.id) },
                                onContactClick = onContactClick,
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

    val listError = state.error
    if (listError != null && state.activities.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            onErrorShown()
        }
    }
}

@Composable
private fun InboxActivityRow(
    activity: Activity,
    onClick: () -> Unit,
    onContactClick: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
    ) {
        Text(activity.title.orEmpty(), style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
        val subtitle = listOfNotNull(
            activity.type?.takeIf { it.isNotBlank() },
            activity.location?.takeIf { it.isNotBlank() },
            activity.date?.take(10),
        ).joinToString(" · ")
        if (subtitle.isNotBlank()) {
            Text(subtitle, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
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
                        // #214: AssistChip's default height (32dp) is below the 48dp touch
                        // target minimum (WCAG 2.5.8) — the semantics node's reported size
                        // is the chip's own measured bounds, so (unlike a real device's
                        // separate touch-dispatch expansion) only a real height constraint
                        // changes what accessibility services see; minimumInteractiveComponentSize
                        // reserves layout space only, it does not change reported bounds.
                        modifier = Modifier.heightIn(min = 48.dp),
                    )
                }
            }
        }
    }
}
