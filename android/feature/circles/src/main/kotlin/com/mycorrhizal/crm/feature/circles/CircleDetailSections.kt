package com.mycorrhizal.crm.feature.circles

import androidx.annotation.StringRes
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleColors
import com.mycorrhizal.crm.model.network.CircleTimelineItem
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.DiscardChangesDialog
import com.mycorrhizal.crm.ui.components.localizedMessage

internal const val CIRCLE_EDIT_NAME_TAG = "circle_edit_name"
internal const val CIRCLE_EDIT_DESCRIPTION_TAG = "circle_edit_description"

@StringRes
private fun circleColorLabelRes(token: String): Int? = when (token) {
    "red" -> R.string.circles_color_red
    "orange" -> R.string.circles_color_orange
    "yellow" -> R.string.circles_color_yellow
    "green" -> R.string.circles_color_green
    "teal" -> R.string.circles_color_teal
    "blue" -> R.string.circles_color_blue
    "purple" -> R.string.circles_color_purple
    "pink" -> R.string.circles_color_pink
    "grey" -> R.string.circles_color_grey
    else -> null
}

@StringRes
private fun timelineTypeRes(type: String): Int? = when (type) {
    "note" -> R.string.circles_timeline_type_note
    "activity" -> R.string.circles_timeline_type_activity
    "completion" -> R.string.circles_timeline_type_completion
    "life_event" -> R.string.circles_timeline_type_life_event
    "external_activity" -> R.string.circles_timeline_type_external_activity
    "gift" -> R.string.circles_timeline_type_gift
    else -> null
}

/** The circle's colour (dot + name) and description; nothing when neither is set. */
@Composable
internal fun CircleHeader(circle: Circle?, modifier: Modifier = Modifier) {
    val color = circle?.color?.let(::circleColorOrNull)
    val description = circle?.description.orEmpty()
    if (color == null && description.isBlank()) return
    Column(
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp)
    ) {
        if (color != null) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    Modifier.size(12.dp).background(color, CircleShape).clearAndSetSemantics {}
                )
                circleColorLabelRes(circle.color)?.let {
                    Text(
                        text = stringResource(it),
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(start = 8.dp)
                    )
                }
            }
        }
        if (description.isNotBlank()) {
            Text(text = description, style = MaterialTheme.typography.bodyMedium)
        }
    }
}

