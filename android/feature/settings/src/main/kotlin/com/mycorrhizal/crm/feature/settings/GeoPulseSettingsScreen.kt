package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material.icons.outlined.PlayArrow
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R

/**
 * Issue #160 (ADR 0033): the GeoPulse connection settings screen — base URL, write-only API token,
 * save / test connection / disconnect. Mirrors web's `GeoPulseSettings.tsx` incl. the token
 * re-entry requirement on first connect and on a base-URL origin change (#1501).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GeoPulseSettingsScreen(
    onBack: () -> Unit,
    viewModel: GeoPulseSettingsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    var showRemoveConfirm by remember { mutableStateOf(false) }

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = { Text(stringResource(R.string.settings_geopulse_title), style = MaterialTheme.typography.titleLarge) },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        GeoPulseSettingsContent(
            state = state,
            onBaseUrlChange = viewModel::onBaseUrlChange,
            onApiKeyChange = viewModel::onApiKeyChange,
            onTest = viewModel::test,
            onSave = viewModel::save,
            onRemove = { showRemoveConfirm = true },
            modifier = Modifier.padding(padding),
        )
    }

    if (showRemoveConfirm) {
        AlertDialog(
            onDismissRequest = { showRemoveConfirm = false },
            title = { Text(stringResource(R.string.geopulse_settings_remove_title)) },
            text = { Text(stringResource(R.string.geopulse_settings_remove_confirm)) },
            confirmButton = {
                TextButton(
                    enabled = !state.isRemoving,
                    onClick = {
                        showRemoveConfirm = false
                        viewModel.remove()
                    },
                ) { Text(stringResource(R.string.geopulse_settings_remove), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                TextButton(onClick = { showRemoveConfirm = false }) { Text(stringResource(R.string.action_cancel)) }
            },
        )
    }
}

/** Stateless body for [GeoPulseSettingsScreen] — testable directly. */
@Composable
fun GeoPulseSettingsContent(
    state: GeoPulseSettingsUiState,
    onBaseUrlChange: (String) -> Unit = {},
    onApiKeyChange: (String) -> Unit = {},
    onTest: () -> Unit = {},
    onSave: () -> Unit = {},
    onRemove: () -> Unit = {},
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        if (state.isLoading) {
            Row(horizontalArrangement = Arrangement.Center, modifier = Modifier.fillMaxWidth()) {
                CircularProgressIndicator()
            }
            return@Column
        }

        Text(
            text = stringResource(R.string.geopulse_settings_description),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        val errors = listOfNotNull(
            state.loadError,
            state.saveError,
            state.saveErrorRes?.let { stringResource(it) },
        )
        errors.forEach {
            Text(
                it,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
            )
        }

        OutlinedTextField(
            value = state.baseUrl,
            onValueChange = onBaseUrlChange,
            label = { Text(stringResource(R.string.geopulse_settings_base_url)) },
            placeholder = { Text("http://geopulse:8080") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = state.apiKey,
            onValueChange = onApiKeyChange,
            label = {
                // Required marker: first connect or origin change (mirrors MUI's "API Token *").
                Text(
                    stringResource(R.string.geopulse_settings_api_key) + if (state.tokenRequired) " *" else "",
                )
            },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
            supportingText = {
                Text(
                    when {
                        state.tokenRequiredByOriginChange ->
                            stringResource(R.string.geopulse_settings_api_key_required_origin_change)
                        state.hasApiKey -> stringResource(R.string.geopulse_settings_api_key_hint_existing)
                        else -> stringResource(R.string.geopulse_settings_api_key_hint_new)
                    },
                )
            },
            modifier = Modifier.fillMaxWidth(),
        )

        if (state.hasApiKey) {
            Button(
                onClick = onTest,
                enabled = !state.isTesting,
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.isTesting) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
                } else {
                    Icon(Icons.Outlined.PlayArrow, contentDescription = null)
                }
                Text(
                    stringResource(if (state.isTesting) R.string.geopulse_settings_testing else R.string.geopulse_settings_test),
                    modifier = Modifier.padding(start = 8.dp),
                )
            }
        }

        state.testResult?.let { result ->
            val text = if (result.ok) {
                stringResource(R.string.geopulse_settings_test_success)
            } else {
                stringResource(
                    R.string.geopulse_settings_test_failed,
                    result.stage ?: "-",
                    result.message ?: stringResource(R.string.geopulse_settings_test_unknown),
                )
            }
            Text(
                text = text,
                color = if (result.ok) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
            )
        }

        val savingLabel = stringResource(R.string.a11y_state_saving)
        Button(
            onClick = onSave,
            enabled = !state.isSaving && state.baseUrl.isNotBlank(),
            modifier = Modifier
                .fillMaxWidth()
                .semantics { if (state.isSaving) stateDescription = savingLabel },
        ) {
            if (state.isSaving) {
                CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
            }
            Text(stringResource(R.string.action_save))
        }

        if (state.hasApiKey) {
            OutlinedButton(
                onClick = onRemove,
                enabled = !state.isRemoving,
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.isRemoving) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
                }
                Text(stringResource(R.string.geopulse_settings_remove), color = MaterialTheme.colorScheme.error)
            }
        }
    }
}
