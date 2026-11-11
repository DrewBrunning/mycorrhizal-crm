package com.mycorrhizal.crm.feature.contacts.entities

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
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityCore
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntitySensitivity
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.FabListContentPadding
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * Issue #2302: the entity list (organisations, places, pets) - one kind tab at a time, name
 * search, add. Reached from Settings, so it has a back arrow, not the drawer menu.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EntitiesScreen(
    onBack: () -> Unit,
    onOpenEntity: (String) -> Unit,
    modifier: Modifier = Modifier,
    viewModel: EntitiesViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { viewModel.onResumed() }
    val snackbar = remember { SnackbarHostState() }
    var showCreate by rememberSaveable { mutableStateOf(false) }

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.shared_entities_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        floatingActionButton = {
            BrandFab(onClick = { showCreate = true }) {
                Icon(
                    Icons.Outlined.Add,
                    contentDescription = stringResource(entityNewRes(state.kind))
                )
            }
        },
        snackbarHost = { SnackbarHost(snackbar) }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            EntitiesContent(
                state = state,
                onSelectKind = viewModel::selectKind,
                onQueryChange = viewModel::onQueryChange,
                onOpenEntity = onOpenEntity,
                onLoadMore = viewModel::loadMore,
                onRetry = viewModel::refresh,
                onAdd = { showCreate = true }
            )
        }
    }

    if (showCreate) {
        EntityFormDialog(
            kind = state.kind,
            entity = null,
            saving = state.isSaving,
            error = state.formError,
            onSave = { input ->
                viewModel.create(input) { created ->
                    showCreate = false
                    onOpenEntity(created.id)
                }
            },
            onDismiss = {
                showCreate = false
                viewModel.onFormDismissed()
            }
        )
    }
}

@Composable
internal fun EntitiesContent(
    state: EntitiesUiState,
    onSelectKind: (String) -> Unit,
    onQueryChange: (String) -> Unit,
    onOpenEntity: (String) -> Unit,
    onLoadMore: () -> Unit,
    onRetry: () -> Unit,
    onAdd: () -> Unit
) {
    Column(Modifier.fillMaxSize()) {
        Text(
            text = stringResource(R.string.shared_entities_description),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
        )
        PrimaryTabRow(
            selectedTabIndex = EntityKind.ALL.indexOf(state.kind).coerceAtLeast(0),
            modifier = Modifier.testTag("entities-tabs")
        ) {
            EntityKind.ALL.forEach { kind ->
                Tab(
                    selected = kind == state.kind,
                    onClick = { onSelectKind(kind) },
                    text = { Text(stringResource(entityKindTabRes(kind))) },
                    modifier = Modifier.testTag("entities-tab-$kind")
                )
            }
        }
        OutlinedTextField(
            value = state.query,
            onValueChange = onQueryChange,
            label = { Text(stringResource(R.string.shared_entities_search)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)
                .testTag("entities-search")
        )
        Box(Modifier.fillMaxWidth().weight(1f)) {
            when {
                state.isLoading && state.entities.isEmpty() -> LoadingSkeleton()

                state.error != null && state.entities.isEmpty() -> Column(
                    modifier = Modifier.fillMaxSize().padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Text(
                        text = stringResource(R.string.shared_entities_loading_failed),
                        color = MaterialTheme.colorScheme.error
                    )
                    Text(
                        text = state.error.localizedMessage(),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    TextButton(onClick = onRetry) { Text(stringResource(R.string.action_retry)) }
                }

                state.entities.isEmpty() -> if (state.query.isNotBlank()) {
                    EmptyState(title = stringResource(R.string.shared_entities_no_results))
                } else {
                    EmptyState(
                        title = stringResource(entityEmptyRes(state.kind)),
                        actionLabel = stringResource(entityNewRes(state.kind)),
                        onAction = onAdd
                    )
                }

                else -> LazyColumn(
                    modifier = Modifier.fillMaxSize().testTag("entities-list"),
                    contentPadding = FabListContentPadding
                ) {
                    items(state.entities, key = { it.id }) { entity ->
                        EntityRow(entity, onClick = { onOpenEntity(entity.id) })
                        HorizontalDivider()
                    }
                    if (state.nextCursor != null) {
                        item(key = "load-more") {
                            val currentOnLoadMore by rememberUpdatedState(onLoadMore)
                            LaunchedEffect(state.nextCursor) { currentOnLoadMore() }
                            LoadingSkeleton(rows = 1)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun EntityRow(entity: Entity, onClick: () -> Unit) {
    val summary = EntityCore.summary(entity.kind, entity.core)
    val openLabel = stringResource(R.string.shared_entities_open, entity.name)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClickLabel = openLabel, role = Role.Button, onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp)
            .testTag("entity-row-${entity.id}"),
        horizontalArrangement = Arrangement.SpaceBetween
    ) {
        Column(Modifier.weight(1f)) {
            Text(
                entity.name,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            if (summary.isNotBlank()) {
                Text(
                    summary,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
        }
        if (entity.sensitivity != EntitySensitivity.NORMAL) {
            Text(
                stringResource(entitySensitivityRes(entity.sensitivity)),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}
