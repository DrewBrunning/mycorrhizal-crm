package com.mycorrhizal.crm.feature.shares

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
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
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.ShareGrantEntityTypes
import com.mycorrhizal.crm.model.network.ShareGrantRevokedReasons
import com.mycorrhizal.crm.model.network.ShareGrantStatuses
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.RefreshableContent
import com.mycorrhizal.crm.ui.components.RowAction
import com.mycorrhizal.crm.ui.components.RowOverflowMenu
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * ADR 0042 decision 12 (owner side): every standing share I have issued with its status, expiry
 * and live circle member count, plus revoke, re-confirm and a narrow-only edit. A blocked
 * recipient reads as `pending` here, exactly as the server reports it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SharedByMeScreen(
    onBack: () -> Unit,
    onOpenContact: (Int) -> Unit = {},
    viewModel: SharedByMeViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.shares_by_me_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        RefreshableContent(
            isRefreshing = state.isLoading && state.grants.isNotEmpty(),
            onRefresh = viewModel::load,
            modifier = Modifier.fillMaxSize().padding(padding)
        ) {
            val loadError = state.loadError
            when {
                state.isLoading && state.grants.isEmpty() -> LoadingSkeleton()

                loadError != null && state.grants.isEmpty() -> EmptyState(
                    title = loadError.localizedMessage(),
                    actionLabel = stringResource(R.string.action_retry),
                    onAction = viewModel::load
                )

                state.grants.isEmpty() ->
                    EmptyState(title = stringResource(R.string.shares_by_me_empty))

                else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
                    items(state.grants, key = { it.id }) { grant ->
                        OutgoingGrantRow(
                            grant = grant,
                            busy = state.busy,
                            onEdit = { viewModel.openEdit(grant) },
                            onRevoke = { viewModel.requestRevoke(grant) },
                            onRemove = { viewModel.requestRemove(grant) },
                            onReconfirm = { viewModel.requestReconfirm(grant) }
                        )
                    }
                }
            }
        }
    }

    state.actionError?.let { error ->
        val message = error.localizedMessage()
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onActionErrorShown()
        }
    }

    state.openContactId?.let { contactId ->
        val openContact by rememberUpdatedState(onOpenContact)
        LaunchedEffect(contactId) {
            viewModel.onContactOpened()
            openContact(contactId)
        }
    }

    state.prompt?.let { prompt ->
        OwnerPromptDialog(
            prompt = prompt,
            busy = state.busy,
            reconfirmCounts = state.reconfirmCounts,
            onDismiss = viewModel::dismissPrompt,
            onConfirm = viewModel::confirmPrompt,
            onOpenContact = { viewModel.openContact(prompt.grant) }
        )
    }
    state.edit?.let { edit ->
        EditGrantDialog(
            edit = edit,
            onToggleSection = viewModel::toggleEditSection,
            onExpiryChange = viewModel::setEditExpiry,
            onSave = viewModel::saveEdit,
            onClose = viewModel::closeEdit
        )
    }
}

@Composable
private fun OwnerPromptDialog(
    prompt: SharedByMePrompt,
    busy: Boolean,
    reconfirmCounts: Map<String, Int>,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit,
    onOpenContact: () -> Unit
) {
    val grant = prompt.grant
    val (title, message, confirm) = when (prompt) {
        is SharedByMePrompt.Remove -> Triple(
            stringResource(R.string.shares_grant_remove_title),
            stringResource(R.string.shares_grant_remove_message, grant.toUsername),
            stringResource(R.string.shares_grant_remove)
        )

        is SharedByMePrompt.Revoke -> Triple(
            stringResource(R.string.shares_grant_revoke_title),
            stringResource(R.string.shares_grant_revoke_message, grant.toUsername),
            stringResource(R.string.shares_grant_revoke)
        )

        is SharedByMePrompt.Reconfirm -> Triple(
            stringResource(R.string.shares_grant_reconfirm_title),
            stringResource(R.string.shares_grant_reconfirm_message, grant.toUsername),
            stringResource(R.string.shares_grant_reconfirm_confirm)
        )
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                Text(message)
                if (prompt is SharedByMePrompt.Reconfirm) {
                    // Issue #2659: what is withheld, with what each section holds today.
                    grant.reconfirmSections.forEach { token ->
                        val name = stringResource(sectionLabelRes(token))
                        val count = reconfirmCounts[token]?.takeIf { it > 0 }
                        Text(
                            text = if (count == null) {
                                name
                            } else {
                                pluralStringResource(
                                    R.plurals.shares_reconfirm_item_count,
                                    count,
                                    name,
                                    count
                                )
                            },
                            style = MaterialTheme.typography.bodyMedium
                        )
                    }
                    if (grant.entityType == ShareGrantEntityTypes.CONTACT) {
                        TextButton(
                            onClick = onOpenContact,
                            enabled = !busy,
                            modifier = Modifier.testTag("grant-prompt-open-contact")
                        ) {
                            Text(stringResource(R.string.shares_open_contact))
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = onConfirm,
                enabled = !busy,
                modifier = Modifier.testTag("grant-prompt-confirm")
            ) {
                Text(confirm)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !busy) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    )
}

@Composable
private fun OutgoingGrantRow(
    grant: OutgoingShareGrant,
    busy: Boolean,
    onEdit: () -> Unit,
    onRevoke: () -> Unit,
    onRemove: () -> Unit,
    onReconfirm: () -> Unit
) {
    val title = grantTitle(grant.label, grant.entityType)
    val needsReconfirm = grant.isLive && grant.reconfirmSections.isNotEmpty()
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = title,
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            GrantCaption(stringResource(entityTypeLabelRes(grant.entityType)))
            GrantStatusChip(
                status = grant.status,
                modifier = Modifier.padding(vertical = 4.dp).testTag("grant-status-${grant.id}")
            )
            // Issue #2627: recipient and expiry share one line. A finished grant has no expiry.
            val to = stringResource(R.string.shares_to, grant.toUsername)
            GrantCaption(
                if (grant.isLive) {
                    stringResource(
                        R.string.shares_grant_meta_join,
                        to,
                        expiryText(grant.expiresAt)
                            ?: stringResource(R.string.shares_grant_no_expiry)
                    )
                } else {
                    to
                }
            )
            if (grant.isLive) {
                sectionsSummary(grant.sections, R.string.shares_sections_shared)
                    ?.let { GrantCaption(it) }
            }
            grant.memberCount?.takeIf { grant.entityType == ShareGrantEntityTypes.CIRCLE }?.let {
                GrantCaption(pluralStringResource(R.plurals.shares_content_members, it, it))
            }
            revokedReasonRes(grant)?.let { GrantCaption(stringResource(it)) }
            if (needsReconfirm) {
                Text(
                    text = stringResource(R.string.shares_grant_needs_reconfirm),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error
                )
                TextButton(onClick = onReconfirm, enabled = !busy) {
                    Text(stringResource(R.string.shares_grant_reconfirm))
                }
            }
        }
        if (grant.isLive) {
            val editLabel = stringResource(R.string.shares_grant_edit)
            val revokeLabel = stringResource(R.string.shares_grant_revoke)
            RowOverflowMenu(
                rowName = title,
                actions = listOf(
                    RowAction(editLabel, onEdit),
                    RowAction(revokeLabel, onRevoke, destructive = true)
                )
            )
        } else if (grant.isEnded) {
            // Issue #2627: an ended grant can only be dismissed from the list.
            RowOverflowMenu(
                rowName = title,
                actions = listOf(
                    RowAction(
                        stringResource(R.string.shares_grant_remove),
                        onRemove,
                        destructive = true
                    )
                )
            )
        }
    }
}

