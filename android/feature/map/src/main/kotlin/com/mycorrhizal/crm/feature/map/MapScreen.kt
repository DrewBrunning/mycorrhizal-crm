package com.mycorrhizal.crm.feature.map

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.FormatListBulleted
import androidx.compose.material.icons.outlined.Map
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/** Test tag on the map canvas slot (the stub in tests carries the same tag). */
const val MAP_CANVAS_TAG = "contact-map-canvas"

/**
 * ADR 0031 / issue #1287: plots every contact address that has coordinates on
 * the instance's configured tile style. The same points are available as a
 * list (the top-bar toggle) because a map canvas is not TalkBack-traversable.
 */
@Composable
fun MapScreen(
    onOpenContact: (Int) -> Unit,
    // Issue #150: null hides the hamburger — there is no drawer at Expanded.
    onMenuClick: (() -> Unit)? = {},
    viewModel: MapViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    MapScreenContent(
        uiState = state,
        onMenuClick = onMenuClick,
        onOpenContact = onOpenContact,
        onSelect = viewModel::select,
        onShowListChange = viewModel::setShowList,
        onRetry = viewModel::load,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MapScreenContent(
    uiState: MapUiState,
    onMenuClick: (() -> Unit)?,
    onOpenContact: (Int) -> Unit,
    onSelect: (String?) -> Unit,
    onShowListChange: (Boolean) -> Unit,
    onRetry: () -> Unit = {},
    // Replaceable so tests (and previews) do not need the native renderer.
    mapCanvas: @Composable (styleUrl: String, points: List<MapPoint>, selectedKey: String?, Modifier) -> Unit =
        { styleUrl, points, selectedKey, modifier ->
            ContactMapView(
                styleUrl = styleUrl,
                points = points,
                selectedKey = selectedKey,
                onSelect = onSelect,
                modifier = modifier,
            )
        },
) {
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
                title = { Text(stringResource(R.string.nav_map), style = MaterialTheme.typography.titleLarge) },
                actions = {
                    if (uiState.points.isNotEmpty()) {
                        AccessibleIconButton(onClick = { onShowListChange(!uiState.showList) }) {
                            Icon(
                                imageVector = if (uiState.showList) Icons.Outlined.Map else Icons.Outlined.FormatListBulleted,
                                contentDescription = stringResource(
                                    if (uiState.showList) R.string.map_show_map else R.string.map_show_list,
                                ),
                            )
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                uiState.isLoading -> LoadingSkeleton()
                uiState.error != null -> Column(
                    modifier = Modifier.fillMaxSize().padding(24.dp),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Text(
                        text = uiState.error,
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodyLarge,
                    )
                    TextButton(onClick = onRetry) { Text(stringResource(R.string.action_retry)) }
                }
                else -> {
                    if (uiState.truncated) {
                        Text(
                            text = stringResource(R.string.map_truncated, uiState.points.size),
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodyMedium,
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
                        )
                    }
                    if (uiState.points.isEmpty()) {
                        EmptyState(message = stringResource(R.string.map_empty))
                    } else if (uiState.showList) {
                        PointList(points = uiState.points, onOpenContact = onOpenContact)
                    } else {
                        Box(modifier = Modifier.fillMaxSize()) {
                            mapCanvas(
                                uiState.styleUrl.orEmpty(),
                                uiState.points,
                                uiState.selectedKey,
                                Modifier.fillMaxSize().testTag(MAP_CANVAS_TAG),
                            )
                            uiState.selected?.let { point ->
                                SelectedCard(
                                    point = point,
                                    onOpenContact = onOpenContact,
                                    // Bottom padding clears MapLibre's attribution (the data licence requires it
                                    // to stay visible).
                                    modifier = Modifier.align(Alignment.BottomCenter).padding(start = 16.dp, end = 16.dp, top = 16.dp, bottom = 56.dp),
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
private fun SelectedCard(point: MapPoint, onOpenContact: (Int) -> Unit, modifier: Modifier = Modifier) {
    Card(modifier = modifier.fillMaxWidth().testTag("map-selected-card")) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(point.contactName, style = MaterialTheme.typography.titleMedium)
            if (point.label.isNotBlank()) {
                Text(point.label, style = MaterialTheme.typography.bodyMedium)
            }
            Button(onClick = { onOpenContact(point.contactId) }) {
                Text(stringResource(R.string.map_open_contact))
            }
        }
    }
}

@Composable
private fun PointList(points: List<MapPoint>, onOpenContact: (Int) -> Unit) {
    LazyColumn(modifier = Modifier.fillMaxSize().testTag("map-point-list")) {
        items(points, key = { it.key }) { point ->
            ListItem(
                headlineContent = { Text(point.contactName) },
                supportingContent = point.label.takeIf { it.isNotBlank() }?.let { label -> { Text(label) } },
                modifier = Modifier.clickable { onOpenContact(point.contactId) },
            )
        }
    }
}