/** The aggregate member timeline (issue #1995/#2361): count, entries, empty/error states, paging. */
internal fun LazyListScope.circleTimelineSection(
    state: CircleDetailUiState,
    onNavigateToContact: (Int) -> Unit,
    onLoadMore: () -> Unit,
    onRetry: () -> Unit
) {
    item(key = "timeline-title") {
        Column(Modifier.fillMaxWidth()) {
            HorizontalDivider()
            Text(
                text = stringResource(R.string.circles_timeline_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 12.dp)
            )
            val count = state.timelineMemberCount ?: state.members.size
            Text(
                text = pluralStringResource(R.plurals.circles_member_count, count, count),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp)
            )
        }
    }
    state.timelineError?.let { error ->
        item(key = "timeline-error") {
            Row(
                modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    text = error.localizedMessage(),
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.weight(1f)
                )
                TextButton(onClick = onRetry) { Text(stringResource(R.string.action_retry)) }
            }
        }
    }
    if (state.isTimelineLoading && state.timeline.isEmpty()) {
        item(key = "timeline-loading") {
            CircularProgressIndicator(Modifier.padding(16.dp).size(24.dp))
        }
    }
    if (state.timelineLoaded && state.timeline.isEmpty() && state.timelineError == null) {
        item(key = "timeline-empty") {
            Text(
                text = stringResource(R.string.circles_timeline_empty),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(16.dp)
            )
        }
    }
    items(state.timeline, key = { "timeline-${it.type}-${it.id}" }) { entry ->
        CircleTimelineRow(entry, onNavigateToContact)
    }
    if (state.timelineCursor != null) {
        item(key = "timeline-more") {
            TextButton(
                onClick = onLoadMore,
                enabled = !state.isTimelineLoading,
                modifier = Modifier.padding(horizontal = 8.dp)
            ) {
                Text(stringResource(R.string.action_load_more))
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun CircleTimelineRow(entry: CircleTimelineItem, onNavigateToContact: (Int) -> Unit) {
    val locale = LocalConfiguration.current.locales[0]
    val endDate = if (entry.type == "activity") entry.data?.get("end_date") as? String else null
    val typeLabel = timelineTypeRes(entry.type)?.let { stringResource(it) } ?: entry.type
    Column(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp)
    ) {
        entry.summary().takeIf { it.isNotEmpty() }?.let {
            Text(text = it, style = MaterialTheme.typography.bodyLarge)
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            entry.members.forEachIndexed { index, member ->
                val suffix = if (index < entry.members.lastIndex) "," else ""
                Text(
                    text = member.displayName() + suffix,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.primary,
                    modifier = if (member.id != 0) {
                        Modifier.clickable { onNavigateToContact(member.id) }
                    } else {
                        Modifier
                    }
                )
            }
        }
        Text(
            text = "$typeLabel · ${formatTimelineDate(entry.date, endDate, locale)}",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}

/** Edit name, description (max 500) and colour of a circle; unsaved edits guard dismissal. */
@Composable
internal fun CircleEditDialog(
    initialName: String,
    initialDescription: String,
    initialColor: String,
    isSaving: Boolean,
    nameError: String?,
    onNameChange: () -> Unit,
    onConfirm: (name: String, description: String, color: String) -> Unit,
    onDismiss: () -> Unit
) {
    var name by rememberSaveable { mutableStateOf(initialName) }
    var description by rememberSaveable { mutableStateOf(initialDescription) }
    var color by rememberSaveable { mutableStateOf(initialColor) }
    var showDiscardDialog by rememberSaveable { mutableStateOf(false) }
    val dirty = name.trim() != initialName ||
        description.trim() != initialDescription ||
        color != initialColor
    val requestDismiss = { if (dirty) showDiscardDialog = true else onDismiss() }
    AlertDialog(
        onDismissRequest = requestDismiss,
        title = { Text(stringResource(R.string.circles_edit)) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                OutlinedTextField(
                    value = name,
                    onValueChange = {
                        name = it
                        onNameChange()
                    },
                    label = { Text(stringResource(R.string.circles_name)) },
                    isError = nameError != null,
                    supportingText = nameError?.let { message -> { Text(message) } },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().testTag(CIRCLE_EDIT_NAME_TAG)
                )
                OutlinedTextField(
                    value = description,
                    onValueChange = { description = it.take(CircleColors.MAX_DESCRIPTION) },
                    label = { Text(stringResource(R.string.circles_description_label)) },
                    supportingText = {
                        Text(
                            stringResource(
                                R.string.circles_description_counter,
                                description.length,
                                CircleColors.MAX_DESCRIPTION
                            )
                        )
                    },
                    minLines = 2,
                    maxLines = 5,
                    modifier = Modifier.fillMaxWidth().testTag(CIRCLE_EDIT_DESCRIPTION_TAG)
                )
                Text(
                    text = stringResource(R.string.circles_color_label),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                FlowColorPicker(selected = color, onSelect = { color = it })
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(name, description, color) },
                enabled = name.isNotBlank() && dirty && !isSaving
            ) {
                Text(stringResource(R.string.action_save))
            }
        },
        dismissButton = {
            TextButton(onClick = requestDismiss) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    )
    if (showDiscardDialog) {
        DiscardChangesDialog(
            entityLabel = stringResource(R.string.entity_circle),
            onKeepEditing = { showDiscardDialog = false },
            onDiscard = {
                showDiscardDialog = false
                onDismiss()
            }
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun FlowColorPicker(selected: String, onSelect: (String) -> Unit) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        ColorOption(
            label = stringResource(R.string.circles_color_none),
            color = null,
            selected = selected.isEmpty(),
            onClick = { onSelect("") }
        )
        CircleColors.ALL.forEach { token ->
            ColorOption(
                label = circleColorLabelRes(token)?.let { stringResource(it) } ?: token,
                color = circleColorOrNull(token),
                selected = selected == token,
                onClick = { onSelect(token) }
            )
        }
    }
}

@Composable
private fun ColorOption(label: String, color: Color?, selected: Boolean, onClick: () -> Unit) {
    Box(
        modifier = Modifier
            .size(48.dp)
            .selectable(selected = selected, role = Role.RadioButton, onClick = onClick)
            .semantics { contentDescription = label },
        contentAlignment = Alignment.Center
    ) {
        val ring = if (selected) {
            Modifier.border(
                BorderStroke(3.dp, MaterialTheme.colorScheme.onSurface),
                CircleShape
            )
        } else {
            Modifier
        }
        if (color != null) {
            Box(Modifier.size(28.dp).then(ring).padding(4.dp).background(color, CircleShape))
        } else {
            Icon(
                imageVector = Icons.Outlined.Block,
                contentDescription = null,
                modifier = Modifier.size(28.dp).then(ring).padding(2.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}
