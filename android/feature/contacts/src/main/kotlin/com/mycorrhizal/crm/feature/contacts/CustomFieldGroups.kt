package com.mycorrhizal.crm.feature.contacts

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldGroup
import com.mycorrhizal.crm.model.network.groupFieldDefinitions
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton

/** Test tag of the list inside an open custom-field group drill-down. */
internal const val CUSTOM_FIELD_GROUP_LIST_TAG = "custom-field-group-list"

/**
 * Issue #1731: the contact detail's custom fields as one drill-down row per group, not one inline
 * row per definition. Each row shows the group label and "N of M filled"; tapping it opens
 * [CustomFieldGroupDialog] with the existing [CustomFieldValueRow] editors. Groups with no filled
 * value still appear (so they can be filled) but are muted and sorted below the filled ones —
 * [groupFieldDefinitions] owns that ordering.
 */
@Composable
internal fun CustomFieldGroupsCard(
    definitions: List<FieldDefinition>,
    values: Map<String, Any?>,
    savingDefinitionId: String?,
    onSaveValue: (String, Any?) -> Unit,
) {
    val groups = remember(definitions, values) { groupFieldDefinitions(definitions, values) }
    // The open group is remembered by label: after a save the groups are recomputed and the
    // dialog follows the same label to the refreshed bucket.
    var openLabel by rememberSaveable { mutableStateOf<String?>(null) }

    SectionCard(stringResource(R.string.contact_custom_fields)) {
        groups.forEach { group ->
            CustomFieldGroupRow(group = group, onClick = { openLabel = group.label })
        }
    }

    val open = openLabel?.let { label -> groups.firstOrNull { it.label == label } }
    if (open != null) {
        CustomFieldGroupDialog(
            group = open,
            values = values,
            savingDefinitionId = savingDefinitionId,
            onSaveValue = onSaveValue,
            onDismiss = { openLabel = null },
        )
    }
}

@Composable
internal fun groupTitle(group: FieldGroup): String =
    if (group.isUngrouped) stringResource(R.string.contact_custom_fields) else group.label

@Composable
private fun CustomFieldGroupRow(group: FieldGroup, onClick: () -> Unit) {
    val title = groupTitle(group)
    val supporting = pluralStringResource(
        R.plurals.contact_custom_field_group_filled,
        group.definitions.size,
        group.filledCount,
        group.definitions.size,
    )
    val muted = !group.hasFilled
    ListItem(
        modifier = Modifier
            .clickable(onClickLabel = stringResource(R.string.contact_custom_field_group_open, title), onClick = onClick)
            .testTag("custom-field-group-${group.label}"),
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        headlineContent = {
            Text(
                text = title,
                color = if (muted) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
            )
        },
        supportingContent = { Text(supporting) },
        trailingContent = {
            Icon(Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null)
        },
    )
}

/**
 * A full-screen drill-down over the detail screen: top bar with the group title and a back
 * arrow, then every field of the group (empty ones included, so they can be filled) as
 * [CustomFieldValueRow]s in `position` order.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun CustomFieldGroupDialog(
    group: FieldGroup,
    values: Map<String, Any?>,
    savingDefinitionId: String?,
    onSaveValue: (String, Any?) -> Unit,
    onDismiss: () -> Unit,
) {
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.surface) {
            Scaffold(
                topBar = {
                    TopAppBar(
                        title = { Text(groupTitle(group)) },
                        navigationIcon = {
                            AccessibleIconButton(onClick = onDismiss) {
                                Icon(
                                    Icons.AutoMirrored.Outlined.ArrowBack,
                                    contentDescription = stringResource(R.string.action_back),
                                )
                            }
                        },
                    )
                },
            ) { padding ->
                LazyColumn(modifier = Modifier.padding(padding).fillMaxSize().testTag(CUSTOM_FIELD_GROUP_LIST_TAG)) {
                    items(group.definitions, key = { it.id }) { definition ->
                        CustomFieldValueRow(
                            definition = definition,
                            value = values[definition.id],
                            saving = savingDefinitionId == definition.id,
                            onSave = { newValue -> onSaveValue(definition.id, newValue) },
                        )
                    }
                }
            }
        }
    }
}
