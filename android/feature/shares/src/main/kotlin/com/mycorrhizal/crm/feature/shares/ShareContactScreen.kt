package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
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
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * M15: the "Share this contact" flow (mirrors web's ShareContactDialog) — pick
 * a recipient from the user directory, choose which fields to include (the
 * same T9 field-section picker with its sensitivity foot-gun guard), and send
 * a one-time filtered copy. Reached from ContactDetailScreen's menu.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ShareContactScreen(
    onBack: () -> Unit,
    viewModel: ShareContactViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    var recipientExpanded by remember { mutableStateOf(false) }
    var showSensitiveConfirm by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                        )
                    }
                },
                title = {
                    Text(stringResource(R.string.shares_share_dialog_title), style = MaterialTheme.typography.titleLarge)
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
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading && state.recipients.isEmpty() -> LoadingSkeleton()
                // No other users on the instance (empty directory, no error): there is
                // nobody to share with — explain rather than showing an unusable form.
                state.recipients.isEmpty() && state.error == null && !state.isLoading ->
                    EmptyState(message = stringResource(R.string.shares_no_recipients))
                state.recipients.isEmpty() && state.error != null ->
                    EmptyState(message = state.error.orEmpty())
                else -> {
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .verticalScroll(rememberScrollState())
                            .padding(16.dp),
                        verticalArrangement = Arrangement.spacedBy(16.dp),
                    ) {
                        Text(
                            text = stringResource(R.string.shares_share_dialog_description),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )

                        RecipientField(
                            recipients = state.recipients.map { it.username },
                            selectedRecipient = state.selectedRecipientId?.let { selectedId ->
                                state.recipients.firstOrNull { it.id == selectedId }?.username
                            },
                            expanded = recipientExpanded,
                            onExpandedChange = { recipientExpanded = it },
                            onSelect = { username ->
                                viewModel.selectRecipient(state.recipients.firstOrNull { it.username == username }?.id)
                            },
                        )

                        Text(
                            text = stringResource(R.string.shares_fields_label),
                            style = MaterialTheme.typography.titleMedium,
                        )
                        val lockedLabel = stringResource(R.string.a11y_share_field_locked)
                        ShareFieldSections.ALL.forEach { section ->
                            val checked = state.selectedSections.contains(section.token)
                            val locked = section.sensitive && !state.sensitiveRevealed
                            // #199: a bare Checkbox has no text of its own — the
                            // adjacent label Text was a separate, unassociated
                            // node, so TalkBack announced the checkbox with no
                            // name. Modifier.toggleable on the row merges the
                            // label into the checkbox's accessible name. When
                            // locked, `enabled = false` alone only announces
                            // "disabled" with no reason — stateDescription names
                            // the sensitivity lock instead of relying on the
                            // (contentDescription = null) lock icon alone.
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .toggleable(
                                        value = if (locked) false else checked,
                                        onValueChange = { viewModel.toggleSection(section.token, it) },
                                        enabled = !locked,
                                        role = Role.Checkbox,
                                    )
                                    .semantics { if (locked) stateDescription = lockedLabel },
                            ) {
                                Checkbox(
                                    checked = if (locked) false else checked,
                                    onCheckedChange = null,
                                    enabled = !locked,
                                )
                                Text(
                                    text = stringResource(sectionLabelRes(section.token)),
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = if (locked) {
                                        MaterialTheme.colorScheme.onSurfaceVariant
                                    } else {
                                        MaterialTheme.colorScheme.onSurface
                                    },
                                )
                                if (locked) {
                                    Icon(
                                        imageVector = Icons.Outlined.Lock,
                                        contentDescription = null,
                                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                                        modifier = Modifier.padding(start = 4.dp),
                                    )
                                }
                            }
                        }

                        if (!state.sensitiveRevealed) {
                            OutlinedButton(
                                onClick = { showSensitiveConfirm = true },
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                Icon(
                                    imageVector = Icons.Outlined.Lock,
                                    contentDescription = null,
                                )
                                Text(stringResource(R.string.shares_reveal_button), modifier = Modifier.padding(start = 4.dp))
                            }
                        }

                        OutlinedButton(
                            onClick = viewModel::share,
                            enabled = state.canShare,
                            modifier = Modifier.fillMaxWidth(),
                        ) {
                            Text(
                                if (state.isSharing) {
                                    stringResource(R.string.shares_sharing)
                                } else {
                                    stringResource(R.string.shares_share_button)
                                },
                            )
                        }
                    }
                }
            }
        }
    }

    if (showSensitiveConfirm) {
        AlertDialog(
            onDismissRequest = { showSensitiveConfirm = false },
            title = { Text(stringResource(R.string.shares_reveal_title)) },
            text = { Text(stringResource(R.string.shares_reveal_confirm)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        viewModel.revealSensitive()
                        showSensitiveConfirm = false
                    },
                ) {
                    Text(stringResource(R.string.shares_reveal_confirm_button))
                }
            },
            dismissButton = {
                TextButton(onClick = { showSensitiveConfirm = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    val errorMessage = state.error ?: state.errorRes?.let { stringResource(it) }
    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }

    LaunchedEffect(state.shared) {
        if (state.shared) onBack()
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun RecipientField(
    recipients: List<String>,
    selectedRecipient: String?,
    expanded: Boolean,
    onExpandedChange: (Boolean) -> Unit,
    onSelect: (String) -> Unit,
) {
    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = onExpandedChange,
    ) {
        OutlinedTextField(
            value = selectedRecipient.orEmpty(),
            onValueChange = {},
            readOnly = true,
            label = { Text(stringResource(R.string.shares_recipient)) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier.fillMaxWidth().menuAnchor(),
        )
        ExposedDropdownMenu(
            expanded = expanded,
            onDismissRequest = { onExpandedChange(false) },
        ) {
            recipients.forEach { username ->
                DropdownMenuItem(
                    text = { Text(username) },
                    onClick = { onSelect(username); onExpandedChange(false) },
                )
            }
        }
    }
}

/** @StringRes label for a section token (mirrors the web's settings.exportFieldPicker.sections.*). */
@StringRes
fun sectionLabelRes(token: String): Int = when (token) {
    "emails" -> R.string.shares_section_emails
    "phones" -> R.string.shares_section_phones
    "addresses" -> R.string.shares_section_addresses
    "organizations" -> R.string.shares_section_organizations
    "anniversaries" -> R.string.shares_section_anniversaries
    "media" -> R.string.shares_section_media
    "online_services" -> R.string.shares_section_online_services
    "links" -> R.string.shares_section_links
    "notes" -> R.string.shares_section_notes
    "keywords" -> R.string.shares_section_keywords
    "related_to" -> R.string.shares_section_related_to
    "personal_info" -> R.string.shares_section_personal_info
    "speak_to_as" -> R.string.shares_section_speak_to_as
    "members" -> R.string.shares_section_members
    "languages" -> R.string.shares_section_languages
    "custom_fields" -> R.string.shares_section_custom_fields
    else -> R.string.shares_section_emails
}
