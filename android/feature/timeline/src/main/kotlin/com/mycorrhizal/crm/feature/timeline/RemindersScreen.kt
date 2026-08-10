package com.mycorrhizal.crm.feature.timeline

import androidx.compose.foundation.clickable
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
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.R

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RemindersScreen(
    onBack: () -> Unit,
    onCreateReminder: () -> Unit,
    onEditReminder: (Int) -> Unit,
    viewModel: RemindersViewModel = viewModel(),
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
                title = {
                    Text(stringResource(R.string.reminders_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
        floatingActionButton = {
            FloatingActionButton(onClick = onCreateReminder) {
                Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.cd_new_reminder))
            }
        },
        snackbarHost = { SnackbarHost(snackbarHostState) },
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading -> LoadingSkeleton()
                state.reminders.isEmpty() && state.error == null ->
                    EmptyState(message = "No reminders yet")
                state.reminders.isEmpty() && (state.errorRes != null || state.error != null) ->
                    EmptyState(state.errorRes?.let { stringResource(it) } ?: state.error.orEmpty())
                else -> {
                    LazyColumn(modifier = Modifier.fillMaxSize()) {
                        items(state.reminders, key = { it.id }) { reminder ->
                            ReminderListItem(
                                reminder = reminder,
                                onClick = { onEditReminder(reminder.id) },
                                onComplete = { viewModel.complete(reminder.id) },
                                isCompleting = state.completingId == reminder.id,
                            )
                        }
                    }
                }
            }
        }
    }

    // When the list is empty the error text is the persistent body content
    // (EmptyState above), so don't toast-and-clear it into a misleading
    // "No reminders yet". Only surface a snackbar for errors over a populated list.
    val listError = state.error
    if (listError != null && state.reminders.isNotEmpty()) {
        LaunchedEffect(listError) {
            snackbarHostState.showSnackbar(listError)
            viewModel.onErrorShown()
        }
    }
}

@Composable
fun ReminderListItem(
    reminder: Reminder,
    onClick: () -> Unit,
    onComplete: () -> Unit,
    isCompleting: Boolean,
    modifier: Modifier = Modifier,
) {
    androidx.compose.material3.ListItem(
        headlineContent = {
            Column {
                Text(
                    text = reminder.message.orEmpty(),
                    style = MaterialTheme.typography.bodyLarge,
                    maxLines = 2,
                    overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis,
                )
                val recurrence = reminder.recurrence?.takeIf { it.isNotBlank() }
                if (recurrence != null) {
                    Text(
                        text = recurrence,
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        },
        trailingContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (!reminder.completed) {
                    IconButton(onClick = onComplete, enabled = !isCompleting) {
                        Icon(Icons.Outlined.Check, contentDescription = stringResource(R.string.cd_complete_reminder))
                    }
                }
            }
        },
        modifier = modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
    )
}
