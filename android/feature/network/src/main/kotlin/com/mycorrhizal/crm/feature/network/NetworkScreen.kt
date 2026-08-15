package com.mycorrhizal.crm.feature.network

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.GraphChain
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * M14: the ego-centric network list over `GET /graph/connections` (the design
 * that replaced the force-graph — see the ticket). A depth-grouped, readable
 * path per reachable contact; every row is a real focusable list item, so the
 * whole surface is TalkBack-traversable, which a drawn graph never is.
 */
@Composable
fun NetworkScreen(
    showMenu: Boolean = false,
    onBack: () -> Unit = {},
    onMenuClick: () -> Unit = {},
    onOpenContact: (Int) -> Unit = {},
    viewModel: NetworkViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    NetworkScreenContent(
        uiState = state,
        showMenu = showMenu,
        onBack = onBack,
        onMenuClick = onMenuClick,
        onOpenContact = onOpenContact,
        onDepthChange = viewModel::setDepth,
        onRelationInputChange = viewModel::onRelationInputChange,
        onRelationApply = viewModel::applyRelation,
        onCircleSelect = viewModel::selectCircle,
        onOpenPicker = viewModel::openPicker,
        onClosePicker = viewModel::closePicker,
        onSearchContacts = viewModel::searchContacts,
        onSelectFrom = viewModel::selectFrom,
        onErrorShown = viewModel::onErrorShown,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NetworkScreenContent(
    uiState: NetworkUiState,
    showMenu: Boolean,
    onBack: () -> Unit,
    onMenuClick: () -> Unit,
    onOpenContact: (Int) -> Unit,
    onDepthChange: (Int) -> Unit,
    onRelationInputChange: (String) -> Unit,
    onRelationApply: () -> Unit,
    onCircleSelect: (String?) -> Unit,
    onOpenPicker: () -> Unit,
    onClosePicker: () -> Unit,
    onSearchContacts: (String) -> Unit,
    onSelectFrom: (ContactSummary) -> Unit,
    onErrorShown: () -> Unit,
) {
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = if (showMenu) onMenuClick else onBack) {
                        Icon(
                            imageVector = if (showMenu) Icons.Outlined.Menu else Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(if (showMenu) R.string.cd_menu else R.string.cd_back),
                        )
                    }
                },
                title = { Text(stringResource(R.string.nav_network), style = MaterialTheme.typography.titleLarge) },
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
            if (uiState.isLoading) {
                // Loading first so the "no starting contact" prompt doesn't
                // flash while the self contact / initial record is resolving.
                Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
                    LoadingSkeleton()
                }
            } else if (!uiState.hasFrom) {
                // No starting contact yet (drawer entry without a self contact)
                // — or a hard start error like "no VCard UID", which is shown
                // above the picker affordance so the user can recover.
                Box(modifier = Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.padding(24.dp),
                    ) {
                        val startError = uiState.errorRes?.let { stringResource(it) }
                        if (startError != null) {
                            Text(
                                text = startError,
                                color = MaterialTheme.colorScheme.error,
                                style = MaterialTheme.typography.bodyLarge,
                            )
                        }
                        Text(
                            text = stringResource(R.string.network_pick_prompt),
                            style = MaterialTheme.typography.bodyLarge,
                        )
                        OutlinedButton(onClick = onOpenPicker) {
                            Text(stringResource(R.string.network_pick_button))
                        }
                    }
                }
            } else {
                NetworkControls(
                    uiState = uiState,
                    onOpenPicker = onOpenPicker,
                    onDepthChange = onDepthChange,
                    onRelationInputChange = onRelationInputChange,
                    onRelationApply = onRelationApply,
                    onCircleSelect = onCircleSelect,
                )
                Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
                    when {
                        uiState.isLoading -> LoadingSkeleton()
                        uiState.groupedChains.isEmpty() && uiState.error == null ->
                            EmptyState(message = stringResource(R.string.network_empty))
                        uiState.groupedChains.isEmpty() && uiState.error != null -> {
                            Text(
                                text = uiState.error.orEmpty(),
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.align(Alignment.Center),
                            )
                        }
                        else -> NetworkList(
                            uiState = uiState,
                            onOpenContact = onOpenContact,
                        )
                    }
                }
            }
        }
    }

    // The snackbar is for fetch failures that leave stale content on screen
    // (an empty result already shows the error inline below the controls) —
    // presenting both for an empty failure would double-announce it.
    if (uiState.groupedChains.isNotEmpty()) {
        uiState.error?.let { message ->
            LaunchedEffect(message) {
                snackbarHostState.showSnackbar(message)
                onErrorShown()
            }
        }
    }

    if (uiState.pickerOpen) {
        ContactPickerDialog(
            uiState = uiState,
            onDismiss = onClosePicker,
            onSearch = onSearchContacts,
            onSelect = onSelectFrom,
        )
    }
}

