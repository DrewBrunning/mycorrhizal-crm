package com.mycorrhizal.crm.feature.imports

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
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.LoadingSkeleton

/**
 * Issue #2013: "Import from Meerkat" — connect to a running Meerkat over its
 * API, review the result through the shared import review step, confirm. See
 * [MeerkatImportViewModel] for the server round-trip.
 */
@Composable
fun MeerkatImportScreen(
    onBack: () -> Unit,
    onDone: () -> Unit,
    viewModel: MeerkatImportViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    MeerkatImportScreenContent(
        uiState = state,
        // The ViewModel cancels the server session in onCleared, so the app-bar
        // icon, the system back button and gesture all clean up the same way.
        onBack = onBack,
        onDone = onDone,
        onBaseUrlChange = viewModel::onBaseUrlChange,
        onAuthModeChange = viewModel::onAuthModeChange,
        onApiTokenChange = viewModel::onApiTokenChange,
        onUsernameChange = viewModel::onUsernameChange,
        onPasswordChange = viewModel::onPasswordChange,
        onConnect = viewModel::connect,
        onAcceptUntestedChange = viewModel::onAcceptUntestedChange,
        onStart = viewModel::startImport,
        onRowActionChange = viewModel::setRowAction,
        onResolveAll = viewModel::resolveAll,
        onConfirm = viewModel::confirm,
        onErrorShown = viewModel::onErrorShown
    )
}

