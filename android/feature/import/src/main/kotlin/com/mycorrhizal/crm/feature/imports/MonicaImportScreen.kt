package com.mycorrhizal.crm.feature.imports

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
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
import com.mycorrhizal.crm.model.network.MonicaConnectResponse
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.components.LoadingSkeleton
import com.mycorrhizal.crm.ui.components.localizedMessage
import kotlin.math.roundToInt

/**
 * Issues #549/#2525: "Import from Monica" — connect to a Monica account over its
 * API, review the result through the shared import review step, confirm. See
 * [MonicaImportViewModel] for the server round-trip.
 */
@Composable
fun MonicaImportScreen(
    onBack: () -> Unit,
    onDone: () -> Unit,
    viewModel: MonicaImportViewModel = hiltViewModel()
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    MonicaImportScreenContent(
        uiState = state,
        // The ViewModel cancels the server session in onCleared, so the app-bar
        // icon, the system back button and gesture all clean up the same way.
        onBack = onBack,
        onDone = onDone,
        onBaseUrlChange = viewModel::onBaseUrlChange,
        onApiTokenChange = viewModel::onApiTokenChange,
        onIncludeRelationshipsChange = viewModel::onIncludeRelationshipsChange,
        onIncludeExtrasChange = viewModel::onIncludeExtrasChange,
        onConnect = viewModel::connect,
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
fun MonicaImportScreenContent(
    uiState: MonicaImportUiState,
    modifier: Modifier = Modifier,
    onBack: () -> Unit = {},
    onDone: () -> Unit = {},
    onBaseUrlChange: (String) -> Unit = {},
    onApiTokenChange: (String) -> Unit = {},
    onIncludeRelationshipsChange: (Boolean) -> Unit = {},
    onIncludeExtrasChange: (Boolean) -> Unit = {},
    onConnect: () -> Unit = {},
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
                        stringResource(R.string.monica_import_title),
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
                MonicaImportStep.WORKING -> Column(modifier = Modifier.fillMaxSize()) {
                    Text(
                        text = stringResource(R.string.monica_import_working),
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.padding(16.dp).testTag("monica-import-working")
                    )
                    LoadingSkeleton()
                }

                MonicaImportStep.CONNECT -> ConnectStep(
                    uiState = uiState,
                    onBaseUrlChange = onBaseUrlChange,
                    onApiTokenChange = onApiTokenChange,
                    onIncludeRelationshipsChange = onIncludeRelationshipsChange,
                    onIncludeExtrasChange = onIncludeExtrasChange,
                    onConnect = onConnect,
                    onStart = onStart
                )

                MonicaImportStep.REVIEW -> uiState.preview?.let { preview ->
                    Column(modifier = Modifier.fillMaxSize()) {
                        if (preview.lossReport.isNotEmpty()) {
                            Text(
                                text = pluralStringResource(
                                    R.plurals.monica_import_loss,
                                    preview.lossReport.size,
                                    preview.lossReport.size
                                ),
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.error,
                                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
                                    .testTag("monica-import-loss")
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

                MonicaImportStep.RESULT -> uiState.result?.let { result ->
                    ImportResultReport(
                        result = result.toImportResult(),
                        onDone = onDone,
                        summary = pluralStringResource(
                            R.plurals.monica_import_result,
                            result.created,
                            result.created,
                            result.updated,
                            result.skipped
                        ),
                        summaryTag = "monica-import-result",
                        doneTag = "monica-import-done"
                    )
                }
            }
        }
    }

    val errorMessage = uiState.errorRes?.let { stringResource(it) }
        ?: uiState.error?.localizedMessage() ?: uiState.errorText
    if (errorMessage != null) {
        LaunchedEffect(errorMessage) {
            snackbarHostState.showSnackbar(errorMessage)
            currentOnErrorShown.value()
        }
    }
}

@Suppress("LongMethod", "LongParameterList") // one linear form: address, token, options, summary
@Composable
private fun ConnectStep(
    uiState: MonicaImportUiState,
    onBaseUrlChange: (String) -> Unit,
    onApiTokenChange: (String) -> Unit,
    onIncludeRelationshipsChange: (Boolean) -> Unit,
    onIncludeExtrasChange: (Boolean) -> Unit,
    onConnect: () -> Unit,
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
                text = stringResource(R.string.monica_import_unavailable),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("monica-import-unavailable")
            )
            return@Column
        }
        Text(
            text = stringResource(R.string.monica_import_description),
            style = MaterialTheme.typography.bodyMedium
        )
        OutlinedTextField(
            value = uiState.baseUrl,
            onValueChange = onBaseUrlChange,
            label = { Text(stringResource(R.string.monica_import_url_label)) },
            placeholder = { Text("https://app.monicahq.com") },
            supportingText = { Text(stringResource(R.string.monica_import_url_help)) },
            singleLine = true,
            enabled = !locked,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            modifier = Modifier.fillMaxWidth().testTag("monica-import-url")
        )
        OutlinedTextField(
            value = uiState.apiToken,
            onValueChange = onApiTokenChange,
            label = { Text(stringResource(R.string.monica_import_token_label)) },
            supportingText = { Text(stringResource(R.string.monica_import_token_help)) },
            singleLine = true,
            enabled = !locked,
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Password,
                autoCorrectEnabled = false
            ),
            visualTransformation = PasswordVisualTransformation(),
            modifier = Modifier.fillMaxWidth().testTag("monica-import-token")
        )
        OptionSwitch(
            label = stringResource(R.string.monica_import_include_relationships),
            checked = uiState.includeRelationships,
            enabled = !locked,
            onCheckedChange = onIncludeRelationshipsChange,
            tag = "monica-import-relationships"
        )
        OptionSwitch(
            label = stringResource(R.string.monica_import_include_extras),
            checked = uiState.includeExtras,
            enabled = !locked,
            onCheckedChange = onIncludeExtrasChange,
            tag = "monica-import-extras"
        )
        if (connected == null) {
            Button(
                onClick = onConnect,
                enabled = uiState.canConnect,
                modifier = Modifier.testTag("monica-import-connect")
            ) {
                Text(
                    stringResource(
                        if (uiState.isConnecting) {
                            R.string.monica_import_connecting
                        } else {
                            R.string.monica_import_connect
                        }
                    )
                )
            }
        } else {
            ConnectedSummary(connected, enabled = uiState.canStart, onStart = onStart)
        }
    }
}