/** The "start from", depth, relation and circle controls above the list. */
@Composable
private fun NetworkControls(
    uiState: NetworkUiState,
    onOpenPicker: () -> Unit,
    onDepthChange: (Int) -> Unit,
    onRelationInputChange: (String) -> Unit,
    onRelationApply: () -> Unit,
    onCircleSelect: (String?) -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        // Start from row.
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Icon(
                imageVector = Icons.Outlined.Person,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = stringResource(R.string.network_start_from),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text(
                    text = uiState.fromName.ifBlank { uiState.fromVCardUid },
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            TextButton(onClick = onOpenPicker) {
                Text(stringResource(R.string.network_change))
            }
        }

        // Depth stepper (1..3, default 2 — the design's mobile range).
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(R.string.network_depth),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            DEPTHS.forEach { depth ->
                FilterChip(
                    selected = uiState.depth == depth,
                    onClick = { onDepthChange(depth) },
                    label = { Text(depth.toString()) },
                    modifier = Modifier.testTag("depth-$depth"),
                )
            }
            if (uiState.circles.isNotEmpty()) {
                Spacer(modifier = Modifier.weight(1f))
                CircleFilter(uiState = uiState, onCircleSelect = onCircleSelect)
            }
        }

        // Relation filter (free text — token or synonym, passed straight through).
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            OutlinedTextField(
                value = uiState.relationInput,
                onValueChange = onRelationInputChange,
                label = { Text(stringResource(R.string.network_relation)) },
                placeholder = { Text(stringResource(R.string.network_relation_hint)) },
                singleLine = true,
                modifier = Modifier.weight(1f),
            )
            TextButton(
                onClick = onRelationApply,
                modifier = Modifier.testTag("relation-apply"),
            ) {
                Text(stringResource(R.string.network_relation_apply))
            }
        }
    }
}

@Composable
private fun CircleFilter(
    uiState: NetworkUiState,
    onCircleSelect: (String?) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val selectedName = uiState.circles.find { it.id == uiState.selectedCircleId }?.name
    Box {
        OutlinedButton(onClick = { expanded = true }, modifier = Modifier.testTag("circle-filter")) {
            Text(
                text = selectedName ?: stringResource(R.string.network_all_circles),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
            DropdownMenuItem(
                text = { Text(stringResource(R.string.network_all_circles)) },
                onClick = {
                    expanded = false
                    onCircleSelect(null)
                },
            )
            uiState.circles.forEach { circle ->
                DropdownMenuItem(
                    text = { Text(circle.name) },
                    onClick = {
                        expanded = false
                        onCircleSelect(circle.id)
                    },
                )
            }
        }
    }
}

@Composable
private fun NetworkList(
    uiState: NetworkUiState,
    onOpenContact: (Int) -> Unit,
) {
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        DEPTHS.forEach { depth ->
            val chains = uiState.groupedChains[depth].orEmpty()
            if (chains.isNotEmpty()) {
                item(key = "header-$depth") {
                    Text(
                        text = if (depth == 1) {
                            stringResource(R.string.network_direct)
                        } else {
                            stringResource(R.string.network_hops, depth)
                        },
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                    )
                }
                items(chains, key = { it.targetVCardUid.ifBlank { it.targetId.toString() } }) { chain ->
                    NetworkRow(chain = chain, onOpenContact = onOpenContact)
                }
            }
        }
    }
}

/**
 * One reachable contact. The row merges its descendants into a single
 * semantics node with a full content description, so TalkBack reads the whole
 * row (name + path) as one traversable item — the accessibility win that
 * justified choosing a list over a canvas. Rows whose target_id is 0 (a
 * soft-deleted intermediate the server degraded) render identically but are
 * not tappable.
 */
@Composable
private fun NetworkRow(
    chain: GraphChain,
    onOpenContact: (Int) -> Unit,
) {
    val contentDescription = stringResource(
        R.string.network_row_description,
        chain.displayName,
        chain.readablePath,
    )
    val clickable = if (chain.targetId != 0) {
        Modifier.clickable { onOpenContact(chain.targetId) }
    } else {
        Modifier
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .semantics(mergeDescendants = true) {
                this.contentDescription = contentDescription
            }
            .then(clickable)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column {
            Text(
                text = chain.displayName,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = chain.readablePath,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
internal fun ContactPickerDialog(
    uiState: NetworkUiState,
    onDismiss: () -> Unit,
    onSearch: (String) -> Unit,
    onSelect: (ContactSummary) -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.network_pick_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = uiState.contactSearchQuery,
                    onValueChange = onSearch,
                    label = { Text(stringResource(R.string.network_search_contacts)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().testTag("picker-search"),
                )
                if (uiState.contactSearchLoading) {
                    CircularProgressIndicator(modifier = Modifier.align(Alignment.CenterHorizontally))
                } else if (uiState.contactSearchResults.isEmpty()) {
                    Text(
                        text = stringResource(R.string.network_pick_no_results),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Column {
                        uiState.contactSearchResults.forEach { contact ->
                            TextButton(
                                onClick = { onSelect(contact) },
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Text(
                                    text = contact.displayName,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis,
                                )
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.action_cancel)) }
        },
    )
}

private val DEPTHS = listOf(1, 2, 3)
