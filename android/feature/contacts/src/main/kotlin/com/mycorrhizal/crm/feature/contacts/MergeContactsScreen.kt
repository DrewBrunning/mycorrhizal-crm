package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.feature.timeline.ContactSearchField
import com.mycorrhizal.crm.model.network.ContactMergeAssociationCounts
import com.mycorrhizal.crm.model.network.ContactMergeFieldConflict
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MergeContactsScreen(
    onBack: () -> Unit,
    keepId: Long,
    viewModel: MergeContactsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    LaunchedEffect(keepId) {
        if (keepId != 0L) viewModel.setPair(keepId, 0L)
    }

    MergeContactsScreenContent(
        uiState = state,
        onBack = onBack,
        onSearchQueryChange = viewModel::onSearchQueryChange,
        onPick = viewModel::selectOther,
        onResolve = viewModel::resolve,
        onCommit = viewModel::commit,
        onErrorShown = viewModel::onErrorShown,
    )
}

/**
 * Stateless merge content, split from [MergeContactsScreen] so the search picker and the
 * association breakdown are directly testable (mirrors `ContactListScreenContent`).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MergeContactsScreenContent(
    uiState: MergeUiState,
    onBack: () -> Unit,
    onSearchQueryChange: (String) -> Unit = {},
    onPick: (ContactSummary) -> Unit = {},
    onResolve: (String, String) -> Unit = { _, _ -> },
    onCommit: () -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val state = uiState
    val snackbarHostState = remember { SnackbarHostState() }
    val conflicts = state.preview
        ?.let { it.resolution.conflicts + it.resolution.fieldValueConflicts }
        .orEmpty()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.merge_title), style = MaterialTheme.typography.titleLarge) },
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
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
        ) {
            Text(
                text = stringResource(R.string.merge_keep_hint),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            // M23: search-based target picker — the reported gap was typing the target's
            // raw numeric ID. The keeper (keepId, the viewed contact) is excluded server-query-side.
            ContactSearchField(
                query = state.searchQuery,
                results = state.searchResults,
                loading = state.isSearching,
                onQueryChange = onSearchQueryChange,
                onPick = onPick,
                labelRes = R.string.merge_pick_other,
                modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp),
            )

            state.pickedOther?.let { other ->
                Text(
                    text = stringResource(R.string.merge_picked_other, other.displayName),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(bottom = 8.dp),
                )
            }

            // A picked target gives immediate feedback while the preview request is in flight
            // (a spinner), instead of dead air until the server responds.
            if (state.isLoading && state.preview == null) {
                CircularProgressIndicator(
                    modifier = Modifier
                        .align(Alignment.CenterHorizontally)
                        .padding(vertical = 12.dp)
                        .testTag("merge-preview-loading"),
                )
            }

            state.preview?.let { preview ->
                if (conflicts.isEmpty()) {
                    Text(
                        stringResource(R.string.merge_no_conflicts),
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.padding(vertical = 8.dp),
                    )
                } else {
                    conflicts.forEach { conflict ->
                        ConflictRow(
                            conflict = conflict,
                            chosen = state.resolutions[conflict.field],
                            onChoose = { value -> onResolve(conflict.field, value) },
                        )
                    }
                }
                AssociationCountsSection(counts = preview.associationCounts)
                Button(
                    onClick = onCommit,
                    enabled = !state.isCommitting && conflicts.all { state.resolutions.containsKey(it.field) },
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
                ) {
                    Text(stringResource(R.string.merge_commit))
                }
            }
            if (state.merged) {
                Text(stringResource(R.string.merge_done), color = MaterialTheme.colorScheme.primary)
            }
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            onErrorShown()
        }
    }
}

/**
 * M23: the full association-count breakdown web's MergeContactsDialog shows — not just
 * notes/activities/edges, but household, circle, tag, life-event, field-value, sync-link
 * and the T107 re-pointed categories. Renders one line per non-zero category; the whole
 * section is absent when there is nothing to move.
 */
@Composable
private fun AssociationCountsSection(counts: ContactMergeAssociationCounts) {
    val lines = listOfNotNull(
        counts.notes.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_notes, it) },
        counts.activities.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_activities, it) },
        counts.reminders.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_reminders, it) },
        counts.reminderCompletions.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_reminder_completions, it) },
        counts.relationshipEdges.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_edges, it) },
        counts.householdMemberships.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_household, it) },
        counts.circleMemberships.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_circles, it) },
        counts.tags.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_tags, it) },
        counts.lifeEvents.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_life_events, it) },
        counts.lifeEventReferences.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_life_event_references, it) },
        counts.conversationAgendaItems.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_agenda, it) },
        counts.giftItems.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_gifts, it) },
        counts.fieldValues.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_field_values, it) },
        counts.contactSyncLinks.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_sync_links, it) },
        counts.attachments.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_attachments, it) },
        counts.preferences.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_preferences, it) },
        counts.externalIdentities.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_external_identities, it) },
        counts.externalActivities.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_external_activities, it) },
        counts.cadencePolicies.takeIf { it > 0 }?.let { stringResource(R.string.merge_count_cadence, it) },
    )
    if (lines.isEmpty()) {
        Text(
            stringResource(R.string.merge_nothing_to_move),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(vertical = 8.dp),
        )
        return
    }
    Column(modifier = Modifier.padding(top = 8.dp)) {
        Text(
            text = stringResource(R.string.merge_move_header),
            style = MaterialTheme.typography.titleSmall,
        )
        lines.forEach { line ->
            Text(
                text = line,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(top = 4.dp),
            )
        }
    }
}

@Composable
private fun ConflictRow(
    conflict: ContactMergeFieldConflict,
    chosen: String?,
    onChoose: (String) -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Text(
            text = conflict.label,
            style = MaterialTheme.typography.titleSmall,
        )
        val options = listOfNotNull(conflict.keeperValue, conflict.loserValue).distinct()
        options.forEach { option ->
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier
                    .fillMaxWidth()
                    .clickable { onChoose(option) },
            ) {
                Checkbox(checked = chosen == option, onCheckedChange = { onChoose(option) })
                Text(option, modifier = Modifier.weight(1f))
            }
        }
    }
}