@Composable
private fun OptionSwitch(
    label: String,
    checked: Boolean,
    enabled: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    tag: String
) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
        modifier = Modifier.fillMaxWidth()
    ) {
        Text(label, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
        Switch(
            checked = checked,
            onCheckedChange = onCheckedChange,
            enabled = enabled,
            modifier = Modifier.testTag(tag)
        )
    }
}

@Composable
private fun ConnectedSummary(
    connected: MonicaConnectResponse,
    enabled: Boolean,
    onStart: () -> Unit
) {
    val totals = connected.totals
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(
            text = stringResource(
                R.string.monica_import_found,
                pluralStringResource(
                    R.plurals.monica_import_found_contacts,
                    totals.contacts,
                    totals.contacts
                ),
                pluralStringResource(
                    R.plurals.monica_import_found_activities,
                    totals.activities,
                    totals.activities
                ),
                pluralStringResource(
                    R.plurals.monica_import_found_notes,
                    totals.notes,
                    totals.notes
                )
            ),
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.testTag("monica-import-found")
        )
        Text(
            text = stringResource(
                R.string.monica_import_estimate,
                estimateMinutes(connected.estimatedFetchSeconds)
            ),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.testTag("monica-import-estimate")
        )
        Button(
            onClick = onStart,
            enabled = enabled,
            modifier = Modifier.testTag("monica-import-start")
        ) {
            Text(stringResource(R.string.monica_import_start))
        }
    }
}

/** Whole minutes, rounded, never below one — the same rule as the web dialog. */
internal fun estimateMinutes(seconds: Int): Int = maxOf(1, (seconds / 60f).roundToInt())