/** Stateless content, split out so it is testable without a Hilt-backed ViewModel. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MeerkatImportScreenContent(
    uiState: MeerkatImportUiState,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
    onDone: () -> Unit = {},
    onBaseUrlChange: (String) -> Unit = {},
    onAuthModeChange: (MeerkatAuthMode) -> Unit = {},
    onApiTokenChange: (String) -> Unit = {},
    onUsernameChange: (String) -> Unit = {},
    onPasswordChange: (String) -> Unit = {},
    onConnect: () -> Unit = {},
    onAcceptUntestedChange: (Boolean) -> Unit = {},
    onStart: () -> Unit = {},
    onRowActionChange: (Int, String) -> Unit = { _, _ -> },
    onResolveAll: () -> Unit = {},
    onConfirm: () -> Unit = {},
    onErrorShown: () -> Unit = {}
) {
    val snackbarHostState = remember { SnackbarHostState() }
    val currentOnErrorShown = rememberUpdatedState(onErrorShown)

    Scaffold(
        modifier = modifier,
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(
                            Icons.AutoMirrored.Outlined.ArrowBack,
                            contentDescription = stringResource(R.string.cd_back)
                        )
                    }
                },
                title = {
                    Text(
                        stringResource(R.string.meerkat_import_title),
                        style = MaterialTheme.typography.titleLarge
                    )
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary
                )
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
            when (uiState.step) {
                MeerkatImportStep.WORKING -> Column(modifier = Modifier.fillMaxSize()) {
                    Text(
                        text = stringResource(R.string.meerkat_import_working),
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.padding(16.dp).testTag("meerkat-import-working")
                    )
                    LoadingSkeleton()
                }

                MeerkatImportStep.CONNECT -> ConnectStep(
                    uiState = uiState,
                    onBaseUrlChange = onBaseUrlChange,
                    onAuthModeChange = onAuthModeChange,
                    onApiTokenChange = onApiTokenChange,
                    onUsernameChange = onUsernameChange,
                    onPasswordChange = onPasswordChange,
                    onConnect = onConnect,
                    onAcceptUntestedChange = onAcceptUntestedChange,
                    onStart = onStart
                )

                MeerkatImportStep.REVIEW -> uiState.preview?.let { preview ->
                    Column(modifier = Modifier.fillMaxSize()) {
                        if (preview.lossReport.isNotEmpty()) {
                            Text(
                                text = pluralStringResource(
                                    R.plurals.meerkat_import_loss,
                                    preview.lossReport.size,
                                    preview.lossReport.size
                                ),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
                                    .testTag("meerkat-import-loss")
                            )
                        }
                        Box(modifier = Modifier.weight(1f)) {
                            ImportReviewStep(
                                rows = preview.rows,
                                rowActions = uiState.rowActions,
                                onRowActionChange = onRowActionChange,
                                onResolveAll = onResolveAll,
                                onConfirm = onConfirm
                            )
                        }
                    }
                }

                MeerkatImportStep.RESULT -> uiState.result?.let { result ->
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(16.dp),
                        modifier = Modifier.fillMaxSize().verticalScroll(
                            rememberScrollState()
                        ).padding(24.dp)
                    ) {
                        Text(
                            text = pluralStringResource(
                                R.plurals.meerkat_import_result,
                                result.created,
                                result.created,
                                result.updated,
                                result.skipped
                            ),
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.padding(top = 48.dp)
                                .testTag("meerkat-import-result")
                        )
                        Button(
                            onClick = onDone,
                            modifier = Modifier.testTag("meerkat-import-done")
                        ) {
                            Text(stringResource(R.string.action_confirm))
                        }
                    }
                }
            }
        }
    }

    val errorMessage = uiState.errorRes?.let { stringResource(it) } ?: uiState.error
    if (errorMessage != null) {
        LaunchedEffect(errorMessage) {
            snackbarHostState.showSnackbar(errorMessage)
            currentOnErrorShown.value()
        }
    }
}

@Suppress("LongMethod", "LongParameterList") // one linear form: address, credential, summary
@Composable
private fun ConnectStep(
    uiState: MeerkatImportUiState,
    onBaseUrlChange: (String) -> Unit,
    onAuthModeChange: (MeerkatAuthMode) -> Unit,
    onApiTokenChange: (String) -> Unit,
    onUsernameChange: (String) -> Unit,
    onPasswordChange: (String) -> Unit,
    onConnect: () -> Unit,
    onAcceptUntestedChange: (Boolean) -> Unit,
    onStart: () -> Unit
) {
    val connected = uiState.connected
    val locked = connected != null
    Column(
        verticalArrangement = Arrangement.spacedBy(12.dp),
        modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp)
    ) {
        if (uiState.unavailable) {
            Text(
                text = stringResource(R.string.meerkat_import_unavailable),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("meerkat-import-unavailable")
            )
            return@Column
        }
        Text(
            text = stringResource(R.string.meerkat_import_description),
            style = MaterialTheme.typography.bodyMedium
        )
        OutlinedTextField(
            value = uiState.baseUrl,
            onValueChange = onBaseUrlChange,
            label = { Text(stringResource(R.string.meerkat_import_url_label)) },
            supportingText = { Text(stringResource(R.string.meerkat_import_url_help)) },
            singleLine = true,
            enabled = !locked,
            keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
                keyboardType = KeyboardType.Uri
            ),
            modifier = Modifier.fillMaxWidth().testTag("meerkat-import-url")
        )
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChip(
                selected = uiState.authMode == MeerkatAuthMode.TOKEN,
                onClick = { onAuthModeChange(MeerkatAuthMode.TOKEN) },
                enabled = !locked,
                label = { Text(stringResource(R.string.meerkat_import_mode_token)) },
                modifier = Modifier.testTag("meerkat-import-mode-token")
            )
            FilterChip(
                selected = uiState.authMode == MeerkatAuthMode.PASSWORD,
                onClick = { onAuthModeChange(MeerkatAuthMode.PASSWORD) },
                enabled = !locked,
                label = { Text(stringResource(R.string.meerkat_import_mode_password)) },
                modifier = Modifier.testTag("meerkat-import-mode-password")
            )
        }
        if (uiState.authMode == MeerkatAuthMode.TOKEN) {
            OutlinedTextField(
                value = uiState.apiToken,
                onValueChange = onApiTokenChange,
                label = { Text(stringResource(R.string.meerkat_import_token_label)) },
                supportingText = { Text(stringResource(R.string.meerkat_import_token_help)) },
                singleLine = true,
                enabled = !locked,
                visualTransformation = PasswordVisualTransformation(),
                modifier = Modifier.fillMaxWidth().testTag("meerkat-import-token")
            )
        } else {
            OutlinedTextField(
                value = uiState.username,
                onValueChange = onUsernameChange,
                label = { Text(stringResource(R.string.meerkat_import_username_label)) },
                singleLine = true,
                enabled = !locked,
                modifier = Modifier.fillMaxWidth().testTag("meerkat-import-username")
            )
            OutlinedTextField(
                value = uiState.password,
                onValueChange = onPasswordChange,
                label = { Text(stringResource(R.string.meerkat_import_password_label)) },
                supportingText = { Text(stringResource(R.string.meerkat_import_password_help)) },
                singleLine = true,
                enabled = !locked,
                visualTransformation = PasswordVisualTransformation(),
                modifier = Modifier.fillMaxWidth().testTag("meerkat-import-password")
            )
        }
        if (connected == null) {
            Button(
                onClick = onConnect,
                enabled = uiState.canConnect,
                modifier = Modifier.testTag("meerkat-import-connect")
            ) {
                Text(
                    stringResource(
                        if (uiState.isConnecting) {
                            R.string.meerkat_import_connecting
                        } else {
                            R.string.meerkat_import_connect
                        }
                    )
                )
            }
        } else {
            ConnectedSummary(uiState, connected.totals.contacts, onAcceptUntestedChange, onStart)
        }
    }
}

@Composable
private fun ConnectedSummary(
    uiState: MeerkatImportUiState,
    contacts: Int,
    onAcceptUntestedChange: (Boolean) -> Unit,
    onStart: () -> Unit
) {
    val connected = uiState.connected ?: return
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        if (connected.insecureTransport) {
            Text(
                text = stringResource(R.string.meerkat_import_insecure),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("meerkat-import-insecure")
            )
        }
        if (!connected.version.supported) {
            Text(
                text = stringResource(R.string.meerkat_import_untested, connected.version.reason),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("meerkat-import-untested")
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                Checkbox(
                    checked = uiState.acceptUntested,
                    onCheckedChange = onAcceptUntestedChange,
                    modifier = Modifier.testTag("meerkat-import-try-anyway")
                )
                Text(stringResource(R.string.meerkat_import_try_anyway))
            }
        }
        Text(
            text = pluralStringResource(R.plurals.meerkat_import_found, contacts, contacts),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.testTag("meerkat-import-found")
        )
        Button(
            onClick = onStart,
            enabled = uiState.canStart,
            modifier = Modifier.testTag("meerkat-import-start")
        ) {
            Text(stringResource(R.string.meerkat_import_start))
        }
    }
}
