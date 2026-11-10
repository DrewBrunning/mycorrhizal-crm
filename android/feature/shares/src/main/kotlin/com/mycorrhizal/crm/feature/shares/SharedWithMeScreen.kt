package com.mycorrhizal.crm.feature.shares

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
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
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.IncomingShareGrant
import com.mycorrhizal.crm.model.network.ShareBlock
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
 * ADR 0042 decisions 7-8: the "Shared with me" surface. A pending row shows who is sharing, the
 * entity type and a short label — and no content; it can be accepted, declined or the sender
 * blocked. An active row opens the read view and can be left. Blocked senders are listed at the
 * bottom so a block can be undone.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SharedWithMeScreen(
    onBack: () -> Unit,
    onOpenGrant: (IncomingShareGrant) -> Unit,
    viewModel: SharedWithMeViewModel = hiltViewModel()
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
                        stringResource(R.string.shares_with_me_title),
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
            isRefreshing = state.isLoading && (state.grants.isNotEmpty() || state.blocks.isNotEmpty()),
            onRefresh = viewModel::load,
            modifier = Modifier.fillMaxSize().padding(padding)
        ) {
            val loadError = state.loadError
            when {
                state.isLoading && state.grants.isEmpty() && state.blocks.isEmpty() ->
                    LoadingSkeleton()

                loadError != null && state.grants.isEmpty() -> EmptyState(
                    title = loadError.localizedMessage(),
                    actionLabel = stringResource(R.string.action_retry),
                    onAction = viewModel::load
                )

                state.grants.isEmpty() && state.blocks.isEmpty() ->
                    EmptyState(title = stringResource(R.string.shares_with_me_empty))

                else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
                    items(state.grants, key = { it.id }) { grant ->
                        IncomingGrantRow(
                            grant = grant,
                            busy = state.busy,
                            onAccept = { viewModel.accept(grant) },
                            onDecline = { viewModel.requestDecline(grant) },
                            onBlock = { viewModel.requestBlock(grant) },
                            onLeave = { viewModel.requestLeave(grant) },
                            onOpen = { onOpenGrant(grant) }
                        )
                    }
                    if (state.blocks.isNotEmpty()) {
                        item(key = "blocked-header") {
                            HorizontalDivider()
                            Text(
                                text = stringResource(R.string.shares_blocked_title),
                                style = MaterialTheme.typography.titleSmall,
                                color = MaterialTheme.colorScheme.primary,
                                modifier = Modifier
                                    .padding(horizontal = 16.dp, vertical = 12.dp)
                                    .semantics { heading() }
                            )
                        }
                        items(state.blocks, key = { "block-${it.userId}" }) { block ->
                            BlockedSenderRow(
                                block = block,
                                busy = state.busy,
                                onUnblock = { viewModel.unblock(block) }
                            )
                        }
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

    state.prompt?.let { prompt -> RecipientPromptDialog(prompt, state.busy, viewModel) }
}

@Composable
private fun RecipientPromptDialog(
    prompt: SharedWithMePrompt,
    busy: Boolean,
    viewModel: SharedWithMeViewModel
) {
    val grant = prompt.grant
    val (title, message, confirm) = when (prompt) {
        is SharedWithMePrompt.Decline -> Triple(
            stringResource(R.string.shares_decline_confirm_title),
            stringResource(R.string.shares_grant_decline_message, grant.label, grant.fromUsername),
            stringResource(R.string.shares_decline)
        )

        is SharedWithMePrompt.Block -> Triple(
            stringResource(R.string.shares_grant_block_title, grant.fromUsername),
            stringResource(R.string.shares_grant_block_message, grant.fromUsername),
            stringResource(R.string.shares_grant_block)
        )

        is SharedWithMePrompt.Leave -> Triple(
            stringResource(R.string.shares_grant_leave_title),
            stringResource(R.string.shares_grant_leave_message, grant.label, grant.fromUsername),
            stringResource(R.string.shares_grant_leave)
        )
    }
    AlertDialog(
        onDismissRequest = viewModel::dismissPrompt,
        title = { Text(title) },
        text = { Text(message) },
        confirmButton = {
            TextButton(
                onClick = viewModel::confirmPrompt,
                enabled = !busy,
                modifier = Modifier.testTag("grant-prompt-confirm")
            ) {
                Text(confirm, color = MaterialTheme.colorScheme.error)
            }
        },
        dismissButton = {
            TextButton(onClick = viewModel::dismissPrompt, enabled = !busy) {
                Text(stringResource(R.string.action_cancel))
            }
        }
    )
}

@Composable
private fun IncomingGrantRow(
    grant: IncomingShareGrant,
    busy: Boolean,
    onAccept: () -> Unit,
    onDecline: () -> Unit,
    onBlock: () -> Unit,
    onLeave: () -> Unit,
    onOpen: () -> Unit
) {
    val pending = grant.status == ShareGrantStatuses.PENDING
    val leaveLabel = stringResource(R.string.shares_grant_leave)
    val blockLabel = stringResource(R.string.shares_grant_block)
    val actions = if (pending) {
        listOf(RowAction(blockLabel, onBlock, destructive = true))
    } else {
        listOf(RowAction(leaveLabel, onLeave, destructive = true))
    }
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp)
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = grant.label.ifBlank { stringResource(entityTypeLabelRes(grant.entityType)) },
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            GrantCaption(stringResource(entityTypeLabelRes(grant.entityType)))
            GrantCaption(stringResource(R.string.shares_from, grant.fromUsername))
            if (pending) {
                GrantCaption(stringResource(R.string.shares_with_me_pending_hint))
            } else {
                expiryText(grant.expiresAt)?.let { GrantCaption(it) }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                if (pending) {
                    TextButton(onClick = onAccept, enabled = !busy) {
                        Text(stringResource(R.string.shares_accept))
                    }
                    TextButton(onClick = onDecline, enabled = !busy) {
                        Text(
                            stringResource(R.string.shares_decline),
                            color = MaterialTheme.colorScheme.error
                        )
                    }
                } else {
                    TextButton(onClick = onOpen) {
                        Text(stringResource(R.string.shares_grant_open))
                    }
                }
            }
        }
        RowOverflowMenu(rowName = grant.label, actions = actions)
    }
}

@Composable
private fun BlockedSenderRow(block: ShareBlock, busy: Boolean, onUnblock: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = block.username,
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.weight(1f)
        )
        TextButton(onClick = onUnblock, enabled = !busy) {
            Text(stringResource(R.string.shares_grant_unblock))
        }
    }
}
