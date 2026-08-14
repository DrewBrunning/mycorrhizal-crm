package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CalendarToday
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.EventNote
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.StickyNote2
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import com.mycorrhizal.crm.ui.R

/**
 * The unified timeline section for a contact detail page (Phase 2 item 10).
 * Renders the merged activity/note/reminder entries grouped under the row's
 * date. Rendering is delegated to the dedicated list screens' item composables
 * for consistency; this section is a flat merge for display, and tapping an
 * item routes to the matching edit form.
 */
@Composable
fun TimelineSection(
    items: List<TimelineItem>,
    onEditActivity: (Int) -> Unit,
    onEditNote: (Int) -> Unit,
    onEditReminder: (Int) -> Unit,
    onCompleteReminder: (Int) -> Unit,
    onUndoCompletion: (Int) -> Unit = {},
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier) {
        if (items.isEmpty()) {
            Text(
                text = stringResource(R.string.timeline_empty),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
            )
            return
        }
        items.forEach { item ->
            when (item) {
                is TimelineItem.ActivityItem -> TimelineActivityRow(
                    item.activity.title.orEmpty(),
                    item.activity.type.orEmpty(),
                    onClick = { onEditActivity(item.activity.id) },
                )
                is TimelineItem.NoteItem -> TimelineNoteRow(
                    item.note.content.orEmpty(),
                    onClick = { onEditNote(item.note.id) },
                )
                is TimelineItem.ReminderItem -> TimelineReminderRow(
                    message = item.reminder.message.orEmpty(),
                    recurrence = item.reminder.recurrence,
                    completed = item.reminder.completed,
                    onClick = { onEditReminder(item.reminder.id) },
                    onComplete = { onCompleteReminder(item.reminder.id) },
                )
                is TimelineItem.CompletionItem -> TimelineCompletionRow(
                    message = item.completion.message.orEmpty(),
                    onUndo = { onUndoCompletion(item.completion.id) },
                )
            }
        }
    }
}

@Composable
private fun TimelineRowBase(
    icon: @Composable () -> Unit,
    title: String,
    subtitle: String,
    onClick: () -> Unit,
    trailing: (@Composable () -> Unit)? = null,
) {
    androidx.compose.material3.ListItem(
        headlineContent = {
            Text(
                text = title,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        },
        supportingContent = {
            if (subtitle.isNotBlank()) {
                Text(
                    text = subtitle,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        leadingContent = { icon() },
        trailingContent = trailing?.let { { it() } },
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
    )
}

@Composable
private fun TimelineActivityRow(title: String, type: String, onClick: () -> Unit) {
    TimelineRowBase(
        icon = { Icon(Icons.Outlined.EventNote, contentDescription = stringResource(R.string.cd_activity)) },
        title = title,
        subtitle = type,
        onClick = onClick,
    )
}

@Composable
private fun TimelineNoteRow(content: String, onClick: () -> Unit) {
    TimelineRowBase(
        icon = { Icon(Icons.Outlined.StickyNote2, contentDescription = stringResource(R.string.cd_note)) },
        title = content,
        subtitle = "",
        onClick = onClick,
    )
}

@Composable
private fun TimelineReminderRow(
    message: String,
    recurrence: String?,
    completed: Boolean,
    onClick: () -> Unit,
    onComplete: () -> Unit,
) {
    TimelineRowBase(
        icon = { Icon(Icons.Outlined.Notifications, contentDescription = stringResource(R.string.cd_reminder)) },
        title = message,
        subtitle = recurrence ?: "",
        onClick = onClick,
        trailing = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (!completed) {
                    androidx.compose.material3.IconButton(onClick = onComplete) {
                        Icon(Icons.Outlined.CalendarToday, contentDescription = stringResource(R.string.cd_complete_reminder))
                    }
                }
            }
        },
    )
}

/** A completed reminder's timeline row (web's "Reminder completed"). Undo deletes it. */
@Composable
private fun TimelineCompletionRow(
    message: String,
    onUndo: () -> Unit,
) {
    TimelineRowBase(
        icon = {
            Icon(
                Icons.Outlined.Notifications,
                contentDescription = stringResource(R.string.cd_reminder),
                tint = MaterialTheme.colorScheme.primary,
            )
        },
        title = stringResource(R.string.reminder_completed),
        subtitle = message,
        onClick = {},
        trailing = {
            IconButton(onClick = onUndo) {
                Icon(
                    Icons.Outlined.Delete,
                    contentDescription = stringResource(R.string.cd_undo_completion),
                    tint = MaterialTheme.colorScheme.error,
                )
            }
        },
    )
}
