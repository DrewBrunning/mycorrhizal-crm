package com.mycorrhizal.crm.feature.settings

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
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
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
import androidx.compose.ui.semantics.heading
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

/**
 * Issue #830: Settings → Data → Custom fields — the field-definition list, mirroring web's
 * `CustomFieldsSettings.tsx`. Shaped like `TagsScreen` (list + FAB create + per-row edit/delete),
 * but reached via back-navigation from Settings rather than the hamburger drawer, so the app bar
 * uses a back arrow like [ImmichSettingsScreen].
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
                        EmptyState(message = stringResource(R.string.settings_custom_fields_empty))
                    state.definitions.isEmpty() && state.error != null -> {
                        Text(
                            text = state.error.orEmpty(),
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.align(Alignment.Center),
                        )
                    }
                    else -> {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(state.definitions, key = { it.id }) { definition ->
                                FieldDefinitionListItem(
                                    definition = definition,
                                    deleting = state.deletingId == definition.id,
                                    onEdit = { onEdit(definition.id) },
                                    onDelete = { viewModel.delete(definition.id) },
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
    deleting: Boolean,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    var confirmingDelete by remember { mutableStateOf(false) }

    if (confirmingDelete) {
        AlertDialog(
            onDismissRequest = { confirmingDelete = false },
            title = {
                Text(
                    stringResource(R.string.settings_custom_fields_delete_title),
                    modifier = Modifier.semantics { heading() },
                )
            },
            text = {
                Text(
                    stringResource(
                        R.string.settings_custom_fields_delete_confirm,
                        definition.label.orEmpty(),
                    ),
                )
            },
            confirmButton = {
                TextButton(
                    enabled = !deleting,
                    onClick = { onDelete(); confirmingDelete = false },
                ) {
                    Text(stringResource(R.string.action_delete), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmingDelete = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    Column(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = definition.label.orEmpty(),
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            AccessibleIconButton(onClick = onEdit) {
                Icon(
                    Icons.Outlined.Edit,
                    contentDescription = stringResource(
                        R.string.settings_custom_fields_edit_named,
                        definition.label.orEmpty(),
                    ),
                    tint = MaterialTheme.colorScheme.primary,
                )
            }
            AccessibleIconButton(onClick = { confirmingDelete = true }) {
                Icon(
                    Icons.Outlined.Delete,
                    contentDescription = stringResource(
                        R.string.settings_custom_fields_delete_named,
                        definition.label.orEmpty(),
                    ),
                )
            }
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            modifier = Modifier.padding(top = 4.dp),
        ) {
            AssistChip(onClick = {}, enabled = false, label = { Text(stringResource(fieldTypeLabelRes(definition.type))) })
            if (definition.constraints?.multi == true) {
                AssistChip(onClick = {}, enabled = false, label = { Text(stringResource(R.string.settings_custom_fields_multi)) })
            }
            AssistChip(
                onClick = {},
                enabled = false,
                label = { Text(stringResource(sensitivityLabelRes(definition.sensitivity))) },
            )
            if (definition.projection != null && definition.projection != "internal-only") {
                AssistChip(onClick = {}, enabled = false, label = { Text(definition.projection.orEmpty()) })
            }
        }
    }
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
