package com.mycorrhizal.crm.feature.occasions

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeView
import com.mycorrhizal.crm.model.registry.OccasionRsvp
import com.mycorrhizal.crm.ui.R

/**
 * One event's invitee/RSVP ledger (docs/adrs/0026-occasions-events.md, issue
 * #1228). The RSVP recorded here is what the user reports the contact told
 * them — no invitation is sent, and the hint says so.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OccasionAttendeesScreen(
    onBack: () -> Unit,
    viewModel: OccasionAttendeesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var pendingRemove by remember { mutableStateOf<OccasionEventAttendeeView?>(null) }

    state.error?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(
                        text = if (state.eventTitle.isBlank()) {
                            stringResource(R.string.occasions_attendees)
                        } else {
                            stringResource(R.string.occasions_attendees_title, state.eventTitle)
                        },
                        style = MaterialTheme.typography.titleLarge,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                },
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
        Box(Modifier.fillMaxSize().padding(padding)) {
            if (state.errorRes != null) {
                CenteredMessage(stringResource(state.errorRes!!))
            } else {
                Column(
                    modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    Text(
                        stringResource(R.string.occasions_rsvp_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )

                    if (state.isLoading && state.attendees.isEmpty()) {
                        Text(stringResource(R.string.a11y_state_loading))
                    } else if (state.attendees.isEmpty()) {
                        Text(stringResource(R.string.occasions_no_attendees), color = MaterialTheme.colorScheme.onSurfaceVariant)
                    } else {
                        state.attendees.forEach { attendee ->
                            AttendeeRow(
                                attendee = attendee,
                                enabled = !state.isMutating,
                                onRsvp = { rsvp -> viewModel.updateRsvp(attendee.entityId, rsvp) },
                                onRemove = { pendingRemove = attendee },
                            )
                        }
                    }

                    HorizontalDivider()

                    Text(stringResource(R.string.occasions_suggest_from_circles), style = MaterialTheme.typography.titleSmall)
                    if (state.circles.isEmpty()) {
                        Text(stringResource(R.string.occasions_no_circles), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    } else {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            state.circles.forEach { circle ->
                                FilterChip(
                                    selected = circle.id in state.selectedCircleIds,
                                    onClick = { viewModel.toggleCircle(circle.id) },
                                    label = { Text(circle.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                                )
                            }
                        }
                        TextButton(
                            enabled = state.selectedCircleIds.isNotEmpty() && !state.suggestionsLoading,
                            onClick = { viewModel.suggest() },
                        ) { Text(stringResource(R.string.occasions_suggest)) }
                    }

                    state.suggestions.forEach { suggestion ->
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Text(suggestion.contactName, modifier = Modifier.weight(1f))
                            TextButton(onClick = { viewModel.addAttendee(suggestion.entityId) }) {
                                Text(stringResource(R.string.action_add))
                            }
                        }
                    }
                    if (state.selectedCircleIds.isNotEmpty() && state.suggestions.isEmpty() && !state.suggestionsLoading) {
                        Text(stringResource(R.string.occasions_no_suggestions), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }

                    HorizontalDivider()

                    OutlinedTextField(
                        value = state.contactQuery,
                        onValueChange = viewModel::onContactQueryChange,
                        label = { Text(stringResource(R.string.occasions_search_contacts)) },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    state.contactResults.forEach { contact ->
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween,
                        ) {
                            Text(contact.displayName, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                            val uid = contact.uid
                            if (uid != null) {
                                IconButton(onClick = { viewModel.addAttendee(uid) }) {
                                    Icon(Icons.Outlined.Add, contentDescription = stringResource(R.string.occasions_add_attendee))
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    pendingRemove?.let { attendee ->
        AlertDialog(
            onDismissRequest = { pendingRemove = null },
            title = { Text(stringResource(R.string.occasions_remove_attendee)) },
            text = { Text(stringResource(R.string.occasions_remove_confirm, attendee.contactName)) },
            confirmButton = {
                TextButton(
                    enabled = !state.isMutating,
                    onClick = { viewModel.removeAttendee(attendee.entityId); pendingRemove = null },
                ) { Text(stringResource(R.string.action_delete)) }
            },
            dismissButton = { TextButton(onClick = { pendingRemove = null }) { Text(stringResource(R.string.action_cancel)) } },
        )
    }
}

@Composable
private fun AttendeeRow(
    attendee: OccasionEventAttendeeView,
    enabled: Boolean,
    onRsvp: (String) -> Unit,
    onRemove: () -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(attendee.contactName, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
        Box {
            OutlinedButton(enabled = enabled, onClick = { expanded = true }) {
                Text(rsvpLabel(attendee.rsvp), maxLines = 1)
            }
            DropdownMenu(expanded = expanded, onDismissRequest = { expanded = false }) {
                OccasionRsvp.ALL.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(rsvpLabel(option)) },
                        onClick = { expanded = false; onRsvp(option) },
                    )
                }
            }
        }
        IconButton(enabled = enabled, onClick = onRemove) {
            Icon(Icons.Outlined.Delete, contentDescription = stringResource(R.string.occasions_remove_attendee))
        }
    }
}

@Composable
private fun rsvpLabel(value: String): String = when (value) {
    OccasionRsvp.ACCEPTED -> stringResource(R.string.occasions_rsvp_accepted)
    OccasionRsvp.DECLINED -> stringResource(R.string.occasions_rsvp_declined)
    OccasionRsvp.MAYBE -> stringResource(R.string.occasions_rsvp_maybe)
    else -> stringResource(R.string.occasions_rsvp_pending)
}
