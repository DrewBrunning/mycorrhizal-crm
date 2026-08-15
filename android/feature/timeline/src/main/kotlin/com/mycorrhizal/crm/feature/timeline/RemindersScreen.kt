package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Email
import androidx.compose.material.icons.outlined.Repeat
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.hilt.navigation.compose.hiltViewModel
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.model.util.DateFormat
import com.mycorrhizal.crm.ui.components.BrandFab
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.theme.MycorrhizalColors
import com.mycorrhizal.crm.ui.R

/**
 * M20: per-contact reminders list. The hilt-backed wrapper; the stateful UI
 * lives in [RemindersScreenContent] so the delete-confirm flow is directly
 * testable (M19's ActivitiesScreen/ActivitiesScreenContent split).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RemindersScreen(
    onBack: () -> Unit,
    onCreateReminder: () -> Unit,
    onEditReminder: (Int) -> Unit,
    viewModel: RemindersViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    RemindersScreenContent(
        uiState = state,
        onBack = onBack,
        onCreateReminder = onCreateReminder,
        onEditReminder = onEditReminder,
        onComplete = viewModel::complete,
        onDelete = viewModel::delete,
        onErrorShown = viewModel::onErrorShown,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RemindersScreenContent(
    uiState: RemindersUiState,
    onBack: () -> Unit = {},
    onCreateReminder: () -> Unit = {},
    onEditReminder: (Int) -> Unit = {},
    onComplete: (Int) -> Unit = {},
    onDelete: (Int) -> Unit = {},
    onErrorShown: () -> Unit = {},
) {
    val state = uiState
    val snackbarHostState = remember { SnackbarHostState() }
    var pendingDelete by remember { mutableStateOf<Reminder?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.reminders_title), style = MaterialTheme.typography.titleLarge)
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
            BrandFab(onClick = onCreateReminder) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.cd_new_reminder))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.reminders.isEmpty() && state.error == null ->
                    EmptyState(message = stringResource(R.string.reminders_empty))
                state.reminders.isEmpty() && (state.errorRes != null || state.error != null) ->
                    EmptyState(state.errorRes?.let { stringResource(it) } ?: state.error.orEmpty())
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.reminders, key = { it.id }) { reminder ->
                            ReminderListItem(
                                reminder = reminder,
                                dateFormat = state.dateFormat,
                                onClick = { onEditReminder(reminder.id) },
                                onComplete = { onComplete(reminder.id) },
                                onDelete = { pendingDelete = reminder },
                                isCompleting = state.completingId == reminder.id,
                                isDeleting = state.deletingId == reminder.id,
                            )
                        }
                    }
                }
            }
        }
    }

    pendingDelete?.let { reminder ->
        AlertDialog(
            onDismissRequest = { pendingDelete = null },
            title = { Text(stringResource(R.string.reminder_delete_title)) },
            text = { Text(stringResource(R.string.reminder_delete_confirm, reminder.message.orEmpty().take(80))) },
            confirmButton = {
                TextButton(onClick = {
                    onDelete(reminder.id)
                    pendingDelete = null
                }) {
                    Text(stringResource(R.string.action_delete))
                }
            },
            dismissButton = {
                TextButton(onClick = { pendingDelete = null }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    // When the list is empty the error text is the persistent body content
    // (EmptyState above), so don't toast-and-clear it into a misleading
    // "No reminders yet". Only surface a snackbar for errors over a populated list.
    val listError = state.error
    if (listError != null && state.reminders.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            onErrorShown()
        }
    }
}

@Composable
fun ReminderListItem(
    reminder: Reminder,
    onClick: () -> Unit,
    onComplete: () -> Unit,
    onDelete: () -> Unit,
    isCompleting: Boolean,
    isDeleting: Boolean = false,
    dateFormat: String? = null,
    modifier: Modifier = Modifier,
) {
    val overdue = reminder.isOverdue()
    val dateText = DateFormat.formatTimestamp(reminder.remindAt, dateFormat ?: DateFormat.EU)

    Card(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 4.dp)
            .clickable(onClick = onClick),
        border = if (overdue) {
            BorderStroke(
                width = 2.dp,
                color = MycorrhizalColors.chanterelle,
            )
        } else {
            null
        },
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(start = 16.dp, top = 12.dp, end = 4.dp, bottom = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = reminder.message.orEmpty(),
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
                val recurrence = reminder.recurrence?.takeIf { it.isNotBlank() && it != ReminderRecurrence.ONCE }
                if (recurrence != null) {
                    Text(
                        text = stringResource(reminder.recurrenceLabel()),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    // Web's date chip is the overdue indicator: it is colored warning
                    // (chanterelle here) when the reminder is overdue.
                    if (dateText.isNotBlank()) {
                        ReminderBadge(
                            text = dateText,
                            leadingIcon = if (overdue) Icons.Outlined.Warning else null,
                            containerColor = if (overdue) MycorrhizalColors.chanterelle else MaterialTheme.colorScheme.surfaceVariant,
                            contentColor = if (overdue) androidx.compose.ui.graphics.Color.White else MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (overdue) {
                        ReminderBadge(
                            text = stringResource(R.string.reminder_overdue),
                            containerColor = MaterialTheme.colorScheme.errorContainer,
                            contentColor = MaterialTheme.colorScheme.onErrorContainer,
                        )
                    }
                    if (reminder.byMail == true) {
                        ReminderBadge(
                            text = stringResource(R.string.reminder_email_chip),
                            leadingIcon = Icons.Outlined.Email,
                            containerColor = MaterialTheme.colorScheme.surfaceVariant,
                            contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    if (reminder.reoccurFromCompletion == true && recurrence != null) {
                        ReminderBadge(
                            text = stringResource(R.string.reminder_flexible),
                            leadingIcon = Icons.Outlined.Repeat,
                            containerColor = MaterialTheme.colorScheme.surfaceVariant,
                            contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
            if (!reminder.completed) {
                IconButton(onClick = onComplete, enabled = !isCompleting) {
                    Icon(Icons.Outlined.Check, contentDescription = stringResource(R.string.cd_complete_reminder))
                }
            }
            IconButton(onClick = onDelete, enabled = !isDeleting) {
                Icon(
                    Icons.Outlined.Delete,
                    contentDescription = stringResource(R.string.cd_delete_reminder),
                    tint = if (isDeleting) MaterialTheme.colorScheme.outline else MaterialTheme.colorScheme.error,
                )
            }
        }
    }
}

/**
 * Matches web's `isOverdue` (ReminderList.tsx): a reminder is overdue only when
 * its due date is **before today**. A reminder due today is not overdue — the
 * off-by-one the M20 test cases pin. The backend normalizes `remind_at` to
 * midnight UTC, so a date-only comparison against today's date is exact.
 */
