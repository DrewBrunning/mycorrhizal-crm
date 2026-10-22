package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectDragGesturesAfterLongPress
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.DragHandle
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
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
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.customActions
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import kotlin.math.roundToInt

/**
 * Issue #830: Settings → Data → Custom fields — the field-definition list, mirroring web's
 * `CustomFieldsSettings.tsx`. Shaped like `TagsScreen` (list + FAB create), but reached via
 * back-navigation from Settings rather than the hamburger drawer, so the app bar uses a back
 * arrow like [ImmichSettingsScreen].
 *
 * Issue #1775: a row is now tap-to-edit with one visible action (a drag handle). Reordering is
 * by drag, with move up/down exposed as TalkBack custom actions; delete moved into the edit
 * screen. The metadata is plain readable text, not disabled chips, and the raw vCard property
 * name is no longer shown here.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FieldDefinitionsScreen(
    onBack: () -> Unit,
    onCreate: () -> Unit,
    onEdit: (String) -> Unit,
    viewModel: FieldDefinitionsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    // Reload when the screen re-enters composition, so a delete performed on the edit screen
    // (issue #1775) is reflected when we come back.
    LaunchedEffect(Unit) { viewModel.load() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_custom_fields_title), style = MaterialTheme.typography.titleLarge)
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
            BrandFab(onClick = onCreate) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.settings_custom_fields_new_title))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            RefreshableContent(
                isRefreshing = state.isLoading && state.definitions.isNotEmpty(),
                onRefresh = viewModel::load,
                modifier = Modifier.fillMaxSize(),
            ) {
                when {
                    state.isLoading && state.definitions.isEmpty() -> LoadingSkeleton()
                    state.definitions.isEmpty() && state.error == null ->
                        EmptyState(
                            title = stringResource(R.string.settings_custom_fields_empty),
                            body = stringResource(R.string.settings_custom_fields_empty_body),
                            actionLabel = stringResource(R.string.settings_custom_fields_new_title),
                            onAction = onCreate,
                        )
                    state.definitions.isEmpty() && state.error != null -> {
                        Text(
                            text = state.error.orEmpty(),
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.align(Alignment.Center),
                        )
                    }
                    else -> {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            itemsIndexed(state.definitions, key = { _, definition -> definition.id }) { index, definition ->
                                FieldDefinitionListItem(
                                    definition = definition,
                                    canMoveUp = index > 0,
                                    canMoveDown = index < state.definitions.lastIndex,
                                    reordering = state.isReordering,
                                    onMoveUp = { viewModel.move(definition.id, -1) },
                                    onMoveDown = { viewModel.move(definition.id, 1) },
                                    onMoveBy = { steps -> viewModel.moveBy(definition.id, steps) },
                                    onEdit = { onEdit(definition.id) },
                                )
                            }
                        }
                    }
                }
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
private fun FieldDefinitionListItem(
    definition: FieldDefinition,
    canMoveUp: Boolean,
    canMoveDown: Boolean,
    reordering: Boolean,
    onMoveUp: () -> Unit,
    onMoveDown: () -> Unit,
    onMoveBy: (Int) -> Unit,
    onEdit: () -> Unit,
) {
    val label = definition.label.orEmpty()
    val moveUpLabel = stringResource(R.string.settings_custom_fields_move_up_named, label)
    val moveDownLabel = stringResource(R.string.settings_custom_fields_move_down_named, label)
    val reorderLabel = stringResource(R.string.settings_custom_fields_reorder_named, label)

    val threshold = with(LocalDensity.current) { 56.dp.toPx() }
    var dragOffset by remember { mutableFloatStateOf(0f) }
    var dragging by remember { mutableStateOf(false) }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .graphicsLayer {
                translationY = dragOffset
                if (dragging) shadowElevation = 8.dp.toPx()
            },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(
            modifier = Modifier
                .weight(1f)
                .clickable(role = Role.Button, onClick = onEdit)
                .padding(start = 16.dp, top = 8.dp, bottom = 8.dp),
        ) {
            Text(
                text = label,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = metadataSummary(definition),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Box(
            modifier = Modifier
                .size(48.dp)
                .pointerInput(definition.id, canMoveUp, canMoveDown, reordering) {
                    detectDragGesturesAfterLongPress(
                        onDragStart = {
                            if (!reordering) {
                                dragging = true
                                dragOffset = 0f
                            }
                        },
                        onDragCancel = {
                            dragging = false
                            dragOffset = 0f
                        },
                        onDragEnd = {
                            if (dragging) {
                                val steps = (dragOffset / threshold).roundToInt()
                                dragging = false
                                dragOffset = 0f
                                if (steps != 0) onMoveBy(steps)
                            }
                        },
                        onDrag = { change, amount ->
                            if (dragging) {
                                change.consume()
                                dragOffset += amount.y
                            }
                        },
                    )
                }
                .semantics(mergeDescendants = true) {
                    contentDescription = reorderLabel
                    customActions = buildList {
                        if (canMoveUp && !reordering) {
                            add(CustomAccessibilityAction(moveUpLabel) { onMoveUp(); true })
                        }
                        if (canMoveDown && !reordering) {
                            add(CustomAccessibilityAction(moveDownLabel) { onMoveDown(); true })
                        }
                    }
                },
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                Icons.Outlined.DragHandle,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** One readable metadata line instead of three low-contrast disabled chips (issue #1775). */
@Composable
private fun metadataSummary(definition: FieldDefinition): String {
    val parts = buildList {
        add(stringResource(fieldTypeLabelRes(definition.type)))
        if (definition.constraints?.multi == true) {
            add(stringResource(R.string.settings_custom_fields_multi))
        }
        if (definition.group.isNotBlank()) {
            add("${stringResource(R.string.settings_custom_fields_group)}: ${definition.group}")
        }
        add(stringResource(sensitivityLabelRes(definition.sensitivity)))
    }
    return parts.joinToString(" · ")
}

/** Localized field-type label, mirroring web's `customFields.types.*` i18n keys. */
fun fieldTypeLabelRes(type: String?): Int = when (type) {
    "string" -> R.string.custom_field_type_string
    "text" -> R.string.custom_field_type_text
    "number" -> R.string.custom_field_type_number
    "boolean" -> R.string.custom_field_type_boolean
    "date" -> R.string.custom_field_type_date
    "datetime" -> R.string.custom_field_type_datetime
    "uri" -> R.string.custom_field_type_uri
    "email" -> R.string.custom_field_type_email
    "phone" -> R.string.custom_field_type_phone
    "enum" -> R.string.custom_field_type_enum
    else -> R.string.custom_field_type_string
}

/** Reuses `RelationshipEdge.Sensitivity`'s existing three-word label set. */
fun sensitivityLabelRes(sensitivity: String?): Int = when (sensitivity) {
    "private" -> R.string.relationships_sensitivity_private
    "secret" -> R.string.relationships_sensitivity_secret
    else -> R.string.relationships_sensitivity_normal
}
