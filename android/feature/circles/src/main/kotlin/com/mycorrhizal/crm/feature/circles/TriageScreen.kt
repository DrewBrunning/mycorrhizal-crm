package com.mycorrhizal.crm.feature.circles

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * M26: the circle/tag-triage screen (web's CircleTagTriagePage). One-time
 * cleanup of legacy free-text circle strings: classify each as circle/tag/skip
 * with inline rename, preview the summary, then apply.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TriageScreen(
    onBack: () -> Unit,
    viewModel: TriageViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.triage_title), style = MaterialTheme.typography.titleLarge) },
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
                state.isLoading -> LoadingSkeleton()
                state.done -> DoneContent(state = state, onBack = onBack)
                state.items.isEmpty() && state.error == null ->
                    EmptyState(message = stringResource(R.string.triage_empty))
                state.items.isEmpty() && state.error != null -> {
                    Text(
                        text = state.error.orEmpty(),
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.align(Alignment.Center),
                    )
                }
                else -> ClassifyContent(
                    state = state,
                    onSetClassification = viewModel::setClassification,
                    onSetName = viewModel::setName,
                    onApply = viewModel::apply,
                )
            }
        }
    }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }
}

@Composable
internal fun ClassifyContent(
    state: TriageUiState,
    onSetClassification: (Int, TriageClassification) -> Unit,
    onSetName: (Int, String) -> Unit,
    onApply: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxSize()) {
        LazyColumn(modifier = Modifier.weight(1f).fillMaxWidth()) {
            itemsIndexed(state.items, key = { _, item -> item.original }) { index, item ->
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
                ) {
                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        OutlinedTextField(
                            value = item.name,
                            onValueChange = { onSetName(index, it) },
                            label = { Text(stringResource(R.string.triage_name)) },
                            singleLine = true,
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Text(
                            text = stringResource(R.string.triage_contacts_count, item.contactCount),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        FilterChip(
                            selected = item.classification == TriageClassification.CIRCLE,
                            onClick = { onSetClassification(index, TriageClassification.CIRCLE) },
                            label = { Text(stringResource(R.string.triage_circle)) },
                        )
                        FilterChip(
                            selected = item.classification == TriageClassification.TAG,
                            onClick = { onSetClassification(index, TriageClassification.TAG) },
                            label = { Text(stringResource(R.string.triage_tag)) },
                        )
                        FilterChip(
                            selected = item.classification == TriageClassification.SKIP,
                            onClick = { onSetClassification(index, TriageClassification.SKIP) },
                            label = { Text(stringResource(R.string.triage_skip)) },
                        )
                    }
                }
            }
        }
        // Preview summary + Apply.
        Column(modifier = Modifier.fillMaxWidth().padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = stringResource(
                    R.string.triage_summary,
                    state.circleItems.size,
                    state.tagItems.size,
                    state.skippedCount,
                ),
                style = MaterialTheme.typography.bodyLarge,
            )
            Button(
                onClick = onApply,
                enabled = state.hasWork && !state.applying,
                modifier = Modifier.fillMaxWidth(),
            ) {
                Text(stringResource(R.string.triage_apply))
            }
        }
    }
}

@Composable
internal fun DoneContent(state: TriageUiState, onBack: () -> Unit) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(
            text = stringResource(R.string.triage_done_title),
            style = MaterialTheme.typography.titleLarge,
        )
        Text(
            text = stringResource(R.string.triage_done_message, state.appliedCircles, state.appliedTags),
            style = MaterialTheme.typography.bodyLarge,
        )
        Button(onClick = onBack, modifier = Modifier.fillMaxWidth()) {
            Text(stringResource(R.string.action_done))
        }
    }
}
