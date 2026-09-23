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
 * Issue #833: the Nextcloud/ownCloud (WebDAV) connection-config settings screen —
 * base URL, username, and app password (shown only once a password is stored,
 * matching web's `NextcloudSettings.tsx`). The file-picker flow already exists
 * (#236); this is the missing configuration surface it depends on. Shape mirrors
 * [ImmichSettingsScreen] (issue #236), minus the sync toggle Nextcloud has no
 * equivalent of, plus the extra basic-auth username field.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NextcloudSettingsScreen(
    onBack: () -> Unit,
    viewModel: NextcloudSettingsViewModel = hiltViewModel(),
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
                title = {
                    Text(stringResource(R.string.settings_nextcloud_title), style = MaterialTheme.typography.titleLarge)
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primary,
                    titleContentColor = MaterialTheme.colorScheme.onPrimary,
                    navigationIconContentColor = MaterialTheme.colorScheme.onPrimary,
                    actionIconContentColor = MaterialTheme.colorScheme.onPrimary,
                ),
            )
        },
    ) { padding ->
        NextcloudSettingsContent(
            state = state,
            onBaseUrlChange = viewModel::onBaseUrlChange,
            onUsernameChange = viewModel::onUsernameChange,
            onAppPasswordChange = viewModel::onAppPasswordChange,
            onTest = viewModel::test,
            onSave = viewModel::save,
            onRemove = { showRemoveConfirm = true },
            modifier = Modifier.padding(padding),
        )
    }

    if (showRemoveConfirm) {
        AlertDialog(
            onDismissRequest = { showRemoveConfirm = false },
            title = { Text(stringResource(R.string.nextcloud_settings_remove_title)) },
            text = { Text(stringResource(R.string.nextcloud_settings_remove_confirm)) },
            confirmButton = {
                TextButton(
                    enabled = !state.isRemoving,
                    onClick = {
                        showRemoveConfirm = false
                        viewModel.remove()
                    },
                ) {
                    Text(stringResource(R.string.action_delete), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { showRemoveConfirm = false }) {
                    Text(stringResource(R.string.action_cancel))
                }
            },
        )
    }
}

/** Stateless body for [NextcloudSettingsScreen] — testable directly. */
@Composable
fun NextcloudSettingsContent(
    state: NextcloudSettingsUiState,
    onBaseUrlChange: (String) -> Unit = {},
    onUsernameChange: (String) -> Unit = {},
    onAppPasswordChange: (String) -> Unit = {},
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
        } else {
            Text(
                text = stringResource(R.string.nextcloud_settings_description),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            state.loadError?.let {
                Text(
                    it,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Assertive },
                )
            }
            state.saveError?.let {
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
                label = { Text(stringResource(R.string.nextcloud_settings_base_url)) },
                placeholder = { Text("https://nextcloud.example.com") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.username,
                onValueChange = onUsernameChange,
                label = { Text(stringResource(R.string.nextcloud_settings_username)) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.appPassword,
                onValueChange = onAppPasswordChange,
                label = { Text(stringResource(R.string.nextcloud_settings_app_password)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                supportingText = {
                    Text(
                        if (state.hasAppPassword) {
                            stringResource(R.string.nextcloud_settings_app_password_hint_existing)
                        } else {
                            stringResource(R.string.nextcloud_settings_app_password_hint_new)
                        },
                    )
                },
                modifier = Modifier.fillMaxWidth(),
            )

            if (state.hasAppPassword) {
                Button(
                    onClick = onTest,
                    enabled = !state.isTesting && state.baseUrl.isNotBlank() && state.username.isNotBlank(),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (state.isTesting) {
                        CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
                    } else {
                        Icon(Icons.Outlined.PlayArrow, contentDescription = null)
                    }
                    Text(
                        stringResource(
                            if (state.isTesting) R.string.nextcloud_settings_testing else R.string.nextcloud_settings_test,
                        ),
                        modifier = Modifier.padding(start = 8.dp),
                    )
                }
            }

            state.testResult?.let { result ->
                Text(
                    text = if (result.ok) {
                        stringResource(R.string.nextcloud_settings_test_success)
                    } else {
                        stringResource(R.string.nextcloud_settings_test_failed, result.message ?: "unknown")
                    },
                    color = if (result.ok) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            val savingLabel = stringResource(R.string.a11y_state_saving)
            Button(
                onClick = onSave,
                enabled = !state.isSaving && state.baseUrl.isNotBlank() && state.username.isNotBlank(),
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics { if (state.isSaving) stateDescription = savingLabel },
            ) {
                if (state.isSaving) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
                }
                Text(stringResource(R.string.action_save))
            }

            if (state.hasAppPassword) {
                OutlinedButton(
                    onClick = onRemove,
                    enabled = !state.isRemoving,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (state.isRemoving) {
                        CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
                    }
                    Text(stringResource(R.string.nextcloud_settings_remove), color = MaterialTheme.colorScheme.error)
                }
            }
        }
    }
}
