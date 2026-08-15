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
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycorrhizal.crm.ui.R

/**
 * M25: ntfy/Gotify notification-channel configuration — distinct from
 * Android's own local OS notification channels (`MycorrhizalNotificationChannels.kt`,
 * the app's local alerts). These channels are the server-side per-user config
 * that delivers reminders to self-hosted push services. Web-push (VAPID) has
 * no Android equivalent (browser-specific), so it is deliberately absent here.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun NotificationChannelsScreen(
    onBack: () -> Unit,
    viewModel: NotificationChannelsViewModel = hiltViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()

    Scaffold(
        topBar = {
            TopAppBar(
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.cd_back))
                    }
                },
                title = {
                    Text(stringResource(R.string.settings_notifications_title), style = MaterialTheme.typography.titleLarge)
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
        NotificationChannelsContent(
            state = state,
            onNtfyUrlChange = viewModel::onNtfyUrlChange,
            onNtfyTopicChange = viewModel::onNtfyTopicChange,
            onNotifyNtfyChange = viewModel::onNotifyNtfyChange,
            onGotifyUrlChange = viewModel::onGotifyUrlChange,
            onGotifyTokenChange = viewModel::onGotifyTokenChange,
            onNotifyGotifyChange = viewModel::onNotifyGotifyChange,
            onTest = viewModel::test,
            onSave = viewModel::save,
            modifier = Modifier.padding(padding),
        )
    }
}

/** Stateless body for [NotificationChannelsScreen] — testable directly (see NotificationChannelsScreenTest). */
@Composable
fun NotificationChannelsContent(
    state: NotificationChannelsUiState,
    onNtfyUrlChange: (String) -> Unit = {},
    onNtfyTopicChange: (String) -> Unit = {},
    onNotifyNtfyChange: (Boolean) -> Unit = {},
    onGotifyUrlChange: (String) -> Unit = {},
    onGotifyTokenChange: (String) -> Unit = {},
    onNotifyGotifyChange: (Boolean) -> Unit = {},
    onTest: (String) -> Unit = {},
    onSave: () -> Unit = {},
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
                text = stringResource(R.string.settings_notifications_description),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            state.loadError?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
            state.saveError?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }

            // ntfy
            Text(stringResource(R.string.settings_notifications_ntfy), style = MaterialTheme.typography.titleMedium)
            ChannelToggle(
                label = stringResource(R.string.settings_notifications_ntfy_enable),
                checked = state.notifyNtfy,
                onCheckedChange = onNotifyNtfyChange,
            )
            OutlinedTextField(
                value = state.ntfyUrl,
                onValueChange = onNtfyUrlChange,
                label = { Text(stringResource(R.string.settings_notifications_ntfy_url)) },
                placeholder = { Text("https://ntfy.sh") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.ntfyTopic,
                onValueChange = onNtfyTopicChange,
                label = { Text(stringResource(R.string.settings_notifications_ntfy_topic)) },
                placeholder = { Text("my-reminders") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            TestButton(
                enabled = state.notifyNtfy && state.ntfyUrl.isNotBlank() && state.ntfyTopic.isNotBlank(),
                testing = state.testingChannel == "ntfy",
                onClick = { onTest("ntfy") },
            )

            // Gotify
            Text(stringResource(R.string.settings_notifications_gotify), style = MaterialTheme.typography.titleMedium)
            ChannelToggle(
                label = stringResource(R.string.settings_notifications_gotify_enable),
                checked = state.notifyGotify,
                onCheckedChange = onNotifyGotifyChange,
            )
            OutlinedTextField(
                value = state.gotifyUrl,
                onValueChange = onGotifyUrlChange,
                label = { Text(stringResource(R.string.settings_notifications_gotify_url)) },
                placeholder = { Text("http://gotify:8080") },
                singleLine = true,
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.gotifyToken,
                onValueChange = onGotifyTokenChange,
                label = { Text(stringResource(R.string.settings_notifications_gotify_token)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                supportingText = {
                    Text(
                        if (state.gotifyHasToken) {
                            stringResource(R.string.settings_notifications_gotify_token_hint_existing)
                        } else {
                            stringResource(R.string.settings_notifications_gotify_token_hint_new)
                        },
                    )
                },
                modifier = Modifier.fillMaxWidth(),
            )
            TestButton(
                enabled = state.notifyGotify && state.gotifyUrl.isNotBlank(),
                testing = state.testingChannel == "gotify",
                onClick = { onTest("gotify") },
            )

            // Test result
            state.testResult["ntfy"]?.let { result ->
                Text(
                    text = resultMessage("ntfy", result.ok, result.error),
                    color = if (result.ok) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            state.testResult["gotify"]?.let { result ->
                Text(
                    text = resultMessage("gotify", result.ok, result.error),
                    color = if (result.ok) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            Button(
                onClick = onSave,
                enabled = !state.isSaving,
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (state.isSaving) {
                    CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
                }
                Text(stringResource(R.string.settings_notifications_save))
            }
        }
    }
}

@Composable
private fun ChannelToggle(
    label: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
    ) {
        Text(label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}

@Composable
private fun TestButton(
    enabled: Boolean,
    testing: Boolean,
    onClick: () -> Unit,
) {
    Button(
        onClick = onClick,
        enabled = enabled && !testing,
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (testing) {
            CircularProgressIndicator(modifier = Modifier.padding(end = 8.dp), strokeWidth = 2.dp)
        } else {
            Icon(Icons.Outlined.PlayArrow, contentDescription = null)
        }
        Text(
            stringResource(
                if (testing) R.string.settings_notifications_testing else R.string.settings_notifications_test,
            ),
            modifier = Modifier.padding(start = 8.dp),
        )
    }
}

@Composable
private fun resultMessage(channel: String, ok: Boolean, error: String?): String =
    if (ok) {
        stringResource(
            R.string.settings_notifications_test_success,
            stringResource(
                if (channel == "ntfy") R.string.settings_notifications_ntfy else R.string.settings_notifications_gotify,
            ),
        )
    } else {
        stringResource(R.string.settings_notifications_test_failed, error ?: "unknown")
    }
