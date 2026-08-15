package com.mycorrhizal.crm.feature.shares

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
import androidx.compose.material.icons.outlined.Menu
import androidx.compose.material.icons.outlined.Share
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
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
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareStatuses
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * M15: the standalone contact-shares screen (mirrors web's ContactSharesPage)
 * — Incoming/Outgoing tabs, per-share status, and for pending incoming shares
 * an Accept (preview-then-confirm) and a confirm-gated Decline.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ContactSharesScreen(
    onMenuClick: () -> Unit,
    viewModel: ContactSharesViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onMenuClick) {
                        Icon(Icons.Outlined.Menu, contentDescription = stringResource(R.string.cd_menu))
                    }
                },
                title = {
                    Text(stringResource(R.string.shares_title), style = MaterialTheme.typography.titleLarge)
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
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            TabRow(selectedTabIndex = if (state.selectedTab == SharesTab.INCOMING) 0 else 1) {
                Tab(
                    selected = state.selectedTab == SharesTab.INCOMING,
                    onClick = { viewModel.selectTab(SharesTab.INCOMING) },
                    text = { Text(stringResource(R.string.shares_tab_incoming)) },
                )
                Tab(
                    selected = state.selectedTab == SharesTab.OUTGOING,
                    onClick = { viewModel.selectTab(SharesTab.OUTGOING) },
                    text = { Text(stringResource(R.string.shares_tab_outgoing)) },
                )
            }
            Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
                val list = if (state.selectedTab == SharesTab.INCOMING) state.incoming else state.outgoing
                when {
                    state.isLoading -> LoadingSkeleton()
                    list.isEmpty() && state.error == null -> EmptyState(
                        message = stringResource(
                            if (state.selectedTab == SharesTab.INCOMING) {
                                R.string.shares_incoming_empty
                            } else {
                                R.string.shares_outgoing_empty
                            },
                        ),
                    )
                    list.isEmpty() && state.error != null -> {
                        Text(
                            text = state.error.orEmpty(),
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.align(Alignment.Center),
                        )
                    }
                    else -> {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(list, key = { it.id }) { share ->
                                val outgoing = state.selectedTab == SharesTab.OUTGOING
                                ShareRow(
                                    share = share,
                                    otherUsername = state.usernames[
                                        (if (outgoing) share.toUserId else share.fromUserId).toString()
                                    ],
                                    outgoing = outgoing,
                                    accepting = state.acceptingShare?.id == share.id,
                                    onAccept = { viewModel.openAccept(share) },
                                    onDecline = { viewModel.requestDecline(share) },
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

    val pendingDeclineId = state.declinePendingId
    val decliningShare = (state.incoming + state.outgoing).firstOrNull { it.id == pendingDeclineId }
    if (decliningShare != null) {
        AlertDialog(
            onDismissRequest = { if (!state.declining) viewModel.cancelDecline() },
            title = { Text(stringResource(R.string.shares_decline_confirm_title)) },
            text = {
                Text(
                    stringResource(
                        R.string.shares_decline_confirm_message,
                        decliningShare.contactDisplayName,
                    ),
                )
            },
            confirmButton = {
                TextButton(
                    onClick = { viewModel.confirmDecline() },
                    enabled = !state.declining,
                    modifier = Modifier.testTag("decline-confirm"),
                ) {
                    Text(stringResource(R.string.shares_decline))
                }
            },
            dismissButton = {
                TextButton(onClick = { viewModel.cancelDecline() }, enabled = !state.declining) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }

    if (state.acceptingShare != null) {
        AcceptContactShareDialog(
            previewLoading = state.previewLoading,
            preview = state.preview,
            previewError = state.previewError,
            confirmAction = state.confirmAction,
            confirming = state.confirming,
            onActionChange = viewModel::setConfirmAction,
            onConfirm = viewModel::confirmAccept,
            onDismiss = viewModel::closeAccept,
        )
    }
}

@Composable
private fun ShareRow(
    share: ContactShare,
    otherUsername: String?,
    outgoing: Boolean,
    accepting: Boolean,
    onAccept: () -> Unit,
    onDecline: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Icon(
            imageVector = Icons.Outlined.Share,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = share.contactDisplayName.ifBlank { share.id },
                style = MaterialTheme.typography.bodyLarge,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = stringResource(
                    if (outgoing) R.string.shares_to else R.string.shares_from,
                    otherUsername ?: (if (outgoing) share.toUserId else share.fromUserId).toString(),
                ),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        StatusLabel(share.status)
        if (!outgoing && share.status == ContactShareStatuses.PENDING) {
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                TextButton(onClick = onAccept, enabled = !accepting) {
                    Text(stringResource(R.string.shares_accept))
                }
                TextButton(onClick = onDecline) {
                    Text(
                        stringResource(R.string.shares_decline),
                        color = MaterialTheme.colorScheme.error,
                    )
                }
            }
        }
    }
}

@Composable
private fun StatusLabel(status: String) {
    val (labelRes, color) = when (status) {
        ContactShareStatuses.ACCEPTED -> R.string.shares_status_accepted to MaterialTheme.colorScheme.tertiary
        ContactShareStatuses.DECLINED -> R.string.shares_status_declined to MaterialTheme.colorScheme.error
        else -> R.string.shares_status_pending to MaterialTheme.colorScheme.onSurfaceVariant
    }
    Text(
        text = stringResource(labelRes),
        style = MaterialTheme.typography.labelMedium,
        color = color,
    )
}
