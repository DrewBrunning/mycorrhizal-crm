package com.mycorrhizal.crm.feature.shares

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.model.network.ShareGrantEntityTypes
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton
import com.mycorrhizal.crm.ui.components.EmptyState
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.localizedMessage

/**
 * Issue #2518 (ADR 0042 decision 12): start a standing share of a note, an activity or a circle
 * (mirrors web's `StandingShareDialog` for those types): pick a recipient, optionally an expiry,
 * and the grant starts pending until they accept. Contacts use [ShareContactScreen] instead.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun StandingShareScreen(
    onBack: () -> Unit,
    // A new grant lands on "Shared by me", where its pending status lives.
    onShared: () -> Unit = onBack,
    viewModel: StandingShareViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val snackbarHostState = remember { SnackbarHostState() }
    val currentOnShared = rememberUpdatedState(onShared)
    var recipientExpanded by remember { mutableStateOf(false) }

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
                        stringResource(standingShareTitleRes(state.entityType)),
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
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when {
                state.isLoading && state.recipients.isEmpty() -> LoadingSkeleton()

                state.recipients.isEmpty() && state.error == null && !state.isLoading ->
                    EmptyState(title = stringResource(R.string.shares_no_recipients))

                state.recipients.isEmpty() && state.error != null ->
                    EmptyState(title = state.error?.localizedMessage().orEmpty())

                else -> Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .verticalScroll(rememberScrollState())
                        .padding(16.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp)
                ) {
                    Text(
                        text = stringResource(standingShareNoticeRes(state.entityType)),
                        style = MaterialTheme.typography.bodyMedium
                    )
                    Text(
                        text = stringResource(R.string.shares_standing_pending_note),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    RecipientField(
                        recipients = state.recipients.map { it.username },
                        selectedRecipient = state.selectedRecipientId?.let { selectedId ->
                            state.recipients.firstOrNull { it.id == selectedId }?.username
                        },
                        expanded = recipientExpanded,
                        onExpandedChange = { recipientExpanded = it },
                        onSelect = { username ->
                            viewModel.selectRecipient(
                                state.recipients.firstOrNull { it.username == username }?.id
                            )
                        }
                    )
                    StandingExpiryChooser(
                        expiryDays = state.expiryDays,
                        onChange = viewModel::setExpiryDays
                    )
                    OutlinedButton(
                        onClick = viewModel::share,
                        enabled = state.canShare,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            stringResource(
                                if (state.isSharing) {
                                    R.string.shares_standing_sharing
                                } else {
                                    R.string.shares_standing_share_button
                                }
                            )
                        )
                    }
                }
            }
        }
    }

    val errorMessage = state.error?.localizedMessage() ?: state.errorRes?.let { stringResource(it) }
    errorMessage?.let { message ->
        LaunchedEffect(message) {
            snackbarHostState.showSnackbar(message)
            viewModel.onErrorShown()
        }
    }

    LaunchedEffect(state.shared) {
        if (state.shared) currentOnShared.value()
    }
}

/** The dialog title per type (web `standingShares.dialogTitle.*`); also the entry-point label. */
@StringRes
internal fun standingShareTitleRes(entityType: String): Int = when (entityType) {
    ShareGrantEntityTypes.ACTIVITY -> R.string.shares_standing_title_activity
    ShareGrantEntityTypes.CIRCLE -> R.string.shares_standing_title_circle
    else -> R.string.shares_standing_title_note
}

/** What the recipient will see, per type (web `standingShares.form.entityNotice.*`). */
@StringRes
internal fun standingShareNoticeRes(entityType: String): Int = when (entityType) {
    ShareGrantEntityTypes.ACTIVITY -> R.string.shares_standing_notice_activity
    ShareGrantEntityTypes.CIRCLE -> R.string.shares_standing_notice_circle
    else -> R.string.shares_standing_notice_note
}