private fun revokedReasonRes(grant: OutgoingShareGrant): Int? =
    if (grant.status != ShareGrantStatuses.REVOKED) {
        null
    } else {
        when (grant.revokedReason) {
            ShareGrantRevokedReasons.OWNER_REVOKED -> R.string.shares_revoked_reason_owner

            ShareGrantRevokedReasons.ENTITY_DELETED -> R.string.shares_revoked_reason_entity_deleted

            ShareGrantRevokedReasons.RECIPIENT_DELETED ->
                R.string.shares_revoked_reason_recipient_deleted

            else -> null
        }
    }

@Composable
private fun EditGrantDialog(
    edit: ShareGrantEditState,
    onToggleSection: (String, Boolean) -> Unit,
    onExpiryChange: (ExpiryChoice) -> Unit,
    onSave: () -> Unit,
    onClose: () -> Unit
) {
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text(stringResource(R.string.shares_edit_title)) },
        text = {
            Column(modifier = Modifier.verticalScroll(rememberScrollState())) {
                Text(
                    text = stringResource(R.string.shares_edit_hint),
                    style = MaterialTheme.typography.bodyMedium
                )
                edit.grant.sections.forEach { token ->
                    val checked = token in edit.sections
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(OptionControlGap),
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(min = MinOptionRowHeight)
                            .toggleable(
                                value = checked,
                                onValueChange = { onToggleSection(token, it) },
                                role = Role.Checkbox
                            )
                    ) {
                        Checkbox(checked = checked, onCheckedChange = null)
                        Text(
                            text = stringResource(sectionLabelRes(token)),
                            style = MaterialTheme.typography.bodyMedium
                        )
                    }
                }
                Text(
                    text = stringResource(R.string.shares_expiry_label),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.padding(top = 8.dp)
                )
                ExpiryOption(
                    label = stringResource(R.string.shares_expiry_keep),
                    selected = edit.expiry == ExpiryChoice.Keep,
                    onSelect = { onExpiryChange(ExpiryChoice.Keep) }
                )
                ExpiryOption(
                    label = stringResource(R.string.shares_expiry_none),
                    selected = edit.expiry == ExpiryChoice.None,
                    onSelect = { onExpiryChange(ExpiryChoice.None) }
                )
                EXPIRY_PRESET_DAYS.forEach { days ->
                    ExpiryOption(
                        label = pluralStringResource(R.plurals.shares_expiry_days, days, days),
                        selected = edit.expiry == ExpiryChoice.Days(days),
                        onSelect = { onExpiryChange(ExpiryChoice.Days(days)) }
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = onSave,
                enabled = !edit.saving,
                modifier = Modifier.testTag("grant-edit-save")
            ) {
                Text(stringResource(R.string.action_save))
            }
        },
        dismissButton = {
            TextButton(onClick = onClose, enabled = !edit.saving) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    )
}

@Composable
internal fun ExpiryOption(label: String, selected: Boolean, onSelect: () -> Unit) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(OptionControlGap),
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = MinOptionRowHeight)
            .selectable(selected = selected, onClick = onSelect, role = Role.RadioButton)
    ) {
        RadioButton(selected = selected, onClick = null)
        Text(text = label, style = MaterialTheme.typography.bodyMedium)
    }
}