fun Reminder.isOverdue(now: java.time.LocalDate = java.time.LocalDate.now()): Boolean {
    val raw = remindAt?.takeIf { it.isNotBlank() } ?: return false
    val due = runCatching { java.time.LocalDate.parse(raw.take(10)) }.getOrNull() ?: return false
    return due.isBefore(now)
}

private fun Reminder.recurrenceLabel(): Int = when (recurrence) {
    ReminderRecurrence.ONCE -> R.string.reminder_recurrence_once
    ReminderRecurrence.WEEKLY -> R.string.reminder_recurrence_weekly
    ReminderRecurrence.MONTHLY -> R.string.reminder_recurrence_monthly
    ReminderRecurrence.QUARTERLY -> R.string.reminder_recurrence_quarterly
    ReminderRecurrence.SIX_MONTHS -> R.string.reminder_recurrence_six_months
    ReminderRecurrence.YEARLY -> R.string.reminder_recurrence_yearly
    else -> R.string.reminder_recurrence_once
}

@Composable
private fun ReminderBadge(
    text: String,
    leadingIcon: androidx.compose.ui.graphics.vector.ImageVector? = null,
    containerColor: androidx.compose.ui.graphics.Color,
    contentColor: androidx.compose.ui.graphics.Color,
) {
    androidx.compose.material3.Surface(
        shape = androidx.compose.foundation.shape.RoundedCornerShape(8.dp),
        color = containerColor,
        contentColor = contentColor,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(2.dp),
        ) {
            if (leadingIcon != null) {
                Icon(leadingIcon, contentDescription = null, modifier = Modifier.size(12.dp))
            }
            Text(text, style = MaterialTheme.typography.labelSmall)
        }
    }
}
