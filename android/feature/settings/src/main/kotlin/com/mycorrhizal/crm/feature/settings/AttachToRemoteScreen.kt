package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Row
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.data.attach.AttachProgress
import com.mycorrhizal.crm.data.attach.AttachStage
import com.mycorrhizal.crm.domain.profile.ServerProfile
import com.mycorrhizal.crm.feature.imports.ImportReviewStep
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.AccessibleIconButton

/**
 * ADR 0028 Decision 3 / issue #1265: "Move this data to a server". Reached from
 * Settings → Servers on the Local profile. Steps: sign in to a Remote profile
 * (without switching), export + upload + review with per-contact add/skip/merge
 * (the shared import review UI), confirm, then the app switches to the Remote
 * profile and the Local one becomes a read-only archive.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AttachToRemoteScreen(
    onBack: () -> Unit,
    viewModel: AttachToRemoteViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val leave = {
        // Before the switch, leaving abandons the run (drops the remote session
        // and the in-memory bundle); after Done there is nothing to drop.
        if (state.step != AttachStep.Done) viewModel.cancel()
        onBack()
    }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    AccessibleIconButton(onClick = leave) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back),
                        )
                    }
                },
                title = { Text(stringResource(R.string.attach_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        Column(modifier = Modifier.fillMaxSize().padding(padding)) {
            val errorText = state.errorRes?.let { stringResource(it) } ?: state.error
            if (errorText != null) {
                Text(
                    text = errorText,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier
                        .padding(horizontal = 16.dp, vertical = 8.dp)
                        .testTag("attach-error")
                        .semantics { liveRegion = LiveRegionMode.Assertive },
                )
            }
            when (state.step) {
                AttachStep.Loading -> Working(progress = null)
                AttachStep.Blocked -> BlockedStep(onBack = leave)
                AttachStep.SignIn -> SignInStep(
                    remoteProfiles = state.remoteProfiles,
                    onSignIn = viewModel::signIn,
                )
                AttachStep.TwoFactor -> TwoFactorStep(onSubmit = viewModel::submitTwoFactor)
                AttachStep.Working -> Working(progress = state.progress)
                AttachStep.PrepareFailed -> PrepareFailedStep(onRetry = viewModel::retryPrepare)
                AttachStep.Review -> ReviewStep(state = state, viewModel = viewModel)
                AttachStep.Done -> DoneStep(state = state, onDone = onBack)
            }
        }
    }

    state.pendingDiscardCount?.let { count ->
        AlertDialog(
            onDismissRequest = viewModel::dismissDiscard,
            title = { Text(stringResource(R.string.attach_discard_title)) },
            text = { Text(stringResource(R.string.attach_discard_body, count)) },
            confirmButton = {
                TextButton(onClick = viewModel::confirmDiscardAndFinish) {
                    Text(stringResource(R.string.attach_discard_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = viewModel::dismissDiscard) { Text(stringResource(R.string.settings_cancel)) }
            },
        )
    }
}

@Composable
private fun BlockedStep(onBack: () -> Unit) {
    // The reason is already announced by the error banner above; repeating it here
    // would have a screen reader read it twice.
    Column(Modifier.padding(16.dp)) {
        TextButton(onClick = onBack) { Text(stringResource(R.string.cd_back)) }
    }
}

@Composable
private fun SignInStep(
    remoteProfiles: List<ServerProfile>,
    onSignIn: (existingProfileId: String?, label: String, url: String, identifier: String, password: String) -> Unit,
) {
    var selected by remember { mutableStateOf(remoteProfiles.firstOrNull()?.id) }
    var label by remember { mutableStateOf("") }
    var url by remember { mutableStateOf("") }
    var identifier by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }

    Column(
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(stringResource(R.string.attach_intro), style = MaterialTheme.typography.bodyMedium)
        Text(stringResource(R.string.attach_server_heading), style = MaterialTheme.typography.titleMedium)
        Column(Modifier.selectableGroup()) {
            remoteProfiles.forEach { profile ->
                ServerChoice(
                    title = profile.label,
                    subtitle = profile.remoteUrl.orEmpty(),
                    selected = selected == profile.id,
                    tag = "attach-server-${profile.id}",
                    onClick = { selected = profile.id },
                )
            }
            ServerChoice(
                title = stringResource(R.string.attach_server_new),
                subtitle = null,
                selected = selected == null,
                tag = "attach-server-new",
                onClick = { selected = null },
            )
        }
        if (selected == null) {
            OutlinedTextField(
                value = label,
                onValueChange = { label = it },
                label = { Text(stringResource(R.string.settings_servers_label)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("attach-label"),
            )
            OutlinedTextField(
                value = url,
                onValueChange = { url = it },
                label = { Text(stringResource(R.string.settings_servers_url)) },
                placeholder = { Text(stringResource(R.string.login_server_url_hint)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().testTag("attach-url"),
            )
        }
        OutlinedTextField(
            value = identifier,
            onValueChange = { identifier = it },
            label = { Text(stringResource(R.string.attach_identifier)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().testTag("attach-identifier"),
        )
        OutlinedTextField(
            value = password,
            onValueChange = { password = it },
            label = { Text(stringResource(R.string.attach_password)) },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            modifier = Modifier.fillMaxWidth().testTag("attach-password"),
        )
        Button(
            onClick = { onSignIn(selected, label, url, identifier, password) },
            modifier = Modifier.fillMaxWidth().testTag("attach-sign-in"),
        ) {
            Text(stringResource(R.string.attach_sign_in))
        }
    }
}

@Composable
private fun ServerChoice(
    title: String,
    subtitle: String?,
    selected: Boolean,
    tag: String,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, onClick = onClick, role = Role.RadioButton)
            .padding(vertical = 8.dp)
            .testTag(tag),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        RadioButton(selected = selected, onClick = null)
        Column {
            Text(title, style = MaterialTheme.typography.bodyLarge)
            if (!subtitle.isNullOrBlank()) {
                Text(
                    subtitle,
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun TwoFactorStep(onSubmit: (String) -> Unit) {
    var code by remember { mutableStateOf("") }
    Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.attach_two_factor_prompt), style = MaterialTheme.typography.bodyLarge)
        OutlinedTextField(
            value = code,
            onValueChange = { code = it },
            label = { Text(stringResource(R.string.attach_two_factor_code)) },
            singleLine = true,
            modifier = Modifier.fillMaxWidth().testTag("attach-2fa-code"),
        )
        Button(
            onClick = { onSubmit(code) },
            enabled = code.isNotBlank(),
            modifier = Modifier.fillMaxWidth().testTag("attach-2fa-submit"),
        ) {
            Text(stringResource(R.string.attach_two_factor_verify))
        }
    }
}

@Composable
private fun Working(progress: AttachProgress?) {
    Box(Modifier.fillMaxSize().testTag("attach-working"), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
            CircularProgressIndicator()
            if (progress != null) {
                Text(
                    text = stringResource(progress.stage.labelRes()),
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
                if (progress.total > 0) {
                    Text(
                        stringResource(R.string.attach_progress, progress.done, progress.total),
                        style = MaterialTheme.typography.labelMedium,
                    )
                }
            }
        }
    }
}

private fun AttachStage.labelRes(): Int = when (this) {
    AttachStage.Exporting -> R.string.attach_stage_exporting
    AttachStage.Uploading -> R.string.attach_stage_uploading
    AttachStage.Preparing -> R.string.attach_stage_preparing
    AttachStage.Importing -> R.string.attach_stage_importing
}

@Composable
private fun PrepareFailedStep(onRetry: () -> Unit) {
    Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(stringResource(R.string.attach_prepare_failed), style = MaterialTheme.typography.bodyLarge)
        Button(onClick = onRetry, modifier = Modifier.fillMaxWidth().testTag("attach-retry")) {
            Text(stringResource(R.string.attach_retry))
        }
    }
}

@Composable
private fun ReviewStep(state: AttachUiState, viewModel: AttachToRemoteViewModel) {
    val attachPreview = state.preview ?: return
    val totals = attachPreview.totals
    Column(Modifier.fillMaxSize()) {
        Text(
            text = stringResource(
                R.string.attach_review_totals,
                totals.contacts,
                totals.notes,
                totals.activities,
            ),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
        )
        val lossCount = attachPreview.preview.lossReport.size
        if (lossCount > 0) {
            Text(
                text = stringResource(R.string.attach_review_loss, lossCount),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp).testTag("attach-loss"),
            )
        }
        Box(Modifier.weight(1f)) {
            ImportReviewStep(
                rows = attachPreview.preview.rows,
                rowActions = state.rowActions,
                onRowActionChange = viewModel::setRowAction,
                onResolveAll = viewModel::resolveAll,
                onConfirm = viewModel::confirm,
            )
        }
    }
}

@Composable
private fun DoneStep(state: AttachUiState, onDone: () -> Unit) {
    val result = state.result
    Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(
            stringResource(R.string.attach_done_title),
            style = MaterialTheme.typography.titleLarge,
            modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
        )
        if (result != null) {
            Text(
                stringResource(R.string.attach_done_result, result.created, result.updated, result.skipped),
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.testTag("attach-result"),
            )
        }
        Text(stringResource(R.string.attach_done_archive_note), style = MaterialTheme.typography.bodyMedium)
        Button(onClick = onDone, modifier = Modifier.fillMaxWidth().testTag("attach-done")) {
            Text(stringResource(R.string.attach_done_button))
        }
    }
}
